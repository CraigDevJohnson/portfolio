package soccer

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/types"
)

func (h *Handler) FetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
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
		if err := partials.SoccerLoginState(h.LoginStateProps(w, r, nil, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := partials.SoccerTableFragment(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) SubscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := io.WriteString(w, `<div class="subscribe-success">✅ Subscribed! Check your email to confirm.</div>`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) DownloadICSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
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
		log.Printf("soccer LPS fetch failed: %v", err)
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

func applyScheduleFetchError(props *partials.SoccerTableFragmentProps, fetchErr error) bool {
	message, hint, clearSession := lps.ScheduleFetchFeedback(fetchErr)
	if errors.Is(fetchErr, ErrSessionExpired) {
		message = "Your imported Let's Play Soccer token expired."
		hint = "Copy a fresh bearer JWT from letsplaysoccer.com and import it again."
		clearSession = true
	}
	if fetchErr != nil && !clearSession {
		log.Printf("soccer LPS fetch failed: %v", fetchErr)
	}
	props.Message = message
	props.Hint = hint
	return clearSession
}

func (h *Handler) handleScheduleDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	_, _, shouldClearSession := lps.ScheduleFetchFeedback(err)
	if errors.Is(err, ErrSessionExpired) {
		shouldClearSession = true
	}
	if shouldClearSession {
		h.clearSession(w, r)
	}
	status, message := lps.ScheduleDownloadError(err)
	if status == http.StatusUnauthorized || status == http.StatusBadRequest {
		h.clearSession(w, r)
	}
	http.Error(w, message, status)
}
