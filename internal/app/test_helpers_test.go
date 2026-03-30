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
		LoginLimiter: newLoginRateLimiter(5, time.Minute),
	}
	app.GoogleHandler = internalgoogle.NewHandler(&app.Config, app.LPSClient, nil)
	app.GoogleHandler.Soccer = newGoogleSoccerBridge(newTestSoccerHandler(app))
	t.Cleanup(func() {
		app.LoginLimiter.Close()
	})
	return app
}

func newTestSoccerHandler(app *App) *internalsoccer.Handler {
	return internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, soccerGoogleHooks{google: app.GoogleHandler})
}

func encryptTestSession(t *testing.T, app *App, session *types.SessionData) string {
	t.Helper()

	encrypted, err := internalsession.EncryptJSONValue(app.Config.SessionKey, session)
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}

	return encrypted
}

func decryptTestSession(t *testing.T, app *App, value string) types.SessionData {
	t.Helper()

	var session types.SessionData
	err := internalsession.DecryptJSONValue(app.Config.SessionKey, value, &session)
	if err != nil {
		t.Fatalf("DecryptJSONValue returned error: %v", err)
	}

	return session
}

func addSessionCookie(t *testing.T, app *App, req *http.Request, session *types.SessionData) {
	t.Helper()
	encrypted := encryptTestSession(t, app, session)
	req.AddCookie(&http.Cookie{Name: config.LPSSessionCookieName, Value: encrypted})
}
