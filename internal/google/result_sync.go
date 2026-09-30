package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

	"portfolio/internal/logging"
	"portfolio/internal/schedule"
	"portfolio/types"
)

const googleCancelledStatus = "cancelled" //nolint:misspell // Google Calendar's deleted-event wire status uses this spelling.

// resultSyncOutcome is what result Sync did with one selected game.
type resultSyncOutcome int

const (
	// resultUpdated: the event's result text now shows the game's result.
	resultUpdated resultSyncOutcome = iota
	// resultCurrent: the event already showed the result.
	resultCurrent
	// resultUnmatched: the calendar holds no event this site added for the
	// game, such as a game never added or one imported from an .ics file.
	resultUnmatched
	// resultDeleted: the site's event for the game was deleted, and no live
	// one is left.
	resultDeleted
	// resultAmbiguous: more than one live site event names the game.
	resultAmbiguous
	// resultChanged: the event changed in Google between Sync reading and
	// updating it.
	resultChanged
	// resultEdited: the event's description no longer holds the one block
	// Add wrote, or a result line holds text in the visitor's own words. A
	// result in the site's own format, or an empty slot, is the site's to
	// update, even if the visitor typed it.
	resultEdited
	resultOutcomeCount
)

// resultSyncReport counts what result Sync did with the selected games.
type resultSyncReport struct {
	outcomes [resultOutcomeCount]int
	// refused counts games whose event Google would not let this account
	// change.
	refused      int
	authRejected bool
}

func (report resultSyncReport) skipped() int {
	skipped := report.refused
	for outcome := resultUnmatched; outcome < resultOutcomeCount; outcome++ {
		skipped += report.outcomes[outcome]
	}
	return skipped
}

// done reports what Sync did before it stopped, or "" when it had not yet
// finished any game.
func (report resultSyncReport) done() string {
	if report.outcomes[resultUpdated]+report.outcomes[resultCurrent]+report.skipped() == 0 {
		return ""
	}
	return report.message()
}

// message reports the updated, current, and skipped games, naming why each
// skipped game was left alone. The updated count is always given; like Add's
// report, it leaves out current and skipped games when there are none.
func (report resultSyncReport) message() string {
	message := fmt.Sprintf("%d game result(s) updated in Google Calendar.", report.outcomes[resultUpdated])
	if current := report.outcomes[resultCurrent]; current > 0 {
		message += fmt.Sprintf(" %d result(s) already current.", current)
	}
	skipped := report.skipped()
	if skipped == 0 {
		return message
	}
	message += fmt.Sprintf(" Skipped %d game(s)", skipped)
	var reasons []string
	for _, reason := range []struct {
		count int
		text  string
	}{
		{report.outcomes[resultUnmatched], "unmatched (no event this site added)"},
		{report.outcomes[resultDeleted], "deleted"},
		{report.outcomes[resultAmbiguous], "with more than one matching event"},
		{report.outcomes[resultChanged], "changed in Google Calendar during Sync"},
		{report.outcomes[resultEdited], "with an edited description"},
		{report.refused, "whose existing event Google Calendar would not let this account change"},
	} {
		if reason.count > 0 {
			reasons = append(reasons, fmt.Sprintf("%d %s", reason.count, reason.text))
		}
	}
	return message + ": " + strings.Join(reasons, ", ") + "."
}

// syncResultEvents updates only the result text of the one event this site
// added for each selected game. It never inserts or restores an event.
func (h *Handler) syncResultEvents(ctx context.Context, calendarID string, token *oauth2.Token, games []types.Game) (resultSyncReport, error) {
	var report resultSyncReport
	for i := range games {
		outcome, rejected, err := h.syncResultEvent(ctx, calendarID, token, &games[i])
		if rejected {
			report.authRejected = true
			return report, nil
		}
		if errors.Is(err, errEventRefused) {
			logging.WithContext(h.Logger, ctx).Warn("google refused one event; result skipped", slog.String("game_id", games[i].ID), slog.Any("error", err))
			report.refused++
			continue
		}
		if err != nil {
			return report, err
		}
		report.outcomes[outcome]++
	}
	return report, nil
}

