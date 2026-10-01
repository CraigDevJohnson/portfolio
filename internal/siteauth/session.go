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
	// IssuedAt is when sign-in created this session. State the session
	// authorizes, such as imported LPS access, records it, so an explicit
	// sign-out after it can end that state. Sessions issued before issue
	// times were recorded hold zero.
	IssuedAt  time.Time `json:"issued_at,omitzero"`
	ExpiresAt time.Time `json:"expires_at"`
}

// signOutRecord is this browser's latest explicit sign-out. Only sign-out
// writes it; it outlives the site session so that a response still in flight
// at sign-out cannot bring back state the sign-out ended.
type signOutRecord struct {
	SignedOutAt time.Time `json:"signed_out_at"`
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

// recordSignOut stores the time of this explicit sign-out in its own cookie,
// replacing any earlier record.
func (h *Handler) recordSignOut(w http.ResponseWriter, r *http.Request, signedOutAt time.Time) error {
	encrypted, err := session.EncryptJSONValue(h.Config.SiteSessionKey, &signOutRecord{SignedOutAt: signedOutAt})
	if err != nil {
		return err
	}
	cookie := httpx.NewSecureCookie(r, config.SiteSignOutCookieName, encrypted, config.SiteCookiePath, int(config.SiteSignOutTTL.Seconds()), http.SameSiteLaxMode) //nolint:gosec // Secure is request-aware for registered local loopback development.
	cookie.Expires = signedOutAt.Add(config.SiteSignOutTTL)
	http.SetCookie(w, cookie)
	return nil
}

// lastSignOut returns when this browser last signed out explicitly, or zero
// when it holds no readable record. A record the site session key cannot
// read, such as one written before the key was rotated, counts as none.
func (h *Handler) lastSignOut(r *http.Request) time.Time {
	cookie, err := r.Cookie(config.SiteSignOutCookieName)
	if err != nil {
		return time.Time{}
	}
	var record signOutRecord
	if err := session.DecryptJSONValue(h.Config.SiteSessionKey, cookie.Value, &record); err != nil {
		return time.Time{}
	}
	return record.SignedOutAt
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
