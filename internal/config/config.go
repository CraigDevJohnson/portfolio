// Package config provides server configuration, environment variable parsing, and feature toggles.
package config

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
)

// Config holds the server's runtime configuration.
type Config struct {
	SessionKey                []byte
	LPSAPIBaseURL             string
	GoogleClientID            string
	GoogleClientSecret        string
	GoogleConnectionTableName string
	SoccerSessionTableName    string

	// Portal fields
	PortalSessionKey         []byte
	PortalCognitoDomain      string
	PortalCognitoClientID    string
	PortalCognitoRedirectURI string
	PortalCognitoLogoutURI   string
	PortalAWSRegion          string
}

// Load reads runtime configuration from the environment.
func Load() Config {
	logger := slog.Default().With(slog.String("component", "config"))
	cfg := Config{
		LPSAPIBaseURL:             envTrimmed("LPS_API_BASE_URL"),
		GoogleClientID:            envTrimmed("CLIENT_ID_KEY"),
		GoogleClientSecret:        envTrimmed("CLIENT_SECRET_KEY"),
		GoogleConnectionTableName: envTrimmed("GOOGLE_CONNECTION_TABLE_NAME"),
		SoccerSessionTableName:    envTrimmed("SOCCER_SESSION_TABLE_NAME"),
	}
	if cfg.LPSAPIBaseURL == "" {
		cfg.LPSAPIBaseURL = DefaultLPSAPIBaseURL
	}
	validatedURL, err := NormalizeLPSAPIBaseURL(cfg.LPSAPIBaseURL)
	if err != nil {
		logger.Warn(
			"invalid LPS API base URL; using default",
			slog.String("configured_url", cfg.LPSAPIBaseURL),
			slog.String("default_url", DefaultLPSAPIBaseURL),
			slog.Any("error", err),
		)
		cfg.LPSAPIBaseURL = DefaultLPSAPIBaseURL
	} else {
		cfg.LPSAPIBaseURL = validatedURL
	}

	keyHex := envTrimmed("LPS_SESSION_KEY")
	if keyHex == "" {
		logger.Info("soccer auth disabled; LPS_SESSION_KEY is not configured")
		loadPortalConfig(logger, &cfg)
		return cfg
	}

	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != sessionKeyLengthBytes {
		logger.Warn("soccer auth disabled; LPS_SESSION_KEY must be a 64-character hex string")
		loadPortalConfig(logger, &cfg)
		return cfg
	}

	cfg.SessionKey = decoded

	loadPortalConfig(logger, &cfg)

	return cfg
}

// loadPortalConfig reads portal-specific env vars and populates cfg.
// It is called from Load() after the soccer session key is resolved.
func loadPortalConfig(logger *slog.Logger, cfg *Config) {
	cfg.PortalAWSRegion = resolvePortalRegion()
	mgmtKeyHex := envTrimmed("MGMT_SESSION_KEY")
	if mgmtKeyHex == "" {
		// Absent or empty — disable portal silently (Req 10.6).
		logger.Info("portal config loaded", slog.Bool("portal_enabled", false), slog.String("aws_region", resolvePortalRegion()))
		return
	}

	decoded, err := hex.DecodeString(mgmtKeyHex)
	if err != nil || len(decoded) != sessionKeyLengthBytes || mgmtKeyHex != strings.ToLower(mgmtKeyHex) {
		// Present but not a valid 64-char hex string — disable + WARN (Req 10.5).
		logger.Warn("portal disabled; MGMT_SESSION_KEY must be a 64-character hex string")
		logger.Info("portal config loaded", slog.Bool("portal_enabled", false), slog.String("aws_region", resolvePortalRegion()))
		return
	}

	cfg.PortalSessionKey = decoded

	cognitoDomain := envTrimmed("MGMT_COGNITO_DOMAIN")
	cognitoClientID := envTrimmed("MGMT_COGNITO_CLIENT_ID")

	validatedDomain, domainErr := NormalizeCognitoDomain(cognitoDomain)
	if cognitoDomain == "" || cognitoClientID == "" || domainErr != nil {
		// Valid key but missing Cognito config — disable + WARN (Req 10.7).
		logger.Warn("portal disabled; MGMT_COGNITO_DOMAIN and MGMT_COGNITO_CLIENT_ID are both required when MGMT_SESSION_KEY is set")
		logger.Info("portal config loaded", slog.Bool("portal_enabled", false), slog.String("aws_region", resolvePortalRegion()))
		return
	}

	cfg.PortalCognitoDomain = validatedDomain
	cfg.PortalCognitoClientID = cognitoClientID
	cfg.PortalCognitoRedirectURI = envTrimmed("MGMT_COGNITO_REDIRECT_URI")
	cfg.PortalCognitoLogoutURI = envTrimmed("MGMT_COGNITO_LOGOUT_URI")
	cfg.PortalAWSRegion = resolvePortalRegion()
	logger.Info("portal config loaded", slog.Bool("portal_enabled", true), slog.String("aws_region", cfg.PortalAWSRegion))
}

