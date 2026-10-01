package config

import (
	"errors"
	"log/slog"
	"net/mail"
	"net/url"
	"strings"
)

// retiredManagementIdentitySettings configured the former management-only
// Cognito sign-in. The portal now uses the site session and management grant.
var retiredManagementIdentitySettings = []string{
	"MGMT_SESSION_KEY",
	"MGMT_COGNITO_DOMAIN",
	"MGMT_COGNITO_ISSUER",
	"MGMT_COGNITO_CLIENT_ID",
	"MGMT_COGNITO_REDIRECT_URI",
	"MGMT_COGNITO_LOGOUT_URI",
	"MGMT_ALLOWED_EMAILS",
	"MGMT_ALLOW_LOCAL_CALLBACK",
}

func loadPortalConfig(logger *slog.Logger, cfg *Config) {
	cfg.PortalAWSRegion = envTrimmed("MGMT_AWS_REGION")
	if cfg.PortalAWSRegion == "" {
		cfg.PortalAWSRegion = DefaultPortalAWSRegion
	}
	var retired []string
	for _, name := range retiredManagementIdentitySettings {
		if envTrimmed(name) != "" {
			retired = append(retired, name)
		}
	}
	if len(retired) > 0 {
		logger.Warn(
			"retired management-only identity settings are ignored; the portal uses site sign-in and the management grant",
			slog.String("settings", strings.Join(retired, ",")),
		)
	}
	logger.Info("portal config loaded", slog.String("aws_region", cfg.PortalAWSRegion))
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
