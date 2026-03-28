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

func encryptJSONValue(data any) (string, error) {
	return internalsession.EncryptJSONValue(configData.SessionKey, data)
}

func decryptJSONValue(value string, out any) error {
	return internalsession.DecryptJSONValue(configData.SessionKey, value, out)
}

func encryptSession(data *SessionData) (string, error) {
	return internalsession.EncryptSession(configData.SessionKey, data)
}

func decryptSession(value string) (SessionData, error) {
	return internalsession.DecryptSession(configData.SessionKey, value)
}

func getSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(config.LPSSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &session, nil
}

func loadSoccerSession(w http.ResponseWriter, r *http.Request) (*SessionData, bool) {
	session, err := getSession(r)
	if errors.Is(err, errSessionExpired) {
		clearSession(w, r)
		return nil, true
	}
	if err != nil {
		log.Printf("soccer session read failed: %v", err)
		clearSession(w, r)
		return nil, true
	}
	return session, false
}

func setSession(w http.ResponseWriter, r *http.Request, session *SessionData) error {
	encrypted, err := encryptSession(session)
	if err != nil {
		return err
	}
	http.SetCookie(w, newSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteStrictMode))
	return nil
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, config.LPSSessionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
