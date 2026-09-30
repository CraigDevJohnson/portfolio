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
	destinationTeamID = "4101"
	nextGameID        = "70001"
	laterGameID       = "70002"
	scoredPastGameID  = "70003"
)

// The Google account that consents in these tests and its calendars. Google
// identifies an account's primary calendar by the account's email address.
const (
	calendarAccount     = "family.calendar@example.net"
	primaryCalendarID   = calendarAccount
	primaryCalendarName = "Family"
	teamCalendarID      = "team-calendar-id"
	teamCalendarName    = "Team Matches"
	readOnlyCalendarID  = "league-calendar-id"
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

// callsSince returns the Calendar events requests received after the first
// start of them, each as "METHOD calendar-id[/event-id]".
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

func TestCalendarCheckRefusedForAnotherReasonThanTheConnectionKeepsItAndItsDestination(t *testing.T) {
	for _, refusal := range []googleRefusal{
		{http.StatusForbidden, "usageLimits", "userRateLimitExceeded"},
		{http.StatusForbidden, "global", "forbidden"},
		{http.StatusForbidden, "usageLimits", "accessNotConfigured"},
	} {
		t.Run(refusal.reason, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.fetch(t)
			world.google.refuseCalendarList(&refusal)

			if added := world.add(t, nextGameID); !strings.Contains(added, "try again later") || strings.Contains(added, "Connect again") {
				t.Fatalf("Add with the calendar check refused as %s did not ask the visitor to retry: %q", refusal.reason, added)
			}
			// The kept connection is shown as connected, not offered again.
			if page := world.page(t); strings.Contains(page, "Not connected") || !strings.Contains(page, calendarAccount) || !strings.Contains(page, "Could not check your calendars right now. Try again in a moment.") {
				t.Fatalf("the page with the calendar check refused as %s did not show the kept connection and ask for a retry", refusal.reason)
			}
			world.google.refuseCalendarList(nil)
			if len(world.store.records) != 1 || !strings.Contains(world.page(t), calendarReady) {
				t.Fatalf("a calendar check refused as %s removed the connection or paused its destination", refusal.reason)
			}
			if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") || len(world.google.events(primaryCalendarID)) != 1 {
				t.Fatalf("retry after the refusal did not add the game to the same calendar: %q", added)
			}
		})
	}
}

// selectedCalendar returns the destination calendar the Google card's select
// marks as chosen, or "" when none is.
func selectedCalendar(t *testing.T, card string) string {
	t.Helper()
	for _, calendarID := range []string{"", primaryCalendarID, teamCalendarID, readOnlyCalendarID} {
		if strings.Contains(card, fmt.Sprintf(`<option value=%q selected`, calendarID)) {
			return calendarID
		}
	}
	t.Fatal("Google card offered no destination calendar choice")
	return ""
}

func TestConsentLeavesThePrimaryCalendarReadyAndAnotherCanBeChosen(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	page := world.connect(t)
	if !strings.Contains(page, calendarReady) || !strings.Contains(page, "Connected to "+primaryCalendarName) || selectedCalendar(t, page) != primaryCalendarID {
		t.Fatal("completed consent did not leave the primary calendar selected and ready")
	}
	if !strings.Contains(page, fmt.Sprintf(`<option value=%q>%s</option>`, teamCalendarID, teamCalendarName)) || strings.Contains(page, readOnlyCalendarID) {
		t.Error("destination choices were not exactly the account's writable calendars")
	}

	// The first Add needs no calendar confirmation.
	world.fetch(t)
	world.add(t, nextGameID)
	if _, added := world.google.events(primaryCalendarID)[nextGameID]; !added {
		t.Fatal("Add straight after consent did not write to the primary calendar")
	}

	card := world.choose(t, teamCalendarID)
	if selectedCalendar(t, card) != teamCalendarID || !strings.Contains(card, "Connected to "+teamCalendarName) {
		t.Fatal("saving another writable calendar did not make it the destination")
	}
	if page := world.page(t); selectedCalendar(t, page) != teamCalendarID || !strings.Contains(page, calendarReady) {
		t.Error("the chosen destination did not stay chosen on the next page load")
	}
}

