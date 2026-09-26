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
	"portfolio/internal/siteidentity"
	"portfolio/internal/soccerarchive"
	"portfolio/types"
)

// SoccerPage renders the full soccer page.
func (h *Handler) SoccerPage(w http.ResponseWriter, r *http.Request) {
	session, _ := h.LoadSession(w, r)
	teamSelection, initialResults, restoreFeedback, manualTeamCodes, restoreErr := h.restoreSoccerWorkflow(r.Context(), session)
	var importNotice *partials.FeedbackProps
	if restoreErr != nil {
		detail := lps.ScheduleErrorDetailsFor(restoreErr)
		importNotice = importNoticeFor(detail)
		if detail.ClearSession {
			h.clearSession(w, r)
			session = nil
			teamSelection = nil
			initialResults = nil
			restoreFeedback = nil
			manualTeamCodes = ""
		}
	}
	authState := h.LoginStateProps(w, r, session, false)
	if authState.LoginAvailable {
		authState.ImportNotice = importNotice
	}
	privateAccessMessage, showSiteSignIn := PrivateAccessNotice(r.Context(), &authState)
	googleMessageKind, googleMessage := soccerGoogleFlash(r.URL.Query().Get("google"), authState.GoogleAvailable, authState.GoogleConnected)
	if initialResults != nil {
		initialResults.GoogleAvailable = authState.GoogleAvailable
		initialResults.GoogleConnected = authState.GoogleConnected
		initialResults.ImportAvailable = authState.LoginAvailable
	}
	props := pages.SoccerProps{
		GoogleMessage:            googleMessage,
		GoogleMessageKind:        googleMessageKind,
		PrivateAccessMessage:     privateAccessMessage,
		ShowSiteSignIn:           showSiteSignIn,
		HistoryCollectionEnabled: h.historyCollectionEnabled(),
		PlayerRemovalMessage:     playerRemovalMessage(r.URL.Query().Get("player_removed")),
		AuthState:                authState,
		InitialTeamSelection:     teamSelection,
		InitialResults:           initialResults,
		InitialFeedback:          restoreFeedback,
		ManualTeamCodes:          manualTeamCodes,
	}
	if err := pages.Soccer(props).Render(r.Context(), w); err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("soccer page render failed", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func playerRemovalMessage(result string) string {
	if result == "1" {
		return "The retained player data was removed. Your current LPS import has been cleared. A later valid import may collect that player again."
	}
	return ""
}

func (h *Handler) historyCollectionEnabled() bool {
	_, enabled := h.ArchiveStore().(soccerarchive.MembershipStore)
	return enabled
}

func (h *Handler) playerRemovalEnabled() bool {
	_, enabled := h.ArchiveStore().(soccerarchive.PlayerRemovalStore)
	return enabled
}

func (h *Handler) restoreSoccerWorkflow(parent context.Context, session *types.SessionData) (*partials.SoccerTeamSelectProps, *partials.SoccerTableFragmentProps, *partials.SoccerLoginFeedbackProps, string, error) {
	if session == nil || session.Workflow.Source == "" {
		return nil, nil, nil, "", nil
	}
	workflow := normalizeWorkflowState(&session.Workflow, session.Players)
	timeout := 15 * time.Second
	if h.LPSClient != nil && h.LPSClient.Timeout > 0 && h.LPSClient.Timeout < timeout {
		timeout = h.LPSClient.Timeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var teamSelection *partials.SoccerTeamSelectProps
	// teamsErr reports linked teams LPS could not serve while the import
	// stays usable; the saved teams' public schedules are still restored.
	var teamsErr error
	if workflow.Source == "imported" && len(workflow.SelectedPlayerIDs) > 0 {
		teams, err := h.resolvePlayerTeams(ctx, session, workflow.SelectedPlayerIDs)
		switch {
		case err == nil:
			teamSelection = &partials.SoccerTeamSelectProps{
				PlayerGroups:    teams.groups,
				PlayerIDs:       workflow.SelectedPlayerIDs,
				SelectedTeamIDs: workflow.SelectedTeamIDs,
				Notice:          teams.notice(),
			}
		case lps.ScheduleErrorDetailsFor(err).ClearSession:
			return nil, nil, nil, "", err
		default:
			teamsErr = err
		}
	}
	if len(workflow.SelectedTeamIDs) == 0 {
		return teamSelection, nil, nil, "", teamsErr
	}

	games, err := lps.FetchAllGamesForTeams(ctx, h.Config.LPSAPIBaseURL, h.LPSClient, workflow.SelectedTeamIDs)
	if err != nil {
		feedback := &partials.SoccerLoginFeedbackProps{
			Kind:    "error",
			Message: "Your saved player and team choices were restored, but the schedule could not be refreshed. Try fetching the selected schedules again.",
		}
		return teamSelection, nil, feedback, joinIntSlice(workflow.SelectedTeamIDs), teamsErr
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
	return teamSelection, results, nil, manualCodes, teamsErr
}

// importNoticeFor explains imported LPS access that LPS refused or could not
// serve, in the LPS connection card beside its recovery actions.
func importNoticeFor(detail lps.ScheduleErrorDetails) *partials.FeedbackProps {
	title := "Linked teams could not be loaded"
	if detail.ClearSession {
		title = "Imported LPS access ended"
	}
	return &partials.FeedbackProps{
		Kind:       partials.FeedbackError,
		Title:      title,
		Message:    detail.FeedbackMessage + " " + detail.FeedbackHint,
		ExtraClass: "soccer-stage-feedback",
	}
}

// publicSoccerPaths closes every private Soccer access explanation.
const publicSoccerPaths = " Team ID lookup and .ics file downloads are still available."

// signInUnavailableMessage explains private access where site sign-in cannot start.
const signInUnavailableMessage = "Linked-player import and Google Calendar need a site account with Soccer access, and site sign-in is not available here." + publicSoccerPaths

// PrivateAccessNotice explains why the visitor cannot use the private Soccer
// actions this server offers and reports whether site sign-in could change
// that. It is empty when nothing offered is withheld.
func PrivateAccessNotice(ctx context.Context, state *partials.SoccerLoginStateProps) (message string, offerSignIn bool) {
	if !state.ImportNeedsGrant && !state.GoogleNeedsGrant {
		return "", false
	}
	if _, signedIn := siteidentity.PrincipalFromContext(ctx); signedIn {
		return "This account has not been granted access to linked players or Google Calendar." + publicSoccerPaths, false
	}
	if !siteidentity.SignInAvailable(ctx) {
		return signInUnavailableMessage, false
	}
	return "Sign in with an invited account to import linked players or connect Google Calendar." + publicSoccerPaths, true
}

// RefusePrivateAction answers a private Soccer request the current site
// session cannot use: 401 without a signed-in account and 403 without the
// soccer grant. A control on an already-open page receives an explanation
// htmx swaps into that control's own target, so an expired session or a
// revoked grant is visible where the visitor acted; other callers receive
// plain text.
func RefusePrivateAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, signedIn := siteidentity.PrincipalFromContext(ctx)
	status, plain := http.StatusUnauthorized, "Sign in with a Soccer grant to use this action."
	notice := partials.SoccerPrivateAccessProps{Kind: partials.FeedbackError}
	switch {
	case signedIn:
		status, plain = http.StatusForbidden, "Soccer access has not been granted to this account."
		notice.Message = plain + publicSoccerPaths
	case !siteidentity.SignInAvailable(ctx):
		notice.Message = signInUnavailableMessage
	default:
		notice.Message = "Sign in again to use Soccer actions." + publicSoccerPaths
		notice.OfferSignIn = true
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Error(w, plain, status)
		return
	}
	w.Header().Set("Content-Type", htmlContentType)
	w.Header().Set("X-Portal-Fragment-Error", "true")
	w.Header().Set("HX-Reswap", "innerHTML")
	w.WriteHeader(status)
	// The status is already sent; a render error can only be a failed write.
	_ = partials.SoccerPrivateAccess(notice).Render(ctx, w)
}

// LoginStateProps builds the shared login-state fragment props.
func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	privateAllowed := siteidentity.SoccerPrivateAllowed(r.Context())
	if !privateAllowed {
		session = nil
	}
	props := partials.SoccerLoginStateProps{
		Authenticated:           session != nil && strings.TrimSpace(session.JWT) != "",
		GoogleAvailable:         privateAllowed && h.googleAvailable(),
		LoginAvailable:          privateAllowed && h.Config.LoginEnabled(),
		HistoryRemovalAvailable: privateAllowed && h.playerRemovalEnabled(),
		GoogleNeedsGrant:        !privateAllowed && h.googleAvailable(),
		ImportNeedsGrant:        !privateAllowed && h.Config.LoginEnabled(),
		SwapOOB:                 swapOOB,
		ResetWorkflow:           swapOOB,
	}
	if principal, ok := siteidentity.PrincipalFromContext(r.Context()); ok && privateAllowed {
		props.GoogleSuggestedEmail = principal.Email
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

	filteredGames := selectedScheduleGames(schedule.UpcomingScheduleGames(games), selectedIDs)
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
