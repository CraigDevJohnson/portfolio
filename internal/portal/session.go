package portal

import (
	"errors"
	"net/http"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/httpx"
	internalsession "portfolio/internal/session"
)

// PortalSession is the decrypted payload stored in the mgmt_session cookie.
type PortalSession struct {
	Username  string    `json:"username"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsValid reports whether the session is usable: the username must be non-empty
// and the expiry must be strictly in the future.
func (s *PortalSession) IsValid() bool {
	return s != nil && s.Username != "" && time.Now().Before(s.ExpiresAt)
}

// OAuthState is the payload stored in the mgmt_oauth_state cookie during the
// OAuth 2.0 Authorization Code + PKCE flow.
type OAuthState struct {
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier"`
}

// setSession encrypts s and writes it to the mgmt_session cookie.
func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, s *PortalSession) error {
	encrypted, err := internalsession.EncryptJSONValue(h.Config.PortalSessionKey, s)
	if err != nil {
		return err
	}
	http.SetCookie(w, httpx.NewSecureCookie(
		r,
		config.PortalSessionCookieName,
		encrypted,
		config.PortalCookiePath,
		int(config.PortalSessionTTL.Seconds()),
		http.SameSiteStrictMode,
	))
	return nil
}

// loadSession reads and decrypts the mgmt_session cookie.
// Returns nil, nil when the cookie is absent.
// Returns nil, err when the cookie value cannot be decrypted.
func (h *Handler) loadSession(r *http.Request) (*PortalSession, error) {
	cookie, err := r.Cookie(config.PortalSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s PortalSession
	if err := internalsession.DecryptJSONValue(h.Config.PortalSessionKey, cookie.Value, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// clearSession expires the mgmt_session cookie immediately.
func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := httpx.NewSecureCookie(r, config.PortalSessionCookieName, "", config.PortalCookiePath, -1, http.SameSiteStrictMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

// setOAuthState encrypts state and writes it to the mgmt_oauth_state cookie.
func (h *Handler) setOAuthState(w http.ResponseWriter, r *http.Request, state *OAuthState) error {
	encrypted, err := internalsession.EncryptJSONValue(h.Config.PortalSessionKey, state)
	if err != nil {
		return err
	}
	http.SetCookie(w, httpx.NewSecureCookie(
		r,
		config.PortalOAuthStateCookieName,
		encrypted,
		config.PortalCookiePath,
		int(config.PortalOAuthStateCookieTTL.Seconds()),
		http.SameSiteLaxMode,
	))
	return nil
}

// loadOAuthState reads and decrypts the mgmt_oauth_state cookie.
// Returns nil, nil when the cookie is absent.
// Returns nil, err when the cookie value cannot be decrypted.
func (h *Handler) loadOAuthState(r *http.Request) (*OAuthState, error) {
	cookie, err := r.Cookie(config.PortalOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state OAuthState
	if err := internalsession.DecryptJSONValue(h.Config.PortalSessionKey, cookie.Value, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// clearOAuthState expires the mgmt_oauth_state cookie immediately.
func (h *Handler) clearOAuthState(w http.ResponseWriter, r *http.Request) {
	cookie := httpx.NewSecureCookie(r, config.PortalOAuthStateCookieName, "", config.PortalCookiePath, -1, http.SameSiteLaxMode) //nolint:gosec // Cookie security attributes are set centrally; Secure remains request-aware for local HTTP development.
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
