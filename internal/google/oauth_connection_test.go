package google

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
)

func TestPopulateLoginStateClearsRevokedGoogleConnection(t *testing.T) {
	store := &fakeConnectionStore{records: map[string]ConnectionRecord{}}
	h := newTestHandler(t, store)

	tokenCiphertext, err := h.EncryptToken(&oauth2.Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("EncryptToken returned error: %v", err)
	}
	store.records["connection-1"] = ConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	}))
	defer tokenServer.Close()

	h.OAuthTokenURL = tokenServer.URL + "/oauth/token"

	req := httptest.NewRequest(http.MethodGet, "/soccer/session", nil)
	req.Host = "example.com"
	req.AddCookie(&http.Cookie{Name: config.GoogleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()
	props := partials.SoccerLoginStateProps{GoogleAvailable: true}

	h.PopulateLoginState(req.Context(), resp, req, &props)

	if props.GoogleConnected {
		t.Fatal("expected revoked Google connection to be cleared")
	}
	if _, ok := store.records["connection-1"]; ok {
		t.Fatal("expected revoked Google connection record to be deleted")
	}
	assertClearedConnectionCookie(t, resp.Result())
}

func assertClearedConnectionCookie(t *testing.T, resp *http.Response) {
	t.Helper()
	var connectionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName {
			connectionCookie = cookie
			break
		}
	}
	if connectionCookie == nil {
		t.Fatal("expected cleared google connection cookie to be set")
	}
	if connectionCookie.Value != "" {
		t.Fatalf("expected cleared google connection cookie value to be empty, got %q", connectionCookie.Value)
	}
	if connectionCookie.MaxAge >= 0 {
		t.Fatalf("expected cleared google connection cookie max-age to be negative, got %d", connectionCookie.MaxAge)
	}
	if !connectionCookie.Expires.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected cleared google connection cookie expiry to be Unix epoch, got %v", connectionCookie.Expires)
	}
}
