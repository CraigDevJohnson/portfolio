package soccer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
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
	// Kickoff is when the completed game started. TeamScore and
	// OpponentScore are a counted result from the team's side. The Team
	// history view shows them; the JSON read leaves them out.
	Kickoff       time.Time `json:"-"`
	TeamScore     int       `json:"-"`
	OpponentScore int       `json:"-"`
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

type provenTeamSeason struct {
	TeamID      int             `json:"team_id"`
	LPSSeasonID int             `json:"lps_season_id"`
	Team        lps.TeamSummary `json:"team"`
	Current     bool            `json:"current"`
}

type teamSeasonsResponse struct {
	PlayerID int `json:"player_id"`
	// CurrentVerified is false when LPS denied or could not answer the
	// player's current team lookup, so the list holds stored proof alone.
	CurrentVerified bool               `json:"current_verified"`
	TeamSeasons     []provenTeamSeason `json:"team_seasons"`
}

// historyReader is the site owner, import, and archive a private history
// read has been authorized with.
type historyReader struct {
	principal siteidentity.Principal
	session   *types.SessionData
	store     soccerarchive.HistoryStore
}

// historyRefusal is why a private history request was refused: its status
// and the reason, which each history route words in its own format.
type historyRefusal struct {
	status  int
	message string
}

// authorizeHistoryPlayer admits a private history read of playerID for a
// current Soccer grantee whose unexpired, same-owner import confirms the
// player, and refuses every other request.
func (h *Handler) authorizeHistoryPlayer(w http.ResponseWriter, r *http.Request, playerID int) (historyReader, *historyRefusal) {
	principal, signedIn := siteidentity.PrincipalFromContext(r.Context())
	if !signedIn || !siteidentity.HasGrantForOwner(r.Context(), siteidentity.GrantSoccer, principal.Issuer, principal.Subject) {
		return historyReader{}, &historyRefusal{http.StatusForbidden, "Soccer access is required"}
	}
	session, _ := h.LoadSession(w, r)
	if session == nil || session.JWT == "" || session.OwnerIssuer != principal.Issuer || session.OwnerSubject != principal.Subject {
		return historyReader{}, &historyRefusal{http.StatusUnauthorized, "A valid same-owner LPS import is required"}
	}
	if _, linked := linkedPlayer(session.Players, playerID); !linked {
		return historyReader{}, &historyRefusal{http.StatusForbidden, "Player is not confirmed by this import"}
	}
	store, enabled := h.ArchiveStore().(soccerarchive.HistoryStore)
	if !enabled {
		return historyReader{}, &historyRefusal{http.StatusServiceUnavailable, "Team history is unavailable"}
	}
	return historyReader{principal: principal, session: session, store: store}, nil
}

// listHistorySeasons lists the team seasons a private read opens for one
// player: the owner's stored proof and the seasons the player's current LPS
// team lookup lists. A storeErr means the membership query failed and
// nothing is listed. A lookupErr means LPS refused or could not answer the
// current lookup, so the seasons hold stored proof alone.
func (h *Handler) listHistorySeasons(ctx context.Context, reader historyReader, playerID int) (seasons []provenTeamSeason, lookupErr, storeErr error) {
	stored, storeErr := reader.store.ListPlayerMemberships(ctx, reader.principal.Issuer, reader.principal.Subject, playerID)
	if storeErr != nil {
		return nil, nil, storeErr
	}
	current, lookupErr := h.currentTeamSeasons(ctx, reader.session.JWT, playerID)
	return provenTeamSeasons(stored, current), lookupErr, nil
}

