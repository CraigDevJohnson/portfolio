package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
	"portfolio/types"
)

const (
	grantWorldConnectionID = "owner-connection"
	grantWorldPendingState = "pending-google-state"
	otherSiteSubject       = "other-subject"
	otherSiteEmail         = "visitor@example.com"
)

// soccerGrantWorld is the real route assembly with site identity read from a
// reviewed invitation map, a fake LPS API and fake Google APIs that count the
// calls they receive, and an in-memory Google connection store holding the
// owner's connection.
type soccerGrantWorld struct {
	app                *App
	mux                http.Handler
	lpsCredentialCalls atomic.Int32
	googleCalls        atomic.Int32
	googleTokenCalls   atomic.Int32
	store              *appTestGoogleConnectionStore
	jwt                string
}

func newSoccerGrantWorld(t *testing.T, invitations map[string][]string) *soccerGrantWorld {
	t.Helper()
	application := newTestApp(t)
	enableTestSiteIdentity(application, invitations)
	return newSoccerGrantWorldFor(t, application)
}

// newSoccerGrantWorldFor attaches the fake LPS and Google services and the
// owner's stored Google connection to an app whose site identity is already
// configured, then assembles its routes.
func newSoccerGrantWorldFor(t *testing.T, application *App) *soccerGrantWorld {
	t.Helper()
	world := &soccerGrantWorld{app: application, jwt: testutil.TestJWT(t, time.Now().Add(time.Hour))}

	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			world.lpsCredentialCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/check":
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true}],"user_players":[{"player_id":1001,"deleted":false}]}`))
		case "/players/1001/my_teams":
			_, _ = w.Write([]byte(`[{"UTeamID":4101,"team_name":"Craig FC","Season":169}]`))
		case "/teams/4101":
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":7001,"SchedGameDateTime":%q,"field_name":"Field 3","UTeam1":4101,"UTeam2":4102,"home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4102,"team_name":"Rivals"},"Season":169}]}`, future)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lps.Close)
	world.app.Config.LPSAPIBaseURL = lps.URL

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		world.googleCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			world.googleTokenCalls.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`))
		case r.URL.Path == "/calendar/v3/users/me/calendarList":
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true,"accessRole":"owner"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/calendar/v3/calendars/primary/events":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/calendar/v3/calendars/primary/events":
			_, _ = w.Write([]byte(`{"id":"inserted-event"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(google.Close)
	world.app.Config.GoogleClientID = "google-client"
	world.app.Config.GoogleClientSecret = "google-secret"
	world.app.Config.GoogleConnectionTableName = "connections"
	world.app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	world.app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	world.app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	ciphertext, err := world.app.GoogleHandler.EncryptToken(&oauth2.Token{AccessToken: "owner-access", RefreshToken: "owner-refresh", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	world.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{
		grantWorldConnectionID: {
			ConnectionID: grantWorldConnectionID, OwnerIssuer: testSiteIssuer, OwnerSubject: testSiteSubject,
			TokenCiphertext: ciphertext, CalendarID: "primary", CalendarSummary: "Primary Calendar",
		},
	}}
	world.app.GoogleHandler.SetStore(world.store)
	world.mux, _ = buildMux(world.app, world.app.Logger, false)
	return world
}

// ownerPrivateState returns the owner's imported LPS access, Google
// connection, and pending Google consent as the owner's browser holds them.
func (world *soccerGrantWorld) ownerPrivateState(t *testing.T) []*http.Cookie {
	t.Helper()
	imported := &types.SessionData{
		JWT:       world.jwt,
		UserName:  "Craig Johnson",
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(time.Hour),
	}
	pending := httptest.NewRecorder()
	err := world.app.GoogleHandler.SetOAuthStateCookie(pending, httptest.NewRequest(http.MethodGet, "https://app.example.com/soccer/google/connect", nil), &internalgoogle.OAuthState{
		ConnectionID: "pending-connection", OwnerIssuer: testSiteIssuer, OwnerSubject: testSiteSubject,
		State: grantWorldPendingState, ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(pending.Result().Cookies(),
		&http.Cookie{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, world.app, ownedBySiteVisitor(imported))},
		&http.Cookie{Name: config.GoogleConnectionCookieName, Value: grantWorldConnectionID},
	)
}

type soccerGrantRoute struct {
	name          string
	method, path  string
	form          url.Values
	grantedStatus int
}

// soccerPrivateRoutes lists every imported-player and Google endpoint with the
// status its handler returns once the grant gate admits the request.
var soccerPrivateRoutes = []soccerGrantRoute{
	{"LPS import", http.MethodPost, "/soccer/import", url.Values{"jwt": {"not-a-jwt"}}, http.StatusOK},
	{"clear LPS import", http.MethodPost, "/soccer/logout", url.Values{}, http.StatusOK},
	{"linked-player team discovery", http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001"}}, http.StatusOK},
	{"linked-player schedule", http.MethodPost, "/soccer/fetch", url.Values{"player_ids": {"1001"}}, http.StatusOK},
	{"discovered-team schedule", http.MethodPost, "/soccer/fetch", url.Values{"selection_mode": {"teams"}, "player_ids": {"1001"}, "team_ids": {"4101"}}, http.StatusOK},
	{"linked-player ICS", http.MethodPost, "/soccer/download", url.Values{"player_ids": {"1001"}, "selected": {"7001"}}, http.StatusOK},
	{"Google connect", http.MethodGet, "/soccer/google/connect", nil, http.StatusSeeOther},
	{"Google consent callback", http.MethodGet, "/soccer?code=auth-code&state=" + grantWorldPendingState, nil, http.StatusSeeOther},
	{"Google add", http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, http.StatusOK},
	{"Google result sync", http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, http.StatusOK},
	{"Google calendar choice", http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"primary"}}, http.StatusOK},
	{"Google disconnect", http.MethodPost, "/soccer/google/disconnect", url.Values{}, http.StatusOK},
}

// privateSoccerPageMarkers are controls and states that only a visitor whose
// private Soccer actions are admitted may see.
var privateSoccerPageMarkers = []string{
	"Imported for this session", "Import access", `hx-post="/soccer/logout"`, "data-open-login-modal",
	"Calendar ready", "Connect Google Calendar", `href="/soccer/google/connect"`,
}

func TestSoccerGrantDecidesEveryPrivateRouteLikeThePage(t *testing.T) {
	for _, visitor := range []struct {
		name        string
		invitations map[string][]string
		siteSession func(t *testing.T, app *App) *http.Cookie
		// deniedStatus is the refusal for private routes; zero means admitted.
		deniedStatus int
		notice       string
	}{
		{
			name:         "signed out holding the owner's private state",
			invitations:  map[string][]string{testSiteEmail: {"soccer"}},
			deniedStatus: http.StatusUnauthorized,
			notice:       "Sign in with an invited account",
		},
		{
			name:        "invited without the soccer grant",
			invitations: map[string][]string{testSiteEmail: {"soccer"}, otherSiteEmail: {}},
			siteSession: func(t *testing.T, app *App) *http.Cookie {
				return testSiteSessionCookie(t, app, otherSiteSubject, otherSiteEmail)
			},
			deniedStatus: http.StatusForbidden,
			notice:       "has not been granted",
		},
		{
			name:        "owner after the soccer grant was revoked",
			invitations: map[string][]string{testSiteEmail: {"management"}},
			siteSession: func(t *testing.T, app *App) *http.Cookie {
				return testSiteSessionCookie(t, app, testSiteSubject, testSiteEmail)
			},
			deniedStatus: http.StatusForbidden,
			notice:       "has not been granted",
		},
		{
			name:        "owner with the soccer grant",
			invitations: map[string][]string{testSiteEmail: {"soccer"}},
			siteSession: func(t *testing.T, app *App) *http.Cookie {
				return testSiteSessionCookie(t, app, testSiteSubject, testSiteEmail)
			},
		},
	} {
		t.Run(visitor.name, func(t *testing.T) {
			cookiesFor := func(world *soccerGrantWorld) []*http.Cookie {
				cookies := world.ownerPrivateState(t)
				if visitor.siteSession != nil {
					cookies = append(cookies, visitor.siteSession(t, world.app))
				}
				return cookies
			}

			world := newSoccerGrantWorld(t, visitor.invitations)
			cookies := cookiesFor(world)
			page := soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil, cookies...)
			fetch := soccerGrantRequest(world.mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}}, cookies...)
			ics := soccerGrantRequest(world.mux, http.MethodPost, "/soccer/download", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, cookies...)
			if page.Code != http.StatusOK {
				t.Fatalf("Soccer page status = %d", page.Code)
			}
			if fetch.Code != http.StatusOK || !strings.Contains(fetch.Body.String(), "Craig FC") {
				t.Errorf("public Team ID fetch: status %d, body %q", fetch.Code, fetch.Body.String())
			}
			if ics.Code != http.StatusOK || ics.Header().Get("Content-Type") != "text/calendar" || !strings.Contains(ics.Body.String(), "BEGIN:VEVENT") {
				t.Errorf("public ICS download: status %d, body %q", ics.Code, ics.Body.String())
			}

			body := page.Body.String()
			admitted := visitor.deniedStatus == 0
			if admitted {
				for _, marker := range []string{"Imported for this session", `hx-post="/soccer/logout"`, "Calendar ready", `href="/soccer/google/connect"`} {
					if !strings.Contains(body, marker) {
						t.Errorf("granted page lacks private control %q", marker)
					}
				}
				if strings.Contains(body, "Private Soccer access") {
					t.Error("granted page explained a missing grant")
				}
			} else {
				for _, marker := range privateSoccerPageMarkers {
					if strings.Contains(body, marker) {
						t.Errorf("page offered private control %q to a visitor the routes refuse", marker)
					}
				}
				if !strings.Contains(body, visitor.notice) {
					t.Errorf("page did not explain private access with %q", visitor.notice)
				}
				if calls := world.lpsCredentialCalls.Load() + world.googleCalls.Load(); calls != 0 {
					t.Errorf("public page and Team ID requests used the owner's private credentials %d time(s)", calls)
				}
			}

			for _, route := range soccerPrivateRoutes {
				// Each private action gets a fresh world so one action's side
				// effects, such as disconnect, cannot decide another's outcome.
				world := newSoccerGrantWorld(t, visitor.invitations)
				resp := soccerGrantRequest(world.mux, route.method, route.path, route.form, cookiesFor(world)...)
				if admitted {
					if resp.Code != route.grantedStatus {
						t.Errorf("%s: granted status = %d, want %d; body %q", route.name, resp.Code, route.grantedStatus, resp.Body.String())
					}
					continue
				}
				if resp.Code != visitor.deniedStatus {
					t.Errorf("%s: status = %d, want %d", route.name, resp.Code, visitor.deniedStatus)
				}
				if calls := world.lpsCredentialCalls.Load() + world.googleCalls.Load(); calls != 0 {
					t.Errorf("%s: refused request reached LPS or Google %d time(s)", route.name, calls)
				}
				if _, kept := world.store.records[grantWorldConnectionID]; !kept {
					t.Errorf("%s: refused request deleted the owner's Google connection", route.name)
				}
			}
		})
	}
}

