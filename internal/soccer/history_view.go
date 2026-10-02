package soccer

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
)

// HistoryViewHandler serves the Team history panel for one linked player,
// under the same authority as the JSON history reads. It lists the player's
// proven team seasons and opens the requested one, or the newest. A season
// the list does not prove is refused as the per-season read refuses it. A
// missing import, or a token LPS rejects, ends the import as every Soccer
// route does, resetting the page's import and hiding the panel.
func (h *Handler) HistoryViewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	query := r.URL.Query()
	playerID, playerOK := positiveHistoryID(query.Get("player_id"))
	rawTeam, rawSeason := query.Get("team_id"), query.Get("season_id")
	teamID, teamOK := positiveHistoryID(rawTeam)
	seasonID, seasonOK := positiveHistoryID(rawSeason)
	requested := rawTeam != "" || rawSeason != ""
	if !playerOK || requested && (!teamOK || !seasonOK) {
		http.Error(w, "a positive player_id is required, with positive team_id and season_id given together", http.StatusBadRequest)
		return
	}
	reader, refusal := h.authorizeHistoryPlayer(w, r, playerID)
	if refusal != nil {
		if refusal.status == http.StatusUnauthorized {
			h.renderHistoryEndedImport(w, r, endedImportDetails)
			return
		}
		h.renderHistoryView(w, r, refusal.status, &partials.SoccerTeamHistoryViewProps{Refusal: historyRefusalNotice(*refusal)})
		return
	}
	players := teamHistoryPlayers(reader.session.Players, playerID)
	refuse := func(refusal historyRefusal) {
		h.renderHistoryView(w, r, refusal.status, &partials.SoccerTeamHistoryViewProps{Players: players, Refusal: historyRefusalNotice(refusal)})
	}

	seasons, lookupErr, storeErr := h.listHistorySeasons(r.Context(), reader, playerID)
	if storeErr != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer membership list failed", slog.Any("error", storeErr))
		refuse(historyRefusal{http.StatusServiceUnavailable, "Team history is unavailable"})
		return
	}
	if lookupErr != nil {
		if detail := lps.ScheduleErrorDetailsFor(lookupErr); detail.ClearSession {
			h.clearSession(w, r)
			h.renderHistoryEndedImport(w, r, detail)
			return
		}
		logging.WithContext(h.Logger, r.Context()).Warn("soccer current membership lookup failed; listing stored proof only", slog.Any("error", lookupErr))
	}

	selected, proven := openHistorySeason(seasons, requested, teamID, seasonID)
	if !proven {
		if lookupErr != nil {
			refuse(currentMembershipRefusal(lookupErr))
		} else {
			refuse(historyRefusal{http.StatusForbidden, "Team-season membership is unverified"})
		}
		return
	}

	player, _ := linkedPlayer(reader.session.Players, playerID)
	props := partials.SoccerTeamHistoryViewProps{
		Players: players,
		Notices: teamHistoryListNotices(playerDisplayName(player), lookupErr == nil, len(seasons)),
		Seasons: teamHistorySeasons(playerID, seasons, selected),
	}
	if selected != nil {
		history, err := readTeamSeasonHistory(r.Context(), reader.store, playerID, selected.TeamID, selected.LPSSeasonID)
		if err != nil {
			logging.WithContext(h.Logger, r.Context()).Error("soccer team history read failed", slog.Any("error", err))
			refuse(historyRefusal{http.StatusServiceUnavailable, "Team history is unavailable"})
			return
		}
		props.Season = teamHistorySeasonDetail(selected, &history)
	}
	h.renderHistoryView(w, r, http.StatusOK, &props)
}

// openHistorySeason picks the season the view opens: the requested team
// season when the list proves it, else the newest. proven is false only for a
// requested season the list does not hold.
func openHistorySeason(seasons []provenTeamSeason, requested bool, teamID, seasonID int) (selected *provenTeamSeason, proven bool) {
	if !requested {
		if len(seasons) == 0 {
			return nil, true
		}
		return &seasons[0], true
	}
	for i := range seasons {
		if seasons[i].TeamID == teamID && seasons[i].LPSSeasonID == seasonID {
			return &seasons[i], true
		}
	}
	return nil, false
}

// renderHistoryView writes the Team history panel with status. A refusal
// keeps its status and is marked for htmx to swap.
func (h *Handler) renderHistoryView(w http.ResponseWriter, r *http.Request, status int, props *partials.SoccerTeamHistoryViewProps) {
	h.setHTMLContentType(w)
	if status != http.StatusOK {
		w.Header().Set("X-Portal-Fragment-Error", "true")
	}
	w.WriteHeader(status)
	if err := partials.SoccerTeamHistoryView(*props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer team history render failed", slog.Any("error", err))
	}
}

