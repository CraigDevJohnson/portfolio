package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
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
	return at.In(mountainTimeLocation).Format("2006-01-02T15:04:05.000") + "Z"
}


func resetSoccerLoginAttempts(t *testing.T) {
	t.Helper()

	previousLimiter := soccerLoginAttempts
	soccerLoginAttempts = newLoginRateLimiter(5, time.Minute)
	t.Cleanup(func() {
		soccerLoginAttempts.Close()
		soccerLoginAttempts = previousLimiter
	})
}


func addSessionCookie(t *testing.T, req *http.Request, session *SessionData) {
	t.Helper()
	encrypted, err := encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: lpsSessionCookieName, Value: encrypted})
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


func configureGoogleTestRuntime(t *testing.T, store googleConnectionStore, authURL, tokenURL, apiBaseURL string) {
	t.Helper()

	previousConfig := configData
	previousStore := googleConnections
	previousAuthURL := googleOAuthAuthURL
	previousTokenURL := googleOAuthTokenURL
	previousAPIBaseURL := googleCalendarAPIBaseURL

	configData = serverConfig{
		SessionKey:                []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL:             defaultLPSAPIBaseURL,
		GoogleClientID:            "google-client-id",
		GoogleClientSecret:        "google-client-secret",
		GoogleConnectionTableName: "google-connections",
	}
	googleConnections = store
	if authURL != "" {
		googleOAuthAuthURL = authURL
	}
	if tokenURL != "" {
		googleOAuthTokenURL = tokenURL
	}
	if apiBaseURL != "" {
		googleCalendarAPIBaseURL = apiBaseURL
	}

	t.Cleanup(func() {
		configData = previousConfig
		googleConnections = previousStore
		googleOAuthAuthURL = previousAuthURL
		googleOAuthTokenURL = previousTokenURL
		googleCalendarAPIBaseURL = previousAPIBaseURL
	})
}

