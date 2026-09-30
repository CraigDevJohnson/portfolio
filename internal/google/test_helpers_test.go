package google

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/siteidentity"
	"portfolio/types"
)

// The verified site owner these tests act as. Owner-bound connections carry
// these coordinates, and the route assembly attaches this identity to every
// request it serves.
const (
	testOwnerIssuer  = "https://issuer.example.com/pool"
	testOwnerSubject = "owner-subject"
	testOwnerEmail   = "owner@example.com"
)

// The Google account that consented to Calendar access for stored
// connections, verified by Google rather than taken from the site sign-in.
const (
	testAccountSubject = "google-account-subject"
	testAccountEmail   = "calendar@example.com"
)

// ownerConnectionCookie is the test owner's Google connection cookie naming
// the given connection.
func ownerConnectionCookie(connectionID string) *http.Cookie {
	return &http.Cookie{Name: ConnectionCookieName(testOwnerIssuer, testOwnerSubject), Value: connectionID}
}

// asGrantedSoccerOwner gives a request that reaches a Google handler directly
// the site identity of the test owner holding the soccer grant.
func asGrantedSoccerOwner(req *http.Request) *http.Request {
	principal := &siteidentity.Principal{Issuer: testOwnerIssuer, Subject: testOwnerSubject, Email: testOwnerEmail}
	ctx := siteidentity.WithRequestIdentity(req.Context(), principal, []siteidentity.Grant{siteidentity.GrantSoccer}, "/soccer")
	return req.WithContext(ctx)
}

func newTestHandler(t *testing.T, store ConnectionStore) *Handler {
	t.Helper()
	cfg := &config.Config{
		SessionKey:                []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL:             config.DefaultLPSAPIBaseURL,
		GoogleClientID:            "google-client-id",
		GoogleClientSecret:        "google-client-secret",
		GoogleConnectionTableName: "google-connections",
	}
	if store == nil {
		store = &fakeConnectionStore{records: map[string]ConnectionRecord{}}
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := NewHandler(cfg, &http.Client{Timeout: 5 * time.Second}, logger, &stubSoccerBridge{})
	// A closed loopback address stands in for every Google endpoint until a
	// test attaches its own fake, so no test can reach live Google.
	const unreachableGoogle = "http://127.0.0.1:1"
	h.OAuthAuthURL = unreachableGoogle + "/oauth/authorize"
	h.OAuthTokenURL = unreachableGoogle + "/oauth/token"
	h.OAuthUserInfoURL = unreachableGoogle + "/userinfo"
	h.CalendarAPIBaseURL = unreachableGoogle + "/calendar/v3"
	h.SetStore(store)
	return h
}

func newTestHandlerWithURLs(t *testing.T, store ConnectionStore, authURL, tokenURL, apiBaseURL string) *Handler {
	t.Helper()
	h := newTestHandler(t, store)
	if authURL != "" {
		h.OAuthAuthURL = authURL
	}
	if tokenURL != "" {
		h.OAuthTokenURL = tokenURL
	}
	if apiBaseURL != "" {
		h.CalendarAPIBaseURL = apiBaseURL
	}
	return h
}

type fakeConnectionStore struct {
	records map[string]ConnectionRecord
}

func (s *fakeConnectionStore) Delete(_ context.Context, connectionID string) error {
	delete(s.records, connectionID)
	return nil
}

func (s *fakeConnectionStore) Get(_ context.Context, connectionID string) (*ConnectionRecord, error) {
	record, ok := s.records[connectionID]
	if !ok {
		return nil, nil
	}
	clone := record
	return &clone, nil
}

func (s *fakeConnectionStore) Put(_ context.Context, record *ConnectionRecord) error {
	s.records[record.ConnectionID] = *record
	return nil
}

type stubSoccerBridge struct {
	lastFeedbackKind       string
	lastFeedbackMessage    string
	loginStateOOBCalls     int
	loginStateRefreshCalls int
	session                *types.SessionData
	lastRefreshSession     *types.SessionData
	games                  []types.Game
	syncResultsGames       []types.Game
	syncResultsMessage     string
}

func (b *stubSoccerBridge) LoadSession(_ http.ResponseWriter, _ *http.Request) (*types.SessionData, bool) {
	return b.session, false
}

func (b *stubSoccerBridge) RenderLoginStateOOB(w http.ResponseWriter, _ *http.Request, _ *types.SessionData) {
	b.loginStateOOBCalls++
	_, _ = w.Write([]byte("login-state-oob-rendered"))
}

func (b *stubSoccerBridge) RenderLoginStateRefresh(w http.ResponseWriter, _ *http.Request, session *types.SessionData) {
	b.loginStateRefreshCalls++
	b.lastRefreshSession = session
	_, _ = w.Write([]byte("login-state-refresh-rendered"))
}

func (b *stubSoccerBridge) RenderLoginFeedback(w http.ResponseWriter, _ *http.Request, kind, message string) {
	b.lastFeedbackKind = kind
	b.lastFeedbackMessage = message
	_, _ = w.Write([]byte(message))
}

func (b *stubSoccerBridge) ResolveGoogleAddSelection(_ http.ResponseWriter, r *http.Request) (*types.SessionData, []types.Game, string, bool) {
	selectedIDs := map[string]struct{}{}
	for _, id := range r.Form["selected"] {
		if id == "" {
			continue
		}
		selectedIDs[id] = struct{}{}
	}
	if len(selectedIDs) == 0 {
		return nil, nil, "Select at least one game to add to Google Calendar.", false
	}

	var filtered []types.Game
	for i := range b.games {
		if _, ok := selectedIDs[b.games[i].ID]; ok {
			filtered = append(filtered, b.games[i])
		}
	}
	if len(filtered) == 0 {
		return nil, nil, "No selected games were found to add.", false
	}
	return nil, filtered, "", true
}

func (b *stubSoccerBridge) ResolveSyncResultsGames(_ http.ResponseWriter, _ *http.Request) (*types.SessionData, []types.Game, string, bool) {
	if b.syncResultsMessage != "" {
		return nil, nil, b.syncResultsMessage, false
	}
	if b.syncResultsGames != nil {
		return nil, b.syncResultsGames, "", true
	}
	return nil, nil, "", true
}
