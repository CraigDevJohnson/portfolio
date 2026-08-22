package google

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

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
	RenderLoginStateOOB(w http.ResponseWriter, r *http.Request, session *types.SessionData)
	RenderLoginStateRefresh(w http.ResponseWriter, r *http.Request, session *types.SessionData)
	RenderLoginFeedback(w http.ResponseWriter, r *http.Request, kind, message string)
	ResolveGoogleAddSelection(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool)
	ResolveSyncResultsGames(w http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool)
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
	Logger             *slog.Logger
	Soccer             SoccerBridge

	storeMu    sync.RWMutex
	store      ConnectionStore
	storeReady bool
}

// NewHandler constructs a Handler with the given dependencies.
func NewHandler(cfg *config.Config, lpsClient *http.Client, logger *slog.Logger, soccer SoccerBridge) *Handler {
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "google"))
	}

	return &Handler{
		Config:             cfg,
		OAuthAuthURL:       OAuthAuthURL,
		OAuthTokenURL:      OAuthTokenURL,
		CalendarAPIBaseURL: CalendarAPIBaseURL,
		LPSClient:          lpsClient,
		Logger:             logger,
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

// GoogleAvailable reports whether configuration and durable connection storage
// are both ready for Google Calendar operations.
func (h *Handler) GoogleAvailable() bool {
	if h == nil || h.Config == nil || !h.Config.GoogleEnabled() {
		return false
	}
	h.storeMu.RLock()
	defer h.storeMu.RUnlock()
	return h.storeReady
}

// SetStore replaces the connection store (thread-safe, called after background init).
func (h *Handler) SetStore(store ConnectionStore) {
	h.storeMu.Lock()
	if store == nil {
		store = NoopStore{}
	}
	h.store = store
	_, noopValue := store.(NoopStore)
	_, noopPointer := store.(*NoopStore)
	h.storeReady = !noopValue && !noopPointer
	h.storeMu.Unlock()
}
