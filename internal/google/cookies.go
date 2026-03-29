// Google cookie operations for connection and OAuth state cookies.
package google

import (
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
)

// GetConnectionID reads the Google connection ID from the request cookie.
func GetConnectionID(r *http.Request) string {
	cookie, err := r.Cookie(config.GoogleConnectionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

// SetConnectionCookie sets the persistent Google connection cookie.
func SetConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleConnectionCookieName, connectionID, config.SoccerCookiePath, 0, http.SameSiteStrictMode)
	cookie.Expires = time.Now().Add(config.GoogleConnectionCookieTTL)
	http.SetCookie(w, cookie)
}

// ClearConnectionCookie removes the Google connection cookie.
func ClearConnectionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleConnectionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

// SetOAuthStateCookie encrypts and sets the OAuth state cookie.
func (h *Handler) SetOAuthStateCookie(w http.ResponseWriter, r *http.Request, state OAuthState) error {
	encrypted, err := h.encryptJSONValue(state)
	if err != nil {
		return err
	}
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleOAuthStateCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteLaxMode)
	cookie.Expires = state.ExpiresAt
	http.SetCookie(w, cookie)
	return nil
}

// GetOAuthStateCookie reads and decrypts the OAuth state cookie.
func (h *Handler) GetOAuthStateCookie(r *http.Request) (*OAuthState, error) {
	cookie, err := r.Cookie(config.GoogleOAuthStateCookieName)
	if err == http.ErrNoCookie {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state OAuthState
	if err := h.decryptJSONValue(cookie.Value, &state); err != nil {
		return nil, err
	}
	if time.Now().After(state.ExpiresAt) {
		return nil, ErrOAuthStateExpired
	}
	return &state, nil
}

// ClearOAuthStateCookie removes the OAuth state cookie.
func ClearOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleOAuthStateCookieName, "", config.SoccerCookiePath, -1, http.SameSiteLaxMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
