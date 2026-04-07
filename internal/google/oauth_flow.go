package google

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

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

// RedirectSoccerWithGoogleStatus redirects to /soccer with an optional google= query parameter.
func RedirectSoccerWithGoogleStatus(w http.ResponseWriter, r *http.Request, status string) {
	target := "/soccer"
	if strings.TrimSpace(status) != "" {
		target += "?google=" + url.QueryEscape(status)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// ConnectHandler initiates the Google OAuth flow.
func (h *Handler) ConnectHandler(w http.ResponseWriter, r *http.Request) {
	if !h.Config.GoogleEnabled() {
		RedirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	connectionID := GetConnectionID(r)
	if connectionID == "" {
		var err error
		connectionID, err = NewRandomHex(16)
		if err != nil {
			h.failConnectf(w, r, "google connection id generation failed: %v", err)
			return
		}
	}
	state, err := NewOAuthState(connectionID)
	if err != nil {
		h.failConnectf(w, r, "google oauth state generation failed: %v", err)
		return
	}
	if err := h.SetOAuthStateCookie(w, r, state); err != nil {
		h.failConnectf(w, r, "google oauth state cookie write failed: %v", err)
		return
	}
	authURL := h.oauthConfigForRequest(r).AuthCodeURL(
		state.State,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	http.Redirect(w, r, authURL, http.StatusSeeOther)
}

// CallbackHandler processes the OAuth callback after user consent.
func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	if !h.Config.GoogleEnabled() {
		RedirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "denied")
		return
	}
	state, err := h.GetOAuthStateCookie(r)
	if errors.Is(err, ErrOAuthStateExpired) {
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err != nil || state == nil || state.State == "" || state.State != strings.TrimSpace(r.URL.Query().Get("state")) {
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	ctx := h.httpContext(r.Context())
	token, err := h.oauthConfigForRequest(r).Exchange(ctx, strings.TrimSpace(r.URL.Query().Get("code")))
	if err != nil {
		h.failCallbackf(w, r, "google token exchange failed: %v", err)
		return
	}
	calendars, err := h.listCalendarsWithToken(ctx, token)
	if err != nil || len(calendars) == 0 {
		h.failCallbackf(w, r, "google calendar list after connect failed: %v", err)
		return
	}
	selectedCalendarID, selectedCalendarSummary := preferredCalendar(calendars)
	encryptedToken, err := h.EncryptToken(token)
	if err != nil {
		h.failCallbackf(w, r, "google token encryption failed: %v", err)
		return
	}
	createdAt := time.Now().UTC()
	if existing, err := h.Store().Get(r.Context(), state.ConnectionID); err == nil && existing != nil {
		createdAt = existing.CreatedAt
	}
	record := ConnectionRecord{
		ConnectionID:    state.ConnectionID,
		TokenCiphertext: encryptedToken,
		CalendarID:      selectedCalendarID,
		CalendarSummary: selectedCalendarSummary,
		CreatedAt:       createdAt,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := h.Store().Put(r.Context(), &record); err != nil {
		h.failCallbackf(w, r, "google connection save failed: %v", err)
		return
	}
	SetConnectionCookie(w, r, state.ConnectionID)
	ClearOAuthStateCookie(w, r)
	RedirectSoccerWithGoogleStatus(w, r, "connected")
}

// DisconnectHandler removes the Google connection.
func (h *Handler) DisconnectHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := h.Soccer.LoadSession(w, r)
	h.DeleteConnection(r.Context(), w, r)
	h.Soccer.RenderLoginState(w, r, session)
}

// NewRandomHex generates a random hex string of the given byte length.
func NewRandomHex(byteLength int) (string, error) {
	random := make([]byte, byteLength)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return hex.EncodeToString(random), nil
}

// NewOAuthState creates a new OAuth state value with expiry.
func NewOAuthState(connectionID string) (OAuthState, error) {
	stateValue, err := NewRandomHex(16)
	if err != nil {
		return OAuthState{}, err
	}
	return OAuthState{
		ConnectionID: connectionID,
		ExpiresAt:    time.Now().Add(config.GoogleOAuthStateTTL),
		State:        stateValue,
	}, nil
}

func (h *Handler) failConnectf(w http.ResponseWriter, r *http.Request, format string, args ...any) {
	log.Printf(format, args...)
	RedirectSoccerWithGoogleStatus(w, r, "failed")
}

func (h *Handler) failCallbackf(w http.ResponseWriter, r *http.Request, format string, args ...any) {
	log.Printf(format, args...)
	ClearOAuthStateCookie(w, r)
	RedirectSoccerWithGoogleStatus(w, r, "failed")
}
