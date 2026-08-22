package soccer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/session"
	"portfolio/internal/testutil"
)

// blockingSoccerStore models an audit write that cannot finish until the test releases it.
type blockingSoccerStore struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingSoccerStore() *blockingSoccerStore {
	return &blockingSoccerStore{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (s *blockingSoccerStore) Put(_ context.Context, _ *SoccerSessionRecord) error {
	s.startedOnce.Do(func() { close(s.started) })
	<-s.release
	return nil
}

func (s *blockingSoccerStore) Release() {
	s.releaseOnce.Do(func() { close(s.release) })
}

type failingSoccerStore struct{ err error }

func (s failingSoccerStore) Put(_ context.Context, _ *SoccerSessionRecord) error { return s.err }

// TestImportHandlerWaitsForSessionPersistence catches a detached audit write that
// lets a Lambda invocation return before its imported-session baseline is written.
func TestImportHandlerWaitsForSessionPersistence(t *testing.T) {
	store := newBlockingSoccerStore()
	t.Cleanup(store.Release)
	handler, token := newImportHandlerForTest(t, store)
	req := newImportRequest(token)
	resp := httptest.NewRecorder()
	completed := make(chan struct{})

	go func() {
		handler.ImportHandler(resp, req)
		close(completed)
	}()

	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("session persistence did not start")
	}

	select {
	case <-completed:
		t.Fatal("import response completed before session persistence was released")
	case <-time.After(100 * time.Millisecond):
	}

	store.Release()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("import response did not complete after session persistence was released")
	}

	if body := resp.Body.String(); !strings.Contains(body, "data-login-success") {
		t.Fatalf("expected normal import success fragment, got %q", body)
	}
}

// TestImportHandlerPreservesSuccessWhenSessionPersistenceFails catches audit-error
// handling that incorrectly turns an otherwise successful browser-session import into a failure.
func TestImportHandlerPreservesSuccessWhenSessionPersistenceFails(t *testing.T) {
	handler, token := newImportHandlerForTest(t, failingSoccerStore{err: errors.New("DynamoDB unavailable")})
	resp := httptest.NewRecorder()

	handler.ImportHandler(resp, newImportRequest(token))

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if body := resp.Body.String(); !strings.Contains(body, "data-login-success") {
		t.Fatalf("expected normal import success fragment, got %q", body)
	}
}

func newImportHandlerForTest(t *testing.T, store SoccerStore) (*Handler, string) {
	t.Helper()

	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/check" {
			t.Fatalf("unexpected LPS path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"first_name": "Craig",
			"last_name": "Johnson",
			"players": [{"UPlayerID": 1001, "FirstName": "Craig", "LastName": "Johnson", "is_main_player": true}],
			"user_players": [{"player_id": 1001, "deleted": false}]
		}`)
	}))
	t.Cleanup(server.Close)

	cfg := &config.Config{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	limiter := session.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys)
	t.Cleanup(limiter.Close)

	return NewHandler(
		cfg,
		server.Client(),
		limiter,
		nil,
		store,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	), token
}

func newImportRequest(token string) *http.Request {
	form := url.Values{"jwt": {"Bearer " + token}}
	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}