// renderHistoryEndedImport answers a Team history request whose import is
// gone or that LPS rejected: it replaces the page's LPS card and private
// stages out of band with the reason, as every Soccer route does, and swaps
// nothing into the panel the reset hides. It leaves the import cookies to the
// caller, so a Team ID lookup the browser holds in place of an ended import
// survives.
func (h *Handler) renderHistoryEndedImport(w http.ResponseWriter, r *http.Request, detail lps.ScheduleErrorDetails) {
	w.Header().Set("HX-Trigger", "soccer-workflow-reset")
	w.Header().Set("HX-Reswap", "none")
	w.Header().Set("X-Portal-Fragment-Error", "true")
	props := h.LoginStateProps(w, r, nil, true)
	props.ImportNotice = importNoticeFor(detail)
	h.setHTMLContentType(w)
	w.WriteHeader(http.StatusUnauthorized)
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer team history reset render failed", slog.Any("error", err))
	}
}

func historyRefusalNotice(refusal historyRefusal) *partials.FeedbackProps {
	title := "Team history unavailable"
	switch refusal.status {
	case http.StatusForbidden:
		title = "Not available"
	case http.StatusBadGateway:
		title = "LPS did not answer"
	}
	return &partials.FeedbackProps{Kind: partials.FeedbackError, Title: title, Message: refusal.message + "."}
}

func teamHistoryPlayers(players []types.LPSPlayer, selectedID int) []partials.SoccerTeamHistoryPlayer {
	shown := make([]partials.SoccerTeamHistoryPlayer, 0, len(players))
	for _, player := range players {
		shown = append(shown, partials.SoccerTeamHistoryPlayer{ID: player.UPlayerID, Name: playerDisplayName(player), Selected: player.UPlayerID == selectedID})
	}
	return shown
}

func teamHistorySeasons(playerID int, seasons []provenTeamSeason, selected *provenTeamSeason) []partials.SoccerTeamHistorySeason {
	shown := make([]partials.SoccerTeamHistorySeason, 0, len(seasons))
	for i := range seasons {
		season := &seasons[i]
		shown = append(shown, partials.SoccerTeamHistorySeason{
			PlayerID: playerID, TeamID: season.TeamID, SeasonID: season.LPSSeasonID,
			TeamName: teamHistoryTeamName(season.Team.TeamName, season.TeamID),
			Division: strings.TrimSpace(season.Team.DivisionName),
			Current:  season.Current,
			Selected: selected != nil && season.TeamID == selected.TeamID && season.LPSSeasonID == selected.LPSSeasonID,
		})
	}
	return shown
}

// teamHistorySeasonDetail is the open season: its team as the season list
// names it, else as the archive does, with the record, collection status,
// notices, and completed games.
func teamHistorySeasonDetail(season *provenTeamSeason, history *historyResponse) *partials.SoccerTeamHistorySeasonDetail {
	name := strings.TrimSpace(season.Team.TeamName)
	if name == "" {
		name = history.Team.TeamName
	}
	division := strings.TrimSpace(season.Team.DivisionName)
	if division == "" {
		division = strings.TrimSpace(history.Team.DivisionName)
	}
	detail := &partials.SoccerTeamHistorySeasonDetail{
		TeamID: season.TeamID, SeasonID: season.LPSSeasonID,
		TeamName: teamHistoryTeamName(name, season.TeamID), Division: division, Current: season.Current,
		Record: partials.SoccerTeamHistoryRecord{
			Label: history.Record.Label, Wins: history.Record.Wins, Losses: history.Record.Losses, Draws: history.Record.Draws,
			ScoredGames: history.Record.ScoredGames, Unclassified: history.Record.Unclassified,
		},
		Notices: teamSeasonNotices(history),
		Games:   teamHistoryGames(history.Games, season.TeamID),
	}
	if history.Coverage.Status == soccerarchive.CoverageFetched && history.Coverage.FetchedAt != nil {
		detail.Collected = fmt.Sprintf("Collected %s · LPS returned %s for this season",
			schedule.FormatMountainTime(*history.Coverage.FetchedAt), teamHistoryCount(history.Coverage.ReturnedGameCount, "game", "games"))
	}
	return detail
}

// teamHistoryGames lists completed games newest kickoff first, each from the
// team's side: its opponent, its score, and its counted result.
func teamHistoryGames(games []historyGame, teamID int) []partials.SoccerTeamHistoryGame {
	ordered := slices.Clone(games)
	slices.SortStableFunc(ordered, func(a, b historyGame) int {
		if c := b.Kickoff.Compare(a.Kickoff); c != 0 {
			return c
		}
		return b.Game.UGameID - a.Game.UGameID
	})
	shown := make([]partials.SoccerTeamHistoryGame, 0, len(ordered))
	for i := range ordered {
		game := &ordered[i]
		shown = append(shown, partials.SoccerTeamHistoryGame{
			ID:         game.Game.UGameID,
			Kickoff:    schedule.FormatMountainTime(game.Kickoff),
			KickoffISO: game.Kickoff.Format(time.RFC3339),
			Opponent:   teamHistoryOpponent(&game.Game, teamID),
			Score:      teamHistoryScore(game),
			Result:     teamHistoryResult(game.Classification),
		})
	}
	return shown
}

