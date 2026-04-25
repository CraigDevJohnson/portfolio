package soccer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/types"
)

// FetchSchedulesHandler renders the schedule table fragment for the current selection.
func (h *Handler) FetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if !parseScheduleRequest(w, r) {
		return
	}

	input := parseScheduleFormInput(r.Form)
	session, swapAuthState := h.LoadSession(w, r)

	// When team_ids[] is submitted (from the discover-teams step), carry them
	// forward in TeamCodes so ICS download and Google add forms work unchanged.
	teamCodes := input.TeamCodes
	if len(input.TeamIDs) > 0 {
		teamCodes = joinIntSlice(input.TeamIDs)
	}

	props := partials.SoccerTableFragmentProps{
		TeamCodes: teamCodes,
		PlayerIDs: input.PlayerIDs,
	}
	if h.googleHooks != nil {
		props.GoogleConnected = h.googleHooks.GoogleConnected(r.Context(), w, r)
	}
	if h.resolveScheduleData(r.Context(), session, input, &props) {
		h.clearSession(w, r)
		swapAuthState = true
	}

	h.setHTMLContentType(w)
	if swapAuthState {
		if err := partials.SoccerLoginState(h.LoginStateProps(w, r, nil, true)).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := partials.SoccerTableFragment(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// DownloadICSHandler exports the selected schedule rows as an ICS download.
func (h *Handler) DownloadICSHandler(w http.ResponseWriter, r *http.Request) {
	if !parseScheduleRequest(w, r) {
		return
	}

	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		http.Error(w, "select at least one game", http.StatusBadRequest)
		return
	}

	input := parseScheduleFormInput(r.Form)
	if hasInvalidPlayerInput(input.RawPlayerIDs, input.PlayerIDs) {
		http.Error(w, "one or more selected players were invalid; clear the imported players and import again to refresh the discovered player list", http.StatusBadRequest)
		return
	}
	session, _ := h.LoadSession(w, r)

	games, err := h.RequestedScheduleGames(r.Context(), session, input.PlayerIDs, input.TeamCodes)
	if errors.Is(err, ErrPlayerSessionRequired) {
		http.Error(w, "import a bearer JWT again before downloading schedules for your discovered players", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, ErrInvalidTeamSelection) {
		http.Error(w, "one or more team IDs were invalid; enter numeric Let's Play Soccer team IDs separated by commas", http.StatusBadRequest)
		return
	}
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer LPS fetch failed", slog.Any("error", err))
		h.handleScheduleDownloadError(w, r, err)
		return
	}

	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		http.Error(w, "no selected games were found", http.StatusBadRequest)
		return
	}

	icsContent := schedule.BuildICS(filteredGames)

	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Disposition", "attachment; filename=soccer_schedule.ics")
	if _, err := io.WriteString(w, icsContent); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) resolveScheduleData(ctx context.Context, session *types.SessionData, input scheduleFormInput, props *partials.SoccerTableFragmentProps) bool {
	if hasInvalidPlayerInput(input.RawPlayerIDs, input.PlayerIDs) {
		props.Message = invalidPlayersMessage
		props.Hint = invalidPlayersHint
		return false
	}

	// When explicit team_ids[] arrive (from the discover-teams step), fetch all
	// games for those teams directly — no player session needed.
	if len(input.TeamIDs) > 0 {
		games, err := lps.FetchAllGamesForTeams(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, input.TeamIDs)
		if err == nil {
			props.Games = games
			return false
		}
		if applyScheduleSelectionError(props, err) {
			return false
		}
		return applyScheduleFetchError(props, err)
	}

	// Player-based fetch always includes past games so results are visible.
	var games []types.Game
	var err error
	if len(input.PlayerIDs) > 0 {
		games, err = h.RequestedAllScheduleGames(ctx, session, input.PlayerIDs, input.TeamCodes)
	} else {
		games, err = h.RequestedScheduleGames(ctx, session, input.PlayerIDs, input.TeamCodes)
	}
	if err == nil {
		props.Games = games
		return false
	}
	if applyScheduleSelectionError(props, err) {
		return false
	}

	return applyScheduleFetchError(props, err)
}

