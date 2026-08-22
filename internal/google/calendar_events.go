package google

import (
	"context"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
	"portfolio/internal/schedule"
	"portfolio/types"
)

type calendarMutationResult struct {
	added        int
	updated      int
	skipped      int
	authRejected bool
}

func (h *Handler) insertCalendarEvents(ctx context.Context, r *http.Request, record *ConnectionRecord, token *oauth2.Token, games []types.Game) (calendarMutationResult, error) {
	var result calendarMutationResult
	for i := range games {
		event, ok := eventPayload(r, &games[i])
		if !ok {
			continue
		}
		action, authRejected, err := h.syncCalendarEvent(h.httpContext(ctx), record.CalendarID, token, &event)
		if err != nil {
			return result, err
		}
		if authRejected {
			result.authRejected = true
			return result, nil
		}
		switch action {
		case calendarEventInserted:
			result.added++
		case calendarEventUpdated:
			result.updated++
		case calendarEventSkipped:
			result.skipped++
		}
	}
	return result, nil
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
		authRejected, apiErr := apiResponseError(h.Logger, resp)
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
		authRejected, apiErr := apiResponseError(h.Logger, resp)
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
	event, found, authRejected, err := h.handleGetEventByIDResponse(resp, gameID)
	if found || authRejected || err != nil {
		return event, found, authRejected, err
	}
	resp.Body.Close()

	// handleListEventsResponse owns and closes the replacement response body.
	//nolint:bodyclose
	resp, err = h.listCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, false, false, err
	}
	return h.handleListEventsResponse(resp, gameID)
}

func (h *Handler) handleGetEventByIDResponse(resp *http.Response, gameID string) (*Event, bool, bool, error) {
	switch resp.StatusCode {
	case http.StatusOK:
		existingEvent, decodeErr := decodeEvent(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		if eventMatchesGameID(existingEvent, gameID) {
			return existingEvent, true, false, nil
		}
		return nil, false, false, nil
	case http.StatusNotFound, http.StatusGone:
		resp.Body.Close()
		return nil, false, false, nil
	default:
		authRejected, apiErr := apiResponseError(h.Logger, resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}
}

func (h *Handler) handleListEventsResponse(resp *http.Response, gameID string) (*Event, bool, bool, error) {
	switch resp.StatusCode {
	case http.StatusOK:
		events, decodeErr := decodeEventList(resp)
		if decodeErr != nil {
			return nil, false, false, decodeErr
		}
		for i := range events {
			if eventMatchesGameID(&events[i], gameID) {
				return &events[i], true, false, nil
			}
		}
		return nil, false, false, nil
	default:
		authRejected, apiErr := apiResponseError(h.Logger, resp)
		if authRejected {
			return nil, false, true, nil
		}
		return nil, false, false, apiErr
	}
}

// eventMatchesGameID checks if a Google Calendar event matches a game ID.
func eventMatchesGameID(event *Event, gameID string) bool {
	if event == nil {
		return false
	}
	gameID = strings.TrimSpace(gameID)
	if gameID == "" {
		return false
	}
	eventID := strings.TrimSpace(event.ID)
	storedGameID := strings.TrimSpace(event.ExtendedProperties.Private["game_id"])
	if eventID == gameID {
		return true
	}
	return storedGameID == gameID
}

// preferredCalendar returns the primary calendar or the first in the list.
func preferredCalendar(calendars []types.GoogleCalendarOption) (string, string) {
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

// eventPayload builds a Google Calendar event from a game.
func eventPayload(r *http.Request, game *types.Game) (Event, bool) {
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
		Reminders: &EventReminders{
			Overrides: []EventReminder{{
				Method:  "popup",
				Minutes: 40,
			}},
			UseDefault: false,
		},
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
