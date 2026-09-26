package google

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

	"portfolio/internal/schedule"
	"portfolio/types"
)

type resultSyncAction uint8

const (
	resultSyncSkipped resultSyncAction = iota
	resultSyncUnchanged
	resultSyncUpdated
)

// syncResultEvents updates only the result line of one unambiguous site-added
// event per selected game. No event is inserted or restored by this path.
func (h *Handler) syncResultEvents(ctx context.Context, calendarID string, token *oauth2.Token, games []types.Game) (calendarMutationResult, error) {
	var result calendarMutationResult
	for i := range games {
		action, rejected, err := h.syncResultEvent(ctx, calendarID, token, &games[i])
		if err != nil {
			return result, err
		}
		if rejected {
			result.authRejected = true
			return result, nil
		}
		switch action {
		case resultSyncUpdated:
			result.updated++
		case resultSyncUnchanged:
			result.unchanged++
		case resultSyncSkipped:
			result.skipped++
		}
	}
	return result, nil
}

func (h *Handler) syncResultEvent(ctx context.Context, calendarID string, token *oauth2.Token, game *types.Game) (resultSyncAction, bool, error) {
	formatted, ok := schedule.CanonicalGameEvent(game)
	if !ok || formatted.ID == "" {
		return resultSyncSkipped, false, nil
	}
	outcome := schedule.ParseGameResult(game.Result, game.PlayerTeamName, game.Home)
	if !outcome.Parsed || (outcome.Outcome != schedule.OutcomeWin && outcome.Outcome != schedule.OutcomeLoss && outcome.Outcome != schedule.OutcomeDraw) {
		return resultSyncSkipped, false, nil
	}

	match, rejected, err := h.findUniqueSiteEvent(ctx, calendarID, token, formatted.ID)
	if err != nil || rejected || match == nil {
		return resultSyncSkipped, rejected, err
	}
	description, safe := replaceOwnedResultLine(match.Description, "Result: "+schedule.FormatResultLine(outcome))
	if !safe {
		return resultSyncSkipped, false, nil
	}
	if description == match.Description {
		return resultSyncUnchanged, false, nil
	}
	return h.patchResultDescription(ctx, calendarID, token, match, description)
}

func (h *Handler) findUniqueSiteEvent(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*Event, bool, error) {
	response, err := h.listCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, false, err
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		response.Body.Close()
		return nil, true, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, readAPIError(response)
	}
	page, err := decodeEventList(response)
	if err != nil {
		return nil, false, err
	}
	if page.NextPageToken != "" {
		return nil, false, nil
	}
	var candidate *Event
	for i := range page.Items {
		event := &page.Items[i]
		if !siteEventMatchesGame(event, gameID) {
			continue
		}
		if candidate != nil || event.ID == "" || strings.EqualFold(event.Status, "canceled") {
			return nil, false, nil
		}
		candidate = event
	}
	if candidate == nil {
		return nil, false, nil
	}

	response, err = h.getCalendarEvent(ctx, calendarID, candidate.ID, token)
	if err != nil {
		return nil, false, err
	}
	switch response.StatusCode {
	case http.StatusOK:
		fresh, decodeErr := decodeEvent(response)
		if decodeErr != nil {
			return nil, false, decodeErr
		}
		if fresh.ID != candidate.ID || !siteEventMatchesGame(fresh, gameID) || strings.EqualFold(fresh.Status, "canceled") || strings.TrimSpace(fresh.ETag) == "" {
			return nil, false, nil
		}
		return fresh, false, nil
	case http.StatusNotFound, http.StatusGone:
		response.Body.Close()
		return nil, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		response.Body.Close()
		return nil, true, nil
	default:
		return nil, false, readAPIError(response)
	}
}

func siteEventMatchesGame(event *Event, gameID string) bool {
	return event != nil && event.ExtendedProperties.Private[eventGameIDProperty] == gameID && event.ExtendedProperties.Private[eventOwnerProperty] == eventOwnerValue
}

// replaceOwnedResultLine changes the canonical Add result slot and leaves every
// other description line untouched. A modified or ambiguous slot is skipped.
func replaceOwnedResultLine(description, desired string) (string, bool) {
	lines := strings.Split(description, "\n")
	if len(lines) < 5 || !strings.Contains(lines[0], " is playing ") ||
		!strings.HasPrefix(lines[1], "Division: ") || !strings.HasPrefix(lines[2], "Facility: ") ||
		!strings.HasPrefix(lines[3], "Field: ") || !strings.HasPrefix(lines[4], "Result: ") {
		return "", false
	}
	for _, line := range lines[5:] {
		if strings.HasPrefix(line, "Result: ") {
			return "", false
		}
	}
	lines[4] = desired
	return strings.Join(lines, "\n"), true
}

func (h *Handler) patchResultDescription(ctx context.Context, calendarID string, token *oauth2.Token, event *Event, description string) (resultSyncAction, bool, error) {
	request, err := h.newAPIRequest(ctx, http.MethodPatch, calendarEventsPath+url.PathEscape(calendarID)+"/events/"+url.PathEscape(event.ID), url.Values{"sendUpdates": {"none"}}, token, map[string]string{"description": description})
	if err != nil {
		return resultSyncSkipped, false, err
	}
	request.Header.Set("If-Match", event.ETag)
	response, err := h.LPSClient.Do(request)
	if err != nil {
		return resultSyncSkipped, false, err
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		response.Body.Close()
		return resultSyncUpdated, false, nil
	case http.StatusNotFound, http.StatusGone, http.StatusConflict, http.StatusPreconditionFailed:
		response.Body.Close()
		return resultSyncSkipped, false, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		response.Body.Close()
		return resultSyncSkipped, true, nil
	default:
		return resultSyncSkipped, false, readAPIError(response)
	}
}
