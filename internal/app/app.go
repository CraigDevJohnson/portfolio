package app

import (
	"net/http"
	"sync"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/session"
)

// App holds all runtime dependencies, replacing package-level mutable state.
type App struct {
	Config       config.Config
	LPSClient    *http.Client
	LoginLimiter *session.LoginRateLimiter
	MountainTZ   *time.Location

	// Google OAuth/Calendar URLs — defaults come from package constants,
	// but tests can override per-App instance.
	GoogleOAuthAuthURL       string
	GoogleOAuthTokenURL      string
	GoogleCalendarAPIBaseURL string

	googleStoreMu sync.RWMutex
	googleStore   googleConnectionStore
}

// New constructs an App with production defaults for the given config.
func New(cfg config.Config) *App {
	return &App{
		Config:                   cfg,
		LPSClient:                &http.Client{Timeout: 15 * time.Second},
		LoginLimiter:             newLoginRateLimiter(5, time.Minute),
		MountainTZ:               loadMountainTimeLocation(),
		googleStore:              noopGoogleConnectionStore{},
		GoogleOAuthAuthURL:       googleOAuthAuthURL,
		GoogleOAuthTokenURL:      googleOAuthTokenURL,
		GoogleCalendarAPIBaseURL: googleCalendarAPIBaseURL,
	}
}

func (app *App) currentGoogleConnectionStore() googleConnectionStore {
	app.googleStoreMu.RLock()
	defer app.googleStoreMu.RUnlock()
	return app.googleStore
}

func (app *App) setGoogleConnectionStore(store googleConnectionStore) {
	app.googleStoreMu.Lock()
	app.googleStore = store
	app.googleStoreMu.Unlock()
}
