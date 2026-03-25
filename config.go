// Server configuration, environment parsing, constants, and package-level state.
package main

import (
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"portfolio/types"
)

const careerStartYear = 2012

const (
	defaultLPSAPIBaseURL       = "https://lps-api-prod.lps-test.com"
	lpsSessionCookieName       = "lps_session"
	googleConnectionCookieName = "google_connection"
	googleOAuthStateCookieName = "google_oauth_state"
	defaultSessionTTL          = 12 * time.Hour
	googleConnectionCookieTTL  = 180 * 24 * time.Hour
	googleOAuthStateTTL        = 10 * time.Minute
	mountainTimeZoneID         = "America/Denver"
)

const rateLimiterMaxKeys = 10000

const (
	maxRequestBodySize     = 1 << 20
	maxLPSResponseBodySize = 2 << 20
	defaultGameDuration    = 45 * time.Minute
	soccerCookiePath       = "/soccer"
)

// Type aliases retained for test compatibility (main_test.go references these names).
type (
	Game        = types.Game
	LPSPlayer   = types.LPSPlayer
	SessionData = types.SessionData
)

type serverConfig struct {
	SessionKey                []byte
	LPSAPIBaseURL             string
	GoogleClientID            string
	GoogleClientSecret        string
	GoogleConnectionTableName string
}

var (
	configData               = loadServerConfig()
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

func loadServerConfig() serverConfig {
	config := serverConfig{
		LPSAPIBaseURL:             strings.TrimSpace(os.Getenv("LPS_API_BASE_URL")),
		GoogleClientID:            strings.TrimSpace(os.Getenv("CLIENT_ID_KEY")),
		GoogleClientSecret:        strings.TrimSpace(os.Getenv("CLIENT_SECRET_KEY")),
		GoogleConnectionTableName: strings.TrimSpace(os.Getenv("GOOGLE_CONNECTION_TABLE_NAME")),
	}
	if config.LPSAPIBaseURL == "" {
		config.LPSAPIBaseURL = defaultLPSAPIBaseURL
	}
	validatedLPSAPIBaseURL, err := normalizeLPSAPIBaseURL(config.LPSAPIBaseURL)
	if err != nil {
		log.Printf("invalid LPS_API_BASE_URL; using default %q", defaultLPSAPIBaseURL)
		config.LPSAPIBaseURL = defaultLPSAPIBaseURL
	} else {
		config.LPSAPIBaseURL = validatedLPSAPIBaseURL
	}
	if (config.GoogleClientID != "" || config.GoogleClientSecret != "" || config.GoogleConnectionTableName != "") &&
		(config.GoogleClientID == "" || config.GoogleClientSecret == "" || config.GoogleConnectionTableName == "") {
		log.Printf("google calendar add disabled: CLIENT_ID_KEY, CLIENT_SECRET_KEY, and GOOGLE_CONNECTION_TABLE_NAME must all be configured")
	}

	keyHex := strings.TrimSpace(os.Getenv("LPS_SESSION_KEY"))
	if keyHex == "" {
		log.Printf("soccer auth disabled: LPS_SESSION_KEY is not configured")
		return config
	}

	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != 32 {
		log.Printf("soccer auth disabled: LPS_SESSION_KEY must be a 64-character hex string")
		return config
	}

	config.SessionKey = decoded

	return config
}

func normalizeLPSAPIBaseURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = defaultLPSAPIBaseURL
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", errors.New("LPS API base URL must be absolute")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("LPS API base URL cannot include credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && (parsed.Scheme != "http" || !isLoopbackHost(parsed.Hostname())) {
		return "", errors.New("LPS API base URL must use https, or http on loopback only")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSpace(host), "localhost") {
		return true
	}
	parsedIP := net.ParseIP(strings.TrimSpace(host))
	return parsedIP != nil && parsedIP.IsLoopback()
}

func publicBindEnabled() bool {
	value := strings.TrimSpace(os.Getenv("APP_BIND_ALL"))
	return strings.EqualFold(value, "1") || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func serverListenAddress() string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		if publicBindEnabled() {
			host = "0.0.0.0"
		} else {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, port)
}

func localServerURL(listenAddress string) string {
	_, port, err := net.SplitHostPort(listenAddress)
	if err != nil || port == "" {
		return "http://localhost:8080"
	}
	return "http://localhost:" + port
}

func loginEnabled() bool {
	return len(configData.SessionKey) == 32
}

func googleEnabled() bool {
	return loginEnabled() &&
		strings.TrimSpace(configData.GoogleClientID) != "" &&
		strings.TrimSpace(configData.GoogleClientSecret) != "" &&
		strings.TrimSpace(configData.GoogleConnectionTableName) != ""
}
