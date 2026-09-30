package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/html"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

// These tests drive the Google-mode review of scored past games (#95) through
// the real route assembly: a granted owner signs in through a fake Cognito,
// looks up North FC (101) and South FC (202) by Team ID and as the teams of
// linked players Craig (1001) and Taylor (1002) from a fake Let's Play Soccer
// API, and may connect a fake Google account.
//
// North FC has one upcoming game (704), a scored game from yesterday that it
// shares with South FC (701), a scored game from about two and a half years
// ago (702), and four past games without a score: one LPS left blank (703)
// and three that LPS marked canceled, final, and postponed (705-707). South FC
// adds a scored game from ten days ago (708).
const (
	pastResultsUpcomingID = "704"
	pastResultsRecentID   = "701"
	pastResultsMiddleID   = "708"
	pastResultsOldestID   = "702"
)

// pastResultsNewestFirst is every scored past game once, newest first.
var pastResultsNewestFirst = []string{pastResultsRecentID, pastResultsMiddleID, pastResultsOldestID}

// pastResultsUnscoredIDs are past games without a score, which never enter
// the result review or its Sync.
var pastResultsUnscoredIDs = []string{"703", "705", "706", "707"}

type pastResultsWorld struct {
	app     *App
	store   *appTestGoogleConnectionStore
	google  *fakeGoogleCalendars
	browser *siteBrowser
	jwt     string
	// googleRequests counts every request the fake Google received, including
	// OAuth and calendar list requests that callCount does not.
	googleRequests atomic.Int32
}

func newPastResultsWorld(t *testing.T) *pastResultsWorld {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	world := &pastResultsWorld{app: cognito.app(t), google: newFakeGoogleCalendars(t), jwt: testutil.TestJWT(t, time.Now().Add(time.Hour))}
	world.app.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	world.app.Config.GoogleClientID = "google-client"
	world.app.Config.GoogleClientSecret = "google-secret"
	world.app.Config.GoogleConnectionTableName = "connections"
	world.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	world.app.GoogleHandler.SetStore(world.store)

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		world.googleRequests.Add(1)
		world.google.ServeHTTP(w, r)
	}))
	t.Cleanup(google.Close)
	world.app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	world.app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	world.app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	world.app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	at := func(offset time.Duration) string { return testutil.MislabelledLPSZuluTime(time.Now().Add(offset)) }
	day := 24 * time.Hour
	game := func(id, kickoff, result, home string, homeID int, away string, awayID int) string {
		return fmt.Sprintf(`{"UGameID":%s,"SchedGameDateTime":%q,"field_name":"Field 1","result":%q,"UTeam1":%d,"UTeam2":%d,`+
			`"home_team":{"UTeamID":%d,"team_name":%q},"visitor_team":{"UTeamID":%d,"team_name":%q}}`,
			id, kickoff, result, homeID, awayID, homeID, home, awayID, away)
	}
	shared := game(pastResultsRecentID, at(-day), "3-1", "North FC", 101, "South FC", 202)
	north := publicScheduleJSON(
		game(pastResultsUpcomingID, at(day), "", "North FC", 101, "Guests", 301),
		game(pastResultsOldestID, at(-900*day), "1-0", "North FC", 101, "Old Rivals", 302),
		game("703", at(-2*day), "", "North FC", 101, "No Score FC", 303),
		game("705", at(-3*day), "canceled", "North FC", 101, "Canceled FC", 304),
		game("706", at(-4*day), "final", "North FC", 101, "Final FC", 305),
		game("707", at(-5*day), "postponed", "North FC", 101, "Postponed FC", 306),
		shared,
	)
	south := publicScheduleJSON(shared, game(pastResultsMiddleID, at(-10*day), "0 - 0", "South FC", 202, "Draw FC", 307))

	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		linked := r.Header.Get("Authorization") == "Bearer "+world.jwt
		switch r.URL.Path {
		case "/users/check":
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson",` +
				`"players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson"}],` +
				`"user_players":[{"player_id":1001,"deleted":false},{"player_id":1002,"deleted":false}]}`))
		case "/players/1001/my_teams":
			if !linked {
				t.Error("North FC discovery omitted the imported LPS token")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":101,"team_name":"North FC","Season":77}]`))
		case "/players/1002/my_teams":
			if !linked {
				t.Error("South FC discovery omitted the imported LPS token")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":202,"team_name":"South FC","Season":77}]`))
		case "/teams/101":
			_, _ = w.Write([]byte(north))
		case "/teams/202":
			_, _ = w.Write([]byte(south))
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lps.Close)
	world.app.Config.LPSAPIBaseURL = lps.URL

	mux, _ := buildMux(world.app, world.app.Logger, false)
	world.browser = newSiteBrowser(t, mux)
	if landing := world.browser.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("owner sign-in status = %d", landing.Code)
	}
	return world
}

// pastResultsSource fetches the schedule for both teams from one schedule
// source and returns the review the route rendered.
type pastResultsSource struct {
	name  string
	fetch func(*testing.T) *html.Node
}

