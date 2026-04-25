package soccer

import (
	"log/slog"
	"net/http"
	"strings"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/internal/logging"
	"portfolio/internal/schedule"
	"portfolio/types"
)

// SoccerPage renders the full soccer page.
func (h *Handler) SoccerPage(w http.ResponseWriter, r *http.Request) {
	googleMessageKind, googleMessage := soccerGoogleFlash(r.URL.Query().Get("google"))
	props := pages.SoccerProps{
		GoogleMessage:     googleMessage,
		GoogleMessageKind: googleMessageKind,
	}
	if err := pages.Soccer(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer page render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// LoginStateProps builds the shared login-state fragment props.
func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: h.Config.GoogleEnabled(),
		LoginAvailable:  h.Config.LoginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.Players = session.Players
	}
	if h.googleHooks != nil {
		h.googleHooks.PopulateLoginState(r.Context(), w, r, &props)
	}
	return props
}

// RenderLoginState renders the soccer login-state fragment.
func (h *Handler) RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	h.setHTMLContentType(w)
	props := h.LoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer login state render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// RenderLoginFeedback renders a login feedback fragment.
func (h *Handler) RenderLoginFeedback(w http.ResponseWriter, r *http.Request, kind, message string) {
	h.setHTMLContentType(w)
	props := partials.SoccerLoginFeedbackProps{Kind: kind, Message: message}
	if err := partials.SoccerLoginFeedback(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer login feedback render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerGoogleFlash(code string) (kind, message string) {
	switch strings.TrimSpace(code) {
	case "connected":
		return "success", "Google Calendar connected. Choose a calendar below and add selected games directly from the schedule table."
	case "denied":
		return "error", "Google Calendar connection was canceled before access was granted."
	case "disconnected":
		return "success", "Google Calendar connection removed."
	case "failed":
		return "error", "Google Calendar connection could not be completed. Try again."
	case "unavailable":
		return "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	default:
		return "", ""
	}
}

func googleAddScheduleErrorMessage(err error) string {
	message, _, ok := scheduleSelectionFeedback(err)
	if ok {
		return message
	}
	return err.Error()
}

func (h *Handler) ResolveGoogleAddSelection(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool) {
	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		return nil, nil, "Select at least one game to add to Google Calendar.", false
	}

	input := parseScheduleFormInput(r.Form)
	if hasInvalidPlayerInput(input.RawPlayerIDs, input.PlayerIDs) {
		return nil, nil, invalidPlayersMessage + " " + invalidPlayersHint, false
	}

	session, _ := h.LoadSession(w, r)
	games, err := h.RequestedAllScheduleGames(r.Context(), session, input.PlayerIDs, input.TeamCodes)
	if err != nil {
		return nil, nil, googleAddScheduleErrorMessage(err), false
	}

	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		return nil, nil, "No selected games were found to add.", false
	}

	return session, filteredGames, "", true
}

func (h *Handler) ResolveSyncResultsGames(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool) {
	input := parseScheduleFormInput(r.Form)
	if hasInvalidPlayerInput(input.RawPlayerIDs, input.PlayerIDs) {
		return nil, nil, invalidPlayersMessage + " " + invalidPlayersHint, false
	}

	session, _ := h.LoadSession(w, r)
	games, err := h.RequestedAllScheduleGames(r.Context(), session, input.PlayerIDs, input.TeamCodes)
	if err != nil {
		return nil, nil, googleAddScheduleErrorMessage(err), false
	}

	return session, schedule.PastGamesWithResults(games), "", true
}
