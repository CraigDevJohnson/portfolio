package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	"portfolio/internal/logging"
	"portfolio/types"
)

const (
	googleUnavailableMessage         = "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	googleReadSelectedGamesMessage   = "Could not read the selected games. Try again."
	googleExpiredConnectionMessage   = "Your Google Calendar connection has expired. Connect again and retry."
	googleInvalidConnectionMessage   = "Your Google Calendar connection is no longer valid. Connect again and retry."
	googleCalendarChoiceMessage      = "Choose a writable calendar before continuing. Writes are paused until you save a destination."
	safeCalendarMutationRetryMessage = "The request reached its time limit. Retry to finish; existing games will be matched instead of duplicated."
	safeResultSyncRetryMessage       = "The request reached its time limit. Retry to finish; results already current will be left unchanged."
	safeResultSyncFailureMessage     = "Could not finish result sync. Retry later; results already current will be left unchanged."
)

// AddHandler adds selected games to Google Calendar.
func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	timeout := h.CalendarMutationTimeout
	if timeout <= 0 {
		timeout = 24 * time.Second
	}
	workCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	workRequest := r.Clone(workCtx)

	if !h.GoogleAvailable() {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleUnavailableMessage)
		return
	}
	if err := parseGoogleForm(workRequest, w); err != nil {
		if calendarMutationContextEnded(workCtx, err) {
			h.renderAddMutationDeadline(w, r, calendarMutationResult{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", googleReadSelectedGamesMessage)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(workCtx, workRequest)
	if !ok {
		if calendarMutationContextEnded(workCtx, nil) {
			h.renderAddMutationDeadline(w, r, calendarMutationResult{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	session, filteredGames, message, ok := h.Soccer.ResolveGoogleAddSelection(w, workRequest)
	if !ok {
		if calendarMutationContextEnded(workCtx, nil) {
			h.renderAddMutationDeadline(w, r, calendarMutationResult{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", message)
		return
	}
	token, err := h.CurrentToken(workCtx, workRequest, record)
	if err != nil {
		logging.WithContext(h.Logger, workCtx).Warn("google token refresh failed", slog.Any("error", err))
		if calendarMutationContextEnded(workCtx, err) {
			h.renderAddMutationDeadline(w, r, calendarMutationResult{})
			return
		}
		h.RenderDisconnectFeedback(w, r, session, googleExpiredConnectionMessage)
		return
	}
	if !h.destinationReady(workCtx, w, r, session, record, token, "Could not verify the selected calendar. No games were added; try again later.") {
		return
	}
	result, err := h.insertCalendarEvents(workCtx, workRequest, record, token, filteredGames)
	if err != nil {
		logging.WithContext(h.Logger, workCtx).Error(
			"google event insert failed",
			slog.Any("error", err),
			slog.Int("selected_game_count", len(filteredGames)),
			slog.Int("added_count", result.added),
			slog.Int("updated_count", result.updated),
			slog.Int("skipped_count", result.skipped),
		)
		if calendarMutationContextEnded(workCtx, err) {
			h.renderAddMutationDeadline(w, r, result)
			return
		}
		if calendarDestinationRejected(err) {
			done := ""
			if result.added+result.updated > 0 {
				done = addMutationMessage(result) + " Those games stay in " + calendarNameOrDefault(record.CalendarSummary) + "; deselect them before adding to a new calendar."
			}
			h.pauseAndRenderCalendarChoice(workCtx, w, r, session, record, done)
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if result.authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	h.Soccer.RenderLoginFeedback(w, r, "success", addMutationMessage(result))
}

// calendarDestinationRejected reports whether the chosen calendar refused an
// event request because it is gone or no longer accepts this account's
// writes, rather than because of the connection or a usage limit.
func calendarDestinationRejected(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.destinationRejected()
}

// destinationReady reports whether the chosen calendar still accepts this
// connection's events. When it does not, it answers the request: a rejected
// connection is removed with a request to reconnect, a lost or unwritable
// calendar pauses writes for a new choice, and a failed check asks the
// visitor to retry with checkFailedMessage.
func (h *Handler) destinationReady(ctx context.Context, w http.ResponseWriter, r *http.Request, session *types.SessionData, record *ConnectionRecord, token *oauth2.Token, checkFailedMessage string) bool {
	writable, err := h.ensureWritableCalendar(ctx, record, token)
	switch {
	case err != nil:
		logging.WithContext(h.Logger, ctx).Warn("google destination check failed", slog.Any("error", err))
		if isGoogleAuthRejected(err) {
			h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		} else {
			h.Soccer.RenderLoginFeedback(w, r, "error", checkFailedMessage)
		}
		return false
	case !writable:
		h.renderCalendarChoiceRequired(w, r, session, "")
		return false
	default:
		return true
	}
}

// renderCalendarChoiceRequired asks the visitor to choose a writable
// calendar. done, when set, first reports what the request already wrote.
func (h *Handler) renderCalendarChoiceRequired(w http.ResponseWriter, r *http.Request, session *types.SessionData, done string) {
	message := googleCalendarChoiceMessage
	if done != "" {
		message = done + " " + message
	}
	h.Soccer.RenderLoginStateOOB(w, r, session)
	h.Soccer.RenderLoginFeedback(w, r, "error", message)
}

// pauseAndRenderCalendarChoice pauses writes after the destination refused
// one, reporting done as renderCalendarChoiceRequired does.
func (h *Handler) pauseAndRenderCalendarChoice(ctx context.Context, w http.ResponseWriter, r *http.Request, session *types.SessionData, record *ConnectionRecord, done string) {
	if err := h.pauseCalendarSelection(ctx, record); err != nil {
		logging.WithContext(h.Logger, ctx).Error("google calendar pause save failed", slog.Any("error", err))
	}
	h.renderCalendarChoiceRequired(w, r, session, done)
}

// calendarNameOrDefault names a destination calendar in a message.
func calendarNameOrDefault(summary string) string {
	if summary = strings.TrimSpace(summary); summary != "" {
		return summary
	}
	return "the previous calendar"
}

// SyncResultsHandler writes the results of the selected scored past games
// into the events this site added for them, changing nothing else and
// inserting no event.
func (h *Handler) SyncResultsHandler(w http.ResponseWriter, r *http.Request) {
	timeout := h.CalendarMutationTimeout
	if timeout <= 0 {
		timeout = 24 * time.Second
	}
	workCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	workRequest := r.Clone(workCtx)

	if !h.GoogleAvailable() {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleUnavailableMessage)
		return
	}
	if err := parseGoogleForm(workRequest, w); err != nil {
		if calendarMutationContextEnded(workCtx, err) {
			h.renderSyncResultsDeadline(w, r, resultSyncReport{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", googleReadSelectedGamesMessage)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(workCtx, workRequest)
	if !ok {
		if calendarMutationContextEnded(workCtx, nil) {
			h.renderSyncResultsDeadline(w, r, resultSyncReport{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", "Connect Google Calendar before syncing results.")
		return
	}
	session, games, message, ok := h.Soccer.ResolveSyncResultsGames(w, workRequest)
	if !ok {
		if message != "" {
			logging.WithContext(h.Logger, workCtx).Info("google result sync skipped", slog.String("reason", message))
		}
		if calendarMutationContextEnded(workCtx, nil) {
			h.renderSyncResultsDeadline(w, r, resultSyncReport{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", message)
		return
	}
	logging.WithContext(h.Logger, workCtx).Info("google result sync candidate games", slog.Int("candidate_game_count", len(games)))
	if len(games) == 0 {
		logging.WithContext(h.Logger, workCtx).Info("google result sync found no past games with results")
		h.Soccer.RenderLoginFeedback(w, r, "success", "No past games with results to sync.")
		return
	}

	token, err := h.CurrentToken(workCtx, workRequest, record)
	if err != nil {
		logging.WithContext(h.Logger, workCtx).Warn("google token refresh failed", slog.Any("error", err))
		if calendarMutationContextEnded(workCtx, err) {
			h.renderSyncResultsDeadline(w, r, resultSyncReport{})
			return
		}
		h.RenderDisconnectFeedback(w, r, session, googleExpiredConnectionMessage)
		return
	}
	if !h.destinationReady(workCtx, w, r, session, record, token, "Could not verify the selected calendar. No results were synced; try again later.") {
		return
	}
	report, err := h.syncResultEvents(h.httpContext(workCtx), record.CalendarID, token, games)
	if err != nil {
		logging.WithContext(h.Logger, workCtx).Error(
			"google result sync failed",
			slog.Any("error", err),
			slog.Int("updated_count", report.outcomes[resultUpdated]),
			slog.Int("current_count", report.outcomes[resultCurrent]),
			slog.Int("skipped_count", report.skipped()),
		)
		if calendarMutationContextEnded(workCtx, err) {
			h.renderSyncResultsDeadline(w, r, report)
			return
		}
		if calendarDestinationRejected(err) {
			h.pauseAndRenderCalendarChoice(workCtx, w, r, session, record, report.done())
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", report.message()+" "+safeResultSyncFailureMessage)
		return
	}
	if report.authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	logging.WithContext(h.Logger, workCtx).Info(
		"google result sync completed",
		slog.Int("updated_count", report.outcomes[resultUpdated]),
		slog.Int("current_count", report.outcomes[resultCurrent]),
		slog.Int("skipped_count", report.skipped()),
	)

	h.Soccer.RenderLoginFeedback(w, r, "success", report.message())
}

func calendarMutationContextEnded(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled)
}

func addMutationMessage(result calendarMutationResult) string {
	message := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", result.added)
	if result.updated > 0 {
		message += fmt.Sprintf(" Updated/restored %d matching game(s).", result.updated)
	}
	return message + skippedGamesMessage(result)
}

// skippedGamesMessage reports the games an Add left alone.
func skippedGamesMessage(result calendarMutationResult) string {
	message := ""
	if result.skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", result.skipped)
	}
	if result.refused > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) whose existing event Google Calendar would not let this account change.", result.refused)
	}
	if result.undated > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) without a start time yet.", result.undated)
	}
	return message
}

func (h *Handler) renderAddMutationDeadline(w http.ResponseWriter, r *http.Request, result calendarMutationResult) {
	h.Soccer.RenderLoginFeedback(w, r, "error", addMutationMessage(result)+" "+safeCalendarMutationRetryMessage)
}

func (h *Handler) renderSyncResultsDeadline(w http.ResponseWriter, r *http.Request, report resultSyncReport) {
	h.Soccer.RenderLoginFeedback(w, r, "error", report.message()+" "+safeResultSyncRetryMessage)
}

// CalendarHandler handles calendar selection changes.
func (h *Handler) CalendarHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := h.Soccer.LoadSession(w, r)
	if !h.GoogleAvailable() {
		h.Soccer.RenderLoginStateRefresh(w, r, session)
		return
	}
	if err := parseGoogleForm(r, w); err != nil {
		h.Soccer.RenderLoginStateRefresh(w, r, session)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(r.Context(), r)
	if !ok {
		h.Soccer.RenderLoginStateRefresh(w, r, session)
		return
	}
	calendars, err := h.ListCalendars(r.Context(), r, record)
	if err != nil {
		logger := logging.WithContext(h.Logger, r.Context())
		if isGoogleAuthRejected(err) {
			logger.Warn("google calendar connection expired", slog.Any("error", err))
			h.DeleteConnection(r.Context(), w, r)
		} else {
			logger.Error("google calendar list failed", slog.Any("error", err))
		}
		h.Soccer.RenderLoginStateRefresh(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.Form.Get("calendar_id"))
	selectedCalendarSummary := calendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		h.Soccer.RenderLoginStateRefresh(w, r, session)
		return
	}
	read := record.UpdatedAt
	record.selectCalendar(selectedCalendarID, selectedCalendarSummary, time.Now().UTC())
	switch err := h.Store().PutIfUnchanged(r.Context(), record, read); {
	case errors.Is(err, ErrConnectionChanged):
		// A connection removed or saved meanwhile keeps its newer state,
		// which the refreshed card shows.
		logging.WithContext(h.Logger, r.Context()).Info("google connection changed before the calendar choice was saved")
	case err != nil:
		logging.WithContext(h.Logger, r.Context()).Error("google calendar selection save failed", slog.Any("error", err))
	}
	h.Soccer.RenderLoginStateRefresh(w, r, session)
}

func apiResponseError(logger *slog.Logger, resp *http.Response) (bool, error) {
	apiErr := readAPIError(resp)
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "google"))
	}
	logger.Warn("google event insert rejected", slog.Any("error", apiErr))
	var googleErr *APIError
	return errors.As(apiErr, &googleErr) && googleErr.credentialsRejected(), apiErr
}

func parseGoogleForm(r *http.Request, w http.ResponseWriter) error {
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	return r.ParseForm()
}

func (h *Handler) loadConnectionRecordOrLog(ctx context.Context, r *http.Request) (*ConnectionRecord, bool) {
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("google connection read failed", slog.Any("error", err))
		return nil, false
	}
	if record == nil {
		return nil, false
	}
	return record, true
}
