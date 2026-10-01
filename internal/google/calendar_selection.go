package google

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/logging"
	"portfolio/types"
)

// reconcileCalendarSelection checks the connection's destination against the
// calendars its account can write now, and saves any change at most once:
//
//   - A paused destination stays paused until the visitor chooses again.
//   - An unset destination becomes the primary calendar. While the account
//     can write no calendar it stays unset: nothing was chosen, so nothing
//     is paused.
//   - A chosen calendar that is gone or no longer writable pauses writes
//     rather than falling back to primary.
//   - A writable destination takes Google's current name for it.
//
// The save applies only over the copy of the connection this request read,
// so it never restores a connection removed meanwhile or undoes another
// request's save; it then returns ErrConnectionChanged. It returns the
// destination and whether it accepts writes now.
func (h *Handler) reconcileCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (id, summary string, writable bool, err error) {
	read := record.UpdatedAt
	id = strings.TrimSpace(record.CalendarID)
	summary = calendarSummary(calendars, id)
	if record.CalendarSelectionRequired {
		return id, summary, false, nil
	}
	now := time.Now().UTC()
	changed := false
	switch {
	case id == "":
		id, summary = preferredCalendar(calendars)
		if id == "" {
			return "", "", false, nil
		}
		record.selectCalendar(id, summary, now)
		changed = true
	case summary == "":
		changed = record.pauseSelection(now)
	case id != record.CalendarID || summary != record.CalendarSummary:
		record.selectCalendar(id, summary, now)
		changed = true
	}
	if changed {
		if err := h.Store().PutIfUnchanged(ctx, record, read); err != nil {
			return id, summary, false, err
		}
	}
	return id, summary, !record.CalendarSelectionRequired, nil
}

// reconnectDestination returns the destination for a connection saved by a
// new consent. When the same Google account consented again over the
// owner's stored connection, the calendar chosen before stays the
// destination while that account can write it, even if writes to it were
// paused. Otherwise the new connection starts at the primary calendar.
func reconnectDestination(previous *ConnectionRecord, accountSubject string, calendars []types.GoogleCalendarOption) (id, summary string) {
	if previous != nil && previous.accountVerified() && previous.AccountSubject == accountSubject {
		if summary := calendarSummary(calendars, previous.CalendarID); summary != "" {
			return previous.CalendarID, summary
		}
	}
	return preferredCalendar(calendars)
}

// SyncCalendarSelection reconciles the connection's destination for a page
// view and returns the destination to show. A save that failed, or that
// another request's save overtook, is logged; the next page view or Add
// reconciles again.
func (h *Handler) SyncCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (calendarID, summary string) {
	calendarID, summary, _, err := h.reconcileCalendarSelection(ctx, record, calendars)
	switch {
	case errors.Is(err, ErrConnectionChanged):
		logging.WithContext(h.Logger, ctx).Info("google connection changed before its destination was saved")
	case err != nil:
		logging.WithContext(h.Logger, ctx).Error("google calendar selection save failed", slog.Any("error", err))
	}
	return calendarID, summary
}

// ensureWritableCalendar reconciles the connection's destination before a
// write and reports whether it accepts writes now.
func (h *Handler) ensureWritableCalendar(ctx context.Context, record *ConnectionRecord, token *oauth2.Token) (bool, error) {
	if record.CalendarSelectionRequired {
		return false, nil
	}
	calendars, err := h.listCalendarsWithToken(h.httpContext(ctx), token)
	if err != nil {
		return false, err
	}
	_, _, writable, err := h.reconcileCalendarSelection(ctx, record, calendars)
	return writable, err
}

// pauseCalendarSelection records that writes wait for a new calendar choice.
// When another request removed or saved the connection since this one read
// it, that newer state stands and nothing is paused.
func (h *Handler) pauseCalendarSelection(ctx context.Context, record *ConnectionRecord) error {
	read := record.UpdatedAt
	if !record.pauseSelection(time.Now().UTC()) {
		return nil
	}
	err := h.Store().PutIfUnchanged(ctx, record, read)
	if errors.Is(err, ErrConnectionChanged) {
		logging.WithContext(h.Logger, ctx).Info("google connection changed during the request; destination not paused")
		return nil
	}
	return err
}
