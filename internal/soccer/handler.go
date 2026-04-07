package soccer

import (
	"context"
	"errors"
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
	ErrSessionExpired        = errors.New("session expired")
	ErrPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	ErrInvalidTeamSelection  = errors.New("one or more team IDs were invalid")
	ErrScheduleSelection     = errors.New("at least one team ID or discovered player is required")
)

type Handler struct {
	Config       *config.Config
	LPSClient    *http.Client
	LoginLimiter *session.LoginRateLimiter

	googleHooks GoogleHooks
}

func NewHandler(cfg *config.Config, lpsClient *http.Client, loginLimiter *session.LoginRateLimiter, googleHooks GoogleHooks) *Handler {
	return &Handler{
		Config:       cfg,
		LPSClient:    lpsClient,
		LoginLimiter: loginLimiter,
		googleHooks:  googleHooks,
	}
}
