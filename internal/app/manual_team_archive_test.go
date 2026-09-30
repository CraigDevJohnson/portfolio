package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/internal/testutil"
	"portfolio/types"
)

// archiveRoute serves the public Soccer routes with the durable archive wired
// to an in-memory DynamoDB table, as an approved activation would wire it.
type archiveRoute struct {
	app     *App
	mux     http.Handler
	handler *internalsoccer.Handler
	table   *archivetest.Table
	store   *soccerarchive.DynamoStore
}

func newArchiveRoute(t *testing.T, lps http.HandlerFunc) *archiveRoute {
	t.Helper()
	app := newTestApp(t)
	lpsServer := httptest.NewServer(lps)
	t.Cleanup(lpsServer.Close)
	app.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(app, app.Logger, false)
	table := archivetest.NewTable()
	store := soccerarchive.NewDynamoStoreWithAPI(table, "portfolio-lambda-dev-soccer-history")
	handler.SetArchiveStore(store)
	return &archiveRoute{app: app, mux: mux, handler: handler, table: table, store: store}
}

func (r *archiveRoute) lookup(t *testing.T, teamCodes string, decorate ...func(*http.Request)) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{"team_codes": {teamCodes}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, apply := range decorate {
		apply(req)
	}
	resp := httptest.NewRecorder()
	r.mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("team %q HTTP status = %d, want 200; body %q", teamCodes, resp.Code, resp.Body.String())
	}
	return resp.Body.String()
}

func (r *archiveRoute) assertNotArchived(t *testing.T, teamID int) {
	t.Helper()
	if _, err := r.store.ReadTeamSeason(context.Background(), teamID, 169); !errors.Is(err, soccerarchive.ErrNoArchive) {
		t.Fatalf("team %d read-back error = %v, want ErrNoArchive", teamID, err)
	}
}

const archiveFacilityResponse = `{"FacilityID":5,"FacilityName":"Downtown","Address":"123 Field St","City":"Boise","State":"ID","ZIP":"83702"}`

func boiseFCSchedule(kickoff string) string {
	return fmt.Sprintf(`{"team":{"UTeamID":479691,"team_name":"Boise FC","division_name":"Open A","FacilityID":5,"facility_name":"Downtown","Season":169},"games":[{"UGameID":8001,"SchedGameDateTime":%q,"FacilityID":5,"Field":2,"Season":169,"UTeam1":479691,"UTeam2":222,"home_team":{"UTeamID":479691,"team_name":"Boise FC"},"visitor_team":{"UTeamID":222,"team_name":"Away FC"},"result":""}]}`, kickoff)
}

func boiseFCLPS(t *testing.T, kickoff string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, boiseFCSchedule(kickoff))
		case "/facilities/5":
			_, _ = fmt.Fprint(w, archiveFacilityResponse)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

func TestManualTeamLookupArchivesReadableTeamSeasonFacts(t *testing.T) {
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))

	before := time.Now()
	body := route.lookup(t, "479691")
	after := time.Now()

	if !strings.Contains(body, "Away FC") || !strings.Contains(body, "Team 479691 added to history collection.") || !strings.Contains(body, "ui-feedback-success") {
		t.Fatalf("schedule or enrollment outcome missing: %q", body)
	}
	history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if history.Team.UTeamID != 479691 || history.Team.TeamName != "Boise FC" || history.Team.DivisionName != "Open A" || history.Team.Season != 169 || history.Team.FacilityID != 5 {
		t.Fatalf("stored team-season context = %#v", history.Team)
	}
	if history.Coverage.Status != soccerarchive.CoverageFetched || history.Coverage.ReturnedGameCount != 1 ||
		history.Coverage.FetchedAt.Before(before.Add(-time.Millisecond)) || history.Coverage.FetchedAt.After(after) {
		t.Fatalf("stored coverage = %#v, want fetched with one game between %s and %s", history.Coverage, before, after)
	}
	if len(history.Games) != 1 || history.Games[0].UGameID != 8001 || history.Games[0].UTeam1 != 479691 || history.Games[0].UTeam2 != 222 || history.Games[0].FacilityID != 5 || history.Games[0].Season != 169 {
		t.Fatalf("stored games = %#v", history.Games)
	}
	if len(history.Facilities) != 1 || history.Facilities[0].FacilityName != "Downtown" || history.Facilities[0].Address != "123 Field St" || history.Facilities[0].ZIP != "83702" {
		t.Fatalf("stored facilities = %#v", history.Facilities)
	}
	items, err := route.table.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	for key, item := range items {
		for _, sessionAttribute := range []string{"ttl", "expires_at"} {
			if _, found := item[sessionAttribute]; found {
				t.Errorf("archive item %s carries the import session attribute %q", key, sessionAttribute)
			}
		}
	}
}

