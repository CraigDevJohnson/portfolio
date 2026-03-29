// Google Calendar API integration: event sync, insert, update, calendar listing, and selection.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
	"portfolio/internal/schedule"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

type googleCalendarListResponse struct {
	Items []googleCalendar `json:"items"`
}

type googleEventListResponse struct {
	Items []googleEvent `json:"items"`
}

type googleCalendar struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
	Summary string `json:"summary"`
}

type googleEventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

type googleEvent struct {
	Description        string              `json:"description,omitempty"`
	End                googleEventDateTime `json:"end"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties"`
	ID       string              `json:"id,omitempty"`
	Location string              `json:"location,omitempty"`
	Source   *googleEventSource  `json:"source,omitempty"`
	Start    googleEventDateTime `json:"start"`
	Status   string              `json:"status,omitempty"`
	Summary  string              `json:"summary"`
}

type googleEventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type googleAPIError struct {
	StatusCode int
	Message    string
}

func (err *googleAPIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return "google api request failed"
}

type googleCalendarEventAction int

const (
	googleCalendarEventSkipped googleCalendarEventAction = iota
	googleCalendarEventInserted
	googleCalendarEventUpdated
)

func (app *App) soccerGoogleAddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !app.Config.GoogleEnabled() {
		renderSoccerLoginFeedback(w, "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		renderSoccerLoginFeedback(w, "error", "Could not read the selected games. Try again.")
		return
	}
	record, err := app.loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		renderSoccerLoginFeedback(w, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	selectedIDs := internalsoccer.ParseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "Select at least one game to add to Google Calendar.")
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := internalsoccer.ParsePlayerIDs(rawPlayerIDs)
	if len(internalsoccer.NonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		renderSoccerLoginFeedback(w, "error", "One or more selected players were invalid. Clear the imported players and import again to refresh the discovered list.")
		return
	}
	session, _ := app.loadSoccerSession(w, r)
	games, err := app.requestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if err != nil {
		renderSoccerLoginFeedback(w, "error", googleAddScheduleErrorMessage(err))
		return
	}
	filteredGames := selectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		renderSoccerLoginFeedback(w, "error", "No selected games were found to add.")
		return
	}
	token, err := app.currentGoogleToken(r.Context(), r, record)
	if err != nil {
		log.Printf("google token refresh failed: %v", err)
		app.renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection has expired. Connect again and retry.")
		return
	}
	added, updated, skipped, authRejected, err := app.insertGoogleCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		log.Printf("google event insert failed: %v", err)
		renderSoccerLoginFeedback(w, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		app.renderGoogleDisconnectFeedback(w, r, session, "Your Google Calendar connection is no longer valid. Connect again and retry.")
		return
	}
	message := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", added)
	if updated > 0 {
		message += fmt.Sprintf(" Updated/restored %d matching game(s).", updated)
	}
	if skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", skipped)
	}
	renderSoccerLoginFeedback(w, "success", message)
}

func (app *App) soccerGoogleCalendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := app.loadSoccerSession(w, r)
	if !app.Config.GoogleEnabled() {
		app.renderSoccerLoginState(w, r, session)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		app.renderSoccerLoginState(w, r, session)
		return
	}
	record, err := app.loadGoogleConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		app.renderSoccerLoginState(w, r, session)
		return
	}
	calendars, err := app.googleListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		app.renderSoccerLoginState(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.FormValue("calendar_id"))
	selectedCalendarSummary := googleCalendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		selectedCalendarID, selectedCalendarSummary = preferredGoogleCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := app.currentGoogleConnectionStore().Put(r.Context(), record); err != nil {
		log.Printf("google calendar selection save failed: %v", err)
	}
	app.renderSoccerLoginState(w, r, session)
}

func googleAPIResponseError(resp *http.Response) (bool, error) {
	apiErr := readGoogleAPIError(resp)
	log.Printf("google event insert rejected: %v", apiErr)
	var googleErr *googleAPIError
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden), apiErr
}

