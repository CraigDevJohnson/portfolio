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

// valid64HexKey is a 64-character lowercase hex string (32 bytes) used by
// tests that require a valid session key.
const valid64HexKey = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

// retiredManagementIdentity is a complete configuration for the former
// management-only Cognito sign-in, as the development infrastructure supplied it.
var retiredManagementIdentity = map[string]string{
	"MGMT_SESSION_KEY":          valid64HexKey,
	"MGMT_COGNITO_DOMAIN":       "https://portal.auth.us-west-2.amazoncognito.com",
	"MGMT_COGNITO_ISSUER":       "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_mgmt",
	"MGMT_COGNITO_CLIENT_ID":    "client",
	"MGMT_COGNITO_REDIRECT_URI": "https://dev.craigdevjohnson.com/callback",
	"MGMT_COGNITO_LOGOUT_URI":   "https://dev.craigdevjohnson.com/login",
	"MGMT_ALLOWED_EMAILS":       "craigdevjohnson@gmail.com",
	"MGMT_ALLOW_LOCAL_CALLBACK": "false",
}

func clearManagementEnvironment(t *testing.T) {
	t.Helper()
	for key := range retiredManagementIdentity {
		t.Setenv(key, "")
	}
	for _, key := range []string{"MGMT_AWS_REGION", "SITE_SESSION_KEY", "LPS_SESSION_KEY"} {
		t.Setenv(key, "")
	}
}

func TestPortalAWSRegionDefaultsAndOverrides(t *testing.T) {
	clearManagementEnvironment(t)
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	if cfg := Load(); cfg.PortalAWSRegion != "us-east-1" {
		t.Fatalf("default portal region = %q, want us-east-1", cfg.PortalAWSRegion)
	}
	t.Setenv("MGMT_AWS_REGION", " us-west-2 ")
	if cfg := Load(); cfg.PortalAWSRegion != "us-west-2" {
		t.Fatalf("configured portal region = %q, want us-west-2", cfg.PortalAWSRegion)
	}
	if capture.hasWarn() {
		t.Fatalf("region-only configuration warned: %v", capture.warnMessages())
	}
}

func TestRetiredManagementIdentitySettingsAreReportedAndIgnored(t *testing.T) {
	clearManagementEnvironment(t)
	setEnv(t, retiredManagementIdentity)
	capture := &logCapture{}
	restore := withLogger(capture)
	defer restore()

	cfg := Load()
	warnings := capture.warnMessages()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "retired") || !strings.Contains(warnings[0], "management grant") {
		t.Fatalf("retired management identity settings were not reported once as ignored: %v", warnings)
	}
	if cfg.SiteEnabled() {
		t.Fatal("retired management identity settings enabled site sign-in")
	}
}
