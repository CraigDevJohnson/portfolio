package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

// The games the fake LPS schedules for destinationTeamID: two upcoming games
// and one scored past game.
const (
	destinationTeamID   = "4101"
	nextGameID          = "70001"
	laterGameID         = "70002"
	scoredPastGameID    = "70003"
	calendarAccount     = "family.calendar@example.net"
	primaryCalendarID   = "family.calendar@example.net"
	teamCalendarID      = "team-calendar-id"
	readOnlyCalendarID  = "league-calendar-id"
	primaryCalendarName = "Family"
	teamCalendarName    = "Team Matches"
)

// fakeCalendar is one calendar in the fake Google account. Access is Google's
// accessRole for the connected account; an empty access means the calendar
// no longer exists for that account.
type fakeCalendar struct {
	id, summary string
	primary     bool
	access      string
	events      map[string]internalgoogle.Event
}

// fakeGoogleCalendars is a Google OAuth, UserInfo, and Calendar API fake for
// one Google account. It records every Calendar events request it receives,
// and a test can refuse event writes the way Google does.
type fakeGoogleCalendars struct {
	t         *testing.T
	mu        sync.Mutex
	calendars []*fakeCalendar
	// revoked answers every Calendar request with 401, as Google does once
	// the account withdraws the grant.
	revoked bool
	// refuseWrites, when set, is Google's status and error reason for every
	// event insert or update; refuseList, for every calendar list request.
	refuseWrites *googleRefusal
	refuseList   *googleRefusal
	eventCalls   []string
}

type googleRefusal struct {
	status         int
	domain, reason string
}

func newFakeGoogleCalendars(t *testing.T) *fakeGoogleCalendars {
	t.Helper()
	return &fakeGoogleCalendars{t: t, calendars: []*fakeCalendar{
		{id: primaryCalendarID, summary: primaryCalendarName, primary: true, access: "owner", events: map[string]internalgoogle.Event{}},
		{id: teamCalendarID, summary: teamCalendarName, access: "writer", events: map[string]internalgoogle.Event{}},
		{id: readOnlyCalendarID, summary: "League Fixtures", access: "reader", events: map[string]internalgoogle.Event{}},
	}}
}

func (fake *fakeGoogleCalendars) calendar(id string) *fakeCalendar {
	for _, calendar := range fake.calendars {
		if calendar.id == id {
			return calendar
		}
	}
	return nil
}

// setAccess changes the connected account's access to a calendar; "" removes
// the calendar from the account.
func (fake *fakeGoogleCalendars) setAccess(id, access string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calendar(id).access = access
}

func (fake *fakeGoogleCalendars) setRevoked(revoked bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.revoked = revoked
}

func (fake *fakeGoogleCalendars) refuseEventWrites(refusal *googleRefusal) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.refuseWrites = refusal
}

func (fake *fakeGoogleCalendars) refuseCalendarList(refusal *googleRefusal) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.refuseList = refusal
}

// events returns a copy of the events stored in a calendar.
func (fake *fakeGoogleCalendars) events(id string) map[string]internalgoogle.Event {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	events := map[string]internalgoogle.Event{}
	for eventID := range fake.calendar(id).events {
		events[eventID] = fake.calendar(id).events[eventID]
	}
	return events
}

// calls returns the Calendar events requests received since the given count,
// as "METHOD calendar-id[/event-id]".
func (fake *fakeGoogleCalendars) callsSince(start int) []string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]string(nil), fake.eventCalls[start:]...)
}

func (fake *fakeGoogleCalendars) callCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return len(fake.eventCalls)
}

