// Package soccer contains soccer-domain handlers and helpers.
package soccer

import (
	"context"
	"net/http"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/session"
)

// GoogleHooks exposes the temporary Google integration points that still live in internal/app.
type GoogleHooks interface {
	GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool
	HandlePageCallback(w http.ResponseWriter, r *http.Request) bool
	PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps)
}

// Handler owns soccer HTTP behavior and its runtime dependencies.
type Handler struct {
	Config       *config.Config
	LPSClient    *http.Client
	LoginLimiter *session.LoginRateLimiter
	MountainTZ   *time.Location

	googleHooks GoogleHooks
}

// NewHandler constructs a soccer handler with the provided dependencies.
func NewHandler(cfg *config.Config, lpsClient *http.Client, loginLimiter *session.LoginRateLimiter, mountainTZ *time.Location, googleHooks GoogleHooks) *Handler {
	return &Handler{
		Config:       cfg,
		LPSClient:    lpsClient,
		LoginLimiter: loginLimiter,
		MountainTZ:   mountainTZ,
		googleHooks:  googleHooks,
	}
}