// Exported constants shared across domain packages.
package config

import "time"

const CareerStartYear = 2012

const (
	DefaultLPSAPIBaseURL       = "https://lps-api-prod.lps-test.com"
	LPSSessionCookieName       = "lps_session"
	GoogleConnectionCookieName = "google_connection"
	GoogleOAuthStateCookieName = "google_oauth_state"
	DefaultSessionTTL          = 12 * time.Hour
	GoogleConnectionCookieTTL  = 180 * 24 * time.Hour
	GoogleOAuthStateTTL        = 10 * time.Minute
	MountainTimeZoneID         = "America/Denver"
)

const RateLimiterMaxKeys = 10000

const (
	MaxRequestBodySize     = 1 << 20
	MaxLPSResponseBodySize = 2 << 20
	DefaultGameDuration    = 45 * time.Minute
	SoccerCookiePath       = "/soccer"
)
