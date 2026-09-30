package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/html"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

// These tests drive result Sync (#96) through the real route assembly: a
// granted owner signs in through a fake Cognito, connects a fake Google
// account, adds upcoming games from a fake Let's Play Soccer API with the
// real Add, and syncs their results once LPS reports them played.
//
// North FC (101) hosts every game; South FC (202) visits it in one.
const (
	syncNorthTeamID = "101"
	syncSouthTeamID = "202"

	// Games Add writes and Sync later updates.
	syncWonGameID   = "9101"
	syncDrawnGameID = "9102"
	// A game never added to any calendar.
	syncMissingGameID = "9103"
	// A played game the visitor leaves unselected.
	syncUnselectedGameID = "9104"
	// Games whose events Sync must leave alone: one imported from an .ics
	// file, one the visitor deleted, one with a second copy, one the visitor
	// edits while Sync runs, one whose result the visitor wrote themselves,
	// and one another client wrote with the game's ID but not the site's
	// marker.
	syncImportedGameID    = "9105"
	syncDeletedGameID     = "9106"
	syncDuplicatedGameID  = "9107"
	syncConcurrentGameID  = "9108"
	syncHandWrittenGameID = "9109"
	syncUnmarkedGameID    = "9110"
	// North FC hosts South FC.
	syncSharedGameID = "9120"
)

// resultSyncGame is one game on the fake LPS schedule.
type resultSyncGame struct {
	id                 string
	homeID, awayID     int
	homeName, awayName string
	field              string
	kickoff            time.Time
	result             string
}

// resultSyncLPS is a fake LPS team schedule API whose games move from
// upcoming to played while a test runs.
type resultSyncLPS struct {
	mu    sync.Mutex
	games []*resultSyncGame
}