// sources lists both schedule sources, Team IDs first.
func (world *pastResultsWorld) sources() []pastResultsSource {
	return []pastResultsSource{{"Team IDs", world.fetchTeamIDs}, {"linked players", world.fetchLinkedPlayers}}
}

// fetchTeamIDs looks up both teams by Team ID, as the manual source does.
func (world *pastResultsWorld) fetchTeamIDs(t *testing.T) *html.Node {
	t.Helper()
	fetched := world.browser.postForm("/soccer/fetch", url.Values{"team_codes": {"101, 202"}})
	if fetched.Code != http.StatusOK {
		t.Fatalf("Team ID fetch status = %d", fetched.Code)
	}
	return parsePlannerHTML(t, fetched.Body.String())
}

// fetchLinkedPlayers imports the fake LPS account, chooses both linked
// players, and fetches the teams the planner offers for them.
func (world *pastResultsWorld) fetchLinkedPlayers(t *testing.T) *html.Node {
	t.Helper()
	if imported := world.browser.postForm("/soccer/import", url.Values{"jwt": {world.jwt}}); imported.Code != http.StatusOK {
		t.Fatalf("LPS import status = %d", imported.Code)
	}
	page := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
	teams := parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", linkedFormValues(t, page, "soccer-player-select-form")).Body.String())
	chosen := linkedFormValues(t, teams, "soccer-team-select-form")
	if !slices.Equal(chosen["team_ids"], []string{"101", "202"}) || !slices.Equal(chosen["player_ids"], []string{"1001", "1002"}) {
		t.Fatalf("linked team choice = %v", chosen)
	}
	fetched := world.browser.postForm("/soccer/fetch", chosen)
	if fetched.Code != http.StatusOK {
		t.Fatalf("linked-player fetch status = %d", fetched.Code)
	}
	return parsePlannerHTML(t, fetched.Body.String())
}

// assertScoredPastResultReview requires the schedule to list every scored
// past game once, newest first and selected, with its count and select-all,
// behind the Google-only gate that keeps it off the .ics path.
func assertScoredPastResultReview(t *testing.T, doc *html.Node, source string) {
	t.Helper()
	past := plannerGameRows(doc, "past-results")
	if got := plannerRowIDs(past); !slices.Equal(got, pastResultsNewestFirst) {
		t.Fatalf("%s past results = %v, want every scored past game once, newest first %v", source, got, pastResultsNewestFirst)
	}
	for _, row := range past {
		if !row.Checked {
			t.Errorf("%s past result %s did not begin selected", source, row.ID)
		}
	}
	if got := plannerRowIDs(plannerGameRows(doc, "upcoming-games")); !slices.Equal(got, []string{pastResultsUpcomingID}) {
		t.Errorf("%s upcoming games = %v, want only %s", source, got, pastResultsUpcomingID)
	}
	for _, id := range pastResultsUnscoredIDs {
		for _, input := range plannerElements(doc, plannerAttrIs("value", id)) {
			if soccerHTMLAttribute(input, "name") == "selected" {
				t.Errorf("%s offered unscored past game %s for selection", source, id)
			}
		}
	}

	count := plannerSingle(t, doc, source+" past selected count", func(node *html.Node) bool {
		return plannerHasAttr(node, "data-selected-count") && soccerHTMLAttribute(node, "data-game-group") == "past-results"
	})
	if got := plannerText(count); got != "3 games selected" {
		t.Errorf("%s past selected count = %q, want %q", source, got, "3 games selected")
	}
	selectAll := plannerSingle(t, doc, source+" past select-all", func(node *html.Node) bool {
		return node.Data == "input" && plannerHasAttr(node, "data-select-all") && soccerHTMLAttribute(node, "data-game-group") == "past-results"
	})
	if soccerHTMLAttribute(selectAll, "type") != "checkbox" || !plannerHasAttr(selectAll, "checked") {
		t.Errorf("%s past select-all is not a checked checkbox while every result is selected", source)
	}
	if label := selectAll.Parent; label == nil || label.Data != "label" || plannerText(label) != "Select all past results" {
		t.Errorf("%s past select-all lacks its visible label", source)
	}
	// The browser remembers explicit deselections for this team set.
	if got := linkedTeamFingerprint(t, doc); got != "101-202" {
		t.Errorf("%s team-set scope = %q, want 101-202", source, got)
	}
	assertPastResultControlsGoogleOnly(t, doc)
}

func TestGoogleModeReviewsScoredPastGamesFromTeamIDsAndLinkedPlayersWithoutCalendarRequests(t *testing.T) {
	world := newPastResultsWorld(t)
	completeGoogleConsent(t, world.browser)
	before := world.google.callCount()

	for _, from := range world.sources() {
		source := from.name
		t.Run(source, func(t *testing.T) {
			doc := from.fetch(t)
			assertScoredPastResultReview(t, doc, source)
			sync := plannerSingle(t, doc, source+" Sync action", func(node *html.Node) bool {
				return plannerHasAttr(node, "data-game-action") && soccerHTMLAttribute(node, "data-game-group") == "past-results"
			})
			if got := soccerHTMLAttribute(sync, "hx-post"); got != "/soccer/google/sync-results" || !strings.Contains(plannerText(sync), "Sync selected results") {
				t.Errorf("%s connected review lacks the explicit Sync action: posts %q", source, got)
			}
		})
	}

	if calls := world.google.callsSince(before); len(calls) != 0 {
		t.Fatalf("reviewing past results sent Calendar event requests %v", calls)
	}
}

