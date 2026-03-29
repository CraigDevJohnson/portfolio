// Google Calendar API integration: event sync, insert, update, calendar listing, and selection.
package google

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
	internallps "portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/types"
)

type calendarListResponse struct {
	Items []calendar `json:"items"`
}

type eventListResponse struct {
	Items []Event `json:"items"`
}

type calendar struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
	Summary string `json:"summary"`
}

// EventDateTime holds a Google Calendar event start/end time.
type EventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

// Event represents a Google Calendar event.
type Event struct {
	Description        string        `json:"description,omitempty"`
	End                EventDateTime `json:"end"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties"`
	ID       string        `json:"id,omitempty"`
	Location string        `json:"location,omitempty"`
	Source   *EventSource  `json:"source,omitempty"`
	Start    EventDateTime `json:"start"`
	Status   string        `json:"status,omitempty"`
	Summary  string        `json:"summary"`
}

// EventSource is the source metadata on a Google Calendar event.
type EventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// APIError represents a Google API error response.
type APIError struct {
	StatusCode int
	Message    string
}

func (err *APIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return "google api request failed"
}

type calendarEventAction int

const (
	calendarEventSkipped calendarEventAction = iota
	calendarEventInserted
	calendarEventUpdated
)

// AddHandler adds selected games to Google Calendar.
func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.Config.GoogleEnabled() {
		h.Soccer.RenderLoginFeedback(w, "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.Soccer.RenderLoginFeedback(w, "error", "Could not read the selected games. Try again.")
		return
	}
	record, err := h.LoadConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		h.Soccer.RenderLoginFeedback(w, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	selectedIDs := h.Soccer.ParseSelectedIDs(r.Form)
	if len(selectedIDs) == 0 {
		h.Soccer.RenderLoginFeedback(w, "error", "Select at least one game to add to Google Calendar.")
		return
	}
	teamCodes := r.FormValue("team_codes")
	rawPlayerIDs := r.Form["player_ids"]
	playerIDs := h.Soccer.ParsePlayerIDs(rawPlayerIDs)
	if len(internallps.NonEmptyStrings(rawPlayerIDs)) > 0 && len(playerIDs) == 0 {
		h.Soccer.RenderLoginFeedback(w, "error", "One or more selected players were invalid. Clear the imported players and import again to refresh the discovered list.")
		return
	}
	session, _ := h.Soccer.LoadSession(w, r)
	games, err := h.Soccer.RequestedScheduleGames(r.Context(), session, playerIDs, teamCodes)
	if err != nil {
		h.Soccer.RenderLoginFeedback(w, "error", h.Soccer.GoogleAddScheduleErrorMessage(err))
		return
	}
	filteredGames := h.Soccer.SelectedScheduleGames(games, selectedIDs)
	if len(filteredGames) == 0 {
		h.Soccer.RenderLoginFeedback(w, "error", "No selected games were found to add.")
		return
	}
	token, err := h.CurrentToken(r.Context(), r, record)
	if err != nil {
		log.Printf("google token refresh failed: %v", err)
		h.RenderDisconnectFeedback(w, r, session, "Your Google Calendar connection has expired. Connect again and retry.")
		return
	}
	added, updated, skipped, authRejected, err := h.insertCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		log.Printf("google event insert failed: %v", err)
		h.Soccer.RenderLoginFeedback(w, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		h.RenderDisconnectFeedback(w, r, session, "Your Google Calendar connection is no longer valid. Connect again and retry.")
		return
	}
	message := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", added)
	if updated > 0 {
		message += fmt.Sprintf(" Updated/restored %d matching game(s).", updated)
	}
	if skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", skipped)
	}
	h.Soccer.RenderLoginFeedback(w, "success", message)
}

// CalendarHandler handles calendar selection changes.
func (h *Handler) CalendarHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := h.Soccer.LoadSession(w, r)
	if !h.Config.GoogleEnabled() {
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	record, err := h.LoadConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	calendars, err := h.ListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.FormValue("calendar_id"))
	selectedCalendarSummary := calendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		selectedCalendarID, selectedCalendarSummary = PreferredCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := h.Store().Put(r.Context(), record); err != nil {
		log.Printf("google calendar selection save failed: %v", err)
	}
	h.Soccer.RenderLoginState(w, r, session)
}

func apiResponseError(resp *http.Response) (bool, error) {
	apiErr := readAPIError(resp)
	log.Printf("google event insert rejected: %v", apiErr)
	var googleErr *APIError
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden), apiErr
}

func (h *Handler) insertCalendarEvents(r *http.Request, record *ConnectionRecord, token *oauth2.Token, games []types.Game) (int, int, int, bool, error) {
	added := 0
	updated := 0
	skipped := 0
	for i := range games {
		event, ok := EventPayload(r, &games[i])
		if !ok {
			continue
		}
		action, authRejected, err := h.syncCalendarEvent(h.httpContext(r.Context()), record.CalendarID, token, &event)
		if err != nil {
			return 0, 0, 0, false, err
		}
		if authRejected {
			return 0, 0, 0, true, nil
		}
		switch action {
		case calendarEventInserted:
			added++
		case calendarEventUpdated:
			updated++
		case calendarEventSkipped:
			skipped++
		}
	}
	return added, updated, skipped, false, nil
}

