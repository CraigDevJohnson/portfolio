package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

// The games the fake LPS schedules for destinationTeamID: two upcoming games,
// one scored past game, and one game LPS has not given a start time yet.
const (
	destinationTeamID = "4101"
	nextGameID        = "70001"
	laterGameID       = "70002"
	scoredPastGameID  = "70003"
	undatedGameID     = "70004"
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
	// hidden is set when the account hid the calendar from its list; Google
	// lists it only when asked to show hidden calendars.
	hidden bool
	events map[string]internalgoogle.Event
}

// fakeGoogleCalendars is a Google OAuth, UserInfo, and Calendar API fake for
// one Google account. It records every Calendar events request it receives,
// including one it refuses after the account revoked access, and a test can
// refuse event writes the way Google does.
type fakeGoogleCalendars struct {
	t         *testing.T
	mu        sync.Mutex
	calendars []*fakeCalendar
	// accountSubject and accountEmail identify the Google account that
	// consents. A test may consent as another account, which the fake lists
	// the same calendars for.
	accountSubject, accountEmail string
	// revoked answers every Calendar request with 401, as Google does once
	// the account withdraws the grant.
	revoked bool
	// refuseWrites, when set, is Google's status and error reason for every
	// event insert or update; refuseList, for every calendar list request.
	refuseWrites *googleRefusal
	refuseList   *googleRefusal
	// refuseReads, when set, is Google's refusal of every events read.
	refuseReads *googleRefusal
	// refuseEvents is Google's refusal of any insert or update of the event
	// with that ID.
	refuseEvents map[string]googleRefusal
	// listPageSize, when set, is how many calendars each calendar list page
	// holds; Google may return fewer than the page size asked for.
	listPageSize int
	// stallList, when set, is how long each calendar list request waits
	// before Google answers, unless the caller gives up first.
	stallList time.Duration
	// listRequests counts the calendar list requests received.
	listRequests int
	// eventSearchPages, when set, splits the events one search by private
	// property finds into the pages Google answers with, which may hold
	// fewer events than asked for, or none, while more follow.
	eventSearchPages func([]internalgoogle.Event) [][]internalgoogle.Event
	// beforeEventWrite, when set, runs as each event insert, update, or
	// patch arrives, before Google answers it.
	beforeEventWrite func()
	// changeBeforePatch holds an edit made in Google to an event, applied as
	// the next patch of that event arrives and before Google checks its
	// If-Match condition, the way a visitor's own edit can land between the
	// site reading and patching an event.
	changeBeforePatch map[string]func(*internalgoogle.Event)
	// etags numbers each version of an event, as Google's ETag does.
	etags      int
	eventCalls []string
	patches    []fakeEventPatch
}

// fakeEventPatch is one events.patch request the fake received: the event it
// named, its If-Match condition, and the fields its body set.
type fakeEventPatch struct {
	calendarID, eventID, ifMatch string
	fields                       []string
}

type googleRefusal struct {
	status         int
	domain, reason string
}

func newFakeGoogleCalendars(t *testing.T) *fakeGoogleCalendars {
	t.Helper()
	return &fakeGoogleCalendars{t: t, accountSubject: "google-family", accountEmail: calendarAccount, calendars: []*fakeCalendar{
		{id: primaryCalendarID, summary: primaryCalendarName, primary: true, access: "owner", events: map[string]internalgoogle.Event{}},
		{id: teamCalendarID, summary: teamCalendarName, access: "writer", events: map[string]internalgoogle.Event{}},
		{id: readOnlyCalendarID, summary: "League Fixtures", access: "reader", events: map[string]internalgoogle.Event{}},
	}}
}

