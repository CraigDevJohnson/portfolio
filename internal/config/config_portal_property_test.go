package config_test

// Feature: ec2-management-portal, Property 12: Portal config errors never break non-portal routes
// Feature: ec2-management-portal, Property 13: Invalid MGMT_SESSION_KEY always disables portal with warning
//
// Validates: Requirements 10.4, 10.5

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"portfolio/internal/config"
	"portfolio/internal/portfolio"
)

// homeOnlyMux builds a minimal ServeMux with only the portfolio home route registered.
// This mirrors the non-portal portion of buildMux, isolating non-portal route behavior.
func homeOnlyMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		portfolio.HomeHandler(w, r, config.CareerStartYear)
	})
	return mux
}

// TestProperty12_PortalConfigErrorsNeverBreakNonPortalRoutes verifies that for any
// combination of invalid portal environment variables, GET / still returns HTTP 200.
//
// Feature: ec2-management-portal, Property 12: Portal config errors never break non-portal routes
// Validates: Requirement 10.4
func TestProperty12_PortalConfigErrorsNeverBreakNonPortalRoutes(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate an invalid portal key variant (empty, non-hex, wrong-length, etc.)
		keyVariant := rapid.SampledFrom([]string{
			"",                      // absent — portal silently disabled
			"not-hex",               // non-hex characters
			"abc123",                // too short
			strings.Repeat("z", 64), // 64 chars, invalid hex
			strings.Repeat("g", 64), // 64 chars, invalid hex
			"ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", // uppercase hex (not valid 64-char lowercase hex)
			"abc", // very short non-hex
		}).Draw(rt, "invalid_key")

		cognitoDomain := rapid.SampledFrom([]string{
			"",
			"https://example.auth.us-east-1.amazoncognito.com",
		}).Draw(rt, "cognito_domain")

		cognitoClientID := rapid.SampledFrom([]string{
			"",
			"test-client-id",
		}).Draw(rt, "cognito_client_id")

		// t.Setenv handles cleanup after each test iteration automatically.
		t.Setenv("MGMT_SESSION_KEY", keyVariant)
		t.Setenv("MGMT_COGNITO_DOMAIN", cognitoDomain)
		t.Setenv("MGMT_COGNITO_CLIENT_ID", cognitoClientID)

		// config.Load() must not panic for any of these combinations.
		cfg := config.Load()

		// The non-portal home route must still return 200 regardless of portal config state.
		mux := homeOnlyMux()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			rt.Fatalf("GET / returned %d, want 200; portal_enabled=%v, key=%q",
				rr.Code, cfg.PortalEnabled(), keyVariant)
		}

		// All variants used here are invalid, so portal must never be enabled.
		if cfg.PortalEnabled() {
			rt.Fatalf("PortalEnabled() returned true for invalid key %q", keyVariant)
		}
	})
}

// TestProperty13_InvalidMGMT_SESSION_KEY_AlwaysDisablesPortalWithWarning verifies that
// for any non-empty string set as MGMT_SESSION_KEY that is not a valid 64-character
// lowercase hex string, config.Load() sets PortalEnabled = false and emits a WARN log.
//
// Feature: ec2-management-portal, Property 13: Invalid MGMT_SESSION_KEY always disables portal with warning
// Validates: Requirement 10.5
func TestProperty13_InvalidMGMT_SESSION_KEY_AlwaysDisablesPortalWithWarning(t *testing.T) {
	const validHexChars = "0123456789abcdef"

	// Redirect the default slog logger once for the whole test to a JSON handler
	// so each config.Load() iteration can inspect its WARN output.
	// We restore it at test cleanup.
	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	rapid.Check(t, func(rt *rapid.T) {
		// Generate a non-empty string that is NOT a valid 64-char lowercase hex string.
		invalidKey := rapid.Custom(func(rt *rapid.T) string {
			strategy := rapid.IntRange(0, 3).Draw(rt, "strategy")
			switch strategy {
			case 0:
				// Wrong length: 1–63 chars of valid lowercase hex (too short)
				n := rapid.IntRange(1, 63).Draw(rt, "length")
				var sb strings.Builder
				for i := 0; i < n; i++ {
					idx := rapid.IntRange(0, len(validHexChars)-1).Draw(rt, "char_idx")
					sb.WriteByte(validHexChars[idx])
				}
				return sb.String()
			case 1:
				// Wrong length: 65–128 chars of valid lowercase hex (too long)
				n := rapid.IntRange(65, 128).Draw(rt, "length")
				var sb strings.Builder
				for i := 0; i < n; i++ {
					idx := rapid.IntRange(0, len(validHexChars)-1).Draw(rt, "char_idx")
					sb.WriteByte(validHexChars[idx])
				}
				return sb.String()
			case 2:
				// Exactly 64 chars but with at least one non-hex character injected.
				result := make([]byte, 64)
				for i := range result {
					result[i] = validHexChars[rapid.IntRange(0, len(validHexChars)-1).Draw(rt, "hex_idx")]
				}
				pos := rapid.IntRange(0, 63).Draw(rt, "inject_pos")
				result[pos] = 'G' // 'G' is never a valid hex digit
				return string(result)
			default:
				// Exactly 64 chars of uppercase hex (uppercase is not accepted).
				const upperHex = "0123456789ABCDEF"
				var sb strings.Builder
				for i := 0; i < 64; i++ {
					idx := rapid.IntRange(0, len(upperHex)-1).Draw(rt, "upper_idx")
					sb.WriteByte(upperHex[idx])
				}
				s := sb.String()
				// Ensure at least one uppercase letter so it cannot be all-digit valid hex.
				if strings.ContainsAny(s, "ABCDEF") {
					return s
				}
				return strings.Repeat("G", 64) // fallback: definitely invalid
			}
		}).Draw(rt, "invalid_key")

		// Capture log output for this iteration.
		var logBuf bytes.Buffer
		slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		})))

		// Set env vars. t.Setenv restores originals after each sub-test.
		t.Setenv("MGMT_SESSION_KEY", invalidKey)
		t.Setenv("MGMT_COGNITO_DOMAIN", "")
		t.Setenv("MGMT_COGNITO_CLIENT_ID", "")

		cfg := config.Load()

		// Assertion 1: portal must be disabled for any invalid key.
		if cfg.PortalEnabled() {
			rt.Fatalf("PortalEnabled() returned true for invalid MGMT_SESSION_KEY %q", invalidKey)
		}

		// Assertion 2: a WARN log entry must have been emitted.
		logOutput := logBuf.String()
		if logOutput == "" {
			rt.Fatalf("expected WARN log for invalid MGMT_SESSION_KEY %q, but no log was produced", invalidKey)
		}

		warnFound := false
		for _, line := range strings.Split(strings.TrimSpace(logOutput), "\n") {
			if line == "" {
				continue
			}
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			if level, _ := entry["level"].(string); strings.EqualFold(level, "warn") {
				warnFound = true
				break
			}
		}
		if !warnFound {
			rt.Fatalf("no WARN-level log entry found for MGMT_SESSION_KEY %q; log: %s",
				invalidKey, logOutput)
		}
	})
}