func writeGoogleError(w http.ResponseWriter, refusal googleRefusal) {
	w.WriteHeader(refusal.status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%d,"message":"refused","errors":[{"domain":%q,"reason":%q,"message":"refused"}]}}`, refusal.status, refusal.domain, refusal.reason)
}

func (fake *fakeGoogleCalendars) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/oauth/token":
		_, _ = w.Write([]byte(`{"access_token":"calendar-access","refresh_token":"calendar-refresh","token_type":"Bearer","expires_in":3600}`))
		return
	case r.URL.Path == "/userinfo":
		_, _ = fmt.Fprintf(w, `{"sub":"google-family","email":%q,"email_verified":true}`, calendarAccount)
		return
	case fake.revoked && strings.HasPrefix(r.URL.Path, "/calendar/v3/"):
		writeGoogleError(w, googleRefusal{status: http.StatusUnauthorized, domain: "global", reason: "authError"})
		return
	case r.URL.Path == "/calendar/v3/users/me/calendarList" && fake.refuseList != nil:
		writeGoogleError(w, *fake.refuseList)
		return
	case r.URL.Path == "/calendar/v3/users/me/calendarList":
		if r.URL.Query().Get("minAccessRole") != "writer" {
			fake.t.Errorf("calendar list asked for access %q, want writer", r.URL.Query().Get("minAccessRole"))
		}
		items := []map[string]any{}
		for _, calendar := range fake.calendars {
			if calendar.access == "owner" || calendar.access == "writer" {
				items = append(items, map[string]any{"id": calendar.id, "summary": calendar.summary, "primary": calendar.primary, "accessRole": calendar.access})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		return
	}
	calendarID, rest, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/") || !ok || (rest != "events" && !strings.HasPrefix(rest, "events/")) {
		fake.t.Errorf("unexpected Google request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	eventID := strings.TrimPrefix(strings.TrimPrefix(rest, "events"), "/")
	fake.eventCalls = append(fake.eventCalls, strings.TrimSuffix(r.Method+" "+calendarID+"/"+eventID, "/"))
	calendar := fake.calendar(calendarID)
	if calendar == nil || calendar.access == "" {
		writeGoogleError(w, googleRefusal{status: http.StatusNotFound, domain: "global", reason: "notFound"})
		return
	}
	write := r.Method == http.MethodPost || r.Method == http.MethodPut
	switch {
	case write && fake.refuseWrites != nil:
		writeGoogleError(w, *fake.refuseWrites)
	case write && calendar.access != "owner" && calendar.access != "writer":
		writeGoogleError(w, googleRefusal{status: http.StatusForbidden, domain: "calendar", reason: "requiredAccessLevel"})
	case r.Method == http.MethodGet && eventID != "":
		event, found := calendar.events[eventID]
		if !found {
			writeGoogleError(w, googleRefusal{status: http.StatusNotFound, domain: "global", reason: "notFound"})
			return
		}
		_ = json.NewEncoder(w).Encode(event)
	case r.Method == http.MethodGet:
		gameID := strings.TrimPrefix(r.URL.Query().Get("privateExtendedProperty"), "game_id=")
		matches := []internalgoogle.Event{}
		for id := range calendar.events {
			if calendar.events[id].ExtendedProperties.Private["game_id"] == gameID {
				matches = append(matches, calendar.events[id])
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": matches})
	case write:
		var event internalgoogle.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			fake.t.Errorf("event body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, exists := calendar.events[event.ID]; exists && r.Method == http.MethodPost {
			writeGoogleError(w, googleRefusal{status: http.StatusConflict, domain: "global", reason: "duplicate"})
			return
		}
		calendar.events[event.ID] = event
		_ = json.NewEncoder(w).Encode(event)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// calendarDestinationWorld is the real route assembly behind fake Cognito
// sign-in, a fake LPS schedule for one team, and a fake Google account with a
// primary calendar, a second writable calendar, and a read-only calendar.
type calendarDestinationWorld struct {
	app     *App
	store   *appTestGoogleConnectionStore
	google  *fakeGoogleCalendars
	browser *siteBrowser
}

func newCalendarDestinationWorld(t *testing.T) *calendarDestinationWorld {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	world := &calendarDestinationWorld{app: cognito.app(t), google: newFakeGoogleCalendars(t)}
	world.app.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	world.app.Config.GoogleClientID = "google-client"
	world.app.Config.GoogleClientSecret = "google-secret"
	world.app.Config.GoogleConnectionTableName = "connections"
	world.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	world.app.GoogleHandler.SetStore(world.store)

	google := httptest.NewServer(world.google)
	t.Cleanup(google.Close)
	world.app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	world.app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	world.app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	world.app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	next := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	later := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-72 * time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/"+destinationTeamID {
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":4101,"team_name":"Craig FC","Season":169},"games":[`+
			`{"UGameID":%s,"UTeam1":4101,"UTeam2":4201,"Season":169,"SchedGameDateTime":%q,"field_name":"Field 1","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4201,"team_name":"Rivals"}},`+
			`{"UGameID":%s,"UTeam1":4101,"UTeam2":4202,"Season":169,"SchedGameDateTime":%q,"field_name":"Field 2","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4202,"team_name":"Strikers"}},`+
			`{"UGameID":%s,"UTeam1":4101,"UTeam2":4203,"Season":169,"SchedGameDateTime":%q,"field_name":"Field 3","result":"2-1","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4203,"team_name":"Old Boys"}}]}`,
			nextGameID, next, laterGameID, later, scoredPastGameID, past)
	}))
	t.Cleanup(lps.Close)
	world.app.Config.LPSAPIBaseURL = lps.URL

	mux, _ := buildMux(world.app, world.app.Logger, false)
	world.browser = newSiteBrowser(t, mux)
	return world
}

