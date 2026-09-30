package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portfolio/internal/testutil"
	"portfolio/types"
)

func TestSoccerRoutesPreventCaching(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		authenticated      bool
		wantStatus         int
	}{
		{"signed out", "GET", "/soccer", false, http.StatusOK},
		{"signed in", "GET", "/soccer", true, http.StatusOK},
		{"invalid import", "POST", "/soccer/import", false, http.StatusOK},
		{"logout", "POST", "/soccer/logout", true, http.StatusOK},
		{"wrong method", "GET", "/soccer/fetch", false, http.StatusMethodNotAllowed},
		{"unknown route", "GET", "/soccer/unknown", true, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			handler, _ := buildMux(app, app.Logger, false)
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.authenticated {
				addSessionCookie(t, app, request, &types.SessionData{JWT: testutil.TestJWT(t, time.Now().Add(time.Hour)), UserName: "Cache Test", Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Cache", LastName: "Test", IsMainPlayer: true}}, ExpiresAt: time.Now().Add(time.Hour)})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q", got)
			}
			if tc.name == "signed in" && !strings.Contains(response.Body.String(), "Imported for this session") {
				t.Fatal("authenticated session was not rendered")
			}
		})
	}
}

func TestSoccerCachePolicyDoesNotChangePortfolioResponses(t *testing.T) {
	app := newTestApp(t)
	handler, _ := buildMux(app, app.Logger, false)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/about", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("unrelated cache policy changed to %q", got)
	}
}
