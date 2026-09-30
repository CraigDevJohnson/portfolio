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
	"path"
	"strings"
	"time"

	"portfolio/cmd/web/pages"
	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/portal"
	"portfolio/internal/siteidentity"
)

// CallbackPath is the registered Cognito redirect path for site sign-in.
const CallbackPath = "/auth/callback"

// reasonNotInvited marks a Cognito identity outside the current invitation map.
const reasonNotInvited = "identity_not_invited"

// Handler owns site sign-in independently of portal AWS client availability.
type Handler struct {
	Config *config.Config
	OIDC   *portal.OIDCClient
	Logger *slog.Logger
	// SignOutHooks let features that keep browser state for the signed-in
	// visitor, such as imported LPS access, end it when the visitor signs out.
	SignOutHooks []func(http.ResponseWriter, *http.Request)
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
		} else if !strings.HasPrefix(r.URL.Path, "/static/") {
			// Responses for a signed-in principal can show account data; shared assets cannot.
			preventStorage(w)
		}
		ctx := siteidentity.WithRequestIdentity(r.Context(), principal, grants, safeReturnTo(r.URL.RequestURI()))
		ctx = siteidentity.WithSignInAvailable(ctx, h.signInAvailable())
		identified := r.WithContext(ctx)
		next.ServeHTTP(w, identified)
		// ServeMux records the matched route on the copy it received; request logging reads the outer request.
		r.Pattern = identified.Pattern
	})
}

// WithCanonicalHost sends requests for the www alias of the registered
// callback host to that host with a 308, which keeps the method and form.
// Site cookies are host-only and Cognito returns only to the registered
// callback, so sign-in and sessions must stay on that one host.
func (h *Handler) WithCanonicalHost(next http.Handler) http.Handler {
	if !h.signInAvailable() {
		return next
	}
	canonical, err := url.Parse(h.Config.SiteCognitoRedirectURI)
	if err != nil || canonical.Host == "" {
		return next
	}
	alias := "www." + canonical.Host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin, err := url.Parse(httpx.RequestBaseURL(r))
		if err != nil || !strings.EqualFold(requestOrigin.Host, alias) {
			next.ServeHTTP(w, r)
			return
		}
		// The scheme and host come from the reviewed callback URL; only the path and query are the request's.
		http.Redirect(w, r, canonical.Scheme+"://"+canonical.Host+r.URL.RequestURI(), http.StatusPermanentRedirect) //nolint:gosec // Fixed canonical host, so the redirect cannot leave this site.
	})
}

// LoginHandler renders a signed-out landing on GET and starts Google sign-in on POST.
func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	preventStorage(w)
	returnTo := safeReturnTo(r.URL.Query().Get("return_to"))
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			h.renderLogin(w, r, http.StatusBadRequest, pages.SiteLoginProps{ReturnTo: "/", Message: "Sign-in could not be started."})
			return
		}
		returnTo = safeReturnTo(r.PostForm.Get("return_to"))
	}
	if _, signedIn := siteidentity.PrincipalFromContext(r.Context()); signedIn {
		redirectLocal(w, returnTo)
		return
	}
	if !h.signInAvailable() {
		h.renderLogin(w, r, http.StatusServiceUnavailable, pages.SiteLoginProps{ReturnTo: returnTo, Message: "Site sign-in is unavailable right now."})
		return
	}
	// Only POST starts sign-in; the GET route also serves HEAD, which must stay side-effect free.
	if r.Method != http.MethodPost {
		h.renderLogin(w, r, http.StatusOK, pages.SiteLoginProps{ReturnTo: returnTo})
		return
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, pages.SiteLoginProps{ReturnTo: returnTo, Message: "Sign-in could not be started."})
		return
	}
	state, err := randomHex(16)
	if err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, pages.SiteLoginProps{ReturnTo: returnTo, Message: "Sign-in could not be started."})
		return
	}
	pending := &oauthState{State: state, CodeVerifier: verifier, ReturnTo: returnTo, ExpiresAt: time.Now().Add(config.SiteOAuthStateTTL)}
	if err := h.setOAuthState(w, r, pending); err != nil {
		h.renderLogin(w, r, http.StatusInternalServerError, pages.SiteLoginProps{ReturnTo: returnTo, Message: "Sign-in could not be started."})
		return
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	http.Redirect(w, r, h.OIDC.AuthorizationURL(state, challenge), http.StatusSeeOther)
}