// HistoryTeamSeasonsHandler lists every team season one imported player is
// proven for, under the same authority as the per-season read: this owner's
// stored authenticated proof, including former seasons LPS no longer lists,
// and the seasons the player's current LPS team lookup lists. Each entry
// opens with HistoryHandler. No proof is an empty list. A token LPS rejects
// ends the import, as on the per-season read; a lookup LPS otherwise denies
// or cannot serve leaves the stored proof, which the per-season read opens
// without asking LPS, listed as unverified.
func (h *Handler) HistoryTeamSeasonsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	playerID, playerOK := positiveHistoryID(r.URL.Query().Get("player_id"))
	if !playerOK {
		http.Error(w, "a positive player_id is required", http.StatusBadRequest)
		return
	}
	reader, refusal := h.authorizeHistoryPlayer(w, r, playerID)
	if refusal != nil {
		http.Error(w, refusal.message, refusal.status)
		return
	}
	seasons, err, storeErr := h.listHistorySeasons(r.Context(), reader, playerID)
	if storeErr != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer membership list failed", slog.Any("error", storeErr))
		http.Error(w, "Team history is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err != nil && lps.ScheduleErrorDetailsFor(err).ClearSession {
		h.refuseUnverifiedCurrentMembership(w, r, err)
		return
	}
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer current membership lookup failed; listing stored proof only", slog.Any("error", err))
	}
	response := teamSeasonsResponse{PlayerID: playerID, CurrentVerified: err == nil, TeamSeasons: seasons}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer team-season list write failed", slog.Any("error", err))
	}
}

