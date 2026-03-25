// Shared cookie builders, HTTPS detection, and base URL resolution.
package main

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

func newSecureCookie(r *http.Request, name, value, path string, maxAge int, sameSite http.SameSite) *http.Cookie { //nolint:unparam // path kept general for reuse outside /soccer
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: sameSite,
	}
}

func getGoogleConnectionID(r *http.Request) string {
	cookie, err := r.Cookie(googleConnectionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setGoogleConnectionCookie(w http.ResponseWriter, r *http.Request, connectionID string) {
	cookie := newSecureCookie(r, googleConnectionCookieName, connectionID, soccerCookiePath, 0, http.SameSiteStrictMode)
	cookie.Expires = time.Now().Add(googleConnectionCookieTTL)
	http.SetCookie(w, cookie)
}

func clearGoogleConnectionCookie(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, googleConnectionCookieName, "", soccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

func setGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request, state googleOAuthState) error {
	encrypted, err := encryptJSONValue(state)
	if err != nil {
		return err
	}
	cookie := newSecureCookie(r, googleOAuthStateCookieName, encrypted, soccerCookiePath, 0, http.SameSiteLaxMode)
	cookie.Expires = state.ExpiresAt
	http.SetCookie(w, cookie)
	return nil
}

func getGoogleOAuthStateCookie(r *http.Request) (*googleOAuthState, error) {
	cookie, err := r.Cookie(googleOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state googleOAuthState
	if err := decryptJSONValue(cookie.Value, &state); err != nil {
		return nil, err
	}
	if time.Now().After(state.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &state, nil
}

func clearGoogleOAuthStateCookie(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, googleOAuthStateCookieName, "", soccerCookiePath, -1, http.SameSiteLaxMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if remoteIP := remoteAddrIP(r.RemoteAddr); remoteIP != nil && isTrustedProxyIP(remoteIP) {
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(r.Host)
}
