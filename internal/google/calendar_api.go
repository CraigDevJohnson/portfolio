package google

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	"portfolio/types"
)

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