// teamHistoryOpponent names the other side by Team ID, as the record places
// sides. A game neither side of which is the team by ID names both.
func teamHistoryOpponent(game *lps.TeamScheduleGame, teamID int) string {
	homeID, awayID := game.HomeTeamID(), game.AwayTeamID()
	home := teamHistoryTeamName(game.HomeTeam.TeamName, homeID)
	away := teamHistoryTeamName(game.VisitorTeam.TeamName, awayID)
	switch {
	case homeID == teamID && awayID != teamID:
		return "vs " + away
	case awayID == teamID && homeID != teamID:
		return "at " + home
	default:
		return home + " vs " + away
	}
}

func teamHistoryTeamName(name string, teamID int) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	if teamID > 0 {
		return "Team " + strconv.Itoa(teamID)
	}
	return "Unknown team"
}

// teamHistoryScore is a counted game's score from the team's side, or LPS's
// own result text for a game the record leaves out.
func teamHistoryScore(game *historyGame) string {
	switch game.Classification {
	case "win", "loss", "draw":
		return fmt.Sprintf("%d–%d", game.TeamScore, game.OpponentScore)
	}
	if result := strings.TrimSpace(game.Game.Result); result != "" {
		return result
	}
	return "No score"
}

func teamHistoryResult(classification string) string {
	switch classification {
	case "win":
		return "Win"
	case "loss":
		return "Loss"
	case "draw":
		return "Draw"
	default:
		return "Not counted"
	}
}

// teamSeasonNotices explains the open season's collection: a season no
// complete LPS response has covered, a season LPS returned empty, one whose
// games have not kicked off, and the team's latest refresh when it failed.
func teamSeasonNotices(history *historyResponse) []partials.FeedbackProps {
	var notices []partials.FeedbackProps
	fetched := history.Coverage.Status == soccerarchive.CoverageFetched
	switch {
	case !fetched:
		message := "No complete LPS response for this season has been saved yet."
		if refresh := history.Refresh; refresh != nil && refresh.Status == soccerarchive.RefreshReady && refresh.NextDueAt != nil {
			message += " The next refresh is due " + schedule.FormatMountainTime(*refresh.NextDueAt) + "."
		}
		notices = append(notices, partials.FeedbackProps{Kind: partials.FeedbackInfo, Title: "Not collected yet", Message: message})
	case len(history.Games) == 0 && history.Coverage.ReturnedGameCount == 0:
		notices = append(notices, partials.FeedbackProps{Kind: partials.FeedbackInfo, Title: "No games this season", Message: "LPS returned no games for this season."})
	case len(history.Games) == 0:
		notices = append(notices, partials.FeedbackProps{Kind: partials.FeedbackInfo, Title: "No completed games yet", Message: "No game LPS returned for this season has kicked off yet."})
	}
	refresh := history.Refresh
	if refresh == nil {
		return notices
	}
	switch refresh.Status {
	case soccerarchive.RefreshRetryable:
		message := "The latest refresh failed: " + refreshFailureReason(refresh) + "."
		if refresh.LastAttemptAt != nil {
			message = "The refresh at " + schedule.FormatMountainTime(*refresh.LastAttemptAt) + " failed: " + refreshFailureReason(refresh) + "."
		}
		if fetched && history.Coverage.FetchedAt != nil {
			message += " Showing the copy collected " + schedule.FormatMountainTime(*history.Coverage.FetchedAt) + "."
		}
		notices = append(notices, partials.FeedbackProps{Kind: partials.FeedbackWarning, Title: "Latest refresh failed", Message: message})
	case soccerarchive.RefreshInvalid:
		notices = append(notices, partials.FeedbackProps{Kind: partials.FeedbackWarning, Title: "LPS no longer accepts this team", Message: "Collected history is kept, but this team is no longer refreshed."})
	}
	return notices
}

func refreshFailureReason(refresh *historyRefresh) string {
	switch refresh.LastErrorKind {
	case soccerarchive.RefreshStoreErrorKind:
		return "the response LPS sent could not be saved"
	case lps.ErrorTeamRefused:
		return "LPS refused to share this team's schedule"
	}
	if refresh.LastErrorStatusCode > 0 {
		return "LPS answered HTTP " + strconv.Itoa(refresh.LastErrorStatusCode)
	}
	return "LPS could not be reached"
}

// teamHistoryListNotices explains a short or empty season list: LPS could not
// confirm the player's current teams, or no season is proven at all.
func teamHistoryListNotices(playerName string, currentVerified bool, seasonCount int) []partials.FeedbackProps {
	var notices []partials.FeedbackProps
	if !currentVerified {
		notices = append(notices, partials.FeedbackProps{
			Kind: partials.FeedbackInfo, Title: "Current teams not confirmed",
			Message: fmt.Sprintf("LPS could not confirm %s's current teams just now, so only seasons recorded by earlier imports are listed.", playerName),
		})
	}
	if seasonCount == 0 {
		notices = append(notices, partials.FeedbackProps{
			Kind: partials.FeedbackInfo, Title: "No team seasons yet",
			Message: fmt.Sprintf("No team seasons are recorded for %s. Seasons come from LPS imports that link this player; Team IDs entered by hand never grant history.", playerName),
		})
	}
	return notices
}

func teamHistoryCount(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + plural
}