// resolvePortalRegion returns the configured AWS region or the default.
func resolvePortalRegion() string {
	if r := envTrimmed("MGMT_AWS_REGION"); r != "" {
		return r
	}
	return DefaultPortalAWSRegion
}

// LoginEnabled reports whether soccer JWT import is configured.
func (c *Config) LoginEnabled() bool {
	return len(c.SessionKey) == sessionKeyLengthBytes
}

// GoogleEnabled reports whether direct Google Calendar add is configured.
func (c *Config) GoogleEnabled() bool {
	return c.LoginEnabled() &&
		c.GoogleClientID != "" &&
		c.GoogleClientSecret != "" &&
		c.GoogleConnectionTableName != ""
}

// SoccerSessionEnabled reports whether DynamoDB soccer session persistence is configured.
func (c *Config) SoccerSessionEnabled() bool {
	return c.LoginEnabled() && c.SoccerSessionTableName != ""
}

// PortalEnabled reports whether the EC2 management portal is fully configured.
// It returns true only when the session key is valid, the Cognito domain is set,
// and the Cognito client ID is set.
func (c *Config) PortalEnabled() bool {
	return len(c.PortalSessionKey) == sessionKeyLengthBytes && c.PortalCognitoDomain != "" && c.PortalCognitoClientID != ""
}

// NormalizeCognitoDomain validates the Cognito hosted UI origin used by OAuth.
// Restricting it to HTTPS and an origin prevents user-controlled endpoint
// construction from becoming an SSRF primitive.
func NormalizeCognitoDomain(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("Cognito domain must be an HTTPS origin without credentials, path, query, or fragment")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// PublicBindEnabled reports whether the server should bind to all interfaces.
func PublicBindEnabled() bool {
	value := envTrimmed("APP_BIND_ALL")
	return strings.EqualFold(value, "1") || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

// ServerListenAddress returns the host:port address the server should bind to.
func ServerListenAddress() string {
	port := envTrimmed("PORT")
	if port == "" {
		port = "8080"
	}
	host := envTrimmed("HOST")
	if host == "" {
		if PublicBindEnabled() {
			host = "0.0.0.0"
		} else {
			host = "127.0.0.1"
		}
	}
	return net.JoinHostPort(host, port)
}

// LocalPortalPreviewRequested reports whether the local mock portal preview was
// explicitly requested. The exact true value keeps this development-only mode
// opt-in rather than treating other non-empty values as enabled.
func LocalPortalPreviewRequested() bool {
	return strings.EqualFold(envTrimmed("MGMT_LOCAL_PREVIEW"), "true")
}

// LocalPortalPreviewEnabled reports whether the mock portal preview may be
// exposed on listenAddress. Preview mode is available only on a loopback
// listener so its deliberately unauthenticated routes cannot be exposed to a
// network interface.
func LocalPortalPreviewEnabled(listenAddress string) bool {
	if !LocalPortalPreviewRequested() {
		return false
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenAddress))
	return err == nil && isLoopbackHost(host)
}

// LocalServerURL returns a localhost URL derived from the bound listen address.
func LocalServerURL(listenAddress string) string {
	_, port, err := net.SplitHostPort(listenAddress)
	if err != nil || port == "" {
		return "http://localhost:8080"
	}
	return "http://localhost:" + port
}

// NormalizeLPSAPIBaseURL validates and normalizes the configured LPS API base URL.
func NormalizeLPSAPIBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultLPSAPIBaseURL
	}
	parsed, err := url.Parse(raw)
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

func envTrimmed(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	parsedIP := net.ParseIP(host)
	return parsedIP != nil && parsedIP.IsLoopback()
}