func TestChangingDestinationLeavesEarlierEventsAndSendsLaterAddsToTheNewCalendar(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.fetch(t)
	world.add(t, nextGameID)
	earlier := world.google.events(primaryCalendarID)[nextGameID]

	world.choose(t, teamCalendarID)
	mark := world.google.callCount()
	world.add(t, laterGameID)
	world.add(t, nextGameID)

	for _, call := range world.google.callsSince(mark) {
		if strings.Contains(call, " "+primaryCalendarID) {
			t.Errorf("an Add after the destination changed sent %q to the old calendar", call)
		}
	}
	primary, team := world.google.events(primaryCalendarID), world.google.events(teamCalendarID)
	if len(primary) != 1 || primary[nextGameID].Summary != earlier.Summary || primary[nextGameID].Start != earlier.Start {
		t.Errorf("the event added before the change did not stay in the old calendar: %v", primary)
	}
	if _, ok := team[laterGameID]; !ok || len(team) != 2 {
		t.Errorf("Adds after the change did not go to the new calendar: %v", team)
	}
}

func TestLostDestinationPausesWritesUntilTheVisitorChoosesAgain(t *testing.T) {
	for _, lost := range []struct{ name, access string }{
		{name: "removed from the account", access: ""},
		{name: "now read-only", access: "reader"},
	} {
		t.Run(lost.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			world.fetch(t)
			world.google.setAccess(teamCalendarID, lost.access)

			mark := world.google.callCount()
			added := world.add(t, nextGameID)
			synced := world.browser.postForm("/soccer/google/sync-results", url.Values{"team_codes": {destinationTeamID}, "selected": {scoredPastGameID}}).Body.String()
			if !strings.Contains(added, calendarChoiceNeeded) || !strings.Contains(synced, calendarChoiceNeeded) {
				t.Fatalf("writes to a lost calendar did not ask for a new choice: add %q", added)
			}
			if calls := world.google.callsSince(mark); len(calls) != 0 {
				t.Fatalf("writes to a lost calendar still sent event requests %v", calls)
			}
			page := world.page(t)
			if strings.Contains(page, calendarReady) || !strings.Contains(page, "Calendar selection needed") || selectedCalendar(t, page) != "" {
				t.Fatal("the page did not pause the lost destination and ask for a new choice")
			}

			// Neither the calendar returning nor an unknown choice resumes writes.
			world.google.setAccess(teamCalendarID, "writer")
			world.choose(t, "not-a-calendar")
			if added := world.add(t, nextGameID); !strings.Contains(added, calendarChoiceNeeded) || len(world.google.callsSince(mark)) != 0 {
				t.Fatalf("writes resumed without an explicit new choice: %q", added)
			}

			world.choose(t, primaryCalendarID)
			world.add(t, nextGameID)
			if _, ok := world.google.events(primaryCalendarID)[nextGameID]; !ok || len(world.google.events(teamCalendarID)) != 0 {
				t.Fatal("the new choice did not receive the next Add")
			}
		})
	}
}

func TestViewingThePageAfterTheDestinationIsLostPausesWritesBeforeAnyAdd(t *testing.T) {
	for _, lost := range []struct{ name, access string }{
		{name: "removed from the account", access: ""},
		{name: "now read-only", access: "reader"},
	} {
		t.Run(lost.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			world.fetch(t)
			world.google.setAccess(teamCalendarID, lost.access)

			page := world.page(t)
			if strings.Contains(page, calendarReady) || !strings.Contains(page, "Calendar selection needed") || selectedCalendar(t, page) != "" {
				t.Fatal("the first page view after the destination was lost did not pause writes and ask for a new choice")
			}
			for _, record := range world.store.records {
				if !record.CalendarSelectionRequired || record.CalendarID != teamCalendarID {
					t.Errorf("stored destination = %q, paused %t; want %q kept and paused", record.CalendarID, record.CalendarSelectionRequired, teamCalendarID)
				}
			}

			// Neither the calendar returning nor primary receives the next Add.
			world.google.setAccess(teamCalendarID, "writer")
			mark := world.google.callCount()
			added := world.add(t, nextGameID)
			if !strings.Contains(added, calendarChoiceNeeded) {
				t.Fatalf("Add after the page paused writes did not ask for a new choice: %q", added)
			}
			if calls := world.google.callsSince(mark); len(calls) != 0 || len(world.google.events(primaryCalendarID)) != 0 {
				t.Fatalf("Add after the page paused writes sent event requests %v", calls)
			}
		})
	}
}

