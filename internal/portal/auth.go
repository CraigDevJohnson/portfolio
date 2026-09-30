package portal

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"portfolio/cmd/web/pages"
	"portfolio/internal/config"
)

type contextKey string

const portalUsernameKey contextKey = "portal_username"

// UsernameFromContext returns the authenticated portal username.
func UsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(portalUsernameKey).(string)
	return username, ok && username != ""
}

// LoginPageHandler renders the sign-in page and starts OAuth only on POST.
func (h *Handler) LoginPageHandler(w http.ResponseWriter, r *http.Request) {
	if session, err := h.loadSession(r); err == nil && session.IsValid() {
		http.Redirect(w, r, "/mgmt", http.StatusFound)
		return
	}
	if h.OIDC == nil || h.Config == nil || !h.Config.PortalEnabled() {
		h.renderErrorPage(w, r, http.StatusServiceUnavailable, "The management portal is not configured.")
		return
	}
	if r.Method != http.MethodPost {
		h.renderComponent(w, r, pages.PortalLogin())
		return
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		h.renderErrorPage(w, r, http.StatusInternalServerError, "Unable to start sign-in.")
		return
	}
	state, err := generateState()
	if err != nil {
		h.renderErrorPage(w, r, http.StatusInternalServerError, "Unable to start sign-in.")
		return
	}
	if err := h.setOAuthState(w, r, &OAuthState{State: state, CodeVerifier: verifier}); err != nil {
		h.Logger.Error("portal OAuth state cookie failed", slog.Any("error", err))
		h.renderErrorPage(w, r, http.StatusInternalServerError, "Unable to start sign-in.")
		return
	}
	http.Redirect(w, r, h.OIDC.AuthorizationURL(state, codeChallenge(verifier)), http.StatusSeeOther)
}

// CallbackHandler completes the Cognito Authorization Code + PKCE flow.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	state, stateErr := h.loadOAuthState(r)
	h.clearOAuthState(w, r)
	providedState := r.URL.Query().Get("state")
	if stateErr != nil || state == nil || state.State == "" || providedState == "" || !constantTimeEqual(state.State, providedState) {
		h.rejectSignIn(w, r, http.StatusBadRequest, "invalid_state")
		return
	}
	if oauthError := r.URL.Query().Get("error"); oauthError != "" {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "provider_rejected")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || h.OIDC == nil {
		h.rejectSignIn(w, r, http.StatusBadRequest, "incomplete_response")
		return
	}
	tokens, err := h.OIDC.ExchangeCode(r.Context(), code, state.CodeVerifier)
	if err != nil {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "token_exchange_failed")
		return
	}
	claims, err := h.OIDC.ValidateIDToken(r.Context(), tokens.IDToken)
	if err != nil {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "invalid_token")
		return
	}
	username, emailErr := config.NormalizePortalEmail(claims.Email)
	if emailErr != nil {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "invalid_email")
		return
	}
	if !claims.EmailVerified {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "unverified_email")
		return
	}
	if !h.Config.PortalEmailAllowed(username) {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "email_not_allowed")
		return
	}
	if err := h.setSession(w, r, &PortalSession{Username: username, ExpiresAt: time.Now().Add(config.PortalSessionTTL)}); err != nil {
		h.rejectSignIn(w, r, http.StatusInternalServerError, "session_creation_failed")
		return
	}
	http.Redirect(w, r, "/mgmt", http.StatusFound)
}

// rejectSignIn expires prior authorization and logs only a fixed reason category.
func (h *Handler) rejectSignIn(w http.ResponseWriter, r *http.Request, status int, reason string) {
	h.clearSession(w, r)
	h.Logger.Warn("portal sign-in rejected", slog.String("reason", reason))
	h.renderErrorPage(w, r, status, "Sign-in could not be completed.")
}

// LogoutHandler clears the local session and delegates logout to Cognito when configured.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	h.clearOAuthState(w, r)
	if h.OIDC != nil {
		if target := h.OIDC.LogoutURL(); target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := h.loadSession(r)
		if err != nil {
			h.Logger.Warn("portal session could not be decrypted", slog.Any("error", err))
		}
		if err != nil || !session.IsValid() {
			h.clearSession(w, r)
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), portalUsernameKey, session.Username)
		next(w, r.WithContext(ctx))
	}
}

// RequireAuth protects a portal route with the encrypted session middleware.
func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return h.requireAuth(next)
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