func (lps *resultSyncLPS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lps.mu.Lock()
	defer lps.mu.Unlock()
	teamNames := map[string]string{syncNorthTeamID: "North FC", syncSouthTeamID: "South FC"}
	teamID, found := strings.CutPrefix(r.URL.Path, "/teams/")
	if !found || teamNames[teamID] == "" {
		http.NotFound(w, r)
		return
	}
	var games []string
	for _, game := range lps.games {
		if fmt.Sprint(game.homeID) != teamID && fmt.Sprint(game.awayID) != teamID {
			continue
		}
		games = append(games, fmt.Sprintf(`{"UGameID":%s,"UTeam1":%d,"UTeam2":%d,"Season":77,"SchedGameDateTime":%q,"field_name":%q,"result":%q,`+
			`"home_team":{"UTeamID":%d,"team_name":%q},"visitor_team":{"UTeamID":%d,"team_name":%q}}`,
			game.id, game.homeID, game.awayID, testutil.MislabelledLPSZuluTime(game.kickoff), game.field, game.result,
			game.homeID, game.homeName, game.awayID, game.awayName))
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":%s,"team_name":%q,"Season":77},"games":[%s]}`, teamID, teamNames[teamID], strings.Join(games, ","))
}

// schedule adds an upcoming game hosted by homeName against awayName.
func (lps *resultSyncLPS) schedule(id string, homeID int, homeName string, awayID int, awayName string) {
	lps.mu.Lock()
	defer lps.mu.Unlock()
	lps.games = append(lps.games, &resultSyncGame{
		id: id, homeID: homeID, homeName: homeName, awayID: awayID, awayName: awayName,
		field:   "Field " + id[len(id)-1:],
		kickoff: time.Now().Add(time.Duration(24+len(lps.games)) * time.Hour),
	})
}

// play reports a game as played with a home-away score.
func (lps *resultSyncLPS) play(id, score string) {
	lps.mu.Lock()
	defer lps.mu.Unlock()
	for _, game := range lps.games {
		if game.id == id {
			game.kickoff = time.Now().Add(-48 * time.Hour)
			game.result = score
			return
		}
	}
	panic("no scheduled game " + id)
}

type resultSyncWorld struct {
	store   *appTestGoogleConnectionStore
	google  *fakeGoogleCalendars
	lps     *resultSyncLPS
	browser *siteBrowser
}

func newResultSyncWorld(t *testing.T) *resultSyncWorld {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	world := &resultSyncWorld{google: newFakeGoogleCalendars(t), lps: &resultSyncLPS{}}
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	application.Config.GoogleClientID = "google-client"
	application.Config.GoogleClientSecret = "google-secret"
	application.Config.GoogleConnectionTableName = "connections"
	world.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	application.GoogleHandler.SetStore(world.store)

	google := httptest.NewServer(world.google)
	t.Cleanup(google.Close)
	application.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	application.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	application.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	application.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	lps := httptest.NewServer(world.lps)
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL

	world.lps.schedule(syncWonGameID, 101, "North FC", 4201, "Rivals")
	world.lps.schedule(syncDrawnGameID, 101, "North FC", 4202, "Strikers")
	world.lps.schedule(syncMissingGameID, 101, "North FC", 4203, "Old Boys")
	world.lps.schedule(syncUnselectedGameID, 101, "North FC", 4204, "Late Adds")
	world.lps.schedule(syncSharedGameID, 101, "North FC", 202, "South FC")
	for i, id := range []string{syncImportedGameID, syncDeletedGameID, syncDuplicatedGameID, syncConcurrentGameID, syncHandWrittenGameID, syncUnmarkedGameID} {
		world.lps.schedule(id, 101, "North FC", 4210+i, "Visitors "+id)
	}

	mux, _ := buildMux(application, application.Logger, false)
	world.browser = newSiteBrowser(t, mux)
	world.browser.signIn("/soccer")
	completeGoogleConsent(t, world.browser)
	return world
}

// choose saves a destination calendar.
func (world *resultSyncWorld) choose(t *testing.T, calendarID string) {
	t.Helper()
	if chosen := world.browser.postForm("/soccer/google/calendar", url.Values{"calendar_id": {calendarID}}); chosen.Code != http.StatusOK {
		t.Fatalf("calendar choice status = %d", chosen.Code)
	}
}

// add adds the upcoming games to the destination calendar with the real Add,
// fetching the schedule of teamID.
func (world *resultSyncWorld) add(t *testing.T, teamID string, gameIDs ...string) {
	t.Helper()
	added := world.browser.postForm("/soccer/google/add", url.Values{"team_codes": {teamID}, "selected": gameIDs})
	if want := fmt.Sprintf("Added %d selected game(s)", len(gameIDs)); added.Code != http.StatusOK || !strings.Contains(added.Body.String(), want) {
		t.Fatalf("Add of %v answered %d %q, want %q", gameIDs, added.Code, added.Body.String(), want)
	}
}

// reviewForm fetches the schedule of teamID in Google mode and returns the
// past results form as the page renders it, every result selected.
func (world *resultSyncWorld) reviewForm(t *testing.T, teamID string) url.Values {
	t.Helper()
	return linkedFormValues(t, world.review(t, teamID), "past-results-form")
}

// review fetches the schedule of teamID and returns the page's review of it.
func (world *resultSyncWorld) review(t *testing.T, teamID string) *html.Node {
	t.Helper()
	fetched := world.browser.postForm("/soccer/fetch", url.Values{"team_codes": {teamID}})
	if fetched.Code != http.StatusOK {
		t.Fatalf("schedule fetch status = %d", fetched.Code)
	}
	return parsePlannerHTML(t, fetched.Body.String())
}

// sync posts the past results form to the Sync action and returns the
// feedback it rendered.
func (world *resultSyncWorld) sync(t *testing.T, form url.Values, gameIDs ...string) string {
	t.Helper()
	form = cloneValues(form)
	form["selected"] = gameIDs
	synced := world.browser.postForm("/soccer/google/sync-results", form)
	if synced.Code != http.StatusOK {
		t.Fatalf("Sync status = %d", synced.Code)
	}
	return synced.Body.String()
}

func cloneValues(values url.Values) url.Values {
	cloned := url.Values{}
	for key, value := range values {
		cloned[key] = slices.Clone(value)
	}
	return cloned
}

// withResult returns an event description with its blank result slot, as Add
// wrote it for an upcoming game, filled with result.
func withResult(t *testing.T, description, result string) string {
	t.Helper()
	switch {
	case strings.Contains(description, "\nResult: \n"):
		return strings.Replace(description, "\nResult: \n", "\nResult: "+result+"\n", 1)
	case strings.HasSuffix(description, "\nResult: "):
		return description + result
	}
	t.Fatalf("description %q has no blank result slot", description)
	return ""
}

// withoutDescription returns the event's fields other than its description
// and version, which result Sync must leave as they were.
func withoutDescription(event *internalgoogle.Event) internalgoogle.Event {
	rest := *event
	rest.Description, rest.ETag = "", ""
	return rest
}

func TestSyncWritesOnlyTheResultTextOfEventsThisSiteAddedToTheChosenCalendar(t *testing.T) {
	world := newResultSyncWorld(t)
	// An earlier Add left a copy of the first game in primary; the visitor
	// then chose the team calendar and added both games there.
	world.add(t, syncNorthTeamID, syncWonGameID)
	world.choose(t, teamCalendarID)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDrawnGameID)

	// The visitor made the first event their own in Google Calendar.
	world.google.editEvent(teamCalendarID, syncWonGameID, func(event *internalgoogle.Event) {
		event.Summary = "Cup final!"
		event.Location = "Grandma's pitch"
		event.Start.DateTime, event.End.DateTime = "2026-10-03T18:00:00", "2026-10-03T19:30:00"
		event.Reminders = &internalgoogle.EventReminders{Overrides: []internalgoogle.EventReminder{{Method: "email", Minutes: 1440}}}
		event.Description = "Bring oranges\n" + event.Description + "\nCarpool: Sam drives"
	})
	primaryBefore := world.google.events(primaryCalendarID)
	teamBefore := world.google.events(teamCalendarID)

	world.lps.play(syncWonGameID, "2-1")
	world.lps.play(syncDrawnGameID, "0 - 0")
	world.lps.play(syncMissingGameID, "1-0")
	world.lps.play(syncUnselectedGameID, "4-0")
	review := world.review(t, syncNorthTeamID)
	form := linkedFormValues(t, review, "past-results-form")
	if got, want := form["selected"], []string{syncWonGameID, syncDrawnGameID, syncMissingGameID, syncUnselectedGameID}; !sameMembers(got, want) {
		t.Fatalf("the review offered past results %v, want %v", got, want)
	}
	// The review says what Sync will and will not do before the visitor acts.
	past := plannerSingle(t, review, "past results form", plannerAttrIs("id", "past-results-form"))
	if text := plannerText(past); !strings.Contains(text, "Sync writes each selected score into the event this site added to your chosen calendar and never adds a past game.") {
		t.Errorf("the past results review does not explain Sync: %q", text)
	}

	calls, patches := world.google.callCount(), world.google.patchCount()
	synced := world.sync(t, form, syncWonGameID, syncDrawnGameID, syncMissingGameID)

	if want := "2 game result(s) updated in Google Calendar. Skipped 1 game(s): 1 unmatched (no event this site added)."; !strings.Contains(synced, want) {
		t.Fatalf("Sync answered %q, want %q", synced, want)
	}
	teamAfter := world.google.events(teamCalendarID)
	if len(teamAfter) != 2 {
		t.Fatalf("Sync left %d events in the chosen calendar, want the 2 Add wrote", len(teamAfter))
	}
	for id, want := range map[string]string{syncWonGameID: "Win (2-1)", syncDrawnGameID: "Draw (0-0)"} {
		before, after := teamBefore[id], teamAfter[id]
		if wantDescription := withResult(t, before.Description, want); after.Description != wantDescription {
			t.Errorf("event %s description = %q, want %q", id, after.Description, wantDescription)
		}
		if !reflect.DeepEqual(withoutDescription(&after), withoutDescription(&before)) {
			t.Errorf("Sync changed event %s beyond its result text:\nbefore %+v\nafter  %+v", id, withoutDescription(&before), withoutDescription(&after))
		}
	}
	if !reflect.DeepEqual(world.google.events(primaryCalendarID), primaryBefore) {
		t.Error("Sync changed the copy left in the calendar chosen before")
	}

	// Each update is one conditional patch of the description alone, and
	// nothing is inserted, replaced, or sent for the unselected game.
	sent := world.google.patchesSince(patches)
	patched := make([]string, 0, len(sent))
	for _, patch := range sent {
		patched = append(patched, patch.eventID)
		if patch.calendarID != teamCalendarID || patch.ifMatch != teamBefore[patch.eventID].ETag || !slices.Equal(patch.fields, []string{"description"}) {
			t.Errorf("Sync sent patch %+v; want a description-only patch of the chosen calendar's event conditional on version %s", patch, teamBefore[patch.eventID].ETag)
		}
	}
	if !sameMembers(patched, []string{syncWonGameID, syncDrawnGameID}) {
		t.Errorf("Sync patched %v, want each updated event once", patched)
	}
	// Each selected game costs one search of the chosen calendar, plus the
	// patch when there is a result to write, or, when the search finds no
	// site event, one read by the site's event ID to tell a deleted event
	// from a missing one.
	var searches int
	for _, call := range world.google.callsSince(calls) {
		switch {
		case call == http.MethodGet+" "+teamCalendarID:
			searches++
		case call == http.MethodPatch+" "+teamCalendarID+"/"+syncWonGameID, call == http.MethodPatch+" "+teamCalendarID+"/"+syncDrawnGameID,
			call == http.MethodGet+" "+teamCalendarID+"/"+syncMissingGameID:
		default:
			t.Errorf("Sync sent %q", call)
		}
	}
	if searches != 3 {
		t.Errorf("Sync searched the chosen calendar %d times for 3 selected games", searches)
	}

	// Syncing again changes nothing.
	settled := world.google.events(teamCalendarID)
	patches = world.google.patchCount()
	repeated := world.sync(t, form, syncWonGameID, syncDrawnGameID, syncMissingGameID)
	if want := "0 game result(s) updated in Google Calendar. 2 result(s) already current. Skipped 1 game(s): 1 unmatched (no event this site added)."; !strings.Contains(repeated, want) {
		t.Fatalf("repeated Sync answered %q; want nothing updated and both results already current", repeated)
	}
	if again := world.google.patchesSince(patches); len(again) != 0 || !reflect.DeepEqual(world.google.events(teamCalendarID), settled) {
		t.Fatalf("repeated Sync changed events again: patches %+v", again)
	}
}

