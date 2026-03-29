package app

import (
	"net/http"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/schedule"
	"portfolio/internal/session"
)

// App holds all runtime dependencies, replacing package-level mutable state.
type App struct {
	Config        config.Config
	LPSClient     *http.Client
	LoginLimiter  *session.LoginRateLimiter
	MountainTZ    *time.Location
	GoogleHandler *internalgoogle.Handler
}

// New constructs an App with production defaults for the given config.
func New(cfg *config.Config) *App {
	app := &App{
		Config:       *cfg,
		LPSClient:    &http.Client{Timeout: 15 * time.Second},
		LoginLimiter: newLoginRateLimiter(5, time.Minute),
		MountainTZ:   schedule.MountainTimeLocation,
	}
	// GoogleHandler is constructed with a nil SoccerBridge initially;
	// it gets wired in Run() after the soccer handler is created.
	app.GoogleHandler = internalgoogle.NewHandler(&app.Config, app.LPSClient, nil)
	return app
}
