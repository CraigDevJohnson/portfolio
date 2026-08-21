package config

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// logCapture is a minimal slog.Handler that records every log record emitted
// during a test so we can assert on level and message content.
type logCapture struct {
	mu      sync.Mutex
	records []capturedRecord
}

type capturedRecord struct {
	level   slog.Level
	message string
	attrs   map[string]string
}

func (c *logCapture) Enabled(_ context.Context, _ slog.Level) bool { return true }

//nolint:gocritic // slog.Handler requires slog.Record by value.
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, capturedRecord{
		level:   r.Level,
		message: r.Message,
		attrs:   attrs,
	})
	return nil
}

func (c *logCapture) WithAttrs(attrs []slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(name string) slog.Handler       { return c }

// hasWarn reports whether any captured record is at WARN level.
func (c *logCapture) hasWarn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		if r.level == slog.LevelWarn {
			return true
		}
	}
	return false
}

// warnMessages returns the messages of all WARN-level records.
func (c *logCapture) warnMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var msgs []string
	for _, r := range c.records {
		if r.level == slog.LevelWarn {
			msgs = append(msgs, r.message)
		}
	}
	return msgs
}

// withLogger installs a test slog.Logger backed by cap and returns a restore func.
// It also saves/restores the process environment variable for the given keys.
func withLogger(capture *logCapture) (restore func()) {
	old := slog.Default()
	slog.SetDefault(slog.New(capture))
	return func() { slog.SetDefault(old) }
}

// setEnv sets the given key=value pairs via t.Setenv (automatically restored at
// the end of the test) and also clears any keys whose value is "".
func setEnv(t *testing.T, pairs map[string]string) {
	t.Helper()
	for k, v := range pairs {
		t.Setenv(k, v)
	}
}

// valid64HexKey is a 64-character lowercase hex string (32 bytes) used in tests
// that require a valid MGMT_SESSION_KEY.
const valid64HexKey = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

// TestPortalAbsentKey verifies Requirement 10.6: when MGMT_SESSION_KEY is not
// set, the portal is disabled and no WARN log is emitted.
func TestPortalAbsentKey(t *testing.T) {
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	// Ensure MGMT_SESSION_KEY is absent.
	setEnv(t, map[string]string{
		"MGMT_SESSION_KEY":       "",
		"MGMT_COGNITO_DOMAIN":    "",
		"MGMT_COGNITO_CLIENT_ID": "",
	})

	cfg := Load()

	if cfg.PortalEnabled() {
		t.Fatal("expected portal to be disabled when MGMT_SESSION_KEY is absent")
	}
	if capture.hasWarn() {
		t.Fatalf("expected no WARN log when key is absent, got: %v", capture.warnMessages())
	}
}

// TestPortalFullyEnabled verifies Requirement 10.3: a 64-char hex key together
// with Cognito domain and client ID results in portal enabled = true.
func TestPortalFullyEnabled(t *testing.T) {
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	setEnv(t, map[string]string{
		"MGMT_SESSION_KEY":       valid64HexKey,
		"MGMT_COGNITO_DOMAIN":    "https://myapp.auth.us-east-1.amazoncognito.com",
		"MGMT_COGNITO_CLIENT_ID": "testclientid",
		"MGMT_AWS_REGION":        "us-west-2",
	})

	cfg := Load()

	if !cfg.PortalEnabled() {
		t.Fatal("expected portal to be enabled with valid key + Cognito domain + client ID")
	}
	if cfg.PortalCognitoDomain != "https://myapp.auth.us-east-1.amazoncognito.com" {
		t.Errorf("unexpected CognitoDomain: %q", cfg.PortalCognitoDomain)
	}
	if cfg.PortalCognitoClientID != "testclientid" {
		t.Errorf("unexpected CognitoClientID: %q", cfg.PortalCognitoClientID)
	}
	if len(cfg.PortalSessionKey) != sessionKeyLengthBytes {
		t.Errorf("unexpected PortalSessionKey length: got %d want %d", len(cfg.PortalSessionKey), sessionKeyLengthBytes)
	}
}