func TestRepeatedManualTeamLookupKeepsOneArchivedGame(t *testing.T) {
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))

	route.lookup(t, "479691")
	storedAfterFirst := route.table.Len()
	body := route.lookup(t, "479691")

	if !strings.Contains(body, "Team 479691 added to history collection.") {
		t.Fatalf("repeated lookup lost the enrollment outcome: %q", body)
	}
	if got := route.table.Len(); got != storedAfterFirst {
		t.Fatalf("stored items after a repeated lookup = %d, want %d", got, storedAfterFirst)
	}
	history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if len(history.Games) != 1 || history.Games[0].UGameID != 8001 {
		t.Fatalf("games after a repeated lookup = %#v, want only game 8001", history.Games)
	}
}

func TestManualTeamLookupSeparatesEmptyScheduleFromInvalidAndFailedLookups(t *testing.T) {
	route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"team_name":"Dormant FC","Season":169},"games":[]}`)
		case "/teams/999999":
			http.NotFound(w, r)
		case "/teams/777777":
			http.Error(w, "upstream unavailable", http.StatusInternalServerError)
		case "/teams/888888":
			_, _ = fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	empty := route.lookup(t, "479691")
	if !strings.Contains(empty, "Let&#39;s Play Soccer accepted the team ID but returned no games.") || !strings.Contains(empty, "Team 479691 added to history collection.") {
		t.Fatalf("accepted empty schedule outcome missing: %q", empty)
	}
	history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason for the empty schedule: %v", err)
	}
	if history.Coverage.Status != soccerarchive.CoverageFetched || history.Coverage.ReturnedGameCount != 0 || len(history.Games) != 0 || history.Team.TeamName != "Dormant FC" {
		t.Fatalf("empty schedule was not archived as a fetched season with no games: %#v", history)
	}

	invalid := route.lookup(t, "999999")
	if !strings.Contains(invalid, "Team ID 999999 was not accepted by Let&#39;s Play Soccer.") || strings.Contains(invalid, "history collection") {
		t.Fatalf("invalid team outcome: %q", invalid)
	}
	route.assertNotArchived(t, 999999)

	failed := route.lookup(t, "777777")
	if !strings.Contains(failed, "Could not load schedules from Let&#39;s Play Soccer right now.") || strings.Contains(failed, "was not accepted") || strings.Contains(failed, "history collection") {
		t.Fatalf("failed fetch outcome: %q", failed)
	}
	route.assertNotArchived(t, 777777)

	unconfirmed := route.lookup(t, "888888")
	if !strings.Contains(unconfirmed, "Team 888888 was not added to history collection") || strings.Contains(unconfirmed, "accepted the team ID") || strings.Contains(unconfirmed, "was not accepted") {
		t.Fatalf("unconfirmed empty response outcome: %q", unconfirmed)
	}
	route.assertNotArchived(t, 888888)

	malformed := route.lookup(t, "not-a-team")
	if !strings.Contains(malformed, "were invalid") || strings.Contains(malformed, "history collection") {
		t.Fatalf("malformed team ID outcome: %q", malformed)
	}
}

func TestPartlyMalformedManualTeamLookupKeepsTheScheduleWithoutEnrollment(t *testing.T) {
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))

	for _, entry := range []string{"479691, not-a-team", "479691, 0"} {
		route.handler.SetArchiveStore(nil)
		usual := route.lookup(t, entry)
		if !strings.Contains(usual, "Away FC") {
			t.Fatalf("usual lookup for %q did not render team 479691: %q", entry, usual)
		}

		route.handler.SetArchiveStore(route.store)
		archived := route.lookup(t, entry)

		if !strings.HasSuffix(archived, usual) {
			t.Fatalf("archive changed the schedule for %q\nusual:    %q\narchived: %q", entry, usual, archived)
		}
		if !strings.Contains(archived, "Teams were not added to history collection because some entries were not valid team IDs.") ||
			!strings.Contains(archived, "ui-feedback-warning") || strings.Contains(archived, "added to history collection.") {
			t.Fatalf("partly malformed lookup %q enrollment outcome: %q", entry, archived)
		}
		route.assertNotArchived(t, 479691)
	}
}

func TestManualTeamLookupWithArchiveKeepsTheUsualSchedule(t *testing.T) {
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, boiseFCSchedule(future))
		case "/facilities/5":
			_, _ = fmt.Fprint(w, archiveFacilityResponse)
		case "/teams/888888":
			// The long-standing manual-lookup fixture shape: games but no team identity.
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":9001,"SchedGameDateTime":%q,"field_name":"Field 3","facilityName":"Boise","home_team":{"team_name":"UNITED NATIONS"},"visitor_team":{"team_name":"GALACTICOS FC"},"Season":169}]}`, future)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	route.handler.SetArchiveStore(nil)
	usual := route.lookup(t, "479691, 888888")
	if !strings.Contains(usual, "Away FC") || !strings.Contains(usual, "GALACTICOS FC") {
		t.Fatalf("usual manual schedule fixture did not render both teams: %q", usual)
	}

	route.handler.SetArchiveStore(route.store)
	archived := route.lookup(t, "479691, 888888")

	if !strings.HasSuffix(archived, usual) {
		t.Fatalf("archive changed the visitor's schedule\nusual:    %q\narchived: %q", usual, archived)
	}
	if !strings.Contains(archived, "Team 479691 added to history collection.") || !strings.Contains(archived, "Team 888888 was not added to history collection") {
		t.Fatalf("enrollment outcome does not separate the confirmed and unconfirmed teams: %q", archived)
	}
	if _, err := route.store.ReadTeamSeason(context.Background(), 479691, 169); err != nil {
		t.Fatalf("confirmed team was not archived: %v", err)
	}
	route.assertNotArchived(t, 888888)
}

