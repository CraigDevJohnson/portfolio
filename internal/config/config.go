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
}

// Load reads runtime configuration from the environment.
func Load() Config {
	logger := slog.Default().With(slog.String("component", "config"))
	cfg := Config{
		LPSAPIBaseURL:             envTrimmed("LPS_API_BASE_URL"),
		GoogleClientID:            envTrimmed("CLIENT_ID_KEY"),
		GoogleClientSecret:        envTrimmed("CLIENT_SECRET_KEY"),
		GoogleConnectionTableName: envTrimmed("GOOGLE_CONNECTION_TABLE_NAME"),
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
		return cfg
	}

	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != sessionKeyLengthBytes {
		logger.Warn("soccer auth disabled; LPS_SESSION_KEY must be a 64-character hex string")
		return cfg
	}

	cfg.SessionKey = decoded

	return cfg
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