func TestSyncSkipsAndReportsEventsItCannotSafelyClaim(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDeletedGameID, syncDuplicatedGameID, syncConcurrentGameID, syncHandWrittenGameID)
	added := world.google.events(primaryCalendarID)

	// An .ics import carries the same description, but not the site's
	// private marker, and Google gives it its own event ID.
	imported := internalgoogle.Event{
		ID: "icsimport9105", Status: "confirmed", Summary: "North FC vs Visitors 9105 - Field 5",
		Description: "North FC is playing Visitors 9105\nDivision: \nFacility: Field 5\nField: Field 5\nResult: ",
	}
	world.google.addEvent(primaryCalendarID, &imported)
	// Another client wrote the game's ID but not the site's marker.
	unmarked := internalgoogle.Event{
		ID: "otherclient9110", Status: "confirmed", Summary: "North FC vs Visitors 9110",
		Description: "North FC is playing Visitors 9110\nDivision: \nFacility: Field 0\nField: Field 0\nResult: ",
	}
	unmarked.ExtendedProperties.Private = map[string]string{"game_id": syncUnmarkedGameID}
	world.google.addEvent(primaryCalendarID, &unmarked)
	world.google.deleteEvent(primaryCalendarID, syncDeletedGameID)
	duplicate := added[syncDuplicatedGameID]
	duplicate.ID = "copyof9107"
	world.google.addEvent(primaryCalendarID, &duplicate)
	world.google.editEvent(primaryCalendarID, syncHandWrittenGameID, func(event *internalgoogle.Event) {
		event.Description = strings.Replace(event.Description, "\nResult: ", "\nResult: we won on penalties!", 1)
	})
	// The visitor moves this game while Sync is reading it.
	world.google.changeEventBeforeNextPatch(syncConcurrentGameID, func(event *internalgoogle.Event) {
		event.Description += "\nMoved to Field 9"
	})
	before := world.google.events(primaryCalendarID)

	games := []string{syncWonGameID, syncMissingGameID, syncImportedGameID, syncUnmarkedGameID, syncDeletedGameID, syncDuplicatedGameID, syncConcurrentGameID, syncHandWrittenGameID}
	for _, id := range games {
		world.lps.play(id, "2-1")
	}
	calls, patches := world.google.callCount(), world.google.patchCount()
	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), games...)

	const want = "1 game result(s) updated in Google Calendar. Skipped 7 game(s): 3 unmatched (no event this site added), 1 deleted, " +
		"1 with more than one matching event, 1 changed in Google Calendar during Sync, 1 with an edited description."
	if !strings.Contains(synced, want) {
		t.Fatalf("Sync answered %q; want %q", synced, want)
	}
	after := world.google.events(primaryCalendarID)
	if len(after) != len(before) {
		t.Fatalf("Sync left %d events, want the %d there before", len(after), len(before))
	}
	for id, event := range before {
		switch id {
		case syncWonGameID:
			if got := after[id].Description; got != withResult(t, event.Description, "Win (2-1)") {
				t.Errorf("the site's own event was not updated: %q", got)
			}
		case syncConcurrentGameID:
			if got := after[id].Description; !strings.HasSuffix(got, "\nResult: \nMoved to Field 9") {
				t.Errorf("Sync overwrote the edit made while it ran: %q", got)
			}
		default:
			if !reflect.DeepEqual(after[id], event) {
				t.Errorf("Sync changed event %s:\nbefore %+v\nafter  %+v", id, event, after[id])
			}
		}
	}
	if after[syncDeletedGameID].Status != googleDeletedStatus {
		t.Error("Sync restored the deleted event")
	}

	// Only the site's own event and the one changed meanwhile were patched;
	// nothing was inserted or replaced.
	sent := world.google.patchesSince(patches)
	patched := make([]string, 0, len(sent))
	for _, patch := range sent {
		patched = append(patched, patch.eventID)
	}
	if !sameMembers(patched, []string{syncWonGameID, syncConcurrentGameID}) {
		t.Errorf("Sync patched %v, want only %s and the attempt on %s", patched, syncWonGameID, syncConcurrentGameID)
	}
	for _, call := range world.google.callsSince(calls) {
		if strings.HasPrefix(call, http.MethodPost+" ") || strings.HasPrefix(call, http.MethodPut+" ") {
			t.Errorf("Sync sent %q", call)
		}
	}
}