func TestManualTeamLookupWithArchiveKeepsScoredPastResultsForGoogleReview(t *testing.T) {
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/479691" {
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":479691,"team_name":"Boise FC","Season":169},"games":[`+
			`{"UGameID":8001,"SchedGameDateTime":%q,"Season":169,"UTeam1":479691,"UTeam2":222,"home_team":{"UTeamID":479691,"team_name":"Boise FC"},"visitor_team":{"UTeamID":222,"team_name":"Away FC"}},`+
			`{"UGameID":8002,"SchedGameDateTime":%q,"Season":169,"UTeam1":479691,"UTeam2":223,"home_team":{"UTeamID":479691,"team_name":"Boise FC"},"visitor_team":{"UTeamID":223,"team_name":"Last Week FC"},"result":"2-1"}]}`,
			future, past)
	})
	route.handler.SetArchiveStore(nil)
	usual := route.lookup(t, "479691")
	if past := plannerRowIDs(plannerGameRows(parsePlannerHTML(t, usual), "past-results")); !slices.Equal(past, []string{"8002"}) {
		t.Fatalf("usual manual lookup past results = %v, want the scored game 8002", past)
	}

	route.handler.SetArchiveStore(route.store)
	archived := route.lookup(t, "479691")

	if !strings.HasSuffix(archived, usual) {
		t.Fatalf("archive changed the visitor's schedule\nusual:    %q\narchived: %q", usual, archived)
	}
	history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil || len(history.Games) != 2 {
		t.Fatalf("archived team season = %v games, error %v; want both games", len(history.Games), err)
	}
}