func (h *Handler) syncResultEvent(ctx context.Context, calendarID string, token *oauth2.Token, game *types.Game) (resultSyncOutcome, bool, error) {
	formatted, ok := schedule.CanonicalGameEvent(game)
	if !ok || formatted.ID == "" {
		return resultUnmatched, false, nil
	}
	outcome := schedule.ParseGameResult(game.Result, game.PlayerTeamName, game.Home)
	if !outcome.Parsed || (outcome.Outcome != schedule.OutcomeWin && outcome.Outcome != schedule.OutcomeLoss && outcome.Outcome != schedule.OutcomeDraw) {
		return resultUnmatched, false, nil
	}

	match, missing, rejected, err := h.findSiteEvent(ctx, calendarID, token, formatted.ID)
	if err != nil || rejected || match == nil {
		return missing, rejected, err
	}
	description, safe := schedule.ReplaceCanonicalResult(match.Description, game)
	if !safe {
		return resultEdited, false, nil
	}
	if description == match.Description {
		return resultCurrent, false, nil
	}
	return h.patchResultDescription(ctx, calendarID, token, match, description)
}

// resultSearchMaxPages bounds how many pages one game's search reads. Each
// page holds at most ten events, so a game still listing more after these
// has far more candidate events than one Add wrote.
const resultSearchMaxPages = 5

// findSiteEvent returns the one live event this site added for gameID in the
// calendar, as one bounded search lists it with its current version. Google
// may answer with a page holding fewer events than asked for, or none, while
// more follow, so it reads every page, up to resultSearchMaxPages, before it
// judges the match. Without one live site event, it returns why: no such
// event, only deleted ones, or more than one live one, or more pages than it
// reads. When the search finds no site event, one read by the site's event ID
// tells a deleted event from a missing one.
func (h *Handler) findSiteEvent(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*Event, resultSyncOutcome, bool, error) {
	var found []Event
	pageToken := ""
	for range resultSearchMaxPages {
		items, next, rejected, err := h.searchSiteEventPage(ctx, calendarID, token, gameID, pageToken)
		if err != nil || rejected {
			return nil, resultUnmatched, rejected, err
		}
		found = append(found, items...)
		if pageToken = next; pageToken == "" {
			return h.soleLiveSiteEvent(ctx, calendarID, token, gameID, found)
		}
	}
	return nil, resultAmbiguous, false, nil
}

// soleLiveSiteEvent picks the one live site event for gameID from every
// event the search found, as findSiteEvent returns it.
func (h *Handler) soleLiveSiteEvent(ctx context.Context, calendarID string, token *oauth2.Token, gameID string, found []Event) (*Event, resultSyncOutcome, bool, error) {
	// A deleted copy does not make the one live event ambiguous: the
	// visitor may have deleted a duplicate to resolve just that.
	var live []*Event
	deleted := 0
	for i := range found {
		switch event := &found[i]; {
		case !siteEventMatchesGame(event, gameID):
		case isDeletedCalendarEvent(event.Status):
			deleted++
		default:
			live = append(live, event)
		}
	}
	switch {
	case len(live) > 1:
		return nil, resultAmbiguous, false, nil
	case len(live) == 0 && deleted > 0:
		return nil, resultDeleted, false, nil
	case len(live) == 0:
		missing, rejected, err := h.deletedSiteEventOutcome(ctx, calendarID, token, gameID)
		return nil, missing, rejected, err
	}
	if strings.TrimSpace(live[0].ETag) == "" {
		// Without its version, a change could not be made conditional.
		return nil, resultChanged, false, nil
	}
	return live[0], resultUnmatched, false, nil
}