func (app *App) insertGoogleCalendarEvents(r *http.Request, record *googleConnectionRecord, token *oauth2.Token, games []Game) (int, int, int, bool, error) {
	added := 0
	updated := 0
	skipped := 0
	for i := range games {
		event, ok := googleEventPayload(r, &games[i])
		if !ok {
			continue
		}
		action, authRejected, err := app.syncGoogleCalendarEvent(app.googleHTTPContext(r.Context()), record.CalendarID, token, &event)
		if err != nil {
			return 0, 0, 0, false, err
		}
		if authRejected {
			return 0, 0, 0, true, nil
		}
		switch action {
		case googleCalendarEventInserted:
			added++
		case googleCalendarEventUpdated:
			updated++
		case googleCalendarEventSkipped:
			skipped++
		}
	}
	return added, updated, skipped, false, nil
}

func (app *App) syncGoogleCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *googleEvent) (googleCalendarEventAction, bool, error) {
	existingEvent, found, authRejected, err := app.googleFindCalendarEventByGameID(ctx, calendarID, token, event.ID)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	if authRejected {
		return googleCalendarEventSkipped, true, nil
	}
	if found {
		return app.refreshGoogleCalendarEvent(ctx, calendarID, token, existingEvent, event)
	}

	resp, err := app.googleInsertCalendarEvent(ctx, calendarID, token, event)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		resp.Body.Close()
		return googleCalendarEventInserted, false, nil
	case http.StatusConflict:
		resp.Body.Close()
		existingEvent, found, authRejected, err = app.googleFindCalendarEventByGameID(ctx, calendarID, token, event.ID)
		if err != nil {
			return googleCalendarEventSkipped, false, err
		}
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		if !found {
			return googleCalendarEventSkipped, false, nil
		}
		return app.refreshGoogleCalendarEvent(ctx, calendarID, token, existingEvent, event)
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		return googleCalendarEventSkipped, false, apiErr
	}
}

func (app *App) refreshGoogleCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, existingEvent, event *googleEvent) (googleCalendarEventAction, bool, error) {
	refreshedEvent := *event
	if existingEvent != nil {
		if existingID := strings.TrimSpace(existingEvent.ID); existingID != "" {
			refreshedEvent.ID = existingID
		}
	}

	resp, err := app.googleUpdateCalendarEvent(ctx, calendarID, refreshedEvent.ID, token, &refreshedEvent)
	if err != nil {
		return googleCalendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		resp.Body.Close()
		return googleCalendarEventUpdated, false, nil
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return googleCalendarEventSkipped, false, nil
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return googleCalendarEventSkipped, true, nil
		}
		return googleCalendarEventSkipped, false, apiErr
	}
}

func (app *App) googleFindCalendarEventByGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*googleEvent, bool, bool, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, false, false, nil
	}

	resp, err := app.googleGetCalendarEvent(ctx, calendarID, gameID, token)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		existingEvent, decodeErr := decodeGoogleEvent(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		if googleEventMatchesGameID(existingEvent, gameID) {
			return existingEvent, true, false, nil
		}
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}

	resp, err = app.googleListCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		events, decodeErr := decodeGoogleEventList(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		for i := range events {
			if googleEventMatchesGameID(&events[i], gameID) {
				return &events[i], true, false, nil
			}
		}
		return nil, false, false, nil
	default:
		authRejected, apiErr := googleAPIResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}
}

func googleEventMatchesGameID(event *googleEvent, gameID string) bool {
	if event == nil {
		return false
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return false
	}
	if strings.TrimSpace(event.ID) == gameID {
		return true
	}
	return strings.TrimSpace(event.ExtendedProperties.Private["game_id"]) == gameID
}

