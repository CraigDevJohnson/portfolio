package google

import (
	"context"
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
// It returns the destination and whether it accepts writes now.
func (h *Handler) reconcileCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (id, summary string, writable bool, err error) {
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
		if err := h.Store().Put(ctx, record); err != nil {
			return id, summary, false, err
		}
	}
	return id, summary, !record.CalendarSelectionRequired, nil
}

// SyncCalendarSelection reconciles the connection's destination for a page
// view and returns the destination to show. A failed save is logged; the
// next page view or Add tries again.
func (h *Handler) SyncCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (calendarID, summary string) {
	calendarID, summary, _, err := h.reconcileCalendarSelection(ctx, record, calendars)
	if err != nil {
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
func (h *Handler) pauseCalendarSelection(ctx context.Context, record *ConnectionRecord) error {
	if !record.pauseSelection(time.Now().UTC()) {
		return nil
	}
	return h.Store().Put(ctx, record)
}
