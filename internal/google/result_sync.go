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
	// resultDeleted: the site's event for the game was deleted.
	resultDeleted
	// resultAmbiguous: more than one site event names the game.
	resultAmbiguous
	// resultChanged: the event changed in Google between Sync reading and
	// updating it.
	resultChanged
	// resultEdited: the event's description no longer holds the result slot
	// Add wrote, or the visitor wrote their own result there.
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
// skipped game was left alone.
func (report resultSyncReport) message() string {
	message := fmt.Sprintf("%d game result(s) updated in Google Calendar.", report.outcomes[resultUpdated])
	if current := report.outcomes[resultCurrent]; current > 0 {
		message += fmt.Sprintf(" %d result(s) already current.", current)
	}
	skipped := report.skipped()
	message += fmt.Sprintf(" Skipped %d game(s)", skipped)
	if skipped == 0 {
		return message + "."
	}
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
		{report.refused, "Google Calendar would not let this account change"},
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

// findSiteEvent returns the one live event this site added for gameID in the
// calendar, as one bounded search lists it with its current version. Without
// one, it returns why: no such event, a deleted one, or more than one.
func (h *Handler) findSiteEvent(ctx context.Context, calendarID string, token *oauth2.Token, gameID string) (*Event, resultSyncOutcome, bool, error) {
	response, err := h.listCalendarEventsByPrivateGameID(ctx, calendarID, token, gameID)
	if err != nil {
		return nil, resultUnmatched, false, err
	}
	if response.StatusCode != http.StatusOK {
		rejected, apiErr := apiResponseError(h.Logger, response)
		return nil, resultUnmatched, rejected, apiErr
	}
	page, err := decodeEventList(response)
	if err != nil {
		return nil, resultUnmatched, false, err
	}
	if page.NextPageToken != "" {
		return nil, resultAmbiguous, false, nil
	}
	var siteEvents []*Event
	for i := range page.Items {
		if event := &page.Items[i]; siteEventMatchesGame(event, gameID) {
			siteEvents = append(siteEvents, event)
		}
	}
	switch {
	case len(siteEvents) == 0:
		return nil, resultUnmatched, false, nil
	case len(siteEvents) > 1:
		return nil, resultAmbiguous, false, nil
	case isDeletedCalendarEvent(siteEvents[0].Status):
		return nil, resultDeleted, false, nil
	}
	if strings.TrimSpace(siteEvents[0].ETag) == "" {
		// Without its version, a change could not be made conditional.
		return nil, resultChanged, false, nil
	}
	return siteEvents[0], resultUnmatched, false, nil
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