func (fake *fakeGoogleCalendars) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/calendar/v3/users/me/calendarList" {
		fake.mu.Lock()
		fake.listRequests++
		stall := fake.stallList
		fake.mu.Unlock()
		if stall > 0 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(stall):
			}
		}
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	calendarID, eventID, eventRequest := fakeGoogleEventPath(r.URL.Path)
	if eventRequest {
		// Record before any refusal, so an events request sent after the
		// account revoked access is still seen.
		fake.eventCalls = append(fake.eventCalls, strings.TrimSuffix(r.Method+" "+calendarID+"/"+eventID, "/"))
	}
	switch {
	case r.URL.Path == "/oauth/token":
		_, _ = w.Write([]byte(`{"access_token":"calendar-access","refresh_token":"calendar-refresh","token_type":"Bearer","expires_in":3600}`))
		return
	case r.URL.Path == "/userinfo":
		_, _ = fmt.Fprintf(w, `{"sub":%q,"email":%q,"email_verified":true}`, fake.accountSubject, fake.accountEmail)
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
			if (calendar.access == "owner" || calendar.access == "writer") && (!calendar.hidden || r.URL.Query().Get("showHidden") == "true") {
				items = append(items, map[string]any{"id": calendar.id, "summary": calendar.summary, "primary": calendar.primary, "accessRole": calendar.access})
			}
		}
		page := map[string]any{"items": items}
		if fake.listPageSize > 0 {
			start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
			end := min(start+fake.listPageSize, len(items))
			page["items"] = items[start:end]
			if end < len(items) {
				page["nextPageToken"] = strconv.Itoa(end)
			}
		}
		_ = json.NewEncoder(w).Encode(page)
		return
	}
	if !eventRequest {
		fake.t.Errorf("unexpected Google request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	calendar := fake.calendar(calendarID)
	if calendar == nil || calendar.access == "" {
		writeGoogleError(w, googleRefusal{status: http.StatusNotFound, domain: "global", reason: "notFound"})
		return
	}
	write := r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch
	switch {
	case write && fake.refuseWrites != nil:
		writeGoogleError(w, *fake.refuseWrites)
	case !write && fake.refuseReads != nil:
		writeGoogleError(w, *fake.refuseReads)
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
			event := calendar.events[id]
			// Google lists deleted events only when asked to show them.
			if event.ExtendedProperties.Private["game_id"] == gameID && (event.Status != googleDeletedStatus || r.URL.Query().Get("showDeleted") == "true") {
				matches = append(matches, event)
			}
		}
		page := map[string]any{"items": matches}
		if fake.eventSearchPages != nil {
			pages := fake.eventSearchPages(matches)
			index, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Query().Get("pageToken"), "p"))
			page["items"] = append([]internalgoogle.Event{}, pages[index]...)
			if index+1 < len(pages) {
				page["nextPageToken"] = fmt.Sprintf("p%d", index+1)
			}
		}
		_ = json.NewEncoder(w).Encode(page)
	case r.Method == http.MethodPatch:
		fake.patchEvent(w, r, calendar, eventID)
	case write:
		var event internalgoogle.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			fake.t.Errorf("event body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if fake.beforeEventWrite != nil {
			fake.beforeEventWrite()
		}
		if refusal, refused := fake.refuseEvents[event.ID]; refused {
			writeGoogleError(w, refusal)
			return
		}
		if _, exists := calendar.events[event.ID]; exists && r.Method == http.MethodPost {
			writeGoogleError(w, googleRefusal{status: http.StatusConflict, domain: "global", reason: "duplicate"})
			return
		}
		fake.etags++
		event.ETag = fmt.Sprintf(`"%d"`, fake.etags)
		calendar.events[event.ID] = event
		_ = json.NewEncoder(w).Encode(event)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// googleDeletedStatus is the status Google gives a deleted event.
const googleDeletedStatus = "cancelled" //nolint:misspell // Google Calendar's wire spelling.

