// Package app provides server startup, route wiring, and dependency injection.
package app

import (
	"log/slog"
	"net/http"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/session"
)

// App holds all runtime dependencies, replacing package-level mutable state.
type App struct {
	Config        config.Config
	LPSClient     *http.Client
	LoginLimiter  *session.LoginRateLimiter
	GoogleHandler *internalgoogle.Handler
	Logger        *slog.Logger
}

// New constructs an App with production defaults for the given config.
func New(cfg *config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	app := &App{
		Config:       *cfg,
		LPSClient:    &http.Client{Timeout: 15 * time.Second},
		LoginLimiter: session.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys),
		Logger:       logger.With(slog.String("component", "app")),
	}
	// GoogleHandler starts without a SoccerBridge and is wired after the soccer
	// handler is created.
	app.GoogleHandler = internalgoogle.NewHandler(
		&app.Config,
		app.LPSClient,
		logger.With(slog.String("component", "google")),
		nil,
	)
	return app
}
