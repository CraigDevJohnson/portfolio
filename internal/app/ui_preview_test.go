package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"portfolio/internal/testutil"
)

var soccerPreviewFixtureNamesForTest = []string{
	"manual", "import", "token-invalid", "token-expired", "token-rejected", "token-upstream-error",
	"players", "no-players", "team-selection", "no-games", "upcoming", "past", "combined",
	"google-disconnected", "google-connected", "google-add-success", "google-add-error",
	"google-sync-success", "google-sync-error", "expired-session-reset", "loading",
}

func TestLocalPreviewSkipsLiveStoreInitialization(t *testing.T) {
	serverSource := readTask2Artifact(t, "internal", "app", "server.go")
	for _, guard := range []string{
		"if !localPortalPreview && app.Config.GoogleEnabled()",
		"if !localPortalPreview && app.Config.SoccerSessionEnabled()",
	} {
		if !strings.Contains(serverSource, guard) {
			t.Errorf("local preview startup lacks live-store guard %q", guard)
		}
	}
}

func TestSoccerPreviewFixtureCoverage(t *testing.T) {
	if !reflect.DeepEqual(soccerPreviewFixtureNames, soccerPreviewFixtureNamesForTest) {
		t.Fatalf("preview fixture names = %#v, want exact closed 21-name set %#v", soccerPreviewFixtureNames, soccerPreviewFixtureNamesForTest)
	}
	if got := len(soccerPreviewFixtureNames); got != 21 {
		t.Fatalf("preview fixture count = %d, want 21", got)
	}
	type fixtureState struct {
		authenticated, loginAvailable bool
		modalOpen                     bool
		players, teamGroups           int
		upcoming, past                int
		googleAvailable               bool
		googleConnected               bool
		calendars                     int
		feedback                      string
		loading                       bool
	}
	wantStates := map[string]fixtureState{
		"manual":                {},
		"import":                {loginAvailable: true},
		"token-invalid":         {loginAvailable: true, modalOpen: true, feedback: "modal:error:Token format is invalid"},
		"token-expired":         {loginAvailable: true, modalOpen: true, feedback: "modal:error:Token expired"},
		"token-rejected":        {loginAvailable: true, modalOpen: true, feedback: "modal:rejected:Token rejected"},
		"token-upstream-error":  {loginAvailable: true, modalOpen: true, feedback: "modal:upstream:Player lookup unavailable"},
		"players":               {authenticated: true, loginAvailable: true, players: 2},
		"no-players":            {authenticated: true, loginAvailable: true},
		"team-selection":        {authenticated: true, loginAvailable: true, players: 2, teamGroups: 2},
		"no-games":              {loginAvailable: true},
		"upcoming":              {upcoming: 2},
		"past":                  {past: 2},
		"combined":              {upcoming: 2, past: 2},
		"google-disconnected":   {authenticated: true, loginAvailable: true, players: 2, upcoming: 2, past: 2, googleAvailable: true},
		"google-connected":      {authenticated: true, loginAvailable: true, players: 2, upcoming: 2, past: 2, googleAvailable: true, googleConnected: true, calendars: 2},
		"google-add-success":    {authenticated: true, loginAvailable: true, players: 2, upcoming: 2, past: 2, googleAvailable: true, googleConnected: true, calendars: 2, feedback: "results:success:Selected games added"},
		"google-add-error":      {authenticated: true, loginAvailable: true, players: 2, upcoming: 2, past: 2, googleAvailable: true, googleConnected: true, calendars: 2, feedback: "results:google-error:Selected games were not added"},
		"google-sync-success":   {authenticated: true, loginAvailable: true, players: 2, past: 2, googleAvailable: true, googleConnected: true, calendars: 2, feedback: "results:success:Selected results synced"},
		"google-sync-error":     {authenticated: true, loginAvailable: true, players: 2, past: 2, googleAvailable: true, googleConnected: true, calendars: 2, feedback: "results:google-error:Selected results were not synced"},
		"expired-session-reset": {loginAvailable: true, feedback: "page:error:Imported session expired"},
		"loading":               {loginAvailable: true, loading: true},
	}
	for _, name := range soccerPreviewFixtureNamesForTest {
		fixture, ok := soccerPreviewFixture(name)
		if !ok {
			t.Errorf("soccerPreviewFixture(%q) did not resolve", name)
			continue
		}
		if !fixture.Page.Preview || !fixture.Page.AuthState.Preview {
			t.Errorf("soccerPreviewFixture(%q) is not explicitly preview-safe", name)
		}
		got := fixtureState{
			authenticated:   fixture.Page.AuthState.Authenticated,
			loginAvailable:  fixture.Page.AuthState.LoginAvailable,
			modalOpen:       fixture.Page.ModalOpen,
			players:         len(fixture.Page.AuthState.Players),
			googleAvailable: fixture.Page.AuthState.GoogleAvailable,
			googleConnected: fixture.Page.AuthState.GoogleConnected,
			calendars:       len(fixture.Page.AuthState.GoogleCalendars),
			loading:         fixture.Loading,
		}
		if fixture.TeamSelection != nil {
			got.teamGroups = len(fixture.TeamSelection.PlayerGroups)
		}
		if fixture.Results != nil {
			got.upcoming = len(fixture.Results.UpcomingGames)
			got.past = len(fixture.Results.PastGames)
			if fixture.Results.GoogleFeedback != nil {
				got.feedback = "results:" + fixture.Results.GoogleFeedback.Kind + ":" + fixture.Results.GoogleFeedback.Title
			}
		}
		if fixture.Page.ModalFeedback != nil {
			got.feedback = "modal:" + fixture.Page.ModalFeedback.Kind + ":" + fixture.Page.ModalFeedback.Title
		}
		if fixture.Feedback != nil {
			got.feedback = "page:" + fixture.Feedback.Kind + ":" + fixture.Feedback.Title
		}
		if want := wantStates[name]; !reflect.DeepEqual(got, want) {
			t.Errorf("soccerPreviewFixture(%q) state = %#v, want %#v", name, got, want)
		}
	}
	if _, ok := soccerPreviewFixture("production"); ok {
		t.Fatal("production unexpectedly resolves as a 22nd preview fixture")
	}
	if _, ok := soccerPreviewFixture("unknown"); ok {
		t.Fatal("unknown preview fixture did not fail closed")
	}
}