// Sync reports a game with two live copies of its event as ambiguous. Once
// the visitor deletes the extra copy, the one left is the game's event, even
// while Google still lists the deleted copy with its properties.
func TestSyncUpdatesTheOneLiveEventLeftAfterTheVisitorDeletesACopy(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncDuplicatedGameID)
	duplicate := world.google.events(primaryCalendarID)[syncDuplicatedGameID]
	duplicate.ID = "copyof9107"
	world.google.addEvent(primaryCalendarID, &duplicate)
	world.lps.play(syncDuplicatedGameID, "2-1")
	form := world.reviewForm(t, syncNorthTeamID)
	if synced := world.sync(t, form, syncDuplicatedGameID); !strings.Contains(synced, "Skipped 1 game(s): 1 with more than one matching event.") {
		t.Fatalf("Sync with two live copies answered %q", synced)
	}

	world.google.editEvent(primaryCalendarID, "copyof9107", func(event *internalgoogle.Event) { event.Status = googleDeletedStatus })
	before := world.google.events(primaryCalendarID)
	calls, patches := world.google.callCount(), world.google.patchCount()
	if synced := world.sync(t, form, syncDuplicatedGameID); !strings.Contains(synced, "1 game result(s) updated in Google Calendar.") {
		t.Fatalf("Sync after the copy was deleted answered %q", synced)
	}
	sent := world.google.patchesSince(patches)
	if len(sent) != 1 || sent[0].eventID != syncDuplicatedGameID || sent[0].ifMatch != before[syncDuplicatedGameID].ETag {
		t.Fatalf("Sync sent patches %+v; want one patch of the live event conditional on %s", sent, before[syncDuplicatedGameID].ETag)
	}
	after := world.google.events(primaryCalendarID)
	if !reflect.DeepEqual(after["copyof9107"], before["copyof9107"]) {
		t.Error("Sync changed or restored the deleted copy")
	}
	for _, call := range world.google.callsSince(calls) {
		if strings.HasPrefix(call, http.MethodPost+" ") || strings.HasPrefix(call, http.MethodPut+" ") {
			t.Errorf("Sync sent %q", call)
		}
	}
}

