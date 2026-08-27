package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/logging"
)

const (
	googleUnavailableMessage         = "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	googleReadSelectedGamesMessage   = "Could not read the selected games. Try again."
	googleExpiredConnectionMessage   = "Your Google Calendar connection has expired. Connect again and retry."
	googleInvalidConnectionMessage   = "Your Google Calendar connection is no longer valid. Connect again and retry."
	safeCalendarMutationRetryMessage = "The request reached its time limit. Retry to finish; existing games will be matched instead of duplicated."
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
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if result.authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	h.Soccer.RenderLoginFeedback(w, r, "success", addMutationMessage(result))
}

// SyncResultsHandler updates previously synced past games with result text.
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
			h.renderSyncResultsDeadline(w, r, calendarMutationResult{})
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", googleReadSelectedGamesMessage)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(workCtx, workRequest)
	if !ok {
		if calendarMutationContextEnded(workCtx, nil) {
			h.renderSyncResultsDeadline(w, r, calendarMutationResult{})
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
			h.renderSyncResultsDeadline(w, r, calendarMutationResult{})
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
			h.renderSyncResultsDeadline(w, r, calendarMutationResult{})
			return
		}
		h.RenderDisconnectFeedback(w, r, session, googleExpiredConnectionMessage)
		return
	}
	result, err := h.insertCalendarEvents(workCtx, workRequest, record, token, games)
	if err != nil {
		logging.WithContext(h.Logger, workCtx).Error(
			"google result sync failed",
			slog.Any("error", err),
			slog.Int("added_count", result.added),
			slog.Int("updated_count", result.updated),
			slog.Int("skipped_count", result.skipped),
		)
		if calendarMutationContextEnded(workCtx, err) {
			h.renderSyncResultsDeadline(w, r, result)
			return
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not sync past game results to Google Calendar. Try again.")
		return
	}
	if result.authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	logging.WithContext(h.Logger, workCtx).Info(
		"google result sync completed",
		slog.Int("updated_count", result.added+result.updated),
		slog.Int("skipped_count", result.skipped),
	)

	h.Soccer.RenderLoginFeedback(w, r, "success", syncResultsMutationMessage(result))
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
	if result.skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", result.skipped)
	}
	return message
}

func syncResultsMutationMessage(result calendarMutationResult) string {
	message := fmt.Sprintf("%d game result(s) updated in Google Calendar.", result.added+result.updated)
	if result.skipped > 0 {
		message += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", result.skipped)
	}
	return message
}

func (h *Handler) renderAddMutationDeadline(w http.ResponseWriter, r *http.Request, result calendarMutationResult) {
	h.Soccer.RenderLoginFeedback(w, r, "error", addMutationMessage(result)+" "+safeCalendarMutationRetryMessage)
}

func (h *Handler) renderSyncResultsDeadline(w http.ResponseWriter, r *http.Request, result calendarMutationResult) {
	h.Soccer.RenderLoginFeedback(w, r, "error", syncResultsMutationMessage(result)+" "+safeCalendarMutationRetryMessage)
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
		selectedCalendarID, selectedCalendarSummary = preferredCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := h.Store().Put(r.Context(), record); err != nil {
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
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden), apiErr
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
