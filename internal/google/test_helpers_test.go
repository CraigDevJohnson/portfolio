package google

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/schedule"
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

func testMislabelledLPSZuluTime(at time.Time) string {
	return at.In(schedule.MountainTimeLocation).Format("2006-01-02T15:04:05.000") + "Z"
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

func (b *stubSoccerBridge) RenderLoginFeedback(w http.ResponseWriter, kind, message string) {
	b.lastFeedbackKind = kind
	b.lastFeedbackMessage = message
	_, _ = w.Write([]byte(message))
}

func (b *stubSoccerBridge) RequestedScheduleGames(_ context.Context, _ *types.SessionData, _ []int, _ string) ([]types.Game, error) {
	return b.games, nil
}

func (b *stubSoccerBridge) SelectedScheduleGames(games []types.Game, selectedIDs map[string]struct{}) []types.Game {
	var filtered []types.Game
	for i := range games {
		if _, ok := selectedIDs[games[i].ID]; ok {
			filtered = append(filtered, games[i])
		}
	}
	return filtered
}

func (b *stubSoccerBridge) GoogleAddScheduleErrorMessage(_ error) string {
	return "schedule error"
}

func (b *stubSoccerBridge) ParseSelectedIDs(form url.Values) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, id := range form["selected"] {
		ids[id] = struct{}{}
	}
	return ids
}

func (b *stubSoccerBridge) ParsePlayerIDs(_ []string) []int {
	return nil
}
