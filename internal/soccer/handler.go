package soccer

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/session"
)

// GoogleHooks exposes temporary Google integration points wired from internal/app.
type GoogleHooks interface {
	GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool
	PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps)
}

var (
	// ErrSessionExpired reports that the imported LPS session is no longer valid.
	ErrSessionExpired = errors.New("session expired")
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
)

// Handler owns the soccer auth and schedule handlers.
type Handler struct {
	Config       *config.Config
	LPSClient    *http.Client
	LoginLimiter *session.LoginRateLimiter
	Logger       *slog.Logger

	googleHooks GoogleHooks
}

// NewHandler constructs a soccer handler with its runtime dependencies.
func NewHandler(cfg *config.Config, lpsClient *http.Client, loginLimiter *session.LoginRateLimiter, googleHooks GoogleHooks, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "soccer"))
	}

	return &Handler{
		Config:       cfg,
		LPSClient:    lpsClient,
		LoginLimiter: loginLimiter,
		Logger:       logger,
		googleHooks:  googleHooks,
	}
}

func (h *Handler) setHTMLContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", htmlContentType)
}