func (h *Handler) resolveScheduleGames(ctx context.Context, session *types.SessionData, playerIDs, teamIDs []int) ([]types.Game, error) {
	switch {
	case session != nil && len(playerIDs) > 0:
		return lps.FetchGamesForPlayers(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, session.JWT, playerIDs)
	case len(teamIDs) > 0:
		return lps.FetchGamesForTeams(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, teamIDs)
	default:
		return nil, nil
	}
}

func (h *Handler) resolveAllScheduleGames(ctx context.Context, session *types.SessionData, playerIDs, teamIDs []int) ([]types.Game, error) {
	switch {
	case session != nil && len(playerIDs) > 0:
		return lps.FetchAllGamesForPlayers(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, session.JWT, playerIDs)
	case len(teamIDs) > 0:
		return lps.FetchAllGamesForTeams(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, teamIDs)
	default:
		return nil, nil
	}
}

func (h *Handler) requestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string, includePast bool) ([]types.Game, error) {
	teamIDs := parseTeamIDs(teamCodes)
	hasSelectedPlayers := len(playerIDs) > 0
	hasManualTeamInput := strings.TrimSpace(teamCodes) != ""

	if hasSelectedPlayers {
		if session == nil {
			return nil, ErrPlayerSessionRequired
		}
		if includePast {
			return h.resolveAllScheduleGames(ctx, session, playerIDs, nil)
		}
		return h.resolveScheduleGames(ctx, session, playerIDs, nil)
	}

	switch {
	case hasManualTeamInput && len(teamIDs) == 0:
		return nil, ErrInvalidTeamSelection
	case len(teamIDs) > 0:
		if includePast {
			return h.resolveAllScheduleGames(ctx, session, nil, teamIDs)
		}
		return h.resolveScheduleGames(ctx, session, nil, teamIDs)
	default:
		return nil, ErrScheduleSelection
	}
}

func applyScheduleFetchError(props *partials.SoccerTableFragmentProps, fetchErr error) bool {
	detail := lps.ScheduleErrorDetailsFor(fetchErr)
	if errors.Is(fetchErr, ErrSessionExpired) {
		detail = lps.ScheduleErrorDetails{
			ClearSession:    true,
			FeedbackMessage: "Your imported Let's Play Soccer token expired.",
			FeedbackHint:    "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.",
		}
	}
	if fetchErr != nil && !detail.ClearSession {
		logging.Component("soccer").Error("soccer LPS fetch failed", slog.Any("error", fetchErr))
	}
	props.Message = detail.FeedbackMessage
	props.Hint = detail.FeedbackHint
	return detail.ClearSession
}

func (h *Handler) handleScheduleDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	detail := lps.ScheduleErrorDetailsFor(err)
	if errors.Is(err, ErrSessionExpired) {
		detail = lps.ScheduleErrorDetails{
			ClearSession:    true,
			DownloadMessage: "your imported Let's Play Soccer token expired; import a fresh bearer JWT from letsplaysoccer.com and try again",
			DownloadStatus:  http.StatusUnauthorized,
		}
	}
	if detail.DownloadStatus == http.StatusUnauthorized || detail.DownloadStatus == http.StatusBadRequest {
		detail.ClearSession = true
	}
	if detail.ClearSession {
		h.clearSession(w, r)
	}
	http.Error(w, detail.DownloadMessage, detail.DownloadStatus)
}

// RequestedScheduleGames resolves schedule data from either imported players or manual team IDs.
func (h *Handler) RequestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error) {
	return h.requestedScheduleGames(ctx, session, playerIDs, teamCodes, false)
}

// RequestedAllScheduleGames resolves both past and upcoming games for the current selection.
func (h *Handler) RequestedAllScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error) {
	return h.requestedScheduleGames(ctx, session, playerIDs, teamCodes, true)
}

func selectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game {
	filteredGames := make([]types.Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	return filteredGames
}

func applyScheduleSelectionError(props *partials.SoccerTableFragmentProps, err error) bool {
	message, hint, ok := scheduleSelectionFeedback(err)
	if !ok {
		return false
	}
	props.Message = message
	props.Hint = hint
	return true
}

func scheduleSelectionFeedback(err error) (message, hint string, ok bool) {
	switch {
	case errors.Is(err, ErrPlayerSessionRequired):
		return "Import a bearer JWT again to fetch schedules for your discovered players.",
			"Your previous session is no longer available.", true
	case errors.Is(err, ErrInvalidTeamSelection):
		return invalidTeamIDsMessage, invalidTeamIDsHint, true
	case errors.Is(err, ErrScheduleSelection):
		return "Enter team IDs or choose at least one discovered player.",
			"Manual team ID entry still works if you do not want to import a token.", true
	default:
		return "", "", false
	}
}

func parseSelectedIDs(form url.Values) map[string]struct{} {
	selectedIDs := make(map[string]struct{})
	for _, id := range form["selected"] {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = struct{}{}
		}
	}
	return selectedIDs
}

