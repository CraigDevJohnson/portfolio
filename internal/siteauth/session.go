package siteauth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/session"
	"portfolio/internal/siteidentity"
)

type siteSession struct {
	Principal siteidentity.Principal `json:"principal"`
	ExpiresAt time.Time              `json:"expires_at"`
}

type oauthState struct {
	State        string    `json:"state"`
	CodeVerifier string    `json:"code_verifier"`
	ReturnTo     string    `json:"return_to"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, value *siteSession) error {
	encrypted, err := session.EncryptJSONValue(h.Config.SiteSessionKey, value)
	if err != nil {
		return err
	}
	maxAge := int(time.Until(value.ExpiresAt).Seconds())
	if maxAge < 1 {
		return errors.New("site session already expired")
	}
	http.SetCookie(w, httpx.NewSecureCookie(r, config.SiteSessionCookieName, encrypted, config.SiteCookiePath, maxAge, http.SameSiteLaxMode))
	return nil
}

func (h *Handler) loadSession(r *http.Request) (*siteSession, error) {
	cookie, err := r.Cookie(config.SiteSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value siteSession
	if err := session.DecryptJSONValue(h.Config.SiteSessionKey, cookie.Value, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (h *Handler) validSession(value *siteSession) bool {
	if value == nil || !time.Now().Before(value.ExpiresAt) || value.Principal.Issuer != h.Config.SiteCognitoIssuer || strings.TrimSpace(value.Principal.Subject) == "" {
		return false
	}
	email, err := config.NormalizePortalEmail(value.Principal.Email)
	return err == nil && email == value.Principal.Email && h.Config.SiteEmailInvited(email)
}

func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r, config.SiteSessionCookieName, http.SameSiteLaxMode)
}

func (h *Handler) setOAuthState(w http.ResponseWriter, r *http.Request, value *oauthState) error {
	encrypted, err := session.EncryptJSONValue(h.Config.SiteSessionKey, value)
	if err != nil {
		return err
	}
	http.SetCookie(w, httpx.NewSecureCookie(r, config.SiteOAuthStateCookieName, encrypted, config.SiteCookiePath, int(config.SiteOAuthStateTTL.Seconds()), http.SameSiteLaxMode))
	return nil
}

func (h *Handler) loadOAuthState(r *http.Request) (*oauthState, error) {
	cookie, err := r.Cookie(config.SiteOAuthStateCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value oauthState
	if err := session.DecryptJSONValue(h.Config.SiteSessionKey, cookie.Value, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (h *Handler) clearOAuthState(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, r, config.SiteOAuthStateCookieName, http.SameSiteLaxMode)
}

func clearCookie(w http.ResponseWriter, r *http.Request, name string, sameSite http.SameSite) {
	cookie := httpx.NewSecureCookie(r, name, "", config.SiteCookiePath, -1, sameSite) //nolint:gosec // Secure is request-aware for registered local loopback development.
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
