package soccer

import (
	"net/http"
	"strings"

	"portfolio/cmd/web/pages"
)

func (h *Handler) SoccerPage(w http.ResponseWriter, r *http.Request) {
	googleMessageKind, googleMessage := soccerGoogleFlash(r.URL.Query().Get("google"))
	props := pages.SoccerProps{
		GoogleMessage:     googleMessage,
		GoogleMessageKind: googleMessageKind,
	}
	if err := pages.Soccer(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func soccerGoogleFlash(code string) (kind, message string) {
	switch strings.TrimSpace(code) {
	case "connected":
		return "success", "Google Calendar connected. Choose a calendar below and add selected games directly from the schedule table."
	case "denied":
		return "error", "Google Calendar connection was canceled before access was granted."
	case "disconnected":
		return "success", "Google Calendar connection removed."
	case "failed":
		return "error", "Google Calendar connection could not be completed. Try again."
	case "unavailable":
		return "error", "Google Calendar add is unavailable until Google OAuth and server-side storage are configured."
	default:
		return "", ""
	}
}