func decodeGoogleEvent(resp *http.Response) (*googleEvent, error) {
	defer resp.Body.Close()
	var event googleEvent
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func decodeGoogleEventList(resp *http.Response) ([]googleEvent, error) {
	defer resp.Body.Close()
	var response googleEventListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (app *App) newGoogleAPIRequest(ctx context.Context, method, requestPath string, query url.Values, token *oauth2.Token, body any) (*http.Request, error) {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(payload)
	}
	endpoint, err := url.JoinPath(app.GoogleCalendarAPIBaseURL, requestPath)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody) //nolint:gosec // endpoint is derived from the constant Google Calendar API base URL and fixed paths.
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (app *App) googleInsertCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *googleEvent) (*http.Response, error) {
	req, err := app.newGoogleAPIRequest(ctx, http.MethodPost, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return app.LPSClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func (app *App) googleGetCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token) (*http.Response, error) {
	req, err := app.newGoogleAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), nil, token, nil)
	if err != nil {
		return nil, err
	}
	return app.LPSClient.Do(req)
}

func (app *App) googleUpdateCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token, event *googleEvent) (*http.Response, error) {
	req, err := app.newGoogleAPIRequest(ctx, http.MethodPut, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return app.LPSClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func (app *App) googleListCalendarEventsByPrivateGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*http.Response, error) {
	req, err := app.newGoogleAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{
		"maxResults":              {"10"},
		"privateExtendedProperty": {"game_id=" + gameID},
		"showDeleted":             {"true"},
	}, token, nil)
	if err != nil {
		return nil, err
	}
	return app.LPSClient.Do(req)
}

func readGoogleAPIError(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, config.MaxRequestBodySize))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &googleAPIError{
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func (app *App) googleListCalendarsWithToken(ctx context.Context, token *oauth2.Token) ([]types.GoogleCalendarOption, error) {
	req, err := app.newGoogleAPIRequest(ctx, http.MethodGet, "users/me/calendarList", url.Values{"minAccessRole": {"writer"}}, token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := app.LPSClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readGoogleAPIError(resp)
	}
	defer resp.Body.Close()
	var response googleCalendarListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&response); err != nil {
		return nil, err
	}
	options := make([]types.GoogleCalendarOption, 0, len(response.Items))
	for _, item := range response.Items {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		options = append(options, types.GoogleCalendarOption{
			ID:      item.ID,
			Primary: item.Primary,
			Summary: item.Summary,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Primary != options[j].Primary {
			return options[i].Primary
		}
		return strings.ToLower(options[i].Summary) < strings.ToLower(options[j].Summary)
	})
	return options, nil
}

func (app *App) googleListCalendars(ctx context.Context, r *http.Request, record *googleConnectionRecord) ([]types.GoogleCalendarOption, error) {
	token, err := app.currentGoogleToken(ctx, r, record)
	if err != nil {
		return nil, err
	}
	return app.googleListCalendarsWithToken(app.googleHTTPContext(ctx), token)
}

func preferredGoogleCalendar(calendars []types.GoogleCalendarOption) (string, string) {
	for _, calendar := range calendars {
		if calendar.Primary {
			return calendar.ID, calendar.Summary
		}
	}
	if len(calendars) == 0 {
		return "", ""
	}
	return calendars[0].ID, calendars[0].Summary
}

func googleCalendarSummary(calendars []types.GoogleCalendarOption, calendarID string) string {
	for _, calendar := range calendars {
		if calendar.ID == calendarID {
			return calendar.Summary
		}
	}
	return ""
}

func googleEventPayload(r *http.Request, game *Game) (googleEvent, bool) {
	formatted, ok := schedule.CanonicalGameEvent(game)
	if !ok {
		return googleEvent{}, false
	}
	event := googleEvent{
		Description: formatted.Description,
		End: googleEventDateTime{
			DateTime: formatted.End.Format("2006-01-02T15:04:05"),
			TimeZone: config.MountainTimeZoneID,
		},
		ID:       formatted.ID,
		Location: formatted.Location,
		Start: googleEventDateTime{
			DateTime: formatted.Start.Format("2006-01-02T15:04:05"),
			TimeZone: config.MountainTimeZoneID,
		},
		Status:  formatted.Status,
		Summary: formatted.Summary,
	}
	event.ExtendedProperties.Private = map[string]string{
		"game_id": formatted.ID,
	}
	event.Source = &googleEventSource{
		Title: "Soccer Schedule",
		URL:   internalhttpx.RequestBaseURL(r) + "/soccer",
	}
	return event, true
}