func TestAConnectionWithoutADestinationTakesPrimaryOnceTheAccountCanWriteIt(t *testing.T) {
	for _, resume := range []string{"page view", "Add"} {
		t.Run("resumed by "+resume, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.fetch(t)
			// A connection saved without a destination, while the account can
			// write no calendar at all.
			for id, record := range world.store.records {
				record.CalendarID, record.CalendarSummary = "", ""
				world.store.records[id] = record
			}
			world.google.setAccess(primaryCalendarID, "reader")
			world.google.setAccess(teamCalendarID, "reader")

			mark := world.google.callCount()
			if page := world.page(t); strings.Contains(page, calendarReady) {
				t.Fatal("the page offered a ready destination while the account can write no calendar")
			}
			if added := world.add(t, nextGameID); !strings.Contains(added, calendarChoiceNeeded) || len(world.google.callsSince(mark)) != 0 {
				t.Fatalf("Add while the account can write no calendar did not ask for one: %q", added)
			}

			// Nothing was chosen, so nothing was lost: primary becomes the
			// destination without a choice once the account can write it.
			world.google.setAccess(primaryCalendarID, "owner")
			if resume == "page view" {
				if page := world.page(t); !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != primaryCalendarID {
					t.Fatal("the page did not make primary the destination")
				}
			}
			if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") || len(world.google.events(primaryCalendarID)) != 1 {
				t.Fatalf("Add did not go to primary once the account could write it: %q", added)
			}
		})
	}
}

func TestAddWritesOnlyExplicitlySelectedUpcomingGames(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	mark := world.google.callCount()
	world.connect(t)
	world.page(t)
	fetched := world.fetch(t)
	if !strings.Contains(fetched, `hx-post="/soccer/google/add"`) || !strings.Contains(fetched, fmt.Sprintf(`value=%q`, laterGameID)) {
		t.Fatal("the fetched schedule did not offer Google Add for its upcoming games")
	}
	if calls := world.google.callsSince(mark); len(calls) != 0 {
		t.Fatalf("connecting, viewing, and fetching sent event requests %v", calls)
	}

	added := world.add(t, nextGameID, scoredPastGameID)
	events := world.google.events(primaryCalendarID)
	if _, ok := events[nextGameID]; !ok || len(events) != 1 || !strings.Contains(added, "Added 1 selected game") {
		t.Fatalf("Add wrote %d events for one selected upcoming game, one past game, and one unselected game: %q", len(events), added)
	}

	mark = world.google.callCount()
	if added := world.add(t, scoredPastGameID); !strings.Contains(added, "No selected games were found to add") || len(world.google.callsSince(mark)) != 0 {
		t.Fatalf("Add of only a past game reached Google: %q", added)
	}
}

func TestAddedEventsCarryGameIdentityAndTheSiteMarkerSoRepeatedAddsMatchThem(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.fetch(t)
	world.add(t, nextGameID)

	event := world.google.events(primaryCalendarID)[nextGameID]
	if event.ID != nextGameID || event.ExtendedProperties.Private["game_id"] != nextGameID {
		t.Errorf("added event identity = id %q, game_id %q; want the game ID %q", event.ID, event.ExtendedProperties.Private["game_id"], nextGameID)
	}
	if event.ExtendedProperties.Private["portfolio_app"] != "soccer" || event.Source == nil || event.Source.URL != "https://app.example.com/soccer" {
		t.Errorf("added event lacks the site's ownership marker: private %v, source %+v", event.ExtendedProperties.Private, event.Source)
	}

	// An event this site added before event IDs followed game IDs is found by
	// its private game ID.
	older := internalgoogle.Event{ID: "olderevent0001", Summary: "Craig FC vs Strikers"}
	older.ExtendedProperties.Private = map[string]string{"game_id": laterGameID}
	world.google.mu.Lock()
	world.google.calendar(primaryCalendarID).events[older.ID] = older
	world.google.mu.Unlock()

	mark := world.google.callCount()
	repeated := world.add(t, nextGameID, laterGameID)
	for _, call := range world.google.callsSince(mark) {
		if strings.HasPrefix(call, http.MethodPost+" ") {
			t.Errorf("repeated Add inserted a new event: %s", call)
		}
	}
	if events := world.google.events(primaryCalendarID); len(events) != 2 || strings.Contains(repeated, "Added 1") || strings.Contains(repeated, "Added 2") {
		t.Fatalf("repeated Add left %d events and answered %q; want the two existing events matched", len(events), repeated)
	}
	if marked := world.google.events(primaryCalendarID)["olderevent0001"]; marked.ExtendedProperties.Private["portfolio_app"] != "soccer" {
		t.Error("the matched older event did not gain the site's ownership marker")
	}
}