// connect signs the visitor in and completes Google Calendar consent as the
// fake account, returning the Soccer page the consent returns to.
func (world *calendarDestinationWorld) connect(t *testing.T) string {
	t.Helper()
	world.browser.signIn("/soccer")
	start := world.browser.get("/soccer/google/connect")
	consent, err := url.Parse(start.Header().Get("Location"))
	if start.Code != http.StatusSeeOther || err != nil {
		t.Fatalf("Google connect did not start consent: %d %q", start.Code, start.Header().Get("Location"))
	}
	callback := world.browser.get("/soccer?code=granted&state=" + url.QueryEscape(consent.Query().Get("state")))
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?google=connected" {
		t.Fatalf("Google consent callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
	return world.page(t)
}

// page returns the Soccer page as the visitor now sees it.
func (world *calendarDestinationWorld) page(t *testing.T) string {
	t.Helper()
	page := world.browser.get("/soccer")
	if page.Code != http.StatusOK {
		t.Fatalf("Soccer page status = %d", page.Code)
	}
	return page.Body.String()
}

// fetch loads the team's schedule the way the planner's fetch control does.
func (world *calendarDestinationWorld) fetch(t *testing.T) string {
	t.Helper()
	fetched := world.browser.postForm("/soccer/fetch", url.Values{"team_codes": {destinationTeamID}})
	if fetched.Code != http.StatusOK {
		t.Fatalf("schedule fetch status = %d", fetched.Code)
	}
	return fetched.Body.String()
}

// add asks the site to add the selected games to Google Calendar and returns
// the route's response.
func (world *calendarDestinationWorld) add(t *testing.T, gameIDs ...string) string {
	t.Helper()
	added := world.browser.postForm("/soccer/google/add", url.Values{"team_codes": {destinationTeamID}, "selected": gameIDs})
	if added.Code != http.StatusOK {
		t.Fatalf("Google add status = %d", added.Code)
	}
	return added.Body.String()
}

// choose saves a destination calendar and returns the refreshed Google card.
func (world *calendarDestinationWorld) choose(t *testing.T, calendarID string) string {
	t.Helper()
	chosen := world.browser.postForm("/soccer/google/calendar", url.Values{"calendar_id": {calendarID}})
	if chosen.Code != http.StatusOK {
		t.Fatalf("calendar choice status = %d", chosen.Code)
	}
	return chosen.Body.String()
}

// The page's words for a connection whose destination calendar can be written
// and for one whose writes are paused until the visitor chooses again.
const (
	calendarReady        = "Calendar ready"
	calendarChoiceNeeded = "Choose a writable calendar"
)

func TestRevokedGoogleAccessAsksToReconnectRatherThanForANewCalendar(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.fetch(t)
	world.google.setRevoked(true)
	before := world.google.callCount()

	added := world.add(t, nextGameID)
	if !strings.Contains(added, "Connect again") || strings.Contains(added, calendarChoiceNeeded) {
		t.Fatalf("Add with revoked Google access did not ask to reconnect: %q", added)
	}
	if writes := world.google.callsSince(before); len(writes) != 0 {
		t.Errorf("Add with revoked Google access sent event requests %v", writes)
	}
	if len(world.store.records) != 0 {
		t.Errorf("revoked Google connection was kept: %d stored", len(world.store.records))
	}
	if page := world.page(t); !strings.Contains(page, "Not connected") {
		t.Error("Soccer page still presented the revoked Google connection")
	}
}

func TestGoogleRefusalOfAnEventWriteDecidesBetweenReconnectRetryAndANewChoice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refusal googleRefusal
		// wantMessage is what the Add response asks the visitor to do.
		wantMessage string
		// wantConnected reports whether the Google connection survives;
		// wantPaused whether writes wait for a new calendar choice.
		wantConnected, wantPaused bool
	}{
		{name: "credentials rejected", refusal: googleRefusal{http.StatusUnauthorized, "global", "authError"}, wantMessage: "Connect again"},
		{name: "Calendar access not granted", refusal: googleRefusal{http.StatusForbidden, "global", "insufficientPermissions"}, wantMessage: "Connect again"},
		{name: "rate limited", refusal: googleRefusal{http.StatusForbidden, "usageLimits", "rateLimitExceeded"}, wantMessage: "Try again", wantConnected: true},
		{name: "calendar refuses writes", refusal: googleRefusal{http.StatusForbidden, "calendar", "requiredAccessLevel"}, wantMessage: calendarChoiceNeeded, wantConnected: true, wantPaused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.fetch(t)
			world.google.refuseEventWrites(&tc.refusal)

			if added := world.add(t, nextGameID); !strings.Contains(added, tc.wantMessage) {
				t.Fatalf("Add refused with %d %s answered %q, want it to say %q", tc.refusal.status, tc.refusal.reason, added, tc.wantMessage)
			}
			world.google.refuseEventWrites(nil)
			if !tc.wantConnected {
				if len(world.store.records) != 0 || !strings.Contains(world.page(t), "Not connected") {
					t.Fatal("a connection Google no longer accepts was kept")
				}
				return
			}
			page := world.page(t)
			if tc.wantPaused != strings.Contains(page, calendarChoiceNeeded) || tc.wantPaused == strings.Contains(page, calendarReady) {
				t.Fatalf("after the refusal the page paused writes = %t, want %t", strings.Contains(page, calendarChoiceNeeded), tc.wantPaused)
			}
			before := world.google.callCount()
			retried := world.add(t, nextGameID)
			wrote := len(world.google.events(primaryCalendarID)) == 1
			if tc.wantPaused {
				if wrote || !strings.Contains(retried, calendarChoiceNeeded) || len(world.google.callsSince(before)) != 0 {
					t.Fatalf("paused writes resumed without a new calendar choice: %q", retried)
				}
				return
			}
			if !wrote || !strings.Contains(retried, "Added 1 selected game") {
				t.Fatalf("retry after the refusal did not add the game to the same calendar: %q", retried)
			}
		})
	}
}

func TestRateLimitedCalendarCheckKeepsTheConnectionAndItsDestination(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.fetch(t)
	world.google.refuseCalendarList(&googleRefusal{http.StatusForbidden, "usageLimits", "userRateLimitExceeded"})

	if added := world.add(t, nextGameID); !strings.Contains(added, "try again later") {
		t.Fatalf("rate-limited Add did not ask the visitor to retry: %q", added)
	}
	world.google.refuseCalendarList(nil)
	if len(world.store.records) != 1 || !strings.Contains(world.page(t), calendarReady) {
		t.Fatal("a Google usage limit removed the connection or paused its destination")
	}
	if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") || len(world.google.events(primaryCalendarID)) != 1 {
		t.Fatalf("retry after the usage limit did not add the game to the same calendar: %q", added)
	}
}
