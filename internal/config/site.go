package config

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
)

func loadSiteConfig(logger *slog.Logger, cfg *Config) {
	keyHex := envTrimmed("SITE_SESSION_KEY")
	if keyHex == "" {
		return
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != sessionKeyLengthBytes || keyHex != strings.ToLower(keyHex) {
		logger.Warn("site sign-in disabled; SITE_SESSION_KEY must be a 64-character lowercase hex string")
		return
	}
	cfg.SiteCognitoDomain, _ = NormalizeCognitoDomain(envTrimmed("SITE_COGNITO_DOMAIN"))
	cfg.SiteCognitoIssuer = envTrimmed("SITE_COGNITO_ISSUER")
	cfg.SiteCognitoClientID = envTrimmed("SITE_COGNITO_CLIENT_ID")
	cfg.SiteCognitoRedirectURI = envTrimmed("SITE_COGNITO_REDIRECT_URI")
	cfg.SiteCognitoLogoutURI = envTrimmed("SITE_COGNITO_LOGOUT_URI")
	switch strings.ToLower(envTrimmed("SITE_ALLOW_LOCAL_CALLBACK")) {
	case "", "false":
	case "true":
		cfg.SiteAllowLocalCallback = true
	default:
		logger.Warn("site sign-in disabled; SITE_ALLOW_LOCAL_CALLBACK must be true or false")
		return
	}
	cfg.SiteInvitations, err = parseSiteInvitations(envTrimmed("SITE_INVITATIONS_JSON"))
	if err != nil {
		logger.Warn("site sign-in disabled; SITE_INVITATIONS_JSON is invalid")
		return
	}
	cfg.SiteSessionKey = key
	if !cfg.SiteEnabled() {
		logger.Warn("site sign-in disabled; complete Cognito URLs and invitations are required")
	}
}

// SiteEnabled reports whether the current environment has complete site identity configuration.
func (c *Config) SiteEnabled() bool {
	if c == nil || len(c.SiteSessionKey) != sessionKeyLengthBytes || strings.TrimSpace(c.SiteCognitoClientID) == "" || len(c.SiteInvitations) == 0 {
		return false
	}
	if _, err := NormalizeCognitoDomain(c.SiteCognitoDomain); err != nil {
		return false
	}
	if !validCognitoIssuer(c.SiteCognitoIssuer) ||
		!validPortalReturnURL(c.SiteCognitoRedirectURI, "/auth/callback", c.SiteAllowLocalCallback) ||
		!validPortalReturnURL(c.SiteCognitoLogoutURI, "/sign-in", false) {
		return false
	}
	for email, grants := range c.SiteInvitations {
		normalized, err := NormalizePortalEmail(email)
		if err != nil || normalized != email {
			return false
		}
		for _, grant := range grants {
			if grant != "soccer" && grant != "management" {
				return false
			}
		}
	}
	return true
}

// SiteEmailInvited checks the current reviewed invitation map.
func (c *Config) SiteEmailInvited(email string) bool {
	if c == nil {
		return false
	}
	normalized, err := NormalizePortalEmail(email)
	if err != nil {
		return false
	}
	_, invited := c.SiteInvitations[normalized]
	return invited
}

// SiteGrantsFor reads grants from current configuration, never from a browser session.
func (c *Config) SiteGrantsFor(email string) []string {
	if c == nil {
		return nil
	}
	normalized, err := NormalizePortalEmail(email)
	if err != nil {
		return nil
	}
	return append([]string(nil), c.SiteInvitations[normalized]...)
}

func parseSiteInvitations(raw string) (map[string][]string, error) {
	var decoded map[string][]string
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil || len(decoded) == 0 {
		return nil, errors.New("invitations must be a nonempty JSON object")
	}
	invitations := make(map[string][]string, len(decoded))
	for rawEmail, rawGrants := range decoded {
		email, err := NormalizePortalEmail(rawEmail)
		if err != nil || email != rawEmail {
			return nil, errors.New("invitation email must be normalized")
		}
		seen := make(map[string]bool, len(rawGrants))
		for _, grant := range rawGrants {
			if grant != "soccer" && grant != "management" {
				return nil, errors.New("unknown page grant")
			}
			if !seen[grant] {
				invitations[email] = append(invitations[email], grant)
				seen[grant] = true
			}
		}
		if invitations[email] == nil {
			invitations[email] = []string{}
		}
	}
	return invitations, nil
}