// patchEvent answers events.patch as Google does: it changes only the fields
// the body names, and refuses the change with 412 when an If-Match condition
// no longer names the event's current version.
func (fake *fakeGoogleCalendars) patchEvent(w http.ResponseWriter, r *http.Request, calendar *fakeCalendar, eventID string) {
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		fake.t.Errorf("event patch body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	patch := fakeEventPatch{calendarID: calendar.id, eventID: eventID, ifMatch: r.Header.Get("If-Match")}
	for field := range fields {
		patch.fields = append(patch.fields, field)
	}
	sort.Strings(patch.fields)
	fake.patches = append(fake.patches, patch)
	if fake.beforeEventWrite != nil {
		fake.beforeEventWrite()
	}
	if refusal, refused := fake.refuseEvents[eventID]; refused {
		writeGoogleError(w, refusal)
		return
	}
	event, found := calendar.events[eventID]
	if !found {
		writeGoogleError(w, googleRefusal{status: http.StatusNotFound, domain: "global", reason: "notFound"})
		return
	}
	if change := fake.changeBeforePatch[eventID]; change != nil {
		delete(fake.changeBeforePatch, eventID)
		change(&event)
		fake.etags++
		event.ETag = fmt.Sprintf(`"%d"`, fake.etags)
		calendar.events[eventID] = event
	}
	if patch.ifMatch != "" && patch.ifMatch != event.ETag {
		writeGoogleError(w, googleRefusal{status: http.StatusPreconditionFailed, domain: "global", reason: "conditionNotMet"})
		return
	}
	var patched map[string]any
	current, _ := json.Marshal(event)
	_ = json.Unmarshal(current, &patched)
	for field, value := range fields {
		var decoded any
		_ = json.Unmarshal(value, &decoded)
		patched[field] = decoded
	}
	merged, _ := json.Marshal(patched)
	event = internalgoogle.Event{}
	if err := json.Unmarshal(merged, &event); err != nil {
		fake.t.Errorf("patched event: %v", err)
	}
	fake.etags++
	event.ETag = fmt.Sprintf(`"%d"`, fake.etags)
	calendar.events[eventID] = event
	_ = json.NewEncoder(w).Encode(event)
}

// fakeGoogleEventPath reads a Calendar events path, which names a calendar and
// may name one event, and reports false for any other path.
func fakeGoogleEventPath(path string) (calendarID, eventID string, ok bool) {
	rest, isCalendar := strings.CutPrefix(path, "/calendar/v3/calendars/")
	calendarID, rest, found := strings.Cut(rest, "/")
	if !isCalendar || !found || (rest != "events" && !strings.HasPrefix(rest, "events/")) {
		return "", "", false
	}
	return calendarID, strings.TrimPrefix(strings.TrimPrefix(rest, "events"), "/"), true
}

func (fake *fakeGoogleCalendars) calendar(id string) *fakeCalendar {
	for _, calendar := range fake.calendars {
		if calendar.id == id {
			return calendar
		}
	}
	return nil
}

// consentAs makes a different Google account consent from now on.
func (fake *fakeGoogleCalendars) consentAs(subject, email string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.accountSubject, fake.accountEmail = subject, email
}

// setAccess changes the connected account's access to a calendar; "" removes
// the calendar from the account.
func (fake *fakeGoogleCalendars) setAccess(id, access string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calendar(id).access = access
}

func (fake *fakeGoogleCalendars) setHidden(id string, hidden bool) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.calendar(id).hidden = hidden
}

func (fake *fakeGoogleCalendars) setListPageSize(size int) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.listPageSize = size
}

func (fake *fakeGoogleCalendars) splitEventSearches(pages func([]internalgoogle.Event) [][]internalgoogle.Event) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.eventSearchPages = pages
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

func (fake *fakeGoogleCalendars) refuseEventReads(refusal *googleRefusal) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.refuseReads = refusal
}

func (fake *fakeGoogleCalendars) refuseEventWrite(eventID string, refusal googleRefusal) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.refuseEvents == nil {
		fake.refuseEvents = map[string]googleRefusal{}
	}
	fake.refuseEvents[eventID] = refusal
}