// CallbackHandler accepts only a signed Cognito identity invited in current configuration.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	preventStorage(w)
	pending, pendingErr := h.loadOAuthState(r)
	h.clearOAuthState(w, r)
	providedState := r.URL.Query().Get("state")
	if pendingErr != nil || pending == nil || pending.State == "" || providedState == "" ||
		!time.Now().Before(pending.ExpiresAt) ||
		subtle.ConstantTimeCompare([]byte(pending.State), []byte(providedState)) != 1 {
		if !h.keepExistingSession(w, r, "invalid_state", "/") {
			h.rejectSignIn(w, r, http.StatusBadRequest, "invalid_state")
		}
		return
	}
	if r.URL.Query().Get("error") != "" {
		if !h.keepExistingSession(w, r, "provider_rejected", pending.ReturnTo) {
			h.rejectSignIn(w, r, http.StatusUnauthorized, "provider_rejected")
		}
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || !h.signInAvailable() {
		if !h.keepExistingSession(w, r, "incomplete_response", pending.ReturnTo) {
			h.rejectSignIn(w, r, http.StatusBadRequest, "incomplete_response")
		}
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
		h.rejectSignIn(w, r, http.StatusUnauthorized, reasonNotInvited)
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
	preventStorage(w)
	h.clearSession(w, r)
	h.clearOAuthState(w, r)
	for _, endFeatureState := range h.SignOutHooks {
		endFeatureState(w, r)
	}
	if h.OIDC != nil {
		if target := h.OIDC.LogoutURL(); target != "" {
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

// keepExistingSession ends a callback that never reached Cognito's token
// exchange without discarding a valid site session, such as one another tab created.
func (h *Handler) keepExistingSession(w http.ResponseWriter, r *http.Request, reason, returnTo string) bool {
	if _, signedIn := siteidentity.PrincipalFromContext(r.Context()); !signedIn {
		return false
	}
	h.Logger.Warn("site sign-in callback ignored for existing session", slog.String("reason", reason))
	redirectLocal(w, returnTo)
	return true
}

func (h *Handler) rejectSignIn(w http.ResponseWriter, r *http.Request, status int, reason string) {
	h.clearSession(w, r)
	h.Logger.Warn("site sign-in rejected", slog.String("reason", reason))
	ctx := siteidentity.WithRequestIdentity(r.Context(), nil, nil, "/")
	h.renderLogin(w, r.WithContext(ctx), status, pages.SiteLoginProps{
		ReturnTo:           "/",
		Message:            "Sign-in could not be completed.",
		OfferAccountSwitch: reason == reasonNotInvited,
	})
}

// preventStorage keeps account state and auth cookies out of browser and shared caches.
func preventStorage(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func redirectLocal(w http.ResponseWriter, returnTo string) {
	w.Header().Set("Location", safeReturnTo(returnTo))
	w.WriteHeader(http.StatusSeeOther)
}

// signInAvailable reports whether this environment can start the Cognito journey.
func (h *Handler) signInAvailable() bool {
	return h.OIDC != nil && h.Config != nil && h.Config.SiteEnabled()
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, status int, props pages.SiteLoginProps) {
	props.Available = h.signInAvailable()
	ctx := siteidentity.WithNavigationReturnTo(r.Context(), props.ReturnTo)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.SiteLogin(props).Render(ctx, w); err != nil {
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
	// Returning to the callback would replay it without state and end the new session.
	if path.Clean(decodedPath) == CallbackPath {
		return "/"
	}
	return parsed.RequestURI()
}