// Google may answer a search with a page holding fewer events than asked
// for, or none, while more follow. Sync reads on until the search ends, and
// judges the match only by every event it found.
func TestSyncReadsEverySearchPageBeforeJudgingTheMatch(t *testing.T) {
	type pages = [][]internalgoogle.Event
	for _, tc := range []struct {
		name  string
		split func([]internalgoogle.Event) pages
		// wantPages is how many pages Sync reads; want, what it reports.
		wantPages int
		want      string
	}{
		{
			name:      "the event on a first page that says more follow",
			split:     func(events []internalgoogle.Event) pages { return pages{events, nil} },
			wantPages: 2, want: "1 game result(s) updated in Google Calendar.",
		},
		{
			name:      "the event after an empty first page",
			split:     func(events []internalgoogle.Event) pages { return pages{nil, events} },
			wantPages: 2, want: "1 game result(s) updated in Google Calendar.",
		},
		{
			name:      "more pages than Sync reads",
			split:     func(events []internalgoogle.Event) pages { return append(make(pages, 5), events) },
			wantPages: 5, want: "0 game result(s) updated in Google Calendar. Skipped 1 game(s): 1 with more than one matching event.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newResultSyncWorld(t)
			world.add(t, syncNorthTeamID, syncWonGameID)
			added := world.google.events(primaryCalendarID)[syncWonGameID]
			world.lps.play(syncWonGameID, "2-1")
			form := world.reviewForm(t, syncNorthTeamID)
			world.google.splitEventSearches(tc.split)

			calls, patches := world.google.callCount(), world.google.patchCount()
			if synced := world.sync(t, form, syncWonGameID); !strings.Contains(synced, tc.want) {
				t.Fatalf("Sync answered %q, want %q", synced, tc.want)
			}
			var searched int
			for _, call := range world.google.callsSince(calls) {
				if call == http.MethodGet+" "+primaryCalendarID {
					searched++
				}
			}
			if searched != tc.wantPages {
				t.Errorf("Sync read %d search pages, want %d", searched, tc.wantPages)
			}
			sent := world.google.patchesSince(patches)
			if !strings.Contains(tc.want, "1 game result(s) updated") {
				if len(sent) != 0 {
					t.Fatalf("Sync patched %+v without reading the whole search", sent)
				}
				return
			}
			if len(sent) != 1 || sent[0].ifMatch != added.ETag {
				t.Fatalf("Sync sent patches %+v; want one conditional on %s", sent, added.ETag)
			}
		})
	}
}