func parseScheduleRequest(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func parsePlayerIDs(values []string) []int {
	return parsePositiveUniqueIDs(values)
}

func parseTeamIDs(raw string) []int {
	return parsePositiveUniqueIDs(splitDelimitedValues(raw))
}

func hasInvalidPlayerInput(rawValues []string, playerIDs []int) bool {
	if len(playerIDs) > 0 {
		return false
	}

	return hasNonEmptyTrimmedValues(rawValues)
}

// scheduleFormInput holds the parsed form values shared by schedule handlers.
type scheduleFormInput struct {
	TeamCodes    string
	RawPlayerIDs []string
	PlayerIDs    []int
	TeamIDs      []int
}

func parseScheduleFormInput(form url.Values) scheduleFormInput {
	rawPlayerIDs := form["player_ids"]
	return scheduleFormInput{
		TeamCodes:    form.Get("team_codes"),
		RawPlayerIDs: rawPlayerIDs,
		PlayerIDs:    parsePlayerIDs(rawPlayerIDs),
		TeamIDs:      parsePlayerIDs(form["team_ids"]),
	}
}

func parsePositiveUniqueIDs(values []string) []int {
	seen := make(map[int]struct{})
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func hasNonEmptyTrimmedValues(rawValues []string) bool {
	for _, value := range rawValues {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}

	return false
}

func splitDelimitedValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
}

// DiscoverTeamsHandler fetches current LPS teams for the selected players and
// returns the team-selection fragment so users can include/exclude teams before
// fetching schedules.
func (h *Handler) DiscoverTeamsHandler(w http.ResponseWriter, r *http.Request) {
	if !parseScheduleRequest(w, r) {
		return
	}

	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(rawPlayerIDs)
	if hasInvalidPlayerInput(rawPlayerIDs, playerIDs) {
		h.setHTMLContentType(w)
		if err := partials.SoccerTableFragment(partials.SoccerTableFragmentProps{
			Message: invalidPlayersMessage,
			Hint:    invalidPlayersHint,
		}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	session, _ := h.LoadSession(w, r)
	if len(playerIDs) == 0 || session == nil {
		h.setHTMLContentType(w)
		if err := partials.SoccerTableFragment(partials.SoccerTableFragmentProps{
			Message: "Import a bearer JWT to discover teams.",
			Hint:    "Choose at least one player after importing.",
		}).Render(r.Context(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	knownTeamIDs := make(map[int]struct{}, len(session.KnownTeams))
	for _, t := range session.KnownTeams {
		knownTeamIDs[t.TeamID] = struct{}{}
	}

	resolver := lps.NewScheduleResolver(h.Config.LPSAPIBaseURL, h.LPSClient, session.JWT)
	playerMap := make(map[int]types.LPSPlayer, len(session.Players))
	for _, p := range session.Players {
		playerMap[p.UPlayerID] = p
	}

	var groups []types.PlayerTeamGroup
	for _, playerID := range playerIDs {
		player, ok := playerMap[playerID]
		if !ok {
			continue
		}
		rawTeams, err := resolver.FetchPlayerTeams(r.Context(), playerID)
		if err != nil {
			continue
		}
		var teams []types.LPSTeam
		for _, t := range rawTeams {
			if t.UTeamID <= 0 {
				continue
			}
			_, isKnown := knownTeamIDs[t.UTeamID]
			teams = append(teams, types.LPSTeam{
				TeamID:    t.UTeamID,
				TeamName:  t.TeamName,
				Season:    t.Season,
				PlayerID:  playerID,
				IsSubTeam: !isKnown,
			})
		}
		if len(teams) > 0 {
			groups = append(groups, types.PlayerTeamGroup{Player: player, Teams: teams})
		}
	}

	h.setHTMLContentType(w)
	if err := partials.SoccerTeamSelect(partials.SoccerTeamSelectProps{
		PlayerGroups: groups,
		PlayerIDs:    playerIDs,
	}).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func joinIntSlice(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