func TestResultSyncTakesNoPastGameWithoutAScore(t *testing.T) {
	world := newPastResultsWorld(t)
	completeGoogleConsent(t, world.browser)
	doc := world.fetchTeamIDs(t)
	before := world.google.callCount()

	// A crafted request names games the review never offered: past games
	// without a score and an upcoming game.
	form := linkedFormValues(t, doc, "past-results-form")
	form["selected"] = append(slices.Clone(pastResultsUnscoredIDs), pastResultsUpcomingID)
	synced := world.browser.postForm("/soccer/google/sync-results", form)

	if !strings.Contains(synced.Body.String(), "No selected past results were found to sync.") {
		t.Fatalf("Sync of games without a score answered %q", synced.Body.String())
	}
	if calls := world.google.callsSince(before); len(calls) != 0 {
		t.Fatalf("Sync of games without a score sent Calendar event requests %v", calls)
	}
	for _, calendar := range []string{primaryCalendarID, teamCalendarID} {
		if events := world.google.events(calendar); len(events) != 0 {
			t.Fatalf("calendar %s gained events %v", calendar, events)
		}
	}
}

func TestResultSyncWithoutAValidGoogleConnectionWritesNothing(t *testing.T) {
	t.Run("never connected", func(t *testing.T) {
		world := newPastResultsWorld(t)
		for _, from := range world.sources() {
			source := from.name
			doc := from.fetch(t)
			assertScoredPastResultReview(t, doc, source)
			if actions := plannerElements(doc, func(node *html.Node) bool {
				return plannerHasAttr(node, "data-game-action") && soccerHTMLAttribute(node, "data-game-group") == "past-results"
			}); len(actions) != 0 {
				t.Errorf("%s review offered Sync without a Google connection", source)
			}
			connect := plannerSingle(t, doc, source+" Google connect prompt", func(node *html.Node) bool {
				return node.Data == "a" && soccerHTMLAttribute(node, "href") == "/soccer/google/connect"
			})
			if gate := plannerOutputGate(connect); gate == nil || soccerHTMLAttribute(gate, "data-soccer-output-only") != "google" {
				t.Errorf("%s Google connect prompt is not limited to Google mode", source)
			}

			synced := world.browser.postForm("/soccer/google/sync-results", linkedFormValues(t, doc, "past-results-form"))
			if !strings.Contains(synced.Body.String(), "Connect Google Calendar before syncing results.") {
				t.Errorf("%s Sync without a connection answered %q", source, synced.Body.String())
			}
		}
		if got := world.googleRequests.Load(); got != 0 {
			t.Fatalf("Sync without a Google connection sent %d requests to Google", got)
		}
	})

	t.Run("access revoked at Google", func(t *testing.T) {
		world := newPastResultsWorld(t)
		completeGoogleConsent(t, world.browser)
		doc := world.fetchTeamIDs(t)
		world.google.setRevoked(true)
		before := world.google.callCount()

		synced := world.browser.postForm("/soccer/google/sync-results", linkedFormValues(t, doc, "past-results-form"))

		if !strings.Contains(synced.Body.String(), "Connect again") {
			t.Fatalf("Sync with revoked Google access did not ask to reconnect: %q", synced.Body.String())
		}
		if calls := world.google.callsSince(before); len(calls) != 0 {
			t.Fatalf("Sync with revoked Google access sent Calendar event requests %v", calls)
		}
		if len(world.store.records) != 0 {
			t.Fatalf("revoked Google connection was kept: %d stored", len(world.store.records))
		}
	})
}

func TestICSDownloadLeavesOutScoredPastGames(t *testing.T) {
	world := newPastResultsWorld(t)
	doc := world.fetchTeamIDs(t)

	form := linkedFormValues(t, doc, "upcoming-games-form")
	form["selected"] = pastResultsNewestFirst
	refused := world.browser.postForm("/soccer/download", form)
	if refused.Code != http.StatusBadRequest || strings.Contains(refused.Body.String(), "BEGIN:VEVENT") {
		t.Fatalf("an .ics download of past results: status %d, body %q", refused.Code, refused.Body.String())
	}

	form["selected"] = append([]string{pastResultsUpcomingID}, pastResultsNewestFirst...)
	downloaded := world.browser.postForm("/soccer/download", form)
	events := icsEvents(testutil.UnfoldICS(downloaded.Body.String()))
	if downloaded.Code != http.StatusOK || len(events) != 1 || events[pastResultsUpcomingID] == "" {
		t.Fatalf("an .ics download of an upcoming game and past results holds %d events, want only %s", len(events), pastResultsUpcomingID)
	}
}
