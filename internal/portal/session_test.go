package portal

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"portfolio/internal/config"
)

// testKey is a hardcoded 32-byte key used for all session helper tests.
var testKey = bytes.Repeat([]byte{0xAB}, 32)

// newTestHandler returns a Handler configured with testKey.
func newTestHandler() *Handler {
	cfg := &config.Config{
		PortalSessionKey: testKey,
	}
	return NewHandler(cfg, nil, nil, nil, nil, nil)
}

// newRequestPair returns a fresh request and response recorder, simulating an
// HTTP connection. The request uses localhost so that RequestIsHTTPS returns
// false (no TLS, loopback origin) — Secure will be false, which matches the
// test environment.
func newRequestPair() (*http.Request, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", http.NoBody)
	w := httptest.NewRecorder()
	return req, w
}

// copyCookiesToRequest copies all cookies set on recorder w into request r, so
// that a subsequent loadSession / loadOAuthState call can read them. This
// simulates the browser round-trip.
func copyCookiesToRequest(w *httptest.ResponseRecorder, r *http.Request) {
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
}

func TestPortalSessionIsValid(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		session *PortalSession
		want    bool
	}{
		{name: "nil session", session: nil, want: false},
		{name: "empty username", session: &PortalSession{ExpiresAt: now.Add(time.Hour)}, want: false},
		{name: "expired", session: &PortalSession{Username: "operator@example.com", ExpiresAt: now.Add(-time.Hour)}, want: false},
		{name: "active", session: &PortalSession{Username: "operator@example.com", ExpiresAt: now.Add(time.Hour)}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.session.IsValid(); got != test.want {
				t.Fatalf("IsValid() = %t, want %t", got, test.want)
			}
		})
	}
}

// ---------- PortalSession round-trip ----------

func TestSetSessionLoadSession_RoundTrip(t *testing.T) {
	h := newTestHandler()
	req, w := newRequestPair()

	want := &PortalSession{
		Username:  "operator@example.com",
		ExpiresAt: time.Now().Add(time.Hour).Truncate(time.Second),
	}

	if err := h.setSession(w, req, want); err != nil {
		t.Fatalf("setSession returned error: %v", err)
	}

	// Replay the cookie on the request.
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/", http.NoBody)
	copyCookiesToRequest(w, req2)

	got, err := h.loadSession(req2)
	if err != nil {
		t.Fatalf("loadSession returned error: %v", err)
	}
	if got == nil {
		t.Fatal("loadSession returned nil session, want non-nil")
	}
	if got.Username != want.Username {
		t.Errorf("Username: got %q, want %q", got.Username, want.Username)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}
}

// ---------- clearSession ----------

func TestClearSession_SetsNegativeMaxAge(t *testing.T) {
	h := newTestHandler()
	req, w := newRequestPair()

	h.clearSession(w, req)

	resp := w.Result()
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name != config.PortalSessionCookieName {
			continue
		}
		found = true
		if c.MaxAge != -1 {
			t.Errorf("MaxAge: got %d, want -1", c.MaxAge)
		}
		// Expires must be at or before the Unix epoch (time.Unix(0,0)).
		epoch := time.Unix(0, 0)
		if c.Expires.After(epoch) {
			t.Errorf("Expires: got %v, want <= epoch (%v)", c.Expires, epoch)
		}
	}
	if !found {
		t.Fatalf("cookie %q not found in response", config.PortalSessionCookieName)
	}
}

// ---------- loadSession — no cookie ----------

func TestLoadSession_NoCookie_ReturnsNilNil(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", http.NoBody)

	got, err := h.loadSession(req)
	if err != nil {
		t.Fatalf("loadSession returned unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("loadSession returned non-nil session, want nil")
	}
}

// ---------- loadSession — corrupted ciphertext ----------

func TestLoadSession_CorruptedCiphertext_ReturnsError(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", http.NoBody)

	// Inject a cookie with obviously invalid ciphertext.
	req.AddCookie(&http.Cookie{
		Name:  config.PortalSessionCookieName,
		Value: "notvalidbase64!!!",
	})

	got, err := h.loadSession(req)
	if err == nil {
		t.Fatal("loadSession expected error for corrupted ciphertext, got nil")
	}
	if got != nil {
		t.Fatalf("loadSession returned non-nil session on error, want nil")
	}
}

// ---------- OAuthState round-trip ----------

func TestSetOAuthStateLoadOAuthState_RoundTrip(t *testing.T) {
	h := newTestHandler()
	req, w := newRequestPair()

	want := &OAuthState{
		State:        "random-state-nonce-abc123",
		CodeVerifier: "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
	}

	if err := h.setOAuthState(w, req, want); err != nil {
		t.Fatalf("setOAuthState returned error: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/", http.NoBody)
	copyCookiesToRequest(w, req2)

	got, err := h.loadOAuthState(req2)
	if err != nil {
		t.Fatalf("loadOAuthState returned error: %v", err)
	}
	if got == nil {
		t.Fatal("loadOAuthState returned nil, want non-nil")
	}
	if got.State != want.State {
		t.Errorf("State: got %q, want %q", got.State, want.State)
	}
	if got.CodeVerifier != want.CodeVerifier {
		t.Errorf("CodeVerifier: got %q, want %q", got.CodeVerifier, want.CodeVerifier)
	}
}

// ---------- clearOAuthState ----------

func TestClearOAuthState_SetsNegativeMaxAge(t *testing.T) {
	h := newTestHandler()
	req, w := newRequestPair()

	h.clearOAuthState(w, req)

	resp := w.Result()
	var found bool
	for _, c := range resp.Cookies() {
		if c.Name != config.PortalOAuthStateCookieName {
			continue
		}
		found = true
		if c.MaxAge != -1 {
			t.Errorf("MaxAge: got %d, want -1", c.MaxAge)
		}
	}
	if !found {
		t.Fatalf("cookie %q not found in response", config.PortalOAuthStateCookieName)
	}
}