func TestManualTeamLookupWithArchiveKeepsOneColorPerSelectedTeam(t *testing.T) {
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	// Team 850's own schedule names no color, while team 100's schedule nests
	// red for it in their shared match. On the archive path, as on the ordinary
	// manual lookup, every row paints team 850 red, including its own game 861.
	route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/100":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":100,"team_name":"Blue FC","Color":"blue","Season":169},"games":[{"UGameID":860,"SchedGameDateTime":%q,"Season":169,"UTeam1":100,"UTeam2":850,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":850,"team_name":"Quiet FC","Color":"red"}}]}`, future)
		case "/teams/850":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":850,"team_name":"Quiet FC","Season":169},"games":[{"UGameID":860,"SchedGameDateTime":%[1]q,"Season":169,"UTeam1":100,"UTeam2":850,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":850,"team_name":"Quiet FC"}},{"UGameID":861,"SchedGameDateTime":%[1]q,"Season":169,"UTeam1":850,"UTeam2":870,"home_team":{"UTeamID":850,"team_name":"Quiet FC"},"visitor_team":{"UTeamID":870,"team_name":"Visitor Seven"}}]}`, future)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	route.handler.SetArchiveStore(nil)
	usual := route.lookup(t, "100, 850")

	route.handler.SetArchiveStore(route.store)
	archived := route.lookup(t, "100, 850")

	if !strings.HasSuffix(archived, usual) {
		t.Fatalf("archive changed the visitor's schedule\nusual:    %q\narchived: %q", usual, archived)
	}
	doc, err := html.Parse(strings.NewReader(archived))
	if err != nil {
		t.Fatalf("parse archived fragment: %v", err)
	}
	rows := soccerMatchRows(doc)
	if shared := onlySoccerRow(t, rows, "860"); htmlAttr(shared, "data-home-color") != "blue" || htmlAttr(shared, "data-away-color") != "red" {
		t.Fatalf("archived shared game colors = %q/%q, want blue/red", htmlAttr(shared, "data-home-color"), htmlAttr(shared, "data-away-color"))
	}
	if own := onlySoccerRow(t, rows, "861"); htmlAttr(own, "data-home-color") != "red" {
		t.Fatalf("archived team 850 row color = %q, want the red another schedule names for it", htmlAttr(own, "data-home-color"))
	}
}

func TestManualTeamLookupKeepsTheScheduleWhenOnlyTheTeamFacilityFails(t *testing.T) {
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			// The team's home facility (9) is not where its game is played (5).
			_, _ = fmt.Fprint(w, strings.Replace(boiseFCSchedule(future), `"UTeamID":479691,"team_name":"Boise FC","division_name":"Open A","FacilityID":5`, `"UTeamID":479691,"team_name":"Boise FC","division_name":"Open A","FacilityID":9`, 1))
		case "/facilities/5":
			_, _ = fmt.Fprint(w, archiveFacilityResponse)
		case "/facilities/9":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	route.handler.SetArchiveStore(nil)
	usual := route.lookup(t, "479691")
	if !strings.Contains(usual, "Away FC") {
		t.Fatalf("usual lookup did not render the game: %q", usual)
	}

	route.handler.SetArchiveStore(route.store)
	archived := route.lookup(t, "479691")

	if !strings.HasSuffix(archived, usual) || !strings.Contains(archived, "Team 479691 added to history collection.") {
		t.Fatalf("team facility failure changed the archived lookup\nusual:    %q\narchived: %q", usual, archived)
	}
	history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if history.Team.FacilityID != 9 || len(history.Facilities) != 1 || history.Facilities[0].FacilityID != 5 {
		t.Fatalf("stored team facility context = %#v, facilities %#v; want team facility 9 without its details and game facility 5", history.Team, history.Facilities)
	}
}

func TestEmptyManualTeamLookupEnrollsWhenTheTeamFacilityFails(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			route := newArchiveRoute(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/teams/479691":
					_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"team_name":"Dormant FC","FacilityID":9,"Season":169},"games":[]}`)
				case "/facilities/9":
					http.Error(w, http.StatusText(status), status)
				default:
					t.Errorf("unexpected LPS request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			route.handler.SetArchiveStore(nil)
			if usual := route.lookup(t, "479691"); !strings.Contains(usual, "There are no upcoming games for the selected teams.") {
				t.Fatalf("usual empty lookup: %q", usual)
			}

			route.handler.SetArchiveStore(route.store)
			archived := route.lookup(t, "479691")

			if !strings.Contains(archived, "Let&#39;s Play Soccer accepted the team ID but returned no games.") ||
				!strings.Contains(archived, "Team 479691 added to history collection.") || strings.Contains(archived, "Could not load schedules") {
				t.Fatalf("empty schedule with a failed team facility lookup: %q", archived)
			}
			history, err := route.store.ReadTeamSeason(context.Background(), 479691, 169)
			if err != nil {
				t.Fatalf("ReadTeamSeason: %v", err)
			}
			if history.Coverage.Status != soccerarchive.CoverageFetched || history.Coverage.ReturnedGameCount != 0 || history.Team.FacilityID != 9 || len(history.Facilities) != 0 {
				t.Fatalf("empty schedule archived as %#v", history)
			}
		})
	}
}

