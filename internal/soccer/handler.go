package soccer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/session"
	"portfolio/internal/soccerarchive"
)

// GoogleHooks exposes the Google integration points wired from internal/app.
type GoogleHooks interface {
	GoogleAvailable() bool
	GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool
	PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps)
}

var (
	// ErrSessionExpired reports that the imported LPS session is no longer valid.
	ErrSessionExpired = errors.New("session expired")
	// ErrSessionOwnerMismatch reports that imported access belongs to another or unknown site owner.
	ErrSessionOwnerMismatch = errors.New("soccer session owner does not match the site session")
	// errSessionWithheld reports the owner's retained imported access while the
	// owner is signed out or lacks the current soccer grant. It is kept, not cleared.
	errSessionWithheld = errors.New("imported LPS access is withheld until its owner signs in with the soccer grant")
	// errImportGuardMismatch reports imported access whose guard cookie is
	// missing or different: sign-out or Clear import removed it, and a
	// response already in flight wrote the payload back.
	errImportGuardMismatch = errors.New("imported LPS access does not match this browser's import guard")
	// ErrPlayerSessionRequired reports that discovered-player operations need an imported session.
	ErrPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	// ErrInvalidTeamSelection reports that one or more manual team IDs were invalid.
	ErrInvalidTeamSelection = errors.New("one or more team IDs were invalid")
	// ErrScheduleSelection reports that no valid schedule selection was provided.
	ErrScheduleSelection = errors.New("at least one team ID or discovered player is required")
)

const (
	htmlContentType       = "text/html; charset=utf-8"
	invalidPlayersMessage = "One or more selected players were invalid."
	invalidPlayersHint    = "Clear the imported players and import again to refresh the discovered player list."
	invalidTeamIDsMessage = "One or more team IDs were invalid."
	invalidTeamIDsHint    = "Enter numeric Let's Play Soccer team IDs separated by commas."
	manualLookupRetryHint = "Let's Play Soccer may be unavailable. Try again in a moment."
)

// Handler owns the soccer auth and schedule handlers.
type Handler struct {
	Config       *config.Config
	LPSClient    *http.Client
	LoginLimiter *session.LoginRateLimiter
	Logger       *slog.Logger

	storeMu      sync.RWMutex
	store        SoccerStore
	archiveStore soccerarchive.Store
	googleHooks  GoogleHooks
}

// NewHandler constructs a soccer handler with its runtime dependencies.
func NewHandler(cfg *config.Config, lpsClient *http.Client, loginLimiter *session.LoginRateLimiter, googleHooks GoogleHooks, store SoccerStore, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "soccer"))
	}
	if store == nil {
		store = NoopSoccerStore{}
	}

	return &Handler{
		Config:       cfg,
		LPSClient:    lpsClient,
		LoginLimiter: loginLimiter,
		Logger:       logger,
		store:        store,
		googleHooks:  googleHooks,
	}
}

// ArchiveStore returns the optional durable team archive (thread-safe).
func (h *Handler) ArchiveStore() soccerarchive.Store {
	h.storeMu.RLock()
	defer h.storeMu.RUnlock()
	return h.archiveStore
}

// SetArchiveStore enables offline manual-team archiving for this handler.
// Production does not wire this until collection activation is approved.
func (h *Handler) SetArchiveStore(store soccerarchive.Store) {
	h.storeMu.Lock()
	h.archiveStore = store
	h.storeMu.Unlock()
}

// Store returns the current soccer session store (thread-safe).
func (h *Handler) Store() SoccerStore {
	h.storeMu.RLock()
	defer h.storeMu.RUnlock()
	return h.store
}

// SetStore replaces the soccer session store (thread-safe, called after background init).
func (h *Handler) SetStore(store SoccerStore) {
	h.storeMu.Lock()
	h.store = store
	h.storeMu.Unlock()
}

func (h *Handler) setHTMLContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", htmlContentType)
}

func (h *Handler) googleAvailable() bool {
	if h.googleHooks != nil {
		return h.googleHooks.GoogleAvailable()
	}
	return h.Config != nil && h.Config.GoogleEnabled()
}
