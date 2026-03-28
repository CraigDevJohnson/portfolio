// Session management: AES-GCM encryption, session cookies, and rate limiting.
package app

import (
	"errors"
	"log"
	"net/http"
	"time"

	"portfolio/internal/config"
	internalsession "portfolio/internal/session"
)

type loginAttempt = internalsession.LoginAttempt

type loginRateLimiter = internalsession.LoginRateLimiter

func newLoginRateLimiter(maxAttempts int, window time.Duration) *loginRateLimiter {
	return internalsession.NewLoginRateLimiter(maxAttempts, window, config.RateLimiterMaxKeys)
}

func (app *App) encryptJSONValue(data any) (string, error) {
	return internalsession.EncryptJSONValue(app.Config.SessionKey, data)
}

func (app *App) decryptJSONValue(value string, out any) error {
	return internalsession.DecryptJSONValue(app.Config.SessionKey, value, out)
}

func (app *App) encryptSession(data *SessionData) (string, error) {
	return internalsession.EncryptSession(app.Config.SessionKey, data)
}

func (app *App) decryptSession(value string) (SessionData, error) {
	return internalsession.DecryptSession(app.Config.SessionKey, value)
}

func (app *App) getSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(config.LPSSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := app.decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &session, nil
}

func (app *App) loadSoccerSession(w http.ResponseWriter, r *http.Request) (*SessionData, bool) {
	session, err := app.getSession(r)
	if errors.Is(err, errSessionExpired) {
		app.clearSession(w, r)
		return nil, true
	}
	if err != nil {
		log.Printf("soccer session read failed: %v", err)
		app.clearSession(w, r)
		return nil, true
	}
	return session, false
}

func (app *App) setSession(w http.ResponseWriter, r *http.Request, session *SessionData) error {
	encrypted, err := app.encryptSession(session)
	if err != nil {
		return err
	}
	http.SetCookie(w, newSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteStrictMode))
	return nil
}

func (app *App) clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, config.LPSSessionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