func TestManualTeamLookupNeverArchivesImportedPlayerAccess(t *testing.T) {
	const playerID = 7654321
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))
	jwt := testutil.TestJWT(t, time.Now().Add(time.Hour))
	session := &types.SessionData{
		JWT:       jwt,
		UserName:  "Casey Keeper",
		Players:   []types.LPSPlayer{{UPlayerID: playerID, FirstName: "Casey", LastName: "Keeper", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	body := route.lookup(t, "479691", func(req *http.Request) { addSessionCookie(t, route.app, req, session) })

	if !strings.Contains(body, "Team 479691 added to history collection.") {
		t.Fatalf("manual lookup with an imported session was not enrolled: %q", body)
	}
	items, err := route.table.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	for key, item := range items {
		for attribute, value := range item {
			if strings.Contains(strings.ToLower(attribute), "player") {
				t.Errorf("archive item %s has player attribute %q", key, attribute)
			}
			stored := fmt.Sprint(value)
			if strings.Contains(stored, jwt) || strings.Contains(stored, fmt.Sprint(playerID)) || strings.Contains(stored, "Keeper") {
				t.Errorf("archive item %s attribute %q retains imported player access: %q", key, attribute, stored)
			}
		}
		if strings.Contains(key, fmt.Sprint(playerID)) {
			t.Errorf("archive key %s references the imported player", key)
		}
	}
}

func TestManualTeamLookupReportsUnsavedHistoryWithoutClaimingEnrollment(t *testing.T) {
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))
	route.table.FailPut = func(string) error { return errors.New("table unavailable") }

	body := route.lookup(t, "479691")

	if !strings.Contains(body, "Away FC") || !strings.Contains(body, "History not saved") || strings.Contains(body, "added to history collection") {
		t.Fatalf("unsaved history outcome: %q", body)
	}
}

func TestPartlySavedManualTeamIsNotEnrolledForRefresh(t *testing.T) {
	route := newArchiveRoute(t, boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))
	route.table.FailPut = func(key string) error {
		if key == "GAME#8001/META" {
			return errors.New("throttled")
		}
		return nil
	}

	body := route.lookup(t, "479691")

	if !strings.Contains(body, "Away FC") || !strings.Contains(body, "History not saved") ||
		!strings.Contains(body, "History collection could not save team 479691. Try again later.") || strings.Contains(body, "added to history collection") {
		t.Fatalf("partly saved history outcome: %q", body)
	}
	route.assertNotArchived(t, 479691)
	route.assertNoDueTeams(t)
}

