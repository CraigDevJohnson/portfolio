package portal

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"portfolio/internal/config"
)

func TestCallbackAuthorizesOnlyVerifiedAllowedIdentity(t *testing.T) {
	fixture := newOIDCFixture(t)
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name            string
		change          func(jwt.MapClaims)
		state           string
		wrongSignature  bool
		exchangeFailure bool
		allowed         bool
		reason          string
	}{
		{name: "approved", allowed: true},
		{name: "normalized approved", allowed: true, change: func(c jwt.MapClaims) { c["email"] = " CRAIGDEVJOHNSON@GMAIL.COM " }},
		{name: "missing email", reason: "invalid_email", change: func(c jwt.MapClaims) { delete(c, "email") }},
		{name: "malformed email", reason: "invalid_email", change: func(c jwt.MapClaims) { c["email"] = "sentinel-email" }},
		{name: "display name", reason: "invalid_email", change: func(c jwt.MapClaims) { c["email"] = "Craig <craigdevjohnson@gmail.com>" }},
		{name: "unverified", reason: "unverified_email", change: func(c jwt.MapClaims) { c["email_verified"] = false }},
		{name: "missing verification", reason: "unverified_email", change: func(c jwt.MapClaims) { delete(c, "email_verified") }},
		{name: "string verification", reason: "unverified_email", change: func(c jwt.MapClaims) { c["email_verified"] = "true" }},
		{name: "other email", reason: "email_not_allowed", change: func(c jwt.MapClaims) { c["email"] = "sentinel-email@example.com" }},
		{name: "plus alias", reason: "email_not_allowed", change: func(c jwt.MapClaims) { c["email"] = "craigdevjohnson+dev@gmail.com" }},
		{name: "dot alias", reason: "email_not_allowed", change: func(c jwt.MapClaims) { c["email"] = "craig.dev.johnson@gmail.com" }},
		{name: "username only", reason: "invalid_email", change: func(c jwt.MapClaims) { delete(c, "email"); c["cognito:username"] = "craigdevjohnson@gmail.com" }},
		{name: "wrong state", state: "wrong-state", reason: "invalid_state"},
		{name: "invalid signature", wrongSignature: true, reason: "invalid_token"},
		{name: "untrusted issuer", reason: "invalid_token", change: func(c jwt.MapClaims) { c["iss"] = "https://sentinel-email@example.com/pool" }},
		{name: "token exchange error", exchangeFailure: true, reason: "token_exchange_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := fixture.claims()
			if tc.change != nil {
				tc.change(claims)
			}
			key := fixture.key
			if tc.wrongSignature {
				key = wrongKey
			}
			rawToken := signTestToken(t, claims, key, jwt.SigningMethodRS256, "test")
			body, err := json.Marshal(TokenResponse{IDToken: rawToken, AccessToken: "sentinel-access-token", RefreshToken: "sentinel-refresh-token"})
			if err != nil {
				t.Fatal(err)
			}
			fixture.tokenStatus, fixture.tokenBody = http.StatusOK, string(body)
			if tc.exchangeFailure {
				fixture.tokenStatus, fixture.tokenBody = http.StatusBadRequest, "sentinel-response-body"
			}
			fixture.code, fixture.verifier = "sentinel-code", "sentinel-verifier"
			var logs bytes.Buffer
			h := NewHandler(&config.Config{PortalSessionKey: make([]byte, 32), PortalAllowedEmails: []string{"craigdevjohnson@gmail.com"}}, fixture.client, nil, nil, nil, slog.New(slog.NewTextHandler(&logs, nil)))
			state := tc.state
			if state == "" {
				state = "expected-state"
			}
			r := httptest.NewRequest(http.MethodGet, "https://app.example/callback?state="+url.QueryEscape(state)+"&code="+fixture.code, nil)
			cookies := httptest.NewRecorder()
			if err := h.setOAuthState(cookies, r, &OAuthState{State: "expected-state", CodeVerifier: fixture.verifier}); err != nil {
				t.Fatal(err)
			}
			if err := h.setSession(cookies, r, &PortalSession{Username: "previous@example.com", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			for _, cookie := range cookies.Result().Cookies() {
				r.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			h.CallbackHandler(response, r)
			if tc.allowed {
				if response.Code != http.StatusFound || response.Header().Get("Location") != "/mgmt" {
					t.Fatalf("approved callback failed: status %d", response.Code)
				}
			} else {
				if response.Code != http.StatusUnauthorized && response.Code != http.StatusBadRequest {
					t.Fatalf("rejected callback returned %d", response.Code)
				}
				if !strings.Contains(response.Body.String(), "Sign-in could not be completed.") {
					t.Fatal("rejection did not use generic failure")
				}
				if !strings.Contains(logs.String(), "reason="+tc.reason) {
					t.Errorf("missing safe reason category %s: %s", tc.reason, logs.String())
				}
			}
			assertCallbackCookies(t, h, response, tc.allowed)
			for _, secret := range []string{rawToken, fixture.code, fixture.verifier, "sentinel-access-token", "sentinel-refresh-token", "sentinel-response-body", "sentinel-email", "craigdevjohnson", "CRAIGDEVJOHNSON"} {
				if strings.Contains(logs.String(), secret) {
					t.Error("callback logs exposed sensitive identity or token data")
				}
			}
		})
	}
}

func assertCallbackCookies(t *testing.T, h *Handler, response *httptest.ResponseRecorder, allowed bool) {
	t.Helper()
	foundSession, clearedState := false, false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.PortalOAuthStateCookieName && cookie.MaxAge < 0 {
			clearedState = true
		}
		if cookie.Name != config.PortalSessionCookieName {
			continue
		}
		foundSession = true
		if !allowed {
			if cookie.Value != "" || cookie.MaxAge >= 0 {
				t.Error("rejected callback retained a usable session")
			}
			continue
		}
		r := httptest.NewRequest(http.MethodGet, "https://app.example/mgmt", nil)
		r.AddCookie(cookie)
		session, err := h.loadSession(r)
		if err != nil || !session.IsValid() || session.Username != "craigdevjohnson@gmail.com" {
			t.Error("approved session could not be decrypted or had wrong identity")
		}
	}
	if !foundSession {
		t.Error("callback did not set or clear portal session")
	}
	if !clearedState {
		t.Error("callback did not clear OAuth state")
	}
}
