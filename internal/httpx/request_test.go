package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIPPrefersTrustedForwardedHeaders(t *testing.T) {
	t.Run("uses cloudflare header from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("CF-Connecting-IP", "198.51.100.24")

		if got := ClientIP(req); got != "198.51.100.24" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("ignores cloudflare header on direct connections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "203.0.113.10:443"
		req.Header.Set("CF-Connecting-IP", "198.51.100.24")

		if got := ClientIP(req); got != "203.0.113.10" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("uses x forwarded for from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("X-Forwarded-For", "198.51.100.25, 10.0.0.5")

		if got := ClientIP(req); got != "198.51.100.25" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("ignores spoofed x forwarded for on direct connections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "203.0.113.11:443"
		req.Header.Set("X-Forwarded-For", "198.51.100.26")

		if got := ClientIP(req); got != "203.0.113.11" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})
}

// Production break caught: an API Gateway origin in request context would be ignored in favor of untrusted transport metadata.
func TestTrustedOriginOverridesUntrustedTransportMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.invalid/soccer", nil)
	req.RemoteAddr = "203.0.113.10:443"
	req = WithTrustedOrigin(req, TrustedOrigin{Scheme: "https", Host: "dev.craigdevjohnson.com"})

	if !RequestIsHTTPS(req) {
		t.Fatal("trusted API Gateway origin was not treated as HTTPS")
	}
	if got := RequestBaseURL(req); got != "https://dev.craigdevjohnson.com" {
		t.Fatalf("RequestBaseURL = %q", got)
	}
}

// Production break caught: invalid values could create a trusted origin and permit an unsafe scheme or empty host.
func TestTrustedOriginRejectsInvalidValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	for _, origin := range []TrustedOrigin{
		{Scheme: "javascript", Host: "example.com"},
		{Scheme: "https", Host: ""},
	} {
		got := WithTrustedOrigin(req, origin)
		if RequestIsHTTPS(got) {
			t.Fatalf("invalid origin became trusted: %#v", origin)
		}
	}
}

// Production break caught: public clients could spoof X-Forwarded-Proto to force HTTPS without a trusted origin.
func TestRequestIsHTTPSOnlyTrustsProxiedHeader(t *testing.T) {
	t.Run("trusts X-Forwarded-Proto from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("X-Forwarded-Proto", "https")

		if !RequestIsHTTPS(req) {
			t.Fatal("expected RequestIsHTTPS to return true for trusted proxy with https proto")
		}
	})

	t.Run("ignores X-Forwarded-Proto from untrusted source", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.RemoteAddr = "203.0.113.10:443"
		req.Header.Set("X-Forwarded-Proto", "https")

		if RequestIsHTTPS(req) {
			t.Fatal("expected RequestIsHTTPS to return false for untrusted source with spoofed proto")
		}
	})

	t.Run("returns true for direct TLS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.TLS = &tls.ConnectionState{}

		if !RequestIsHTTPS(req) {
			t.Fatal("expected RequestIsHTTPS to return true for direct TLS")
		}
	})
}
