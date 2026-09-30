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

const (
	RateLimiterMaxKeys    = 10000
	sessionKeyLengthBytes = 32
)

const (
	MaxRequestBodySize     = 1 << 20
	MaxLPSResponseBodySize = 2 << 20
	DefaultGameDuration    = 45 * time.Minute
	SoccerCookiePath       = "/soccer"
)

const (
	SiteSessionCookieName    = "site_session"
	SiteOAuthStateCookieName = "site_oauth_state"
	SiteCookiePath           = "/"
	// SiteSessionTTL bounds how long a copied session cookie survives sign-out;
	// the session carries no server-side revocation state.
	SiteSessionTTL    = time.Hour
	SiteOAuthStateTTL = 10 * time.Minute
)

// DefaultPortalAWSRegion is the management portal's region when MGMT_AWS_REGION is unset.
const DefaultPortalAWSRegion = "us-east-1"