func (fake *fakeGoogleCalendars) onEventWrite(hook func()) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.beforeEventWrite = hook
}

// addEvent places an event in a calendar as if another client had written it.
func (fake *fakeGoogleCalendars) addEvent(calendarID string, event *internalgoogle.Event) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.etags++
	stored := *event
	stored.ETag = fmt.Sprintf(`"%d"`, fake.etags)
	fake.calendar(calendarID).events[event.ID] = stored
}

// editEvent changes an event as the visitor could in Google Calendar, giving
// it a new version.
func (fake *fakeGoogleCalendars) editEvent(calendarID, eventID string, edit func(*internalgoogle.Event)) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	event, found := fake.calendar(calendarID).events[eventID]
	if !found {
		fake.t.Fatalf("calendar %s holds no event %s to edit", calendarID, eventID)
	}
	edit(&event)
	fake.etags++
	event.ETag = fmt.Sprintf(`"%d"`, fake.etags)
	fake.calendar(calendarID).events[eventID] = event
}

// deleteEvent deletes an event as Google does: it stays, marked deleted,
// but holds only what Google guarantees a deleted event keeps, its ID. Its
// private properties are gone, so a search by them no longer finds it; a
// read by its ID still returns it. To model Google still listing a deleted
// event with its properties, set its status with editEvent instead.
func (fake *fakeGoogleCalendars) deleteEvent(calendarID, eventID string) {
	fake.editEvent(calendarID, eventID, func(event *internalgoogle.Event) {
		*event = internalgoogle.Event{ID: event.ID, Status: googleDeletedStatus}
	})
}

// dropETag stores an event without the version Google normally lists it
// with.
func (fake *fakeGoogleCalendars) dropETag(calendarID, eventID string) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	event := fake.calendar(calendarID).events[eventID]
	event.ETag = ""
	fake.calendar(calendarID).events[eventID] = event
}

// changeEventBeforeNextPatch makes edit land on the event just before the
// site's next patch of it reaches Google.
func (fake *fakeGoogleCalendars) changeEventBeforeNextPatch(eventID string, edit func(*internalgoogle.Event)) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.changeBeforePatch == nil {
		fake.changeBeforePatch = map[string]func(*internalgoogle.Event){}
	}
	fake.changeBeforePatch[eventID] = edit
}

// patchesSince returns the events.patch requests received after the first
// start of them.
func (fake *fakeGoogleCalendars) patchesSince(start int) []fakeEventPatch {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]fakeEventPatch(nil), fake.patches[start:]...)
}

func (fake *fakeGoogleCalendars) patchCount() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return len(fake.patches)
}

func (fake *fakeGoogleCalendars) stallCalendarList(stall time.Duration) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.stallList = stall
}

func (fake *fakeGoogleCalendars) calendarListRequests() int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.listRequests
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
	cognito *fakeSiteCognito
	app     *App
	store   *appTestGoogleConnectionStore
	google  *fakeGoogleCalendars
	browser *siteBrowser
}

func newCalendarDestinationWorld(t *testing.T) *calendarDestinationWorld {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	world := &calendarDestinationWorld{cognito: cognito, app: cognito.app(t)}
	world.app.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	world.google, world.store = wireFakeGoogleAccount(t, world.app)

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
			`{"UGameID":%s,"UTeam1":4101,"UTeam2":4203,"Season":169,"SchedGameDateTime":%q,"field_name":"Field 3","result":"2-1","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4203,"team_name":"Old Boys"}},`+
			`{"UGameID":%s,"UTeam1":4101,"UTeam2":4204,"Season":169,"field_name":"Field 4","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4204,"team_name":"Late Adds"}}]}`,
			nextGameID, next, laterGameID, later, scoredPastGameID, past, undatedGameID)
	}))
	t.Cleanup(lps.Close)
	world.app.Config.LPSAPIBaseURL = lps.URL

	mux, _ := buildMux(world.app, world.app.Logger, false)
	world.browser = newSiteBrowser(t, mux)
	return world
}

