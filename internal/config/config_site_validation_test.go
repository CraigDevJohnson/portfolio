package config

import (
	"strings"
	"testing"
)

const siteOwnerInvitation = `{"craigdevjohnson@gmail.com":["soccer","management"]}`

func TestSiteSessionKeyMustBeLowercaseHex(t *testing.T) {
	for _, tc := range []struct{ name, key string }{
		{"too short", "aabbccdd"},
		{"non-hex characters", strings.Repeat("zz", 32)},
		{"odd length", valid64HexKey[:63]},
		{"uppercase hex", strings.ToUpper(valid64HexKey)},
		{"too long", valid64HexKey + "aa"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setSiteEnvironment(t, siteOwnerInvitation)
			t.Setenv("SITE_SESSION_KEY", tc.key)
			capture := &logCapture{}
			restore := withLogger(capture)
			defer restore()
			if cfg := Load(); cfg.SiteEnabled() {
				t.Fatalf("session key %q enabled site sign-in", tc.key)
			}
			if !capture.hasWarn() {
				t.Fatal("invalid session key was not reported")
			}
		})
	}
}

func TestSiteRejectsIncompleteOrInvalidIdentityConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"SITE_COGNITO_CLIENT_ID", ""},
		{"SITE_COGNITO_DOMAIN", ""},
		{"SITE_COGNITO_DOMAIN", "http://example.auth.us-west-2.amazoncognito.com"},
		{"SITE_COGNITO_DOMAIN", "https://example.auth.us-west-2.amazoncognito.com/oauth2"},
		{"SITE_COGNITO_ISSUER", ""},
		{"SITE_COGNITO_ISSUER", "https://cognito-idp.us-west-2.amazonaws.com"},
		{"SITE_COGNITO_ISSUER", "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_site/extra"},
		{"SITE_COGNITO_ISSUER", "http://issuer.example/pool"},
		{"SITE_COGNITO_ISSUER", "https://user:password@issuer.example/pool"},
		{"SITE_COGNITO_ISSUER", "https://issuer.example/pool?query=1"},
		{"SITE_COGNITO_ISSUER", "https://issuer.example/pool#fragment"},
		{"SITE_COGNITO_ISSUER", "https://issuer.example/pool?"},
		{"SITE_COGNITO_REDIRECT_URI", ""},
		{"SITE_COGNITO_LOGOUT_URI", ""},
		{"SITE_ALLOW_LOCAL_CALLBACK", "tru"},
		{"SITE_ALLOW_LOCAL_CALLBACK", "yes"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			setSiteEnvironment(t, siteOwnerInvitation)
			t.Setenv(tc.key, tc.value)
			if cfg := Load(); cfg.SiteEnabled() {
				t.Fatal("invalid configuration enabled site sign-in")
			}
		})
	}
}

func TestSiteCallbackMustMatchRegisteredRoute(t *testing.T) {
	for _, tc := range []struct {
		name          string
		callback      string
		allowLoopback bool
		wantEnabled   bool
	}{
		{name: "development callback", callback: "https://dev.craigdevjohnson.com/auth/callback", wantEnabled: true},
		{name: "HTTPS loopback callback", callback: "https://localhost:8080/auth/callback", wantEnabled: true},
		{name: "HTTP localhost opt-in", callback: "http://localhost:8080/auth/callback", allowLoopback: true, wantEnabled: true},
		{name: "HTTP IPv4 loopback opt-in", callback: "http://127.0.0.1:8080/auth/callback", allowLoopback: true, wantEnabled: true},
		{name: "HTTP IPv6 loopback opt-in", callback: "http://[::1]:8080/auth/callback", allowLoopback: true, wantEnabled: true},
		{name: "HTTP loopback requires opt-in", callback: "http://localhost:8080/auth/callback"},
		{name: "opt-in never allows HTTP on another host", callback: "http://app.example/auth/callback", allowLoopback: true},
		{name: "retired management callback", callback: "https://app.example/callback"},
		{name: "different route", callback: "https://app.example/sign-in"},
		{name: "missing path", callback: "https://app.example"},
		{name: "relative path", callback: "/auth/callback"},
		{name: "trailing slash", callback: "https://app.example/auth/callback/"},
		{name: "case mismatch", callback: "https://app.example/auth/Callback"},
		{name: "encoded path", callback: "https://app.example/auth/%63allback"},
		{name: "encoded slash", callback: "https://app.example/auth%2fcallback"},
		{name: "path normalization", callback: "https://app.example/x/../auth/callback"},
		{name: "credentials", callback: "https://user:password@app.example/auth/callback"},
		{name: "query", callback: "https://app.example/auth/callback?code=1"},
		{name: "empty query", callback: "https://app.example/auth/callback?"},
		{name: "fragment", callback: "https://app.example/auth/callback#fragment"},
		{name: "empty fragment", callback: "https://app.example/auth/callback#"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setSiteEnvironment(t, siteOwnerInvitation)
			t.Setenv("SITE_COGNITO_REDIRECT_URI", tc.callback)
			if tc.allowLoopback {
				t.Setenv("SITE_ALLOW_LOCAL_CALLBACK", "true")
			}
			if cfg := Load(); cfg.SiteEnabled() != tc.wantEnabled {
				t.Fatalf("SiteEnabled() = %v for callback %q", !tc.wantEnabled, tc.callback)
			}
		})
	}
}

