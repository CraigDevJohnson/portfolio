// HTTP handlers for soccer pages, JWT import, schedule fetch, ICS download, and login state rendering.
package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"strings"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
)

func soccerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("code") != "" || r.URL.Query().Get("error") != "" || r.URL.Query().Get("state") != "" {
		soccerGoogleCallbackHandler(w, r)
		return
	}
	props := pages.SoccerProps{
		GoogleMessage:     soccerGoogleFlashMessage(r.URL.Query().Get("google")),
		GoogleMessageKind: soccerGoogleFlashKind(r.URL.Query().Get("google")),
	}
	err := pages.Soccer(props).Render(context.Background(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerSessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session, _ := loadSoccerSession(w, r)

	renderSoccerLoginState(w, r, session)
}

func soccerImportHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !loginEnabled() {
		renderSoccerLoginFeedback(w, "error", "JWT import is unavailable until the session encryption key is configured on the server.")
		return
	}
	if !soccerLoginAttempts.Allow(clientIP(r)) {
		renderSoccerLoginFeedback(w, "error", "Too many import attempts. Wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginFeedback(w, "error", "Could not read the import form. Try again.")
		return
	}

	jwt, err := normalizeImportedJWT(r.FormValue("jwt"))
	if err != nil {
		renderSoccerLoginFeedback(w, "error", err.Error())
		return
	}

	discovery, err := lpsFetchUserPlayers(r.Context(), jwt)
	if err != nil {
		var fetchErr *lpsFetchError
		if errors.As(err, &fetchErr) {
			switch fetchErr.Kind {
			case lpsErrorUnauthorized, lpsErrorForbidden:
				renderSoccerLoginFeedback(w, "error", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
				return
			case lpsErrorUpstream:
				renderSoccerLoginFeedback(w, "error", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
				return
			}
		}
		renderSoccerLoginFeedback(w, "error", err.Error())
		return
	}
	if len(discovery.Players) == 0 {
		renderSoccerLoginFeedback(w, "error", "No linked players found for this account.")
		return
	}

	session := SessionData{
		JWT:       jwt,
		UserName:  discovery.UserName,
		Players:   discovery.Players,
		ExpiresAt: importedSessionExpiry(jwt),
	}
	if err := setSession(w, r, &session); err != nil {
		log.Printf("soccer import session write failed: %v", err)
		renderSoccerLoginFeedback(w, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, &session, true)).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved for this browser session. Choose your players below.</div>`)
}

func soccerLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	renderSoccerLoginState(w, r, nil)
}

func fetchSchedulesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(r.Form["player_ids"])
	session, swapAuthState := loadSoccerSession(w, r)

	googleConnected := false
	if googleEnabled() {
		record, googleErr := loadGoogleConnectionRecord(r.Context(), r)
		if googleErr != nil {
			log.Printf("google connection read failed: %v", googleErr)
			clearGoogleConnectionCookie(w, r)
		} else {
			googleConnected = record != nil
		}
	}
	props := partials.SoccerTableFragmentProps{TeamCodes: teamCodes, PlayerIDs: playerIDs, GoogleConnected: googleConnected}
	if resolveScheduleData(r.Context(), session, playerIDs, teamCodes, rawPlayerIDs, &props) {
		clearSession(w, r)
		swapAuthState = true
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if swapAuthState {
		if err := partials.SoccerLoginState(soccerLoginStateProps(w, r, nil, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := partials.SoccerTableFragment(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, err := io.WriteString(w, `<div class="subscribe-success">✅ Subscribed! Check your email to confirm.</div>`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func downloadICSHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	selectedIDs := make(map[string]struct{})
	for _, selectedID := range r.Form["selected"] {
		selectedID = strings.TrimSpace(selectedID)
		if selectedID != "" {
			selectedIDs[selectedID] = struct{}{}
		}
	}
	if len(selectedIDs) == 0 {
		http.Error(w, "select at least one game", http.StatusBadRequest)
		return
	}

	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := parsePlayerIDs(r.Form["player_ids"])
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		http.Error(w, "one or more selected players were invalid; clear the imported players and import again to refresh the discovered player list", http.StatusBadRequest)
		return
	}
	session, _ := loadSoccerSession(w, r)

	games, err := requestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if errors.Is(err, errPlayerSessionRequired) {
		http.Error(w, "import a bearer JWT again before downloading schedules for your discovered players", http.StatusUnauthorized)
		return
	}
	if errors.Is(err, errInvalidTeamSelection) {
		http.Error(w, "one or more team IDs were invalid; enter numeric Let's Play Soccer team IDs separated by commas", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("soccer LPS fetch failed: %v", err)
		handleScheduleDownloadError(w, r, err)
		return
	}

	filteredGames := make([]Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	if len(filteredGames) == 0 {
		http.Error(w, "no selected games were found", http.StatusBadRequest)
		return
	}

	icsContent := buildICS(filteredGames)

	w.Header().Set("Content-Type", "text/calendar")
	w.Header().Set("Content-Disposition", "attachment; filename=soccer_schedule.ics")
	_, err = io.WriteString(w, icsContent) //nolint:gosec // ICS content is returned as a calendar attachment, not rendered as HTML.
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerLoginStateProps(w http.ResponseWriter, r *http.Request, session *SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: googleEnabled(),
		LoginAvailable:  loginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.UserName = session.UserName
		props.Players = session.Players
	}
	if !props.GoogleAvailable {
		return props
	}
	record, err := loadGoogleConnectionRecord(r.Context(), r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		clearGoogleConnectionCookie(w, r)
		return props
	}
	if record == nil {
		return props
	}
	calendars, err := googleListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		var apiErr *googleAPIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			deleteGoogleConnection(r.Context(), w, r)
		}
		return props
	}
	props.GoogleConnected = true
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = syncGoogleCalendarSelection(r.Context(), record, calendars)
	return props
}

func renderSoccerLoginState(w http.ResponseWriter, r *http.Request, session *SessionData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := soccerLoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderSoccerLoginFeedback(w http.ResponseWriter, kind, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	role := "status"
	if kind == "error" {
		role = "alert"
	}
	_, _ = io.WriteString(w, fmt.Sprintf(`<div class="soccer-login-message soccer-login-message-%s" role="%s">%s</div>`, kind, role, html.EscapeString(message)))
}

func populateScheduleProps(ctx context.Context, session *SessionData, playerIDs []int, props *partials.SoccerTableFragmentProps) bool {
	games, fetchErr := resolveScheduleGames(ctx, session, playerIDs, nil)
	message, hint, clearSession := scheduleFetchFeedback(fetchErr)
	if fetchErr != nil && !clearSession {
		log.Printf("soccer LPS fetch failed: %v", fetchErr)
	}
	if fetchErr != nil {
		props.Message = message
		props.Hint = hint
	} else {
		props.Games = games
	}
	return clearSession
}

func resolveScheduleData(ctx context.Context, session *SessionData, playerIDs []int, teamCodes string, rawPlayerIDs []string, props *partials.SoccerTableFragmentProps) bool {
	if len(nonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		props.Message = "One or more selected players were invalid."
		props.Hint = "Clear the imported players and import again to refresh the discovered player list."
		return false
	}
	if session != nil && len(playerIDs) > 0 {
		return populateScheduleProps(ctx, session, playerIDs, props)
	}
	if len(playerIDs) > 0 {
		props.Message = "Import a bearer JWT again to fetch schedules for your discovered players."
		props.Hint = "Your previous session is no longer available."
		return false
	}
	if strings.TrimSpace(teamCodes) != "" {
		teamIDs := parseTeamIDs(teamCodes)
		if len(teamIDs) == 0 {
			props.Message = "One or more team IDs were invalid."
			props.Hint = "Enter numeric Let's Play Soccer team IDs separated by commas."
			return false
		}
		games, fetchErr := resolveScheduleGames(ctx, session, playerIDs, teamIDs)
		message, hint, clearSession := scheduleFetchFeedback(fetchErr)
		if fetchErr != nil && !clearSession {
			log.Printf("soccer LPS fetch failed: %v", fetchErr)
		}
		if fetchErr != nil {
			props.Message = message
			props.Hint = hint
			return clearSession
		}
		props.Games = games
		return false
	}
	props.Message = "Enter team IDs or choose at least one discovered player."
	props.Hint = "Manual team ID entry still works if you do not want to import a token."
	return false
}

func resolveScheduleGames(ctx context.Context, session *SessionData, playerIDs, teamIDs []int) ([]Game, error) {
	switch {
	case session != nil && len(playerIDs) > 0:
		return lpsFetchGamesForPlayers(ctx, session.JWT, playerIDs)
	case len(teamIDs) > 0:
		return lpsFetchGamesForTeams(ctx, teamIDs)
	default:
		return nil, nil
	}
}

func handleScheduleDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	_, _, shouldClearSession := scheduleFetchFeedback(err)
	if shouldClearSession {
		clearSession(w, r)
	}
	status, message := scheduleDownloadError(err)
	if status == http.StatusUnauthorized || status == http.StatusBadRequest {
		clearSession(w, r)
	}
	http.Error(w, message, status)
}

func requestedScheduleGames(ctx context.Context, session *SessionData, playerIDs []int, teamCodes string) ([]Game, error) {
	teamIDs := parseTeamIDs(teamCodes)
	switch {
	case session != nil && len(playerIDs) > 0:
		return resolveScheduleGames(ctx, session, playerIDs, nil)
	case len(playerIDs) > 0:
		return nil, errPlayerSessionRequired
	case strings.TrimSpace(teamCodes) != "" && len(teamIDs) == 0:
		return nil, errInvalidTeamSelection
	case len(teamIDs) > 0:
		return resolveScheduleGames(ctx, session, nil, teamIDs)
	default:
		return nil, errScheduleSelection
	}
}

func googleAddScheduleErrorMessage(err error) string {
	switch {
	case errors.Is(err, errPlayerSessionRequired):
		return "Import a bearer JWT again before adding schedules for discovered players to Google Calendar."
	case errors.Is(err, errInvalidTeamSelection):
		return "Enter one or more numeric Let's Play Soccer team IDs separated by commas."
	case errors.Is(err, errScheduleSelection):
		return "Enter team IDs or choose at least one discovered player."
	default:
		return err.Error()
	}
}

func selectedScheduleGames(games []Game, selectedIDs map[string]struct{}) []Game {
	filteredGames := make([]Game, 0, len(games))
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filteredGames = append(filteredGames, games[i])
		}
	}
	return filteredGames
}

func soccerGoogleFlashKind(code string) string {
	switch strings.TrimSpace(code) {
	case "failed", "denied", "unavailable":
		return "error"
	default:
		return "success"
	}
}

func soccerGoogleFlashMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "connected":
		return "Google Calendar connected. Choose a calendar below and add selected games directly from the schedule table."
	case "denied":
		return "Google Calendar connection was canceled before access was granted."
	case "disconnected":
		return "Google Calendar connection removed."
	case "failed":
		return "Google Calendar connection could not be completed. Try again."
	case "unavailable":
		return "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	default:
		return ""
	}
}
