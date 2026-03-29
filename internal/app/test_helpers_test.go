package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/schedule"
)

func unfoldICS(ics string) string {
	return strings.ReplaceAll(ics, "\r\n ", "")
}

func testJWT(t *testing.T, expiresAt time.Time) string {
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

func testMislabelledLPSZuluTime(at time.Time) string {
	return at.In(schedule.MountainTimeLocation).Format("2006-01-02T15:04:05.000") + "Z"
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := &App{
		Config: config.Config{
			SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
			LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
		},
		LPSClient:    &http.Client{Timeout: 5 * time.Second},
		LoginLimiter: newLoginRateLimiter(5, time.Minute),
		MountainTZ:   schedule.MountainTimeLocation,
	}
	app.GoogleHandler = internalgoogle.NewHandler(&app.Config, app.LPSClient, nil)
	app.GoogleHandler.Soccer = newGoogleSoccerBridge(app.newSoccerHandler())
	t.Cleanup(func() {
		app.LoginLimiter.Close()
	})
	return app
}

func addSessionCookie(t *testing.T, app *App, req *http.Request, session *SessionData) {
	t.Helper()
	encrypted, err := app.encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: config.LPSSessionCookieName, Value: encrypted})
}