// Before events carried portfolio_app=soccer, the site wrote each one with
// the game's ID as its event ID and private game_id, and itself as the
// source. Sync claims such an event, since Add cannot re-add a past game to
// mark it; the same shape without the site as its source stays unmatched.
func TestSyncClaimsEventsTheSiteAddedBeforeItsOwnershipMarker(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDrawnGameID)
	for _, id := range []string{syncWonGameID, syncDrawnGameID} {
		world.google.editEvent(primaryCalendarID, id, func(event *internalgoogle.Event) {
			event.ExtendedProperties.Private = map[string]string{"game_id": id}
			if id == syncDrawnGameID {
				event.Source = nil
			}
		})
	}
	before := world.google.events(primaryCalendarID)
	if source := before[syncWonGameID].Source; source == nil || source.Title != "Soccer Schedule" || !strings.HasSuffix(source.URL, "/soccer") {
		t.Fatalf("the pre-marker event names source %+v", source)
	}
	world.lps.play(syncWonGameID, "2-1")
	world.lps.play(syncDrawnGameID, "1-1")

	calls, patches := world.google.callCount(), world.google.patchCount()
	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), syncWonGameID, syncDrawnGameID)
	if want := "1 game result(s) updated in Google Calendar. Skipped 1 game(s): 1 unmatched (no event this site added)."; !strings.Contains(synced, want) {
		t.Fatalf("Sync answered %q, want %q", synced, want)
	}
	after := world.google.events(primaryCalendarID)
	if got, want := after[syncWonGameID].Description, withResult(t, before[syncWonGameID].Description, "Win (2-1)"); got != want {
		t.Errorf("the pre-marker event reads %q, want %q", got, want)
	}
	if !reflect.DeepEqual(after[syncDrawnGameID], before[syncDrawnGameID]) {
		t.Errorf("Sync changed the event without the site's source: %+v", after[syncDrawnGameID])
	}
	sent := world.google.patchesSince(patches)
	if len(sent) != 1 || sent[0].eventID != syncWonGameID || sent[0].ifMatch != before[syncWonGameID].ETag || !slices.Equal(sent[0].fields, []string{"description"}) {
		t.Errorf("Sync sent patches %+v; want one description-only patch of %s conditional on %s", sent, syncWonGameID, before[syncWonGameID].ETag)
	}
	for _, call := range world.google.callsSince(calls) {
		if strings.HasPrefix(call, http.MethodPost+" ") || strings.HasPrefix(call, http.MethodPut+" ") {
			t.Errorf("Sync sent %q", call)
		}
	}
}

// A game two followed teams play is one event, worded for the team whose
// schedule Add used. Its result stays in that team's words when Sync reads
// the game from the other team's schedule.
func TestSyncWritesTheResultForTheTeamTheEventNames(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncSouthTeamID, syncSharedGameID)
	before := world.google.events(primaryCalendarID)[syncSharedGameID]
	if !strings.HasPrefix(before.Description, "South FC is playing North FC\n") {
		t.Fatalf("Add from South FC's schedule wrote %q", before.Description)
	}

	// North FC won 3-1 at home.
	world.lps.play(syncSharedGameID, "3-1")
	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), syncSharedGameID)

	if !strings.Contains(synced, "1 game result(s) updated in Google Calendar.") {
		t.Fatalf("Sync answered %q", synced)
	}
	if got, want := world.google.events(primaryCalendarID)[syncSharedGameID].Description, withResult(t, before.Description, "Loss (1-3)"); got != want {
		t.Fatalf("South FC's event reads %q, want %q", got, want)
	}
}

