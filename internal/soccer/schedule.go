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

	props := partials.SoccerTableFragmentProps{
		TeamCodes: input.TeamCodes,
		PlayerIDs: input.PlayerIDs,
	}
	if h.googleHooks != nil {
		props.GoogleConnected = h.googleHooks.GoogleConnected(r.Context(), w, r)
	}
	if h.resolveScheduleData(r.Context(), session, input, &props) {
		h.clearSession(w, r)
		swapAuthState = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
		props.Message = "One or more selected players were invalid."
		props.Hint = "Clear the imported players and import again to refresh the discovered player list."
		return false
	}

	games, err := h.RequestedScheduleGames(ctx, session, input.PlayerIDs, input.TeamCodes)
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
		return "One or more team IDs were invalid.",
			"Enter numeric Let's Play Soccer team IDs separated by commas.", true
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
}

func parseScheduleFormInput(form url.Values) scheduleFormInput {
	rawPlayerIDs := form["player_ids"]
	return scheduleFormInput{
		TeamCodes:    form.Get("team_codes"),
		RawPlayerIDs: rawPlayerIDs,
		PlayerIDs:    parsePlayerIDs(rawPlayerIDs),
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
