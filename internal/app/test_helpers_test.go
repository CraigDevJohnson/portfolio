package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
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
	return at.In(loadMountainTimeLocation()).Format("2006-01-02T15:04:05.000") + "Z"
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	app := &App{
		Config: config.Config{
			SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
			LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
		},
		LPSClient:               &http.Client{Timeout: 5 * time.Second},
		LoginLimiter:             newLoginRateLimiter(5, time.Minute),
		MountainTZ:               loadMountainTimeLocation(),
		googleStore:              noopGoogleConnectionStore{},
		GoogleOAuthAuthURL:       googleOAuthAuthURL,
		GoogleOAuthTokenURL:      googleOAuthTokenURL,
		GoogleCalendarAPIBaseURL: googleCalendarAPIBaseURL,
	}
	t.Cleanup(func() {
		app.LoginLimiter.Close()
	})
	return app
}

func newTestAppWithGoogle(t *testing.T, store googleConnectionStore, authURL, tokenURL, apiBaseURL string) *App {
	t.Helper()
	app := newTestApp(t)
	app.Config.GoogleClientID = "google-client-id"
	app.Config.GoogleClientSecret = "google-client-secret"
	app.Config.GoogleConnectionTableName = "google-connections"
	app.setGoogleConnectionStore(store)
	if authURL != "" {
		app.GoogleOAuthAuthURL = authURL
	}
	if tokenURL != "" {
		app.GoogleOAuthTokenURL = tokenURL
	}
	if apiBaseURL != "" {
		app.GoogleCalendarAPIBaseURL = apiBaseURL
	}
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

type fakeGoogleConnectionStore struct {
	records map[string]googleConnectionRecord
}

func (store *fakeGoogleConnectionStore) Delete(_ context.Context, connectionID string) error {
	delete(store.records, connectionID)
	return nil
}

func (store *fakeGoogleConnectionStore) Get(_ context.Context, connectionID string) (*googleConnectionRecord, error) {
	record, ok := store.records[connectionID]
	if !ok {
		return nil, nil
	}
	clone := record
	return &clone, nil
}

func (store *fakeGoogleConnectionStore) Put(_ context.Context, record *googleConnectionRecord) error {
	store.records[record.ConnectionID] = *record
	return nil
}
