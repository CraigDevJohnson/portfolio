package google

import (
	"context"
	"net/http"
	"testing"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/types"
)

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
	h := NewHandler(cfg, &http.Client{Timeout: 5 * time.Second}, &stubSoccerBridge{})
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
	lastFeedbackKind    string
	lastFeedbackMessage string
	games               []types.Game
	syncResultsGames    []types.Game
	syncResultsMessage  string
}

func (b *stubSoccerBridge) LoadSession(_ http.ResponseWriter, _ *http.Request) (*types.SessionData, bool) {
	return nil, false
}

func (b *stubSoccerBridge) LoginStateProps(_ http.ResponseWriter, _ *http.Request, _ *types.SessionData, _ bool) partials.SoccerLoginStateProps {
	return partials.SoccerLoginStateProps{}
}

func (b *stubSoccerBridge) RenderLoginState(w http.ResponseWriter, _ *http.Request, _ *types.SessionData) {
	_, _ = w.Write([]byte("login-state-rendered"))
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
