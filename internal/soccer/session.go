package soccer

import (
	"errors"
	"log"
	"net/http"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/httpx"
	internalsession "portfolio/internal/session"
	"portfolio/types"
)

func (h *Handler) getSession(r *http.Request) (*types.SessionData, error) {
	cookie, err := r.Cookie(config.LPSSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session types.SessionData
	err = internalsession.DecryptJSONValue(h.Config.SessionKey, cookie.Value, &session)
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}
	return &session, nil
}

// LoadSession returns the decrypted soccer session and clears invalid or expired cookies.
func (h *Handler) LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool) {
	session, err := h.getSession(r)
	if errors.Is(err, ErrSessionExpired) {
		h.clearSession(w, r)
		return nil, true
	}
	if err != nil {
		log.Printf("soccer session read failed: %v", err)
		h.clearSession(w, r)
		return nil, true
	}
	return session, false
}

func (h *Handler) setSession(w http.ResponseWriter, r *http.Request, session *types.SessionData) error {
	encrypted, err := internalsession.EncryptJSONValue(h.Config.SessionKey, session)
	if err != nil {
		return err
	}
	http.SetCookie(w, httpx.NewSecureCookie(r, config.LPSSessionCookieName, encrypted, config.SoccerCookiePath, 0, http.SameSiteStrictMode))
	return nil
}

func (h *Handler) clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := httpx.NewSecureCookie(r, config.LPSSessionCookieName, "", config.SoccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
