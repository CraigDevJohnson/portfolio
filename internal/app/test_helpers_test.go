package app

import (
	"net/http"
	"testing"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	internalsession "portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := &App{
		Config: config.Config{
			SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
			LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
		},
		LPSClient:    &http.Client{Timeout: 5 * time.Second},
		LoginLimiter: internalsession.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys),
	}
	app.GoogleHandler = internalgoogle.NewHandler(&app.Config, app.LPSClient, nil)
	app.GoogleHandler.Soccer = newTestSoccerHandler(app)
	t.Cleanup(func() {
		app.LoginLimiter.Close()
	})
	return app
}

// newTestSoccerHandler returns a handler wired to the test app dependencies.
func newTestSoccerHandler(app *App) *internalsoccer.Handler {
	return internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, app.GoogleHandler)
}

// encryptTestSession creates an encrypted session cookie payload for tests.
func encryptTestSession(t *testing.T, app *App, session *types.SessionData) string {
	t.Helper()

	encrypted, err := internalsession.EncryptJSONValue(app.Config.SessionKey, session)
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}

	return encrypted
}

// decryptTestSession decodes an encrypted test session cookie payload.
func decryptTestSession(t *testing.T, app *App, value string) types.SessionData {
	t.Helper()

	var session types.SessionData
	err := internalsession.DecryptJSONValue(app.Config.SessionKey, value, &session)
	if err != nil {
		t.Fatalf("DecryptJSONValue returned error: %v", err)
	}

	return session
}

// addSessionCookie attaches an encrypted soccer session cookie to the request.
func addSessionCookie(t *testing.T, app *App, req *http.Request, session *types.SessionData) {
	t.Helper()
	encrypted := encryptTestSession(t, app, session)
	req.AddCookie(&http.Cookie{Name: config.LPSSessionCookieName, Value: encrypted})
}

// findSessionCookie returns the soccer session cookie from an HTTP response.
func findSessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == config.LPSSessionCookieName {
			return cookie
		}
	}
	return nil
}

// assertClearedSessionCookie verifies that the session cookie was explicitly cleared.
func assertClearedSessionCookie(t *testing.T, resp *http.Response) {
	t.Helper()
	sessionCookie := findSessionCookie(t, resp)
	if sessionCookie == nil {
		t.Fatal("expected cleared session cookie to be set")
	}
	if sessionCookie.Value != "" {
		t.Fatalf("expected cleared session cookie value to be empty, got %q", sessionCookie.Value)
	}
	if sessionCookie.MaxAge >= 0 {
		t.Fatalf("expected cleared session cookie max-age to be negative, got %d", sessionCookie.MaxAge)
	}
	if !sessionCookie.Expires.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected cleared session cookie expiry to be Unix epoch, got %v", sessionCookie.Expires)
	}
}