// wireFakeGoogleAccount configures the app's Google Calendar connection
// against a fake Google account and an in-memory connection store.
func wireFakeGoogleAccount(t *testing.T, app *App) (*fakeGoogleCalendars, *appTestGoogleConnectionStore) {
	t.Helper()
	account := newFakeGoogleCalendars(t)
	store := &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	app.Config.GoogleClientID = "google-client"
	app.Config.GoogleClientSecret = "google-secret"
	app.Config.GoogleConnectionTableName = "connections"
	app.GoogleHandler.SetStore(store)

	google := httptest.NewServer(account)
	t.Cleanup(google.Close)
	app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"
	return account, store
}

// connect signs the visitor in and completes Google Calendar consent as the
// fake account, returning the Soccer page the consent returns to.
func (world *calendarDestinationWorld) connect(t *testing.T) string {
	t.Helper()
	world.browser.signIn("/soccer")
	completeGoogleConsent(t, world.browser)
	return world.page(t)
}

// completeGoogleConsent starts Google Calendar consent from the signed-in
// browser and returns from the fake Google account's grant.
func completeGoogleConsent(t *testing.T, browser *siteBrowser) {
	t.Helper()
	start := browser.get("/soccer/google/connect")
	consent, err := url.Parse(start.Header().Get("Location"))
	if start.Code != http.StatusSeeOther || err != nil {
		t.Fatalf("Google connect did not start consent: %d %q", start.Code, start.Header().Get("Location"))
	}
	callback := browser.get("/soccer?code=granted&state=" + url.QueryEscape(consent.Query().Get("state")))
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?google=connected" {
		t.Fatalf("Google consent callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
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

func TestGoogleRefusingAnUpdateToOneEventSkipsThatGameAndKeepsTheDestination(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.fetch(t)
	// The primary calendar holds another organizer's copy of the next game
	// under the game's ID, and Google refuses this account's changes to it.
	world.google.addEvent(primaryCalendarID, &internalgoogle.Event{ID: nextGameID, Summary: "Craig FC vs Rivals (invited)"})
	world.google.refuseEventWrite(nextGameID, googleRefusal{http.StatusForbidden, "calendar", "forbiddenForNonOrganizer"})

	added := world.add(t, nextGameID, laterGameID)
	if strings.Contains(added, calendarChoiceNeeded) || !strings.Contains(added, "Added 1 selected game") || !strings.Contains(added, "Skipped 1 game(s) whose existing event Google Calendar would not let this account change") {
		t.Fatalf("Add with one refused event update answered %q; want the other game added and the refused one reported", added)
	}
	if _, ok := world.google.events(primaryCalendarID)[laterGameID]; !ok {
		t.Fatal("the game after the refused update was not added")
	}
	if page := world.page(t); !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != primaryCalendarID {
		t.Fatal("a refusal of one event paused the whole destination")
	}
}

func TestADestinationLostPartwayThroughAnAddReportsTheGamesAlreadyAdded(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.choose(t, teamCalendarID)
	world.fetch(t)
	// The calendar stops accepting this account's writes after the first game.
	world.google.refuseEventWrite(laterGameID, googleRefusal{http.StatusForbidden, "calendar", "requiredAccessLevel"})

	added := world.add(t, nextGameID, laterGameID)
	if _, ok := world.google.events(teamCalendarID)[nextGameID]; !ok {
		t.Fatal("the first game was not added before the calendar refused writes")
	}
	for _, want := range []string{"Added 1 selected game", "stay in " + teamCalendarName, calendarChoiceNeeded} {
		if !strings.Contains(added, want) {
			t.Errorf("Add that lost its destination partway answered %q; want it to say %q", added, want)
		}
	}
}

func TestAPauseDuringAnAddLeavesAConnectionChangedMeanwhileAsItNowIs(t *testing.T) {
	for _, meanwhile := range []struct {
		name   string
		change func(records map[string]internalgoogle.ConnectionRecord)
		check  func(t *testing.T, world *calendarDestinationWorld)
	}{
		{
			name: "disconnected in another tab",
			change: func(records map[string]internalgoogle.ConnectionRecord) {
				clear(records)
			},
			check: func(t *testing.T, world *calendarDestinationWorld) {
				if len(world.store.records) != 0 {
					t.Fatal("the paused Add saved the disconnected connection again")
				}
			},
		},
		{
			name: "primary chosen in another tab",
			change: func(records map[string]internalgoogle.ConnectionRecord) {
				for id, record := range records {
					record.CalendarID, record.CalendarSummary = primaryCalendarID, primaryCalendarName
					record.UpdatedAt = time.Now().UTC()
					records[id] = record
				}
			},
			check: func(t *testing.T, world *calendarDestinationWorld) {
				if page := world.page(t); !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != primaryCalendarID {
					t.Fatal("the paused Add overwrote the calendar chosen meanwhile")
				}
				if added := world.add(t, laterGameID); !strings.Contains(added, "Added 1 selected game") || len(world.google.events(primaryCalendarID)) != 1 {
					t.Fatalf("Add after the other tab's choice did not go to it: %q", added)
				}
			},
		},
	} {
		t.Run(meanwhile.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			world.fetch(t)
			// Google reports the team calendar gone only once the insert
			// arrives, after another tab has changed the connection.
			world.google.refuseEventWrite(nextGameID, googleRefusal{http.StatusNotFound, "global", "notFound"})
			world.google.onEventWrite(func() { world.store.edit(meanwhile.change) })

			world.add(t, nextGameID)
			world.google.onEventWrite(nil)
			meanwhile.check(t, world)
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

func TestWritableCalendarsOnLaterListPagesOrHiddenFromTheListStayDestinations(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	// Google lists one calendar per page, so the team calendar is on the
	// second page.
	world.google.setListPageSize(1)
	world.connect(t)
	if card := world.choose(t, teamCalendarID); selectedCalendar(t, card) != teamCalendarID {
		t.Fatal("a writable calendar on the second list page could not be chosen")
	}

	world.google.setHidden(teamCalendarID, true)
	if page := world.page(t); !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != teamCalendarID {
		t.Fatal("hiding the chosen calendar from the Google list paused it")
	}
	world.fetch(t)
	if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") {
		t.Fatalf("Add to a hidden writable calendar answered %q", added)
	}
	if _, ok := world.google.events(teamCalendarID)[nextGameID]; !ok {
		t.Fatal("Add did not write to the hidden writable calendar")
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

func TestAddReportsASelectedUpcomingGameWithoutAStartTimeAsSkipped(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	if fetched := world.fetch(t); !strings.Contains(fetched, fmt.Sprintf(`value=%q`, undatedGameID)) {
		t.Fatal("the fetched schedule did not offer the upcoming game without a start time")
	}

	mark := world.google.callCount()
	added := world.add(t, nextGameID, undatedGameID)
	if !strings.Contains(added, "Added 1 selected game") || !strings.Contains(added, "Skipped 1 game(s) without a start time") {
		t.Fatalf("Add of a dated and an undated upcoming game answered %q; want one added and one reported skipped", added)
	}
	for _, call := range world.google.callsSince(mark) {
		if strings.Contains(call, undatedGameID) {
			t.Errorf("Add sent %q for a game without a start time", call)
		}
	}
	if events := world.google.events(primaryCalendarID); len(events) != 1 {
		t.Errorf("Add wrote %d events; want only the dated game", len(events))
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
	world.google.addEvent(primaryCalendarID, &older)

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
