// Type aliases and Google API constants retained in internal/app.
// Config struct, loading, and feature toggles live in internal/config.
// Runtime state lives on the App struct (app.go).
package app

import (
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

const (
	googleOAuthAuthURL       = "https://accounts.google.com/o/oauth2/auth"
	googleOAuthTokenURL      = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public OAuth endpoint URL, not a credential
	googleCalendarAPIBaseURL = "https://www.googleapis.com/calendar/v3"
)
