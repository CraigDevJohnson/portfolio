// Package siteauth provides the site's invite-only Cognito sign-in journey.
package siteauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portfolio/cmd/web/pages"
	"portfolio/internal/config"
	"portfolio/internal/portal"
	"portfolio/internal/siteidentity"
)

// Handler owns site sign-in independently of portal AWS client availability.
type Handler struct {
	Config *config.Config
	OIDC   *portal.OIDCClient
	Logger *slog.Logger
}

// NewHandler constructs the shared site sign-in handler.
func NewHandler(cfg *config.Config, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{Config: cfg, Logger: logger.With(slog.String("component", "site_auth"))}
	if cfg != nil && cfg.SiteEnabled() {
		h.OIDC = portal.NewOIDCClient(cfg.SiteCognitoDomain, cfg.SiteCognitoIssuer, cfg.SiteCognitoClientID, cfg.SiteCognitoRedirectURI, cfg.SiteCognitoLogoutURI)
	}
	return h
}

// WithIdentity supplies a verified principal and current invitation grants to each request.
func (h *Handler) WithIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var principal *siteidentity.Principal
		var grants []siteidentity.Grant
		if h.Config != nil && h.Config.SiteEnabled() {
			stored, err := h.loadSession(r)
			if err == nil && stored != nil && h.validSession(stored) {
				principal = &stored.Principal
				for _, grant := range h.Config.SiteGrantsFor(principal.Email) {
					grants = append(grants, siteidentity.Grant(grant))
				}
			}
		}
		if principal == nil {
			if _, cookieErr := r.Cookie(config.SiteSessionCookieName); cookieErr == nil {
				h.clearSession(w, r)
			}
		}
		ctx := siteidentity.WithRequestIdentity(r.Context(), principal, grants, safeReturnTo(r.URL.RequestURI()))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoginHandler renders a signed-out landing on GET and starts Google sign-in on POST.
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			h.renderLogin(w, r, http.StatusBadRequest, "/", "Sign-in could not be started.")
			return
		}
		returnTo = safeReturnTo(r.PostForm.Get("return_to"))
	}
	if _, signedIn := siteidentity.PrincipalFromContext(r.Context()); signedIn {
		redirectLocal(w, returnTo)
		return
	}
	if h.OIDC == nil || h.Config == nil || !h.Config.SiteEnabled() {
		h.renderLogin(w, r, http.StatusServiceUnavailable, returnTo, "Site sign-in is unavailable right now.")
		return
	}
	if r.Method == http.MethodGet {
		h.renderLogin(w, r, http.StatusOK, returnTo, "")
		return
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, returnTo, "Sign-in could not be started.")
		return
	}
	state, err := randomHex(16)
	if err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, returnTo, "Sign-in could not be started.")
		return
	}
	pending := &oauthState{State: state, CodeVerifier: verifier, ReturnTo: returnTo, ExpiresAt: time.Now().Add(config.SiteOAuthStateTTL)}
	if err := h.setOAuthState(w, r, pending); err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, returnTo, "Sign-in could not be started.")
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	http.Redirect(w, r, h.OIDC.AuthorizationURL(state, challenge), http.StatusSeeOther)
}

// CallbackHandler accepts only a signed Cognito identity invited in current configuration.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	pending, pendingErr := h.loadOAuthState(r)
	h.clearOAuthState(w, r)
	providedState := r.URL.Query().Get("state")
	if pendingErr != nil || pending == nil || pending.State == "" || providedState == "" ||
		!time.Now().Before(pending.ExpiresAt) ||
		subtle.ConstantTimeCompare([]byte(pending.State), []byte(providedState)) != 1 {
		h.rejectSignIn(w, r, http.StatusBadRequest, "invalid_state")
		return
	}
	if r.URL.Query().Get("error") != "" {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "provider_rejected")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || h.OIDC == nil || h.Config == nil || !h.Config.SiteEnabled() {
		h.rejectSignIn(w, r, http.StatusBadRequest, "incomplete_response")
		return
	}
	tokens, err := h.OIDC.ExchangeCode(r.Context(), code, pending.CodeVerifier)
	if err != nil || tokens.IDToken == "" {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "token_exchange_failed")
		return
	}
	claims, err := h.OIDC.ValidateIDToken(r.Context(), tokens.IDToken)
	if err != nil || claims.Issuer != h.Config.SiteCognitoIssuer || strings.TrimSpace(claims.Sub) == "" {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "invalid_token")
		return
	}
	email, err := config.NormalizePortalEmail(claims.Email)
	if err != nil || !claims.EmailVerified || !h.Config.SiteEmailInvited(email) {
		h.rejectSignIn(w, r, http.StatusUnauthorized, "identity_not_invited")
		return
	}
	expiresAt := time.Now().Add(config.SiteSessionTTL)
	if tokenExpiry := time.Unix(claims.Expiry, 0); tokenExpiry.Before(expiresAt) {
		expiresAt = tokenExpiry
	}
	value := &siteSession{Principal: siteidentity.Principal{Issuer: claims.Issuer, Subject: claims.Sub, Email: email}, ExpiresAt: expiresAt}
	if err := h.setSession(w, r, value); err != nil {
		h.rejectSignIn(w, r, http.StatusInternalServerError, "session_creation_failed")
		return
	}
	redirectLocal(w, pending.ReturnTo)
}

// LogoutHandler expires site identity and delegates the managed login journey to Cognito.
func (h *Handler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	h.clearSession(w, r)
	h.clearOAuthState(w, r)
	if h.OIDC != nil {
		if target := h.OIDC.LogoutURL(); target != "" {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

func (h *Handler) rejectSignIn(w http.ResponseWriter, r *http.Request, status int, reason string) {
	h.clearSession(w, r)
	h.Logger.Warn("site sign-in rejected", slog.String("reason", reason))
	ctx := siteidentity.WithRequestIdentity(r.Context(), nil, nil, "/")
	h.renderLogin(w, r.WithContext(ctx), status, "/", "Sign-in could not be completed.")
}

func redirectLocal(w http.ResponseWriter, returnTo string) {
	w.Header().Set("Location", safeReturnTo(returnTo))
	w.WriteHeader(http.StatusSeeOther)
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, status int, returnTo, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.SiteLogin(pages.SiteLoginProps{ReturnTo: returnTo, Message: message, Available: status == http.StatusOK}).Render(r.Context(), w); err != nil {
		h.Logger.Error("site sign-in page render failed", slog.Any("error", err))
	}
}

func randomURLSafe(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// safeReturnTo accepts a path on this site and discards redirect targets with another origin.
func safeReturnTo(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsAny(raw, "\\\r\n\x00") || strings.Contains(raw, "#") {
		return "/"
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" {
		return "/"
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || strings.HasPrefix(decodedPath, "//") || strings.ContainsAny(decodedPath, "\\\r\n\x00") {
		return "/"
	}
	return parsed.RequestURI()
}
