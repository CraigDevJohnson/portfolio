package portal

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
)

func (h *Handler) renderErrorPage(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.setHTMLContentType(w)
	w.WriteHeader(status)
	if err := pages.PortalError(pages.ErrorPageProps{StatusCode: status, Message: message}).Render(r.Context(), w); err != nil {
		h.Logger.Error("portal error page render failed", slog.Any("error", err))
	}
}

func (h *Handler) renderDashboard(w http.ResponseWriter, r *http.Request, props pages.DashboardProps) {
	h.renderComponent(w, r, pages.PortalDashboard(props))
}

func (h *Handler) renderFragment(w http.ResponseWriter, r *http.Request, component templ.Component) {
	h.renderComponent(w, r, component)
}

func (h *Handler) renderComponent(w http.ResponseWriter, r *http.Request, component templ.Component) {
	h.setHTMLContentType(w)
	if err := component.Render(r.Context(), w); err != nil {
		h.Logger.Error("portal component render failed", slog.String("path", r.URL.Path), slog.Any("error", err))
		return
	}
}

func (h *Handler) renderActionResult(w http.ResponseWriter, r *http.Request, success bool, message string) {
	h.renderFragment(w, r, partials.ActionResult(partials.ActionResultProps{Success: success, Message: message}))
}
