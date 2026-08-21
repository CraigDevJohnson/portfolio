package soccer

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/internal/logging"
	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/types"
)

// SoccerPage renders the full soccer page.
func (h *Handler) SoccerPage(w http.ResponseWriter, r *http.Request) {
	session, _ := h.LoadSession(w, r)
	authState := h.LoginStateProps(w, r, session, false)
	googleMessageKind, googleMessage := soccerGoogleFlash(r.URL.Query().Get("google"), authState.GoogleAvailable, authState.GoogleConnected)
	teamSelection, initialResults, restoreFeedback, manualTeamCodes := h.restoreSoccerWorkflow(r.Context(), session)
	if initialResults != nil {
		initialResults.GoogleAvailable = authState.GoogleAvailable
		initialResults.GoogleConnected = authState.GoogleConnected
		initialResults.ImportAvailable = authState.LoginAvailable
	}
	props := pages.SoccerProps{
		GoogleMessage:        googleMessage,
		GoogleMessageKind:    googleMessageKind,
		AuthState:            authState,
		InitialTeamSelection: teamSelection,
		InitialResults:       initialResults,
		InitialFeedback:      restoreFeedback,
		ManualTeamCodes:      manualTeamCodes,
	}
	if err := pages.Soccer(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer page render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) restoreSoccerWorkflow(parent context.Context, session *types.SessionData) (*partials.SoccerTeamSelectProps, *partials.SoccerTableFragmentProps, *partials.SoccerLoginFeedbackProps, string) {
	if session == nil || session.Workflow.Source == "" {
		return nil, nil, nil, ""
	}
	workflow := normalizeWorkflowState(&session.Workflow, session.Players)
	timeout := 15 * time.Second
	if h.LPSClient != nil && h.LPSClient.Timeout > 0 && h.LPSClient.Timeout < timeout {
		timeout = h.LPSClient.Timeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var teamSelection *partials.SoccerTeamSelectProps
	if workflow.Source == "imported" && len(workflow.SelectedPlayerIDs) > 0 {
		groups := h.resolvePlayerTeams(ctx, session, workflow.SelectedPlayerIDs)
		teamSelection = &partials.SoccerTeamSelectProps{
			PlayerGroups:    groups,
			PlayerIDs:       workflow.SelectedPlayerIDs,
			SelectedTeamIDs: workflow.SelectedTeamIDs,
		}
	}
	if len(workflow.SelectedTeamIDs) == 0 {
		return teamSelection, nil, nil, ""
	}

	games, err := lps.FetchAllGamesForTeams(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, workflow.SelectedTeamIDs)
	if err != nil {
		feedback := &partials.SoccerLoginFeedbackProps{
			Kind:    "error",
			Message: "Your saved player and team choices were restored, but the schedule could not be refreshed. Try fetching the selected schedules again.",
		}
		return teamSelection, nil, feedback, joinIntSlice(workflow.SelectedTeamIDs)
	}
	results := &partials.SoccerTableFragmentProps{
		TeamCodes: joinIntSlice(workflow.SelectedTeamIDs),
	}
	if workflow.Source == "imported" {
		results.PlayerIDs = workflow.SelectedPlayerIDs
	}
	setTableFragmentGames(results, games)
	manualCodes := ""
	if workflow.Source == "manual" {
		manualCodes = results.TeamCodes
	}
	return teamSelection, results, nil, manualCodes
}

// LoginStateProps builds the shared login-state fragment props.
func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil && strings.TrimSpace(session.JWT) != "",
		GoogleAvailable: h.googleAvailable(),
		LoginAvailable:  h.Config.LoginEnabled(),
		SwapOOB:         swapOOB,
		ResetWorkflow:   swapOOB,
	}
	if session != nil {
		props.Players = session.Players
		props.SelectedPlayerIDs = session.Workflow.SelectedPlayerIDs
		props.SelectedTeamIDs = session.Workflow.SelectedTeamIDs
		props.ConfirmedTeamCount = confirmedTeamChoiceCount(&session.Workflow)
	}
	if h.googleHooks != nil {
		h.googleHooks.PopulateLoginState(r.Context(), w, r, &props)
	}
	return props
}

func confirmedTeamChoiceCount(workflow *types.SoccerWorkflowState) int {
	if workflow == nil || len(workflow.SelectedTeamIDs) == 0 {
		return 0
	}
	selected := make(map[int]struct{}, len(workflow.SelectedTeamIDs))
	for _, teamID := range workflow.SelectedTeamIDs {
		if teamID > 0 {
			selected[teamID] = struct{}{}
		}
	}
	return len(selected)
}

// RenderLoginStateOOB refreshes only the Google connection card OOB when the
// request's primary target is a local action-feedback region.
func (h *Handler) RenderLoginStateOOB(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	h.setHTMLContentType(w)
	props := h.LoginStateProps(w, r, session, false)
	props.RefreshCalendar = true
	if err := partials.SoccerGoogleConnection(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer OOB login state render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// RenderLoginStateRefresh replaces only the Google connection card without
// discarding the loaded workflow.
func (h *Handler) RenderLoginStateRefresh(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	h.setHTMLContentType(w)
	props := h.LoginStateProps(w, r, session, false)
	if err := partials.SoccerGoogleConnection(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer login state refresh render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// RenderWorkflowReset renders the unauthenticated Source state and clears all
// downstream dynamic stages after logout or an expired imported session.
func (h *Handler) RenderWorkflowReset(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	h.setHTMLContentType(w)
	props := h.LoginStateProps(w, r, session, false)
	props.ResetWorkflow = true
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer workflow reset render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

func soccerGoogleFlash(code string, available, connected bool) (kind, message string) {
	switch strings.TrimSpace(code) {
	case "connected":
		if !available {
			return "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
		}
		if !connected {
			return "error", "Google Calendar connection was not restored. Connect again; your imported players, teams, and schedule are still available below."
		}
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
	selectedIDs := parseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		return nil, nil, "Select at least one past result to sync.", false
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

	filteredGames := selectedScheduleGames(schedule.PastGamesWithResults(games), selectedIDs)
	if len(filteredGames) == 0 {
		return nil, nil, "No selected past results were found to sync.", false
	}

	return session, filteredGames, "", true
}
