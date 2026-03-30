package google

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
)

func setCookieWithExpiry(w http.ResponseWriter, cookie *http.Cookie, expires time.Time) {
	cookie.Expires = expires
	http.SetCookie(w, cookie)
}

func GetConnectionID(r *http.Request) string {
	cookie, err := r.Cookie(config.GoogleConnectionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func SetConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleConnectionCookieName, connectionID, config.SoccerCookiePath, 0, http.SameSiteStrictMode)
	setCookieWithExpiry(w, cookie, time.Now().Add(config.GoogleConnectionCookieTTL))
}

func ClearConnectionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleConnectionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	setCookieWithExpiry(w, cookie, time.Unix(0, 0))
}

func (h *Handler) SetOAuthStateCookie(w http.ResponseWriter, r *http.Request, state OAuthState) error {
	encrypted, err := h.encryptJSONValue(state)
	if err != nil {
		return err
	}
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleOAuthStateCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteLaxMode)
	setCookieWithExpiry(w, cookie, state.ExpiresAt)
	return nil
}

func (h *Handler) GetOAuthStateCookie(r *http.Request) (*OAuthState, error) {
	cookie, err := r.Cookie(config.GoogleOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
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

func ClearOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	cookie := internalhttpx.NewSecureCookie(r, config.GoogleOAuthStateCookieName, "", config.SoccerCookiePath, -1, http.SameSiteLaxMode)
	setCookieWithExpiry(w, cookie, time.Unix(0, 0))
}