// TestPortalInvalidHexKey verifies Requirement 10.5: when MGMT_SESSION_KEY is
// present but not a valid 64-character hex string, the portal is disabled and a
// WARN log is emitted.
func TestPortalInvalidHexKey(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"too short", "aabbccdd"},
		{"non-hex characters", strings.Repeat("zz", 32)},
		{"odd length", "aabbccddeeff0011223344556677889900112233445566778899aabbccddee"},
		{"uppercase hex", strings.ToUpper(valid64HexKey)},
		{"63 chars", valid64HexKey[:63]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &logCapture{}
			restore := withLogger(capture)
			defer restore()

			setEnv(t, map[string]string{
				"MGMT_SESSION_KEY":       tc.key,
				"MGMT_COGNITO_DOMAIN":    "https://myapp.auth.us-east-1.amazoncognito.com",
				"MGMT_COGNITO_CLIENT_ID": "testclientid",
			})

			cfg := Load()

			if cfg.PortalEnabled() {
				t.Fatalf("expected portal disabled for key %q", tc.key)
			}
			if !capture.hasWarn() {
				t.Fatalf("expected WARN log for invalid key %q, got no WARN logs", tc.key)
			}
		})
	}
}

// TestPortalMissingCognitoDomain verifies Requirement 10.7: a valid session key
// but absent MGMT_COGNITO_DOMAIN disables the portal and emits a WARN log.
func TestPortalMissingCognitoDomain(t *testing.T) {
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	setEnv(t, map[string]string{
		"MGMT_SESSION_KEY":       valid64HexKey,
		"MGMT_COGNITO_DOMAIN":    "",
		"MGMT_COGNITO_CLIENT_ID": "testclientid",
	})

	cfg := Load()

	if cfg.PortalEnabled() {
		t.Fatal("expected portal disabled when MGMT_COGNITO_DOMAIN is absent")
	}
	if !capture.hasWarn() {
		t.Fatalf("expected WARN log when MGMT_COGNITO_DOMAIN is absent, got no WARN logs")
	}
}

// TestPortalMissingCognitoClientID verifies Requirement 10.7: a valid session key
// but absent MGMT_COGNITO_CLIENT_ID disables the portal and emits a WARN log.
func TestPortalMissingCognitoClientID(t *testing.T) {
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	setEnv(t, map[string]string{
		"MGMT_SESSION_KEY":       valid64HexKey,
		"MGMT_COGNITO_DOMAIN":    "https://myapp.auth.us-east-1.amazoncognito.com",
		"MGMT_COGNITO_CLIENT_ID": "",
	})

	cfg := Load()

	if cfg.PortalEnabled() {
		t.Fatal("expected portal disabled when MGMT_COGNITO_CLIENT_ID is absent")
	}
	if !capture.hasWarn() {
		t.Fatalf("expected WARN log when MGMT_COGNITO_CLIENT_ID is absent, got no WARN logs")
	}
}

// TestPortalDefaultAWSRegion verifies Requirement 5.5 / 10.1: when MGMT_AWS_REGION
// is not set, the resolved region defaults to "us-east-1".
func TestPortalDefaultAWSRegion(t *testing.T) {
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	setEnv(t, map[string]string{
		"MGMT_SESSION_KEY":       valid64HexKey,
		"MGMT_COGNITO_DOMAIN":    "https://myapp.auth.us-east-1.amazoncognito.com",
		"MGMT_COGNITO_CLIENT_ID": "testclientid",
		"MGMT_AWS_REGION":        "",
	})

	cfg := Load()

	if !cfg.PortalEnabled() {
		t.Fatal("expected portal to be enabled for region default test")
	}
	if cfg.PortalAWSRegion != DefaultPortalAWSRegion {
		t.Errorf("expected PortalAWSRegion %q, got %q", DefaultPortalAWSRegion, cfg.PortalAWSRegion)
	}
	if cfg.PortalAWSRegion != "us-east-1" {
		t.Errorf("expected default region us-east-1, got %q", cfg.PortalAWSRegion)
	}
}
