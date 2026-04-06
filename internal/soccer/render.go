package soccer

import (
	"net/http"

	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func (h *Handler) LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps {
	props := partials.SoccerLoginStateProps{
		Authenticated:   session != nil,
		GoogleAvailable: h.Config.GoogleEnabled(),
		LoginAvailable:  h.Config.LoginEnabled(),
		SwapOOB:         swapOOB,
	}
	if session != nil {
		props.Players = session.Players
	}
	if h.googleHooks != nil {
		h.googleHooks.PopulateLoginState(r.Context(), w, r, &props)
	}
	return props
}

func (h *Handler) RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := h.LoginStateProps(w, r, session, false)
	if err := partials.SoccerLoginState(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func RenderLoginFeedback(w http.ResponseWriter, r *http.Request, kind, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	props := partials.SoccerLoginFeedbackProps{Kind: kind, Message: message}
	if err := partials.SoccerLoginFeedback(props).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
