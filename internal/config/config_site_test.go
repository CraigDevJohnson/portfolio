package config

import (
	"slices"
	"testing"
)

func setSiteEnvironment(t *testing.T, invitations string) {
	t.Helper()
	for key, value := range map[string]string{
		"LPS_SESSION_KEY":           "",
		"SITE_SESSION_KEY":          valid64HexKey,
		"SITE_COGNITO_DOMAIN":       "https://example.auth.us-west-2.amazoncognito.com",
		"SITE_COGNITO_ISSUER":       "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_site",
		"SITE_COGNITO_CLIENT_ID":    "siteclient",
		"SITE_COGNITO_REDIRECT_URI": "https://dev.example.com/auth/callback",
		"SITE_COGNITO_LOGOUT_URI":   "https://dev.example.com/sign-in",
		"SITE_INVITATIONS_JSON":     invitations,
		"SITE_ALLOW_LOCAL_CALLBACK": "false",
	} {
		t.Setenv(key, value)
	}
}

func TestSiteConfigurationLoadsInvitationsWithoutSoccerOrManagement(t *testing.T) {
	setSiteEnvironment(t, `{"craigdevjohnson@gmail.com":["soccer","management"],"visitor@example.com":[]}`)
	cfg := Load()
	if !cfg.SiteEnabled() || cfg.LoginEnabled() {
		t.Fatal("site sign-in should be independently enabled")
	}
	if !cfg.SiteEmailInvited(" CRAIGDEVJOHNSON@GMAIL.COM ") || cfg.SiteEmailInvited("craigdevjohnson+other@gmail.com") {
		t.Fatal("invitations must match normalized bare emails exactly")
	}
	if grants := cfg.SiteGrantsFor("craigdevjohnson@gmail.com"); !slices.Equal(grants, []string{"soccer", "management"}) {
		t.Fatalf("current grants = %v", grants)
	}
	if grants := cfg.SiteGrantsFor("visitor@example.com"); len(grants) != 0 {
		t.Fatalf("invited visitor received unconfigured grants: %v", grants)
	}
}

func TestSiteConfigurationRejectsUnreviewedOrUnsafeInputs(t *testing.T) {
	for _, test := range []struct {
		name        string
		invitations string
	}{
		{name: "unknown grant", invitations: `{"owner@example.com":["admin"]}`},
		{name: "display name", invitations: `{"Owner <owner@example.com>":["soccer"]}`},
		{name: "empty map", invitations: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			setSiteEnvironment(t, test.invitations)
			cfg := Load()
			if cfg.SiteEnabled() {
				t.Fatal("invalid invitation configuration enabled sign-in")
			}
		})
	}
}
