package soccer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
)

const scoredRecordLabel = "Calculated from numeric game scores; not official standings"

type scoredRecord struct {
	Label        string `json:"label"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	Draws        int    `json:"draws"`
	ScoredGames  int    `json:"scored_games"`
	Unclassified int    `json:"unclassified"`
}

type historyGame struct {
	Game           lps.TeamScheduleGame `json:"game"`
	Classification string               `json:"classification"`
}

type historyCoverage struct {
	Status            soccerarchive.CoverageStatus `json:"status"`
	FetchedAt         *time.Time                   `json:"fetched_at,omitempty"`
	ReturnedGameCount int                          `json:"returned_game_count"`
}

type historyRefresh struct {
	Status              soccerarchive.RefreshStatus `json:"status"`
	LastAttemptAt       *time.Time                  `json:"last_attempt_at,omitempty"`
	NextDueAt           *time.Time                  `json:"next_due_at,omitempty"`
	LastErrorKind       lps.ErrorKind               `json:"last_error_kind,omitempty"`
	LastErrorStatusCode int                         `json:"last_error_status_code,omitempty"`
}

type historyResponse struct {
	PlayerID    int             `json:"player_id"`
	TeamID      int             `json:"team_id"`
	LPSSeasonID int             `json:"lps_season_id"`
	Team        lps.TeamSummary `json:"team"`
	Coverage    historyCoverage `json:"coverage"`
	Refresh     *historyRefresh `json:"refresh"`
	Record      scoredRecord    `json:"record"`
	Games       []historyGame   `json:"games"`
}

// HistoryHandler serves one proven player-team-LPS-season read for a current
// Soccer grantee with an unexpired, same-owner imported LPS credential.
func (h *Handler) HistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	playerID, playerOK := positiveHistoryID(r.URL.Query().Get("player_id"))
	teamID, teamOK := positiveHistoryID(r.URL.Query().Get("team_id"))
	seasonID, seasonOK := positiveHistoryID(r.URL.Query().Get("season_id"))
	if !playerOK || !teamOK || !seasonOK {
		http.Error(w, "positive player_id, team_id, and season_id are required", http.StatusBadRequest)
		return
	}
	principal, signedIn := siteidentity.PrincipalFromContext(r.Context())
	if !signedIn || !siteidentity.HasGrantForOwner(r.Context(), siteidentity.GrantSoccer, principal.Issuer, principal.Subject) {
		http.Error(w, "Soccer access is required", http.StatusForbidden)
		return
	}
	session, _ := h.LoadSession(w, r)
	if session == nil || session.JWT == "" || session.OwnerIssuer != principal.Issuer || session.OwnerSubject != principal.Subject {
		http.Error(w, "A valid same-owner LPS import is required", http.StatusUnauthorized)
		return
	}
	linkedPlayer := false
	for _, player := range session.Players {
		if player.UPlayerID == playerID {
			linkedPlayer = true
			break
		}
	}
	if !linkedPlayer {
		http.Error(w, "Player is not confirmed by this import", http.StatusForbidden)
		return
	}
	store, enabled := h.ArchiveStore().(soccerarchive.HistoryStore)
	if !enabled {
		http.Error(w, "Team history is unavailable", http.StatusServiceUnavailable)
		return
	}
	proven, err := store.HasPlayerMembership(r.Context(), principal.Issuer, principal.Subject, playerID, teamID, seasonID)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer membership read failed", slog.Any("error", err))
		http.Error(w, "Team history is unavailable", http.StatusServiceUnavailable)
		return
	}
	if !proven {
		proven, err = h.currentTeamSeasonMembership(r.Context(), session.JWT, playerID, teamID, seasonID)
		if err != nil {
			logging.WithContext(h.Logger, r.Context()).Warn("soccer current membership lookup failed", slog.Any("error", err))
			http.Error(w, "Current team membership could not be verified", http.StatusBadGateway)
			return
		}
	}
	if !proven {
		http.Error(w, "Team-season membership is unverified", http.StatusForbidden)
		return
	}

	response, err := readTeamSeasonHistory(r.Context(), store, playerID, teamID, seasonID)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer team history read failed", slog.Any("error", err))
		http.Error(w, "Team history is unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer history response write failed", slog.Any("error", err))
	}
}

// currentTeamSeasonMembership reports whether the imported player's current
// authenticated LPS team lookup lists the exact team and LPS season.
func (h *Handler) currentTeamSeasonMembership(ctx context.Context, jwt string, playerID, teamID, seasonID int) (bool, error) {
	teams, err := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, jwt).FetchPlayerTeams(ctx, playerID)
	if err != nil {
		return false, err
	}
	for _, team := range teams {
		if team.UTeamID == teamID && team.Season == seasonID {
			return true, nil
		}
	}
	return false, nil
}

// readTeamSeasonHistory builds an authorized team season's response from the
// archive. A team no response has archived yet reads as not fetched.
func readTeamSeasonHistory(ctx context.Context, store soccerarchive.HistoryStore, playerID, teamID, seasonID int) (historyResponse, error) {
	history, err := store.ReadTeamSeason(ctx, teamID, seasonID)
	if errors.Is(err, soccerarchive.ErrNoArchive) {
		history = soccerarchive.TeamSeason{Team: lps.TeamSummary{UTeamID: teamID, Season: seasonID}, Coverage: soccerarchive.Coverage{Status: soccerarchive.CoverageNotFetched}}
	} else if err != nil {
		return historyResponse{}, err
	}
	if history.Team.UTeamID != teamID || history.Team.Season != seasonID {
		return historyResponse{}, fmt.Errorf("archive returned team %d season %d for team %d season %d", history.Team.UTeamID, history.Team.Season, teamID, seasonID)
	}
	refresh, err := store.ReadRefreshState(ctx, teamID)
	if err != nil && !errors.Is(err, soccerarchive.ErrNotEnrolled) {
		return historyResponse{}, fmt.Errorf("read team %d refresh state: %w", teamID, err)
	}
	response := buildHistoryResponse(&history, playerID, teamID, seasonID, time.Now())
	if err == nil {
		response.Refresh = buildHistoryRefresh(&refresh)
	}
	return response, nil
}

func buildHistoryRefresh(state *soccerarchive.RefreshState) *historyRefresh {
	view := &historyRefresh{Status: state.Status, LastErrorKind: state.LastErrorKind, LastErrorStatusCode: state.LastErrorStatusCode}
	if !state.LastAttemptAt.IsZero() {
		lastAttemptAt := state.LastAttemptAt
		view.LastAttemptAt = &lastAttemptAt
	}
	if !state.NextDueAt.IsZero() {
		nextDueAt := state.NextDueAt
		view.NextDueAt = &nextDueAt
	}
	return view
}

func buildHistoryResponse(history *soccerarchive.TeamSeason, playerID, teamID, seasonID int, now time.Time) historyResponse {
	response := historyResponse{
		PlayerID: playerID, TeamID: teamID, LPSSeasonID: seasonID, Team: history.Team,
		Coverage: historyCoverage{Status: history.Coverage.Status, ReturnedGameCount: history.Coverage.ReturnedGameCount},
		Record:   scoredRecord{Label: scoredRecordLabel}, Games: make([]historyGame, 0, len(history.Games)),
	}
	if !history.Coverage.FetchedAt.IsZero() {
		fetchedAt := history.Coverage.FetchedAt
		response.Coverage.FetchedAt = &fetchedAt
	}
	for i := range history.Games {
		game := &history.Games[i]
		started, parseableTime := schedule.ParseScheduleTime(game.SchedGameDateTime)
		if (!parseableTime || started.After(now)) && strings.TrimSpace(game.Result) == "" {
			continue
		}
		classification := classifyHistoryGame(&response.Record, game, teamID)
		response.Games = append(response.Games, historyGame{Game: *game, Classification: classification})
	}
	return response
}

func positiveHistoryID(raw string) (int, bool) {
	value, err := strconv.Atoi(raw)
	return value, err == nil && value > 0
}

func classifyHistoryGame(record *scoredRecord, game *lps.TeamScheduleGame, teamID int) string {
	result := schedule.ParseGameResult(game.Result, "", "")
	if !result.Parsed || (result.Outcome != schedule.OutcomeWin && result.Outcome != schedule.OutcomeLoss && result.Outcome != schedule.OutcomeDraw) {
		record.Unclassified++
		return "unclassified"
	}
	// The sides come from the game's team IDs as the schedule and archive
	// read them; a team name never places a side.
	homeID, awayID := game.HomeTeamID(), game.AwayTeamID()
	if !(homeID == teamID && awayID > 0 && awayID != teamID || awayID == teamID && homeID > 0 && homeID != teamID) {
		record.Unclassified++
		return "unclassified"
	}
	teamScore, opponentScore := result.HomeScore, result.AwayScore
	if awayID == teamID {
		teamScore, opponentScore = result.AwayScore, result.HomeScore
	}
	record.ScoredGames++
	switch {
	case teamScore > opponentScore:
		record.Wins++
		return "win"
	case teamScore < opponentScore:
		record.Losses++
		return "loss"
	default:
		record.Draws++
		return "draw"
	}
}
