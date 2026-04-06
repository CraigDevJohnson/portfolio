package soccer

import (
	"errors"
	"io"
	"log"
	"net/http"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/lps"
	"portfolio/types"
)

func (h *Handler) SessionHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := h.LoadSession(w, r)
	h.RenderLoginState(w, r, session)
}

func (h *Handler) ImportHandler(w http.ResponseWriter, r *http.Request) {
	if !h.Config.LoginEnabled() {
		RenderLoginFeedback(w, r, "error", "JWT import is unavailable until the session encryption key is configured on the server.")
		return
	}
	if !h.LoginLimiter.Allow(httpx.ClientIP(r)) {
		RenderLoginFeedback(w, r, "error", "Too many import attempts. Wait a minute and try again.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxRequestBodySize)
	if err := r.ParseForm(); err != nil {
		RenderLoginFeedback(w, r, "error", "Could not read the import form. Try again.")
		return
	}

	jwt, err := lps.NormalizeImportedJWT(r.FormValue("jwt"))
	if err != nil {
		RenderLoginFeedback(w, r, "error", err.Error())
		return
	}

	discovery, err := lps.FetchUserPlayers(r.Context(), h.Config.LPSAPIBaseURL, h.LPSClient, jwt)
	if err != nil {
		var fetchErr *lps.FetchError
		if errors.As(err, &fetchErr) {
			switch fetchErr.Kind {
			case lps.ErrorUnauthorized, lps.ErrorForbidden:
				RenderLoginFeedback(w, r, "error", "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and try again.")
				return
			case lps.ErrorUpstream:
				RenderLoginFeedback(w, r, "error", "Could not reach Let's Play Soccer to look up your players. Try again in a moment.")
				return
			}
		}
		RenderLoginFeedback(w, r, "error", err.Error())
		return
	}
	if len(discovery.Players) == 0 {
		RenderLoginFeedback(w, r, "error", "No linked players found for this account.")
		return
	}

	session := types.SessionData{
		JWT:       jwt,
		UserName:  discovery.UserName,
		Players:   discovery.Players,
		ExpiresAt: lps.ImportedSessionExpiry(jwt),
	}
	if err := h.setSession(w, r, &session); err != nil {
		log.Printf("soccer import session write failed: %v", err)
		RenderLoginFeedback(w, r, "error", "The import succeeded, but the session cookie could not be saved.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := partials.SoccerLoginState(h.LoginStateProps(w, r, &session, true)).Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.WriteString(w, `<div class="soccer-login-success" data-login-success>Import saved for this browser session. Choose your players below.</div>`)
}

func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	w.Header().Set("HX-Trigger", "soccer-logout")
	h.RenderLoginState(w, r, nil)
}
