package soccer

import (
	"context"
	"net/http"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/session"
)

// GoogleHooks exposes temporary Google integration points wired from internal/app.
type GoogleHooks interface {
	GoogleConnected(ctx context.Context, w http.ResponseWriter, r *http.Request) bool
	HandlePageCallback(w http.ResponseWriter, r *http.Request) bool
	PopulateLoginState(ctx context.Context, w http.ResponseWriter, r *http.Request, props *partials.SoccerLoginStateProps)
}

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
