package config

import (
	"encoding/hex"
	"errors"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
)

func loadPortalConfig(logger *slog.Logger, cfg *Config) {
	cfg.PortalAWSRegion = envTrimmed("MGMT_AWS_REGION")
	if cfg.PortalAWSRegion == "" {
		cfg.PortalAWSRegion = DefaultPortalAWSRegion
	}
	defer func() {
		logger.Info("portal config loaded", slog.Bool("portal_enabled", cfg.PortalEnabled()), slog.String("aws_region", cfg.PortalAWSRegion))
	}()
	keyHex := envTrimmed("MGMT_SESSION_KEY")
	if keyHex == "" {
		return
	}
	decoded, err := hex.DecodeString(keyHex)
	if err != nil || len(decoded) != sessionKeyLengthBytes || keyHex != strings.ToLower(keyHex) {
		logger.Warn("portal disabled; MGMT_SESSION_KEY must be a 64-character lowercase hex string")
		return
	}
	cfg.PortalCognitoDomain, _ = NormalizeCognitoDomain(envTrimmed("MGMT_COGNITO_DOMAIN"))
	cfg.PortalCognitoIssuer = envTrimmed("MGMT_COGNITO_ISSUER")
	cfg.PortalCognitoClientID = envTrimmed("MGMT_COGNITO_CLIENT_ID")
	cfg.PortalCognitoRedirectURI = envTrimmed("MGMT_COGNITO_REDIRECT_URI")
	cfg.PortalCognitoLogoutURI = envTrimmed("MGMT_COGNITO_LOGOUT_URI")
	switch strings.ToLower(envTrimmed("MGMT_ALLOW_LOCAL_CALLBACK")) {
	case "", "false":
	case "true":
		cfg.PortalAllowLocalCallback = true
	default:
		logger.Warn("portal disabled; MGMT_ALLOW_LOCAL_CALLBACK must be true or false")
		return
	}
	cfg.PortalAllowedEmails, err = parsePortalAllowedEmails(envTrimmed("MGMT_ALLOWED_EMAILS"))
	if err != nil {
		logger.Warn("portal disabled; MGMT_ALLOWED_EMAILS must contain comma-separated bare email addresses")
		return
	}
	cfg.PortalSessionKey = decoded
	if !cfg.PortalEnabled() {
		logger.Warn("portal disabled; valid Cognito domain, issuer, client ID, callback, logout, and allowed emails are required")
	}
}

// PortalEnabled reports whether all portal identity and session settings are valid.
func (c *Config) PortalEnabled() bool {
	if c == nil || len(c.PortalSessionKey) != sessionKeyLengthBytes || strings.TrimSpace(c.PortalCognitoClientID) == "" || len(c.PortalAllowedEmails) == 0 {
		return false
	}
	if _, err := NormalizeCognitoDomain(c.PortalCognitoDomain); err != nil {
		return false
	}
	if !validCognitoIssuer(c.PortalCognitoIssuer) || !validPortalReturnURL(c.PortalCognitoRedirectURI, "/callback", c.PortalAllowLocalCallback) || !validPortalReturnURL(c.PortalCognitoLogoutURI, "", false) {
		return false
	}
	for _, email := range c.PortalAllowedEmails {
		if _, err := NormalizePortalEmail(email); err != nil {
			return false
		}
	}
	return true
}

// NormalizePortalEmail accepts only a bare mailbox address and normalizes case.
// Dots and plus-address suffixes remain significant for exact authorization.
func NormalizePortalEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return "", errors.New("email must be a bare mailbox address")
	}
	return email, nil
}

// PortalEmailAllowed compares normalized, validated email addresses exactly.
func (c *Config) PortalEmailAllowed(email string) bool {
	if c == nil {
		return false
	}
	normalized, err := NormalizePortalEmail(email)
	if err != nil {
		return false
	}
	for _, allowed := range c.PortalAllowedEmails {
		candidate, candidateErr := NormalizePortalEmail(allowed)
		if candidateErr == nil && normalized == candidate {
			return true
		}
	}
	return false
}

func parsePortalAllowedEmails(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	emails := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, part := range parts {
		email, err := NormalizePortalEmail(part)
		if err != nil {
			return nil, err
		}
		if !seen[email] {
			seen[email] = true
			emails = append(emails, email)
		}
	}
	return emails, nil
}

// NormalizeCognitoDomain validates the HTTPS hosted UI origin used for OAuth.
func NormalizeCognitoDomain(raw string) (string, error) {
	parsed, err := parsePortalURL(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || (parsed.Path != "" && parsed.Path != "/") {
		return "", errors.New("Cognito domain must be an HTTPS origin without credentials, path, query, or fragment")
	}
	parsed.Path = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func validCognitoIssuer(raw string) bool {
	parsed, err := parsePortalURL(raw)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	pool := strings.TrimPrefix(parsed.Path, "/")
	return pool != "" && pool != "." && pool != ".." && !strings.Contains(pool, "/")
}

func validPortalReturnURL(raw, requiredPath string, allowLoopback bool) bool {
	parsed, err := parsePortalURL(raw)
	if err != nil || (requiredPath != "" && parsed.EscapedPath() != requiredPath) {
		return false
	}
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && allowLoopback && isLoopbackHost(parsed.Hostname()))
}

func parsePortalURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.Opaque != "" || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || strings.Contains(raw, "#") {
		return nil, errors.New("portal URL must be absolute without credentials, query, or fragment")
	}
	return parsed, nil
}
