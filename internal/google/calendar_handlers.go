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
	googleUnavailableMessage       = "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	googleReadSelectedGamesMessage = "Could not read the selected games. Try again."
	googleExpiredConnectionMessage = "Your Google Calendar connection has expired. Connect again and retry."
	googleInvalidConnectionMessage = "Your Google Calendar connection is no longer valid. Connect again and retry."
)

// AddHandler adds selected games to Google Calendar.
func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	if !h.GoogleAvailable() {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleUnavailableMessage)
		return
	}
	if err := parseGoogleForm(r, w); err != nil {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleReadSelectedGamesMessage)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(r.Context(), r)
	if !ok {
		h.Soccer.RenderLoginFeedback(w, r, "error", "Connect Google Calendar before adding selected games.")
		return
	}
	session, filteredGames, message, ok := h.Soccer.ResolveGoogleAddSelection(w, r)
	if !ok {
		h.Soccer.RenderLoginFeedback(w, r, "error", message)
		return
	}
	token, err := h.CurrentToken(r.Context(), r, record)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("google token refresh failed", slog.Any("error", err))
		h.RenderDisconnectFeedback(w, r, session, googleExpiredConnectionMessage)
		return
	}
	added, updated, skipped, authRejected, err := h.insertCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error(
			"google event insert failed",
			slog.Any("error", err),
			slog.Int("selected_game_count", len(filteredGames)),
		)
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	successMessage := fmt.Sprintf("Added %d selected game(s) to Google Calendar.", added)
	if updated > 0 {
		successMessage += fmt.Sprintf(" Updated/restored %d matching game(s).", updated)
	}
	if skipped > 0 {
		successMessage += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", skipped)
	}
	h.Soccer.RenderLoginFeedback(w, r, "success", successMessage)
}

// SyncResultsHandler updates previously synced past games with result text.
func (h *Handler) SyncResultsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.GoogleAvailable() {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleUnavailableMessage)
		return
	}
	if err := parseGoogleForm(r, w); err != nil {
		h.Soccer.RenderLoginFeedback(w, r, "error", googleReadSelectedGamesMessage)
		return
	}
	record, ok := h.loadConnectionRecordOrLog(r.Context(), r)
	if !ok {
		h.Soccer.RenderLoginFeedback(w, r, "error", "Connect Google Calendar before syncing results.")
		return
	}
	session, games, message, ok := h.Soccer.ResolveSyncResultsGames(w, r)
	if !ok {
		if message != "" {
			logging.WithContext(h.Logger, r.Context()).Info("google result sync skipped", slog.String("reason", message))
		}
		h.Soccer.RenderLoginFeedback(w, r, "error", message)
		return
	}
	logging.WithContext(h.Logger, r.Context()).Info("google result sync candidate games", slog.Int("candidate_game_count", len(games)))
	if len(games) == 0 {
		logging.WithContext(h.Logger, r.Context()).Info("google result sync found no past games with results")
		h.Soccer.RenderLoginFeedback(w, r, "success", "No past games with results to sync.")
		return
	}

	token, err := h.CurrentToken(r.Context(), r, record)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Warn("google token refresh failed", slog.Any("error", err))
		h.RenderDisconnectFeedback(w, r, session, googleExpiredConnectionMessage)
		return
	}
	added, updated, skipped, authRejected, err := h.insertCalendarEvents(r, record, token, games)
	if err != nil {
		logging.WithContext(h.Logger, r.Context()).Error("google result sync failed", slog.Any("error", err))
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not sync past game results to Google Calendar. Try again.")
		return
	}
	if authRejected {
		h.RenderDisconnectFeedback(w, r, session, googleInvalidConnectionMessage)
		return
	}
	logging.WithContext(h.Logger, r.Context()).Info(
		"google result sync completed",
		slog.Int("updated_count", added+updated),
		slog.Int("skipped_count", skipped),
	)

	successMessage := fmt.Sprintf("%d game result(s) updated in Google Calendar.", added+updated)
	if skipped > 0 {
		successMessage += fmt.Sprintf(" Skipped %d game(s) that could not be matched to the same Google game ID.", skipped)
	}
	h.Soccer.RenderLoginFeedback(w, r, "success", successMessage)
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