// Google Calendar's editor saves a description the visitor edited as HTML,
// its lines joined by <br>. Sync still finds the block Add wrote and changes
// only its result.
func TestSyncWritesTheResultIntoADescriptionEditedInGoogleCalendar(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncWonGameID)
	world.google.editEvent(primaryCalendarID, syncWonGameID, func(event *internalgoogle.Event) {
		event.Description = "<b>Bring oranges</b><br>" + strings.ReplaceAll(event.Description, "\n", "<br>") + "<br><i>Carpool: Sam drives</i>"
	})
	before := world.google.events(primaryCalendarID)[syncWonGameID]
	if !strings.Contains(before.Description, "<br>Result: <br><i>Carpool") {
		t.Fatalf("the edited description reads %q", before.Description)
	}

	world.lps.play(syncWonGameID, "2-1")
	form := world.reviewForm(t, syncNorthTeamID)
	if synced := world.sync(t, form, syncWonGameID); !strings.Contains(synced, "1 game result(s) updated in Google Calendar.") {
		t.Fatalf("Sync answered %q", synced)
	}
	after := world.google.events(primaryCalendarID)[syncWonGameID]
	if want := strings.Replace(before.Description, "<br>Result: <br>", "<br>Result: Win (2-1)<br>", 1); after.Description != want {
		t.Fatalf("the edited event reads %q, want %q", after.Description, want)
	}
	if !reflect.DeepEqual(withoutDescription(&after), withoutDescription(&before)) {
		t.Errorf("Sync changed the edited event beyond its result text:\nbefore %+v\nafter  %+v", withoutDescription(&before), withoutDescription(&after))
	}

	patches := world.google.patchCount()
	if repeated := world.sync(t, form, syncWonGameID); !strings.Contains(repeated, "1 result(s) already current.") || len(world.google.patchesSince(patches)) != 0 {
		t.Fatalf("repeated Sync answered %q and sent %d patches; want the result already current", repeated, len(world.google.patchesSince(patches)))
	}
}

func TestGoogleRefusingSyncDecidesBetweenReconnectRetryAndANewChoice(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reading bool
		refusal googleRefusal
		// wantMessage is what the Sync response asks the visitor to do.
		wantMessage string
		// wantConnected reports whether the Google connection survives;
		// wantPaused whether writes wait for a new calendar choice.
		wantConnected, wantPaused bool
	}{
		{name: "credentials rejected", refusal: googleRefusal{http.StatusUnauthorized, "global", "authError"}, wantMessage: "Connect again"},
		{name: "Calendar access not granted", refusal: googleRefusal{http.StatusForbidden, "global", "insufficientPermissions"}, wantMessage: "Connect again"},
		{name: "rate limited", refusal: googleRefusal{http.StatusForbidden, "usageLimits", "rateLimitExceeded"}, wantMessage: "Retry later", wantConnected: true},
		{name: "rate limited while reading", reading: true, refusal: googleRefusal{http.StatusForbidden, "usageLimits", "userRateLimitExceeded"}, wantMessage: "Retry later", wantConnected: true},
		{name: "calendar refuses writes", refusal: googleRefusal{http.StatusForbidden, "calendar", "requiredAccessLevel"}, wantMessage: calendarChoiceNeeded, wantConnected: true, wantPaused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newResultSyncWorld(t)
			world.add(t, syncNorthTeamID, syncWonGameID)
			added := world.google.events(primaryCalendarID)[syncWonGameID]
			world.lps.play(syncWonGameID, "2-1")
			form := world.reviewForm(t, syncNorthTeamID)
			if tc.reading {
				world.google.refuseEventReads(&tc.refusal)
			} else {
				world.google.refuseEventWrites(&tc.refusal)
			}

			if synced := world.sync(t, form, syncWonGameID); !strings.Contains(synced, tc.wantMessage) {
				t.Fatalf("Sync refused with %d %s answered %q, want it to say %q", tc.refusal.status, tc.refusal.reason, synced, tc.wantMessage)
			}
			world.google.refuseEventReads(nil)
			world.google.refuseEventWrites(nil)
			if got := world.google.events(primaryCalendarID)[syncWonGameID]; !reflect.DeepEqual(got, added) {
				t.Fatalf("the refused Sync changed the event: %+v", got)
			}
			page := world.browser.get("/soccer").Body.String()
			if !tc.wantConnected {
				if len(world.store.records) != 0 || !strings.Contains(page, "Not connected") {
					t.Fatal("a connection Google no longer accepts was kept")
				}
				return
			}
			if len(world.store.records) != 1 || strings.Contains(page, "Not connected") {
				t.Fatal("Sync removed a connection Google still accepts")
			}
			if tc.wantPaused != strings.Contains(page, calendarChoiceNeeded) || tc.wantPaused == strings.Contains(page, calendarReady) {
				t.Fatalf("after the refusal the page paused writes = %t, want %t", strings.Contains(page, calendarChoiceNeeded), tc.wantPaused)
			}
			retried := world.sync(t, form, syncWonGameID)
			if tc.wantPaused {
				if !strings.Contains(retried, calendarChoiceNeeded) {
					t.Fatalf("paused writes resumed without a new calendar choice: %q", retried)
				}
				return
			}
			if !strings.Contains(retried, "1 game result(s) updated") {
				t.Fatalf("retry after the refusal did not update the result: %q", retried)
			}
		})
	}
}

