package google

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
)

// AddHandler adds selected games to Google Calendar.
func (h *Handler) AddHandler(w http.ResponseWriter, r *http.Request) {
	if !h.Config.GoogleEnabled() {
		h.Soccer.RenderLoginFeedback(w, r, "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not read the selected games. Try again.")
		return
	}
	record, err := h.LoadConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
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
		log.Printf("google token refresh failed: %v", err)
		h.RenderDisconnectFeedback(w, r, session, "Your Google Calendar connection has expired. Connect again and retry.")
		return
	}
	added, updated, skipped, authRejected, err := h.insertCalendarEvents(r, record, token, filteredGames)
	if err != nil {
		log.Printf("google event insert failed: %v", err)
		h.Soccer.RenderLoginFeedback(w, r, "error", "Could not add the selected games to Google Calendar. Try again.")
		return
	}
	if authRejected {
		h.RenderDisconnectFeedback(w, r, session, "Your Google Calendar connection is no longer valid. Connect again and retry.")
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

// CalendarHandler handles calendar selection changes.
func (h *Handler) CalendarHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := h.Soccer.LoadSession(w, r)
	if !h.Config.GoogleEnabled() {
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	record, err := h.LoadConnectionRecord(r.Context(), r)
	if err != nil || record == nil {
		if err != nil {
			log.Printf("google connection read failed: %v", err)
		}
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	calendars, err := h.ListCalendars(r.Context(), r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		h.Soccer.RenderLoginState(w, r, session)
		return
	}
	selectedCalendarID := strings.TrimSpace(r.FormValue("calendar_id"))
	selectedCalendarSummary := calendarSummary(calendars, selectedCalendarID)
	if selectedCalendarSummary == "" {
		selectedCalendarID, selectedCalendarSummary = preferredCalendar(calendars)
	}
	record.CalendarID = selectedCalendarID
	record.CalendarSummary = selectedCalendarSummary
	record.UpdatedAt = time.Now().UTC()
	if err := h.Store().Put(r.Context(), record); err != nil {
		log.Printf("google calendar selection save failed: %v", err)
	}
	h.Soccer.RenderLoginState(w, r, session)
}

func apiResponseError(resp *http.Response) (bool, error) {
	apiErr := readAPIError(resp)
	log.Printf("google event insert rejected: %v", apiErr)
	var googleErr *APIError
	return errors.As(apiErr, &googleErr) && (googleErr.StatusCode == http.StatusUnauthorized || googleErr.StatusCode == http.StatusForbidden), apiErr
}
