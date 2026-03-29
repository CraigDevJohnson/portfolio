package soccer

import (
	"context"
	"net/http"

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