func TestSiteLogoutMustMatchSignInRoute(t *testing.T) {
	for _, tc := range []struct {
		name               string
		logout             string
		allowLocalCallback bool
		wantEnabled        bool
	}{
		{name: "development logout", logout: "https://dev.craigdevjohnson.com/sign-in", wantEnabled: true},
		{name: "HTTPS loopback logout", logout: "https://localhost:8080/sign-in", wantEnabled: true},
		{name: "HTTP logout", logout: "http://app.example/sign-in"},
		{name: "HTTP loopback logout", logout: "http://localhost:8080/sign-in"},
		{name: "callback opt-in does not allow HTTP logout", logout: "http://127.0.0.1:8080/sign-in", allowLocalCallback: true},
		{name: "retired management logout", logout: "https://app.example/login"},
		{name: "protected route", logout: "https://app.example/mgmt"},
		{name: "callback route", logout: "https://app.example/auth/callback"},
		{name: "relative path", logout: "/sign-in"},
		{name: "missing path", logout: "https://app.example"},
		{name: "root path", logout: "https://app.example/"},
		{name: "trailing slash", logout: "https://app.example/sign-in/"},
		{name: "case mismatch", logout: "https://app.example/Sign-in"},
		{name: "encoded path", logout: "https://app.example/%73ign-in"},
		{name: "path normalization", logout: "https://app.example/auth/../sign-in"},
		{name: "credentials", logout: "https://user:password@app.example/sign-in"},
		{name: "query", logout: "https://app.example/sign-in?return_to=/mgmt"},
		{name: "empty query", logout: "https://app.example/sign-in?"},
		{name: "fragment", logout: "https://app.example/sign-in#fragment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setSiteEnvironment(t, siteOwnerInvitation)
			t.Setenv("SITE_COGNITO_LOGOUT_URI", tc.logout)
			if tc.allowLocalCallback {
				t.Setenv("SITE_ALLOW_LOCAL_CALLBACK", "true")
				t.Setenv("SITE_COGNITO_REDIRECT_URI", "http://127.0.0.1:8080/auth/callback")
			}
			if cfg := Load(); cfg.SiteEnabled() != tc.wantEnabled {
				t.Fatalf("SiteEnabled() = %v for logout %q", !tc.wantEnabled, tc.logout)
			}
		})
	}
}

func TestSiteInvitationsMatchNormalizedAddressesExactly(t *testing.T) {
	setSiteEnvironment(t, `{" CRAIGDEVJOHNSON@gmail.com ":["management"]}`)
	if cfg := Load(); cfg.SiteEnabled() {
		t.Fatal("a reviewed invitation map must already hold normalized addresses")
	}
	setSiteEnvironment(t, `{"craigdevjohnson@gmail.com":["management"]}`)
	cfg := Load()
	if !cfg.SiteEnabled() || !cfg.SiteEmailInvited("craigdevjohnson@gmail.com") || !cfg.SiteEmailInvited(" CraigDevJohnson@Gmail.com ") {
		t.Fatal("a verified address should match its normalized invitation")
	}
	for _, email := range []string{"", "craigdevjohnson+dev@gmail.com", "craig.dev.johnson@gmail.com", "other@gmail.com", "Craig <craigdevjohnson@gmail.com>", "<craigdevjohnson@gmail.com>", "craigdevjohnson@gmail.com (Craig)"} {
		if cfg.SiteEmailInvited(email) || len(cfg.SiteGrantsFor(email)) != 0 {
			t.Fatalf("unexpected invitation match for %q", email)
		}
	}
}