func (h *Handler) syncCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *Event) (calendarEventAction, bool, error) {
	existingEvent, found, authRejected, err := h.findCalendarEventByGameID(ctx, calendarID, token, event.ID)
	if err != nil {
		return calendarEventSkipped, false, err
	}
	if authRejected {
		return calendarEventSkipped, true, nil
	}
	if found {
		return h.refreshCalendarEvent(ctx, calendarID, token, existingEvent, event)
	}

	resp, err := h.insertCalendarEvent(ctx, calendarID, token, event)
	if err != nil {
		return calendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		resp.Body.Close()
		return calendarEventInserted, false, nil
	case http.StatusConflict:
		resp.Body.Close()
		existingEvent, found, authRejected, err = h.findCalendarEventByGameID(ctx, calendarID, token, event.ID)
		if err != nil {
			return calendarEventSkipped, false, err
		}
		if authRejected {
			return calendarEventSkipped, true, nil
		}
		if !found {
			return calendarEventSkipped, false, nil
		}
		return h.refreshCalendarEvent(ctx, calendarID, token, existingEvent, event)
	default:
		authRejected, apiErr := apiResponseError(resp)
		if authRejected {
			return calendarEventSkipped, true, nil
		}
		return calendarEventSkipped, false, apiErr
	}
}

func (h *Handler) refreshCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, existingEvent, event *Event) (calendarEventAction, bool, error) {
	refreshedEvent := *event
	if existingEvent != nil {
		if existingID := strings.TrimSpace(existingEvent.ID); existingID != "" {
			refreshedEvent.ID = existingID
		}
	}

	resp, err := h.updateCalendarEvent(ctx, calendarID, refreshedEvent.ID, token, &refreshedEvent)
	if err != nil {
		return calendarEventSkipped, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		resp.Body.Close()
		return calendarEventUpdated, false, nil
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return calendarEventSkipped, false, nil
	default:
		authRejected, apiErr := apiResponseError(resp)
		if authRejected {
			return calendarEventSkipped, true, nil
		}
		return calendarEventSkipped, false, apiErr
	}
}

func (h *Handler) findCalendarEventByGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*Event, bool, bool, error) {
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return nil, false, false, nil
	}

	resp, err := h.getCalendarEvent(ctx, calendarID, gameID, token)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		existingEvent, decodeErr := decodeEvent(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		if EventMatchesGameID(existingEvent, gameID) {
			return existingEvent, true, false, nil
		}
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
	default:
		authRejected, apiErr := apiResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}

	resp, err = h.listCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, false, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		events, decodeErr := decodeEventList(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		for i := range events {
			if EventMatchesGameID(&events[i], gameID) {
				return &events[i], true, false, nil
			}
		}
		return nil, false, false, nil
	default:
		authRejected, apiErr := apiResponseError(resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}
}

// EventMatchesGameID checks if a Google Calendar event matches a game ID.
func EventMatchesGameID(event *Event, gameID string) bool {
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

func decodeEvent(resp *http.Response) (*Event, error) {
	defer resp.Body.Close()
	var event Event
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func decodeEventList(resp *http.Response) ([]Event, error) {
	defer resp.Body.Close()
	var response eventListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (h *Handler) newAPIRequest(ctx context.Context, method, requestPath string, query url.Values, token *oauth2.Token, body any) (*http.Request, error) {
	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(payload)
	}
	endpoint, err := url.JoinPath(h.CalendarAPIBaseURL, requestPath)
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

func (h *Handler) insertCalendarEvent(ctx context.Context, calendarID string, token *oauth2.Token, event *Event) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodPost, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func (h *Handler) getCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), nil, token, nil)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req)
}

func (h *Handler) updateCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token, event *Event) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodPut, "calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req) //nolint:gosec // request is created from the constant Google Calendar API base URL and fixed paths.
}

func (h *Handler) listCalendarEventsByPrivateGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodGet, "calendars/"+url.PathEscape(calendarID)+"/events", url.Values{
		"maxResults":              {"10"},
		"privateExtendedProperty": {"game_id=" + gameID},
		"showDeleted":             {"true"},
	}, token, nil)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req)
}

func readAPIError(resp *http.Response) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, config.MaxRequestBodySize))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
	}
}

func (h *Handler) listCalendarsWithToken(ctx context.Context, token *oauth2.Token) ([]types.GoogleCalendarOption, error) {
	req, err := h.newAPIRequest(ctx, http.MethodGet, "users/me/calendarList", url.Values{"minAccessRole": {"writer"}}, token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.LPSClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readAPIError(resp)
	}
	defer resp.Body.Close()
	var response calendarListResponse
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

// PreferredCalendar returns the primary calendar or the first in the list.
func PreferredCalendar(calendars []types.GoogleCalendarOption) (string, string) {
	for _, cal := range calendars {
		if cal.Primary {
			return cal.ID, cal.Summary
		}
	}
	if len(calendars) == 0 {
		return "", ""
	}
	return calendars[0].ID, calendars[0].Summary
}

func calendarSummary(calendars []types.GoogleCalendarOption, calendarID string) string {
	for _, cal := range calendars {
		if cal.ID == calendarID {
			return cal.Summary
		}
	}
	return ""
}

// EventPayload builds a Google Calendar event from a game.
func EventPayload(r *http.Request, game *types.Game) (Event, bool) {
	formatted, ok := schedule.CanonicalGameEvent(game)
	if !ok {
		return Event{}, false
	}
	event := Event{
		Description: formatted.Description,
		End: EventDateTime{
			DateTime: formatted.End.Format("2006-01-02T15:04:05"),
			TimeZone: config.MountainTimeZoneID,
		},
		ID:       formatted.ID,
		Location: formatted.Location,
		Start: EventDateTime{
			DateTime: formatted.Start.Format("2006-01-02T15:04:05"),
			TimeZone: config.MountainTimeZoneID,
		},
		Status:  formatted.Status,
		Summary: formatted.Summary,
	}
	event.ExtendedProperties.Private = map[string]string{
		"game_id": formatted.ID,
	}
	event.Source = &EventSource{
		Title: "Soccer Schedule",
		URL:   internalhttpx.RequestBaseURL(r) + "/soccer",
	}
	return event, true
}