func TestGrantedVisitorCannotInheritOwnerlessOrAnotherOwnersPrivateState(t *testing.T) {
	invitations := map[string][]string{testSiteEmail: {"soccer"}, otherSiteEmail: {"soccer"}}
	for _, tc := range []struct {
		name    string
		cookies func(t *testing.T, world *soccerGrantWorld) []*http.Cookie
	}{
		{
			name: "legacy ownerless import and Google connection",
			cookies: func(t *testing.T, world *soccerGrantWorld) []*http.Cookie {
				legacy := world.store.records[grantWorldConnectionID]
				legacy.ConnectionID, legacy.OwnerIssuer, legacy.OwnerSubject = "legacy-connection", "", ""
				world.store.records[legacy.ConnectionID] = legacy
				imported := &types.SessionData{
					JWT:       world.jwt,
					Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Legacy", LastName: "Player", IsMainPlayer: true}},
					ExpiresAt: time.Now().Add(time.Hour),
				}
				return []*http.Cookie{
					testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail),
					{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, world.app, imported)},
					{Name: config.GoogleConnectionCookieName, Value: legacy.ConnectionID},
				}
			},
		},
		{
			name: "another granted visitor holding the owner's state",
			cookies: func(t *testing.T, world *soccerGrantWorld) []*http.Cookie {
				return append(world.ownerPrivateState(t), testSiteSessionCookie(t, world.app, otherSiteSubject, otherSiteEmail))
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newSoccerGrantWorld(t, invitations)
			page := soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil, tc.cookies(t, world)...)
			if page.Code != http.StatusOK {
				t.Fatalf("Soccer page status = %d", page.Code)
			}
			for _, inherited := range []string{"Imported for this session", "Legacy Player", "Calendar ready"} {
				if strings.Contains(page.Body.String(), inherited) {
					t.Errorf("page presented inherited private state %q", inherited)
				}
			}
			if cleared := findSessionCookie(t, page.Result()); cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
				t.Error("page kept an imported LPS session this visitor does not own")
			}

			for _, route := range []struct {
				method, path string
				form         url.Values
				status       int
				want         string
			}{
				{http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001"}}, http.StatusOK, "Import a bearer JWT to discover teams."},
				{http.MethodPost, "/soccer/fetch", url.Values{"player_ids": {"1001"}}, http.StatusOK, "Import a bearer JWT again to fetch schedules for your discovered players."},
				{http.MethodPost, "/soccer/download", url.Values{"player_ids": {"1001"}, "selected": {"7001"}}, http.StatusUnauthorized, "import a bearer JWT again"},
				{http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, http.StatusOK, "Connect Google Calendar before adding selected games."},
				{http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, http.StatusOK, "Connect Google Calendar before syncing results."},
				{http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"primary"}}, http.StatusOK, "Connect Google Calendar"},
				{http.MethodPost, "/soccer/google/disconnect", url.Values{}, http.StatusOK, "Connect Google Calendar"},
			} {
				world := newSoccerGrantWorld(t, invitations)
				cookies := tc.cookies(t, world)
				stored := len(world.store.records)
				resp := soccerGrantRequest(world.mux, route.method, route.path, route.form, cookies...)
				if resp.Code != route.status || !strings.Contains(resp.Body.String(), route.want) {
					t.Errorf("%s %s = %d, want %d with %q; body %q", route.method, route.path, resp.Code, route.status, route.want, resp.Body.String())
				}
				if calls := world.lpsCredentialCalls.Load() + world.googleCalls.Load(); calls != 0 {
					t.Errorf("%s %s used inherited LPS or Google credentials %d time(s)", route.method, route.path, calls)
				}
				if len(world.store.records) != stored {
					t.Errorf("%s %s changed a Google connection this visitor does not own", route.method, route.path)
				}
			}
		})
	}
}