// provenTeamSeasons merges stored proof with the current lookup's seasons,
// newest LPS season first. A season LPS lists now is current and takes the
// lookup's team facts.
func provenTeamSeasons(stored []soccerarchive.PlayerMembership, current []lps.TeamSummary) []provenTeamSeason {
	type teamSeasonKey struct{ teamID, seasonID int }
	byKey := make(map[teamSeasonKey]provenTeamSeason, len(stored)+len(current))
	for i := range stored {
		team := stored[i].Team
		byKey[teamSeasonKey{team.UTeamID, team.Season}] = provenTeamSeason{TeamID: team.UTeamID, LPSSeasonID: team.Season, Team: team}
	}
	for _, team := range current {
		byKey[teamSeasonKey{team.UTeamID, team.Season}] = provenTeamSeason{TeamID: team.UTeamID, LPSSeasonID: team.Season, Team: team, Current: true}
	}
	seasons := make([]provenTeamSeason, 0, len(byKey))
	for _, season := range byKey {
		seasons = append(seasons, season)
	}
	sort.Slice(seasons, func(i, j int) bool {
		if seasons[i].LPSSeasonID != seasons[j].LPSSeasonID {
			return seasons[i].LPSSeasonID > seasons[j].LPSSeasonID
		}
		return seasons[i].TeamID < seasons[j].TeamID
	})
	return seasons
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
	reader, refusal := h.authorizeHistoryPlayer(w, r, playerID)
	if refusal != nil {
		http.Error(w, refusal.message, refusal.status)
		return
	}
	principal, session, store := reader.principal, reader.session, reader.store
	proven, err := store.HasPlayerMembership(r.Context(), principal.Issuer, principal.Subject, playerID, teamID, seasonID)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer membership read failed", slog.Any("error", err))
		http.Error(w, "Team history is unavailable", http.StatusServiceUnavailable)
		return
	}
	if !proven {
		proven, err = h.currentTeamSeasonMembership(r.Context(), session.JWT, playerID, teamID, seasonID)
		if err != nil {
			h.refuseUnverifiedCurrentMembership(w, r, err)
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
	teams, err := h.currentTeamSeasons(ctx, jwt, playerID)
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

// currentTeamSeasons returns the team seasons the imported player's current
// authenticated LPS team lookup lists. A team without an LPS season proves no
// season. A player LPS rejects as invalid has no current team-seasons, as on
// import, rather than a lookup to retry.
func (h *Handler) currentTeamSeasons(ctx context.Context, jwt string, playerID int) ([]lps.TeamSummary, error) {
	teams, err := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, jwt).FetchPlayerTeams(ctx, playerID)
	var fetchErr *lps.FetchError
	if errors.As(err, &fetchErr) && fetchErr.Kind == lps.ErrorInvalidPlayer {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	seasons := make([]lps.TeamSummary, 0, len(teams))
	for _, team := range teams {
		if team.UTeamID > 0 && team.Season > 0 {
			seasons = append(seasons, team)
		}
	}
	return seasons, nil
}

// refuseUnverifiedCurrentMembership answers a current team lookup LPS
// refused or could not serve, as every other Soccer route judges it. A token
// LPS rejects ends the import, so the proof it stored opens nothing more; a
// player LPS denies is not confirmed by the import; and an unavailable LPS
// says nothing about the import, which stays usable.
func (h *Handler) refuseUnverifiedCurrentMembership(w http.ResponseWriter, r *http.Request, err error) {
	detail := lps.ScheduleErrorDetailsFor(err)
	if detail.ClearSession {
		h.clearSession(w, r)
		http.Error(w, detail.DownloadMessage, detail.DownloadStatus)
		return
	}
	refusal := currentMembershipRefusal(err)
	if refusal.status == http.StatusBadGateway {
		logging.WithContext(h.Logger, r.Context()).Warn("soccer current membership lookup failed", slog.Any("error", err))
	}
	http.Error(w, refusal.message, refusal.status)
}

// currentMembershipRefusal answers a current team lookup LPS refused or could
// not serve, other than a token LPS rejects, which ends the import: a player
// LPS denies is not confirmed by the import, and an unavailable LPS says
// nothing about the import, which stays usable.
func currentMembershipRefusal(err error) historyRefusal {
	var fetchErr *lps.FetchError
	if errors.As(err, &fetchErr) && fetchErr.Kind == lps.ErrorForbidden {
		return historyRefusal{http.StatusForbidden, "Player is not confirmed by this import"}
	}
	return historyRefusal{http.StatusBadGateway, "Current team membership could not be verified"}
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
	// A completed game is one that has kicked off, as for the schedule's past
	// results: a score on a game dated later or with no readable kickoff is
	// not a completed result, while a past game without one is unclassified.
	for i := range history.Games {
		game := &history.Games[i]
		started, parseableTime := schedule.ParseScheduleTime(game.SchedGameDateTime)
		if !parseableTime || !started.Before(now) {
			continue
		}
		classification, teamScore, opponentScore := classifyHistoryGame(&response.Record, game, teamID)
		response.Games = append(response.Games, historyGame{
			Game: *game, Classification: classification, Kickoff: started, TeamScore: teamScore, OpponentScore: opponentScore,
		})
	}
	return response
}

func positiveHistoryID(raw string) (int, bool) {
	value, err := strconv.Atoi(raw)
	return value, err == nil && value > 0
}

// classifyHistoryGame counts one completed game in the record and returns
// its classification with the score from the team's side, which is zero for
// an unclassified game.
func classifyHistoryGame(record *scoredRecord, game *lps.TeamScheduleGame, teamID int) (classification string, teamScore, opponentScore int) {
	result := schedule.ParseGameResult(game.Result, "", "")
	if !result.Parsed || (result.Outcome != schedule.OutcomeWin && result.Outcome != schedule.OutcomeLoss && result.Outcome != schedule.OutcomeDraw) {
		record.Unclassified++
		return "unclassified", 0, 0
	}
	// The sides come from the game's team IDs as the schedule and archive
	// read them; a team name never places a side.
	homeID, awayID := game.HomeTeamID(), game.AwayTeamID()
	if !(homeID == teamID && awayID > 0 && awayID != teamID || awayID == teamID && homeID > 0 && homeID != teamID) {
		record.Unclassified++
		return "unclassified", 0, 0
	}
	teamScore, opponentScore = result.HomeScore, result.AwayScore
	if awayID == teamID {
		teamScore, opponentScore = result.AwayScore, result.HomeScore
	}
	record.ScoredGames++
	switch {
	case teamScore > opponentScore:
		record.Wins++
		return "win", teamScore, opponentScore
	case teamScore < opponentScore:
		record.Losses++
		return "loss", teamScore, opponentScore
	default:
		record.Draws++
		return "draw", teamScore, opponentScore
	}
}