func TestGoogleRefusingToChangeOneEventSkipsItsResultAndKeepsSyncing(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDrawnGameID)
	// The first event became another organizer's, whose changes Google
	// refuses to this account.
	world.google.refuseEventWrite(syncWonGameID, googleRefusal{http.StatusForbidden, "calendar", "forbiddenForNonOrganizer"})
	world.lps.play(syncWonGameID, "2-1")
	world.lps.play(syncDrawnGameID, "1-1")

	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), syncWonGameID, syncDrawnGameID)
	if want := "1 game result(s) updated in Google Calendar. Skipped 1 game(s): 1 Google Calendar would not let this account change."; !strings.Contains(synced, want) {
		t.Fatalf("Sync answered %q, want %q", synced, want)
	}
	if !strings.HasSuffix(world.google.events(primaryCalendarID)[syncDrawnGameID].Description, "\nResult: Draw (1-1)") {
		t.Fatal("the game after the refused event was not updated")
	}
	if page := world.browser.get("/soccer").Body.String(); !strings.Contains(page, calendarReady) || len(world.store.records) != 1 {
		t.Fatal("a refusal of one event paused the destination or removed the connection")
	}
}

func TestADestinationLostPartwayThroughSyncReportsTheResultsAlreadyWritten(t *testing.T) {
	world := newResultSyncWorld(t)
	world.choose(t, teamCalendarID)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDrawnGameID)
	world.lps.play(syncWonGameID, "2-1")
	world.lps.play(syncDrawnGameID, "1-1")
	// The calendar stops accepting this account's writes after the first
	// result.
	world.google.refuseEventWrite(syncDrawnGameID, googleRefusal{http.StatusForbidden, "calendar", "requiredAccessLevel"})

	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), syncWonGameID, syncDrawnGameID)
	for _, want := range []string{"1 game result(s) updated in Google Calendar.", calendarChoiceNeeded} {
		if !strings.Contains(synced, want) {
			t.Errorf("Sync that lost its destination partway answered %q; want it to say %q", synced, want)
		}
	}
	if !strings.HasSuffix(world.google.events(teamCalendarID)[syncWonGameID].Description, "\nResult: Win (2-1)") {
		t.Error("the first result was not written before the calendar refused writes")
	}
}

func TestCredentialsRejectedPartwayThroughSyncReportTheResultsAlreadyWritten(t *testing.T) {
	world := newResultSyncWorld(t)
	world.add(t, syncNorthTeamID, syncWonGameID, syncDrawnGameID)
	world.lps.play(syncWonGameID, "2-1")
	world.lps.play(syncDrawnGameID, "1-1")
	// Google stops accepting the connection after the first result.
	world.google.refuseEventWrite(syncDrawnGameID, googleRefusal{http.StatusUnauthorized, "global", "authError"})

	synced := world.sync(t, world.reviewForm(t, syncNorthTeamID), syncWonGameID, syncDrawnGameID)
	for _, want := range []string{"1 game result(s) updated in Google Calendar.", "Your Google Calendar connection is no longer valid. Connect again and retry."} {
		if !strings.Contains(synced, want) {
			t.Errorf("Sync whose credentials Google rejected partway answered %q; want it to say %q", synced, want)
		}
	}
	if !strings.HasSuffix(world.google.events(primaryCalendarID)[syncWonGameID].Description, "\nResult: Win (2-1)") {
		t.Error("the first result was not written before Google rejected the connection")
	}
	if page := world.browser.get("/soccer").Body.String(); len(world.store.records) != 0 || !strings.Contains(page, "Not connected") {
		t.Error("a connection Google no longer accepts was kept")
	}
}

// sameMembers reports whether got holds exactly the values of want, in any
// order.
func sameMembers(got, want []string) bool {
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	return slices.Equal(got, want)
}