// searchSiteEventPage reads one page of the search by private game ID.
func (h *Handler) searchSiteEventPage(ctx context.Context, calendarID string, token *oauth2.Token, gameID, pageToken string) ([]Event, string, bool, error) {
	response, err := h.listCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID, pageToken)
	if err != nil {
		return nil, "", false, err
	}
	if response.StatusCode != http.StatusOK {
		rejected, apiErr := apiResponseError(h.Logger, response)
		return nil, "", rejected, apiErr
	}
	page, err := decodeEventList(response)
	if err != nil {
		return nil, "", false, err
	}
	return page.Items, page.NextPageToken, false, nil
}

// deletedSiteEventOutcome tells a deleted site event from a missing one when
// the search found neither. Google guarantees a deleted event keeps only its
// ID, so the search by private game ID may no longer find it, but a read by
// ID always returns it. The site gives each event the game's ID as its event
// ID, which an .ics import or an event created in Google never has.
func (h *Handler) deletedSiteEventOutcome(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (resultSyncOutcome, bool, error) {
	response, err := h.getCalendarEvent(ctx, calendarID, gameID, token)
	if err != nil {
		return resultUnmatched, false, err
	}
	switch response.StatusCode {
	case http.StatusOK:
		event, err := decodeEvent(response)
		if err != nil {
			return resultUnmatched, false, err
		}
		if isDeletedCalendarEvent(event.Status) {
			return resultDeleted, false, nil
		}
		return resultUnmatched, false, nil
	case http.StatusGone:
		response.Body.Close()
		return resultDeleted, false, nil
	case http.StatusNotFound:
		response.Body.Close()
		return resultUnmatched, false, nil
	default:
		rejected, apiErr := apiResponseError(h.Logger, response)
		return resultUnmatched, rejected, markEventRefused(apiErr)
	}
}

// siteEventMatchesGame reports whether the event is one this site added for
// the game: it carries the game's ID and the site's private marker. An event
// the site added before it wrote that marker is recognised by the rest of its
// provenance, which it has always written: its event ID and private game ID
// are both the game's ID, and its source is the site's Soccer page. An .ics
// import carries neither the private game ID nor the source.
func siteEventMatchesGame(event *Event, gameID string) bool {
	if event == nil || strings.TrimSpace(event.ID) == "" || event.ExtendedProperties.Private[eventGameIDProperty] != gameID {
		return false
	}
	if event.ExtendedProperties.Private[eventOwnerProperty] == eventOwnerValue {
		return true
	}
	return event.ID == gameID && event.Source != nil && event.Source.Title == eventSourceTitle &&
		strings.HasSuffix(event.Source.URL, "/soccer")
}

func isDeletedCalendarEvent(status string) bool {
	return strings.EqualFold(status, googleCancelledStatus) || strings.EqualFold(status, "canceled")
}

// patchResultDescription changes only the event's description, and only while
// the event is still the version Sync read.
func (h *Handler) patchResultDescription(ctx context.Context, calendarID string, token *oauth2.Token, event *Event, description string) (resultSyncOutcome, bool, error) {
	request, err := h.newAPIRequest(ctx, http.MethodPatch, calendarEventsPath+url.PathEscape(calendarID)+"/events/"+url.PathEscape(event.ID), url.Values{"sendUpdates": {"none"}}, token, map[string]string{"description": description})
	if err != nil {
		return resultUnmatched, false, err
	}
	request.Header.Set("If-Match", event.ETag)
	response, err := h.LPSClient.Do(request)
	if err != nil {
		return resultUnmatched, false, err
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		response.Body.Close()
		return resultUpdated, false, nil
	case http.StatusConflict, http.StatusPreconditionFailed:
		response.Body.Close()
		return resultChanged, false, nil
	case http.StatusNotFound, http.StatusGone:
		response.Body.Close()
		return resultDeleted, false, nil
	default:
		rejected, apiErr := apiResponseError(h.Logger, response)
		return resultUnmatched, rejected, markEventRefused(apiErr)
	}
}
