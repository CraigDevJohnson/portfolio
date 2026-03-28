// Package-level state, type aliases, and sentinel errors retained temporarily.
// Config struct, loading, constants, and feature toggles live in internal/config.
package app

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"portfolio/internal/config"
	"portfolio/types"
)

// Type aliases retained for test compatibility.
type (
	Game        = types.Game
	LPSPlayer   = types.LPSPlayer
	SessionData = types.SessionData
)

// serverConfig is a temporary alias so tests can construct config values
// without importing internal/config directly (migrated in later tasks).
type serverConfig = config.Config

var (
	configData               = config.Load()
	lpsHTTPClient            = &http.Client{Timeout: 15 * time.Second}
	mountainTimeLocation     = loadMountainTimeLocation()
	soccerLoginAttempts      = newLoginRateLimiter(5, time.Minute)
	errSessionExpired        = errors.New("session expired")
	errPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	errInvalidTeamSelection  = errors.New("one or more team IDs were invalid")
	errScheduleSelection     = errors.New("at least one team ID or discovered player is required")
	googleConnectionsMu      sync.RWMutex
	googleConnections        googleConnectionStore = noopGoogleConnectionStore{}
	googleOAuthAuthURL                             = "https://accounts.google.com/o/oauth2/auth"
	googleOAuthTokenURL                            = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth endpoint URL, not a credential
	googleCalendarAPIBaseURL                       = "https://www.googleapis.com/calendar/v3"
)

func normalizeLPSAPIBaseURL(raw string) (string, error) {
	return config.NormalizeLPSAPIBaseURL(raw)
}

func loginEnabled() bool {
	return configData.LoginEnabled()
}

func googleEnabled() bool {
	return configData.GoogleEnabled()
}