func TestMultiTeamLookupReportsEachTeamsHistoryOutcome(t *testing.T) {
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lps := func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, boiseFCSchedule(future))
		case "/teams/555555":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":555555,"team_name":"Second FC","Season":169},"games":[{"UGameID":8002,"SchedGameDateTime":%q,"FacilityID":5,"Season":169,"UTeam1":555555,"UTeam2":333,"home_team":{"UTeamID":555555,"team_name":"Second FC"},"visitor_team":{"UTeamID":333,"team_name":"Other FC"}}]}`, future)
		case "/facilities/5":
			_, _ = fmt.Fprint(w, archiveFacilityResponse)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}
	for _, tc := range []struct {
		failingTeam, savedTeam int
		failingGame            string
	}{
		{failingTeam: 479691, savedTeam: 555555, failingGame: "GAME#8001/META"},
		{failingTeam: 555555, savedTeam: 479691, failingGame: "GAME#8002/META"},
	} {
		t.Run(fmt.Sprint("team ", tc.failingTeam, " fails"), func(t *testing.T) {
			route := newArchiveRoute(t, lps)
			route.table.FailPut = func(key string) error {
				if key == tc.failingGame {
					return errors.New("throttled")
				}
				return nil
			}

			body := route.lookup(t, "479691, 555555")

			if !strings.Contains(body, "Away FC") || !strings.Contains(body, "Other FC") {
				t.Fatalf("multi-team schedule missing: %q", body)
			}
			if !strings.Contains(body, fmt.Sprintf("Team %d added to history collection.", tc.savedTeam)) ||
				!strings.Contains(body, fmt.Sprintf("History collection could not save team %d. Try again later.", tc.failingTeam)) ||
				!strings.Contains(body, "History partly saved") || !strings.Contains(body, "ui-feedback-warning") {
				t.Fatalf("multi-team outcome does not name each team's result: %q", body)
			}
			if _, err := route.store.ReadTeamSeason(context.Background(), tc.savedTeam, 169); err != nil {
				t.Fatalf("saved team %d read-back: %v", tc.savedTeam, err)
			}
			route.assertNotArchived(t, tc.failingTeam)
		})
	}
}

func (r *archiveRoute) assertNoDueTeams(t *testing.T) {
	t.Helper()
	items, err := r.table.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	for key, item := range items {
		if _, due := item["due_pk"]; due {
			t.Errorf("archive item %s is in the due-team index", key)
		}
	}
}

func TestLambdaAssemblyKeepsEnteredTeamEnrollmentDisabled(t *testing.T) {
	lpsServer := httptest.NewServer(boiseFCLPS(t, testutil.MislabelledLPSZuluTime(time.Now().Add(24*time.Hour))))
	t.Cleanup(lpsServer.Close)
	for _, name := range []string{"CLIENT_ID_KEY", "CLIENT_SECRET_KEY", "GOOGLE_CONNECTION_TABLE_NAME", "SOCCER_SESSION_TABLE_NAME", "MGMT_SESSION_KEY"} {
		t.Setenv(name, "")
	}
	t.Setenv("LPS_API_BASE_URL", lpsServer.URL)
	t.Setenv("LOG_LEVEL", "error")
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	handler, err := NewLambdaHandler(context.Background())
	if err != nil {
		t.Fatalf("NewLambdaHandler: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{"team_codes": {"479691"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), "Away FC") {
		t.Fatalf("live manual lookup lost the usual schedule: status %d, body %q", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "history collection") || strings.Contains(resp.Body.String(), "History not saved") {
		t.Fatalf("live assembly reported durable enrollment before activation: %q", resp.Body.String())
	}
}
