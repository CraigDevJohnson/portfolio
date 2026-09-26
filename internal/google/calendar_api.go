package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	"portfolio/types"
)

const calendarEventsPath = "calendars/"

func decodeEvent(resp *http.Response) (*Event, error) {
	defer resp.Body.Close()
	var event Event
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&event); err != nil {
		return nil, err
	}
	return &event, nil
}

func decodeEventList(resp *http.Response) (eventListResponse, error) {
	defer resp.Body.Close()
	var response eventListResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, config.MaxRequestBodySize)).Decode(&response); err != nil {
		return eventListResponse{}, err
	}
	return response, nil
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
	req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
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
	req, err := h.newAPIRequest(ctx, http.MethodPost, calendarEventsPath+url.PathEscape(calendarID)+"/events", url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req)
}

func (h *Handler) getCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodGet, calendarEventsPath+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), nil, token, nil)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req)
}

func (h *Handler) updateCalendarEvent(ctx context.Context, calendarID, eventID string, token *oauth2.Token, event *Event) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodPut, calendarEventsPath+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID), url.Values{"sendUpdates": {"none"}}, token, event)
	if err != nil {
		return nil, err
	}
	return h.LPSClient.Do(req)
}

func (h *Handler) listCalendarEventsByPrivateGameID(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*http.Response, error) {
	req, err := h.newAPIRequest(ctx, http.MethodGet, calendarEventsPath+url.PathEscape(calendarID)+"/events", url.Values{
		"maxResults":              {"10"},
		"privateExtendedProperty": {eventGameIDProperty + "=" + gameID},
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
	var googleError struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	reason := ""
	if json.Unmarshal(body, &googleError) == nil && len(googleError.Error.Errors) > 0 {
		reason = googleError.Error.Errors[0].Reason
	}
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Reason:     reason,
	}
}

// calendarListMaxPages bounds how many calendar list pages one check reads:
// Google returns at most 250 calendars a page.
const calendarListMaxPages = 20

// listCalendarsWithToken lists every calendar the account can write,
// including calendars it hid from its Google Calendar list, so a chosen
// destination is never mistaken for a lost one.
func (h *Handler) listCalendarsWithToken(ctx context.Context, token *oauth2.Token) ([]types.GoogleCalendarOption, error) {
	var items []calendar
	pageToken := ""
	for range calendarListMaxPages {
		page, err := h.listCalendarPage(ctx, token, pageToken)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Items...)
		if pageToken = page.NextPageToken; pageToken == "" {
			return calendarOptions(items), nil
		}
	}
	return nil, errors.New("google calendar list has more pages than the site reads")
}

func (h *Handler) listCalendarPage(ctx context.Context, token *oauth2.Token, pageToken string) (*calendarListResponse, error) {
	query := url.Values{"minAccessRole": {"writer"}, "showHidden": {"true"}, "maxResults": {"250"}}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	req, err := h.newAPIRequest(ctx, http.MethodGet, "users/me/calendarList", query, token, nil)
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
	return &response, nil
}

// calendarOptions turns listed calendars into destination choices, primary
// first and the rest by name.
func calendarOptions(items []calendar) []types.GoogleCalendarOption {
	options := make([]types.GoogleCalendarOption, 0, len(items))
	for _, item := range items {
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
	return options
}
