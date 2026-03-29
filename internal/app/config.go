// Type aliases retained for test compatibility.
package app

import (
	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/types"
)

// Type aliases retained for test compatibility.
type (
	Game        = types.Game
	LPSPlayer   = types.LPSPlayer
	SessionData = types.SessionData
)

// Google type aliases retained for test compatibility (Task-009 will remove these).
type (
	googleConnectionRecord = internalgoogle.ConnectionRecord
	googleConnectionStore  = internalgoogle.ConnectionStore
	googleOAuthState       = internalgoogle.OAuthState
	googleEvent            = internalgoogle.Event
	googleAPIError         = internalgoogle.APIError
)

// serverConfig is a temporary alias so tests can construct config values
// without importing internal/config directly (migrated in later tasks).
type serverConfig = config.Config
