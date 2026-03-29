// Shared cookie builders, HTTPS detection, and base URL resolution.
package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
)

func newSecureCookie(r *http.Request, name, value, path string, maxAge int, sameSite http.SameSite) *http.Cookie { //nolint:unparam // path kept general for reuse outside /soccer
	return internalhttpx.NewSecureCookie(r, name, value, path, maxAge, sameSite)
}

func getGoogleConnectionID(r *http.Request) string {
	cookie, err := r.Cookie(config.GoogleConnectionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setGoogleConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	cookie := newSecureCookie(r, config.GoogleConnectionCookieName, connectionID, config.SoccerCookiePath, 0, http.SameSiteStrictMode)
	cookie.Expires = time.Now().Add(config.GoogleConnectionCookieTTL)
	http.SetCookie(w, cookie)
}

func clearGoogleConnectionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, config.GoogleConnectionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

func (app *App) setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state googleOAuthState) error {
	encrypted, err := app.encryptJSONValue(state)
	if err != nil {
		return err
	}
	cookie := newSecureCookie(r, config.GoogleOAuthStateCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteLaxMode)
	cookie.Expires = state.ExpiresAt
	http.SetCookie(w, cookie)
	return nil
}

func (app *App) getGoogleOAuthStateCookie(r *http.Request) (*googleOAuthState, error) {
	cookie, err := r.Cookie(config.GoogleOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state googleOAuthState
	if err := app.decryptJSONValue(cookie.Value, &state); err != nil {
		return nil, err
	}
	if time.Now().After(state.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &state, nil
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, config.GoogleOAuthStateCookieName, "", config.SoccerCookiePath, -1, http.SameSiteLaxMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
