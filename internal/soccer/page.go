package soccer

import (
	"context"
	"net/http"
	"strings"

	"portfolio/cmd/web/pages"
)

// SoccerPage renders the soccer page shell and delegates OAuth callbacks to the temporary Google bridge.
func (h *Handler) SoccerPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.googleHooks != nil && h.googleHooks.HandlePageCallback(w, r) {
		return
	}
	props := pages.SoccerProps{
		GoogleMessage:     soccerGoogleFlashMessage(r.URL.Query().Get("google")),
		GoogleMessageKind: soccerGoogleFlashKind(r.URL.Query().Get("google")),
	}
	if err := pages.Soccer(props).Render(context.Background(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerGoogleFlashKind(code string) string {
	switch strings.TrimSpace(code) {
	case "failed", "denied", "unavailable":
		return "error"
	default:
		return "success"
	}
}

func soccerGoogleFlashMessage(code string) string {
	switch strings.TrimSpace(code) {
	case "connected":
		return "Google Calendar connected. Choose a calendar below and add selected games directly from the schedule table."
	case "denied":
		return "Google Calendar connection was canceled before access was granted."
	case "disconnected":
		return "Google Calendar connection removed."
	case "failed":
		return "Google Calendar connection could not be completed. Try again."
	case "unavailable":
		return "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	default:
		return ""
	}
}
