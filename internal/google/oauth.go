package google

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	internalhttpx "portfolio/internal/httpx"
	internalsession "portfolio/internal/session"
	"portfolio/types"
)

const (
	OAuthAuthURL       = "https://accounts.google.com/o/oauth2/auth"
	OAuthTokenURL      = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth endpoint URL, not a credential
	CalendarAPIBaseURL = "https://www.googleapis.com/calendar/v3"
)

// SoccerBridge exposes soccer-domain operations that Google handlers need.
type SoccerBridge interface {
	LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool)
	LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps
	RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData)
	RenderLoginFeedback(w http.ResponseWriter, kind, message string)
	RequestedScheduleGames(ctx context.Context, session *types.SessionData, playerIDs []int, teamCodes string) ([]types.Game, error)
	SelectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game
	GoogleAddScheduleErrorMessage(err error) string
	ParseSelectedIDs(form url.Values) map[string]struct{}
	ParsePlayerIDs(values []string) []int
}

type OAuthState struct {
	ConnectionID string    `json:"connection_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	State        string    `json:"state"`
}

type Handler struct {
	Config             *config.Config
	OAuthAuthURL       string
	OAuthTokenURL      string
	CalendarAPIBaseURL string
	LPSClient          *http.Client
	Soccer             SoccerBridge

	storeMu sync.RWMutex
	store   ConnectionStore
}

// NewHandler constructs a Handler with the given dependencies.
func NewHandler(cfg *config.Config, lpsClient *http.Client, soccer SoccerBridge) *Handler {
	return &Handler{
		Config:             cfg,
		OAuthAuthURL:       OAuthAuthURL,
		OAuthTokenURL:      OAuthTokenURL,
		CalendarAPIBaseURL: CalendarAPIBaseURL,
		LPSClient:          lpsClient,
		Soccer:             soccer,
		store:              NoopStore{},
	}
}

// Store returns the current connection store.
func (h *Handler) Store() ConnectionStore {
	h.storeMu.RLock()
	defer h.storeMu.RUnlock()
	return h.store
}

// SetStore replaces the connection store (thread-safe, called after background init).
func (h *Handler) SetStore(store ConnectionStore) {
	h.storeMu.Lock()
	h.store = store
	h.storeMu.Unlock()
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.Config.GoogleEnabled() {
		RedirectSoccerWithGoogleStatus(w, r, "unavailable")
		return
	}
	connectionID := GetConnectionID(r)
	if connectionID == "" {
		var err error
		connectionID, err = NewRandomHex(16)
		if err != nil {
			log.Printf("google connection id generation failed: %v", err)
			RedirectSoccerWithGoogleStatus(w, r, "failed")
			return
		}
	}
	state, err := NewOAuthState(connectionID)
	if err != nil {
		log.Printf("google oauth state generation failed: %v", err)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	if err := h.SetOAuthStateCookie(w, r, state); err != nil {
		log.Printf("google oauth state cookie write failed: %v", err)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
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
		log.Printf("google token exchange failed: %v", err)
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	calendars, err := h.listCalendarsWithToken(ctx, token)
	if err != nil || len(calendars) == 0 {
		log.Printf("google calendar list after connect failed: %v", err)
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	selectedCalendarID, selectedCalendarSummary := preferredCalendar(calendars)
	encryptedToken, err := h.EncryptToken(token)
	if err != nil {
		log.Printf("google token encryption failed: %v", err)
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
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
		log.Printf("google connection save failed: %v", err)
		ClearOAuthStateCookie(w, r)
		RedirectSoccerWithGoogleStatus(w, r, "failed")
		return
	}
	SetConnectionCookie(w, r, state.ConnectionID)
	ClearOAuthStateCookie(w, r)
	RedirectSoccerWithGoogleStatus(w, r, "connected")
}

// DisconnectHandler removes the Google connection.
func (h *Handler) DisconnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, _ := h.Soccer.LoadSession(w, r)
	h.DeleteConnection(r.Context(), w, r)
	h.Soccer.RenderLoginState(w, r, session)
}

// RenderDisconnectFeedback removes the Google connection and renders status UI.
func (h *Handler) RenderDisconnectFeedback(w http.ResponseWriter, r *http.Request, session *types.SessionData, message string) {
	h.DeleteConnection(r.Context(), w, r)
	if session != nil {
		if err := partials.SoccerLoginState(h.Soccer.LoginStateProps(w, r, session, true)).Render(context.Background(), w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	h.Soccer.RenderLoginFeedback(w, "error", message)
}

// SyncCalendarSelection ensures the connection record has a valid calendar selection.
func (h *Handler) SyncCalendarSelection(ctx context.Context, record *ConnectionRecord, calendars []types.GoogleCalendarOption) (calendarID, summary string) {
	calendarID = strings.TrimSpace(record.CalendarID)
	if calendarID == "" {
		calendarID, summary = preferredCalendar(calendars)
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			log.Printf("google connection default calendar save failed: %v", err)
		}
		return calendarID, summary
	}
	summary = calendarSummary(calendars, calendarID)
	if summary == "" {
		calendarID, summary = preferredCalendar(calendars)
	}
	if summary != "" && (record.CalendarID != calendarID || record.CalendarSummary != summary) {
		record.CalendarID = calendarID
		record.CalendarSummary = summary
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			log.Printf("google connection calendar sync failed: %v", err)
		}
	}
	return calendarID, summary
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

// EncryptToken encrypts an OAuth token for storage.
func (h *Handler) EncryptToken(token *oauth2.Token) (string, error) {
	return h.encryptJSONValue(token)
}

// DecryptToken decrypts a stored OAuth token ciphertext.
func (h *Handler) DecryptToken(ciphertext string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := h.decryptJSONValue(ciphertext, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// LoadConnectionRecord loads the Google connection for the current request.
func (h *Handler) LoadConnectionRecord(ctx context.Context, r *http.Request) (*ConnectionRecord, error) {
	connectionID := GetConnectionID(r)
	if connectionID == "" {
		return nil, nil
	}
	return h.Store().Get(ctx, connectionID)
}

// DeleteConnection removes the Google connection and clears the cookie.
func (h *Handler) DeleteConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	connectionID := GetConnectionID(r)
	if connectionID != "" {
		if err := h.Store().Delete(ctx, connectionID); err != nil {
			log.Printf("google connection delete failed: %v", err)
		}
	}
	ClearConnectionCookie(w, r)
}

// CurrentToken retrieves and refreshes the stored OAuth token.
func (h *Handler) CurrentToken(ctx context.Context, r *http.Request, record *ConnectionRecord) (*oauth2.Token, error) {
	storedToken, err := h.DecryptToken(record.TokenCiphertext)
	if err != nil {
		return nil, err
	}
	tokenSource := h.oauthConfigForRequest(r).TokenSource(h.httpContext(ctx), storedToken)
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = storedToken.RefreshToken
	}
	if token.AccessToken != storedToken.AccessToken || token.RefreshToken != storedToken.RefreshToken || !token.Expiry.Equal(storedToken.Expiry) {
		encryptedToken, encryptErr := h.EncryptToken(token)
		if encryptErr != nil {
			return nil, encryptErr
		}
		record.TokenCiphertext = encryptedToken
		record.UpdatedAt = time.Now().UTC()
		if err := h.Store().Put(ctx, record); err != nil {
			return nil, err
		}
	}
	return token, nil
}

// ListCalendars retrieves writable calendars for a connection.
func (h *Handler) ListCalendars(ctx context.Context, r *http.Request, record *ConnectionRecord) ([]types.GoogleCalendarOption, error) {
	token, err := h.CurrentToken(ctx, r, record)
	if err != nil {
		return nil, err
	}
	return h.listCalendarsWithToken(h.httpContext(ctx), token)
}

// Connected returns true if a valid Google connection exists for the request.
func (h *Handler) Connected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if !h.Config.GoogleEnabled() {
		return false
	}
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		ClearConnectionCookie(w, r)
		return false
	}
	return record != nil
}

// PopulateLoginState fills Google-related fields on login state props.
func (h *Handler) PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps) {
	if !props.GoogleAvailable {
		return
	}
	record, err := h.LoadConnectionRecord(ctx, r)
	if err != nil {
		log.Printf("google connection read failed: %v", err)
		ClearConnectionCookie(w, r)
		return
	}
	if record == nil {
		return
	}
	calendars, err := h.ListCalendars(ctx, r, record)
	if err != nil {
		log.Printf("google calendar list failed: %v", err)
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			h.DeleteConnection(ctx, w, r)
		}
		return
	}
	props.GoogleConnected = true
	props.GoogleCalendars = calendars
	props.SelectedGoogleCalendarID, props.GoogleCalendarSummary = h.SyncCalendarSelection(ctx, record, calendars)
}

func (h *Handler) encryptJSONValue(data any) (string, error) {
	return internalsession.EncryptJSONValue(h.Config.SessionKey, data)
}

func (h *Handler) decryptJSONValue(value string, out any) error {
	return internalsession.DecryptJSONValue(h.Config.SessionKey, value, out)
}

func (h *Handler) oauthConfigForRequest(r *http.Request) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.Config.GoogleClientID,
		ClientSecret: h.Config.GoogleClientSecret,
		RedirectURL:  internalhttpx.RequestBaseURL(r) + "/soccer",
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar.events",
			"https://www.googleapis.com/auth/calendar.calendarlist.readonly",
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  h.OAuthAuthURL,
			TokenURL: h.OAuthTokenURL,
		},
	}
}

func (h *Handler) httpContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, h.LPSClient)
}