func TestSoccerPreviewRoutesRequireSafeMode(t *testing.T) {
	app := newTestApp(t)
	safeMux, _ := buildMux(app, app.Logger, true)
	normalMux, _ := buildMux(app, app.Logger, false)

	for _, name := range soccerPreviewFixtureNamesForTest {
		assertSoccerPreviewStatus(t, safeMux, http.MethodGet, "/__preview/soccer/"+name, "", http.StatusOK)
		assertSoccerPreviewStatus(t, normalMux, http.MethodGet, "/__preview/soccer/"+name, "", http.StatusNotFound)
	}
	assertSoccerPreviewStatus(t, safeMux, http.MethodGet, "/__preview/soccer/not-a-fixture", "", http.StatusNotFound)
	assertSoccerPreviewStatus(t, normalMux, http.MethodPost, "/__preview/soccer/download", url.Values{"selected": {"preview-upcoming-1"}}.Encode(), http.StatusNotFound)
}

func TestSoccerPreviewRoutesUseNoExternalDependencies(t *testing.T) {
	app := newTestApp(t)
	var calls atomic.Int32
	app.LPSClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("preview attempted an external request")
	})
	mux, _ := buildMux(app, app.Logger, true)

	for _, name := range soccerPreviewFixtureNamesForTest {
		assertSoccerPreviewStatus(t, mux, http.MethodGet, "/__preview/soccer/"+name, "", http.StatusOK)
	}
	assertSoccerPreviewStatus(t, mux, http.MethodPost, "/__preview/soccer/download", url.Values{
		"selected":   {"preview-upcoming-1"},
		"team_codes": {"479691"},
	}.Encode(), http.StatusOK)
	if got := calls.Load(); got != 0 {
		t.Fatalf("preview routes made %d external call(s), want 0", got)
	}
}

func TestSoccerPreviewActionsAreInert(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, app.Logger, true)

	for _, name := range soccerPreviewFixtureNamesForTest {
		req := httptest.NewRequest(http.MethodGet, "/__preview/soccer/"+name, nil)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		body := resp.Body.String()
		for _, forbidden := range []string{
			`hx-post="/soccer/`, `hx-get="/soccer/`, `hx-put="/soccer/`, `hx-delete="/soccer/`,
			`action="/soccer/`, `href="/soccer/google/`,
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("fixture %q contains actionable production marker %q", name, forbidden)
			}
		}
		if strings.Contains(body, `action="/__preview/soccer/download"`) && !strings.Contains(body, `data-native-download`) {
			t.Errorf("fixture %q preview download is not the native ICS form", name)
		}
	}
}

func TestSoccerPreviewDownloadUsesInMemoryGames(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, app.Logger, true)
	form := url.Values{
		"selected":   {"preview-upcoming-1"},
		"team_codes": {"479691"},
		"player_ids": {"1669080"},
	}
	req := httptest.NewRequest(http.MethodPost, "/__preview/soccer/download", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("preview download status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); got != "text/calendar" {
		t.Errorf("preview download Content-Type = %q, want text/calendar", got)
	}
	if got := resp.Header().Get("Content-Disposition"); got != "attachment; filename=soccer_schedule.ics" {
		t.Errorf("preview download Content-Disposition = %q", got)
	}
	unfolded := testutil.UnfoldICS(resp.Body.String())
	for _, marker := range []string{"BEGIN:VCALENDAR", "BEGIN:VEVENT", "UID:preview-upcoming-1", "END:VEVENT", "END:VCALENDAR"} {
		if !strings.Contains(unfolded, marker) {
			t.Errorf("preview ICS lacks %q: %s", marker, unfolded)
		}
	}
	if strings.Contains(unfolded, "UID:preview-upcoming-2") {
		t.Errorf("preview ICS contains unselected game: %s", unfolded)
	}
}

func TestSoccerPreviewDownloadRejectsUnknownGames(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, app.Logger, true)
	tests := []url.Values{
		{},
		{"selected": {"unknown-game"}},
		{"selected": {"preview-upcoming-1", "unknown-game"}},
	}
	for _, form := range tests {
		assertSoccerPreviewStatus(t, mux, http.MethodPost, "/__preview/soccer/download", form.Encode(), http.StatusBadRequest)
	}
}

func assertSoccerPreviewStatus(t *testing.T, handler http.Handler, method, path, body string, want int) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != want {
		t.Fatalf("%s %s status = %d, want %d: %s", method, path, resp.Code, want, resp.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
