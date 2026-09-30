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
	// refusedFacilityTeamID is a public team whose game is at a facility the
	// fake LPS refuses to describe with 401; refusedFacilityGameID is that game.
	refusedFacilityTeamID = "4109"
	refusedFacilityGameID = "7009"
	// The Google account the fake Google reports as having consented.
	grantWorldGoogleSubject = "owner-google-subject"
	grantWorldGoogleEmail   = "owner.calendar@example.net"
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
	googleEventInserts atomic.Int32
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
		case "/teams/" + refusedFacilityTeamID:
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":%[1]s,"SchedGameDateTime":%[2]q,"field_name":"Field 9","FacilityID":55,"UTeam1":%[3]s,"UTeam2":4102,"home_team":{"UTeamID":%[3]s,"team_name":"Far Venue FC"},"visitor_team":{"UTeamID":4102,"team_name":"Rivals"},"Season":169}]}`, refusedFacilityGameID, future, refusedFacilityTeamID)
		case "/facilities/55":
			// LPS refuses to describe this facility to a request without a token.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		case r.URL.Path == "/userinfo":
			_, _ = fmt.Fprintf(w, `{"sub":%q,"email":%q,"email_verified":true}`, grantWorldGoogleSubject, grantWorldGoogleEmail)
		case r.URL.Path == "/calendar/v3/users/me/calendarList":
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true,"accessRole":"owner"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/calendar/v3/calendars/primary/events":
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/calendar/v3/calendars/primary/events":
			world.googleEventInserts.Add(1)
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
	world.app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"

	ciphertext, err := world.app.GoogleHandler.EncryptToken(&oauth2.Token{AccessToken: "owner-access", RefreshToken: "owner-refresh", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	world.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{
		grantWorldConnectionID: {
			ConnectionID: grantWorldConnectionID, OwnerIssuer: testSiteIssuer, OwnerSubject: testSiteSubject,
			AccountSubject: grantWorldGoogleSubject, AccountEmail: grantWorldGoogleEmail,
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
	cookies := append(pending.Result().Cookies(), importedAccessCookies(t, world.app, ownedBySiteVisitor(imported))...)
	return append(cookies, &http.Cookie{Name: ownerGoogleConnectionName, Value: grantWorldConnectionID})
}

type soccerGrantRoute struct {
	name          string
	method, path  string
	form          url.Values
	grantedStatus int
	// grantedEffect checks what the admitted handler did with the owner's
	// private state, since a status alone cannot tell an admitted action
	// from one that silently lost the owner's connection or import.
	grantedEffect func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder)
}

// soccerPrivateRoutes lists every imported-player and Google endpoint with the
// status and effect its handler produces once the grant gate admits the
// owner's request.
var soccerPrivateRoutes = []soccerGrantRoute{
	{
		name: "LPS import", method: http.MethodPost, path: "/soccer/import", form: url.Values{"jwt": {"not-a-jwt"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, _ *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if !strings.Contains(resp.Body.String(), `role="alert"`) || strings.Contains(resp.Body.String(), "Private Soccer access") {
				t.Errorf("LPS import did not reach JWT validation: %q", resp.Body.String())
			}
		},
	},
	{
		name: "clear LPS import", method: http.MethodPost, path: "/soccer/logout", form: url.Values{}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, _ *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if cleared := findSessionCookie(t, resp.Result()); cleared == nil || cleared.MaxAge >= 0 {
				t.Error("clearing the LPS import kept the owner's imported session")
			}
		},
	},
	{
		name: "linked-player team discovery", method: http.MethodPost, path: "/soccer/discover-teams", form: url.Values{"player_ids": {"1001"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if !strings.Contains(resp.Body.String(), "Craig FC") || world.lpsCredentialCalls.Load() == 0 {
				t.Errorf("team discovery did not use the owner's imported LPS access: %q", resp.Body.String())
			}
		},
	},
	{
		name: "linked-player schedule", method: http.MethodPost, path: "/soccer/fetch", form: url.Values{"player_ids": {"1001"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if !strings.Contains(resp.Body.String(), `value="7001"`) || world.lpsCredentialCalls.Load() == 0 {
				t.Errorf("linked-player schedule did not use the owner's imported LPS access: %q", resp.Body.String())
			}
		},
	},
	{
		name: "discovered-team schedule", method: http.MethodPost, path: "/soccer/fetch", form: url.Values{"selection_mode": {"teams"}, "player_ids": {"1001"}, "team_ids": {"4101"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, _ *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if !strings.Contains(resp.Body.String(), `value="7001"`) {
				t.Errorf("discovered-team schedule did not list the team's game: %q", resp.Body.String())
			}
		},
	},
	{
		name: "linked-player ICS", method: http.MethodPost, path: "/soccer/download", form: url.Values{"player_ids": {"1001"}, "selected": {"7001"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if !strings.Contains(resp.Body.String(), "BEGIN:VEVENT") || world.lpsCredentialCalls.Load() == 0 {
				t.Errorf("linked-player ICS did not use the owner's imported LPS access: %q", resp.Body.String())
			}
		},
	},
	{
		name: "Google connect", method: http.MethodGet, path: "/soccer/google/connect", grantedStatus: http.StatusSeeOther,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if location := resp.Header().Get("Location"); !strings.HasPrefix(location, world.app.GoogleHandler.OAuthAuthURL+"?") {
				t.Errorf("Google connect redirected to %q instead of Google consent", location)
			}
		},
	},
	{
		name: "Google consent callback", method: http.MethodGet, path: "/soccer?code=auth-code&state=" + grantWorldPendingState, grantedStatus: http.StatusSeeOther,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if location := resp.Header().Get("Location"); location != "/soccer?google=connected" {
				t.Errorf("Google consent callback redirected to %q", location)
			}
			if connected := world.store.records["pending-connection"]; connected.OwnerIssuer != testSiteIssuer || connected.OwnerSubject != testSiteSubject {
				t.Errorf("Google consent callback did not store the owner's connection: %#v", connected)
			}
		},
	},
	{
		name: "Google add", method: http.MethodPost, path: "/soccer/google/add", form: url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if world.googleEventInserts.Load() == 0 || strings.Contains(resp.Body.String(), "Connect Google Calendar before") {
				t.Errorf("Google add did not add the game to the owner's calendar: %q", resp.Body.String())
			}
		},
	},
	{
		name: "Google result sync", method: http.MethodPost, path: "/soccer/google/sync-results", form: url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, grantedStatus: http.StatusOK,
		// The selected game is upcoming, so the sync ends at game selection,
		// which it reaches only after loading the owner's connection.
		grantedEffect: func(t *testing.T, _ *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if body := resp.Body.String(); strings.Contains(body, "Connect Google Calendar before") || !strings.Contains(body, "No selected past results were found to sync.") {
				t.Errorf("Google result sync did not reach the owner's connection: %q", body)
			}
		},
	},
	{
		name: "Google calendar choice", method: http.MethodPost, path: "/soccer/google/calendar", form: url.Values{"calendar_id": {"primary"}}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, resp *httptest.ResponseRecorder) {
			if record := world.store.records[grantWorldConnectionID]; record.CalendarID != "primary" || record.CalendarSummary != "Primary Calendar" {
				t.Errorf("Google calendar choice lost the owner's calendar: %#v", record)
			}
			if !strings.Contains(resp.Body.String(), "Calendar ready") || world.googleCalls.Load() == 0 {
				t.Errorf("Google calendar choice did not show the owner's ready calendar: %q", resp.Body.String())
			}
		},
	},
	{
		name: "Google disconnect", method: http.MethodPost, path: "/soccer/google/disconnect", form: url.Values{}, grantedStatus: http.StatusOK,
		grantedEffect: func(t *testing.T, world *soccerGrantWorld, _ *httptest.ResponseRecorder) {
			if _, kept := world.store.records[grantWorldConnectionID]; kept {
				t.Error("Google disconnect kept the owner's connection")
			}
		},
	},
}

// privateSoccerPageMarkers are controls and states that only a visitor whose
// private Soccer actions are admitted may see.
var privateSoccerPageMarkers = []string{
	"Imported in this browser", "Import access", `hx-post="/soccer/logout"`, "data-open-login-modal",
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
				for _, marker := range []string{"Imported in this browser", `hx-post="/soccer/logout"`, "Calendar ready", `href="/soccer/google/connect"`} {
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
					route.grantedEffect(t, world, resp)
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
		// releasedByDisconnect is a legacy ownerless connection that
		// disconnect deletes, since holding its cookie was the authority to
		// delete it and deleting grants no access.
		releasedByDisconnect string
	}{
		{
			name:                 "legacy ownerless import and Google connection",
			releasedByDisconnect: "legacy-connection",
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
			for _, inherited := range []string{"Imported in this browser", "Legacy Player", "Calendar ready"} {
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
				if _, kept := world.store.records[grantWorldConnectionID]; !kept {
					t.Errorf("%s %s deleted the owner's Google connection", route.method, route.path)
				}
				want := stored
				if route.path == "/soccer/google/disconnect" && tc.releasedByDisconnect != "" {
					want--
					if _, kept := world.store.records[tc.releasedByDisconnect]; kept {
						t.Errorf("%s %s left the ownerless Google connection and its token stored", route.method, route.path)
					}
				}
				if len(world.store.records) != want {
					t.Errorf("%s %s stored %d Google connections, want %d", route.method, route.path, len(world.store.records), want)
				}
			}
		})
	}
}
