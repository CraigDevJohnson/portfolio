package google

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/types"
)

const (
	OAuthAuthURL       = "https://accounts.google.com/o/oauth2/auth"
	OAuthTokenURL      = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth endpoint URL, not a credential
	CalendarAPIBaseURL = "https://www.googleapis.com/calendar/v3"
)

var ErrOAuthStateExpired = errors.New("oauth state expired")

// SoccerBridge exposes soccer-domain operations that Google handlers need.
type SoccerBridge interface {
	LoadSession(w http.ResponseWriter, r *http.Request) (*types.SessionData, bool)
	LoginStateProps(w http.ResponseWriter, r *http.Request, session *types.SessionData, swapOOB bool) partials.SoccerLoginStateProps
	RenderLoginState(w http.ResponseWriter, r *http.Request, session *types.SessionData)
	RenderLoginFeedback(w http.ResponseWriter, r *http.Request, kind, message string)
	ResolveGoogleAddSelection(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool)
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
