package testutil

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"portfolio/internal/schedule"
)

func UnfoldICS(ics string) string {
	return strings.ReplaceAll(ics, "\r\n ", "")
}

func TestJWT(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"exp": expiresAt.Unix()})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	payloadToken := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return strings.Join([]string{header, payloadToken, signature}, ".")
}

// MislabelledLPSZuluTime mirrors the upstream API quirk where Mountain time is
// formatted with a trailing Z suffix.
func MislabelledLPSZuluTime(at time.Time) string {
	return at.In(schedule.MountainTimeLocation()).Format("2006-01-02T15:04:05.000") + "Z"
}
