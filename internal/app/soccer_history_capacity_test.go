package app

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/soccerarchive"
)

// useArchiveLimits rewires the route's durable archive, over the same table,
// with the given reviewed limits.
func (route *teamHistoryRoute) useArchiveLimits(t *testing.T, limits soccerarchive.Limits) {
	t.Helper()
	store, err := soccerarchive.NewDynamoStoreWithAPI(route.table, "portfolio-lambda-dev-soccer-history", limits)
	if err != nil {
		t.Fatalf("NewDynamoStoreWithAPI: %v", err)
	}
	route.store = store
	route.handler.SetArchiveStore(store)
}

// lookupTeam submits the public Team ID lookup from browser and returns the
// schedule fragment.
func lookupTeam(t *testing.T, browser *siteBrowser, teamID int) string {
	t.Helper()
	response := browser.postForm("/soccer/fetch", url.Values{"team_codes": {fmt.Sprint(teamID)}})
	if response.Code != http.StatusOK {
		t.Fatalf("Team ID %d lookup: status %d, body %q", teamID, response.Code, response.Body.String())
	}
	return response.Body.String()
}

// seasonSchedule is a team's LPS schedule in season: game gameID, a past
// 1 - 0 home win, and game gameID+100, still to be played.
func seasonSchedule(teamID, season, gameID int) string {
	return fmt.Sprintf(`{"team":{"UTeamID":%[1]d,"team_name":"Team %[1]d","Season":%[2]d},"games":[`+
		`{"UGameID":%[3]d,"Season":%[2]d,"UTeam1":%[1]d,"UTeam2":5999,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"1 - 0"},`+
		`{"UGameID":%[4]d,"Season":%[2]d,"UTeam1":5999,"UTeam2":%[1]d,"SchedGameDateTime":"2099-01-05T19:00:00Z","result":""}]}`, teamID, season, gameID, gameID+100)
}

// A full history capacity refuses only new Team IDs: every team enrolled
// before it filled keeps its daily refresh, and no refused team is polled.
// Two of four slots take entered Team IDs; the other two are reserved for
// player-linked teams.
func TestHistoryCapacityRefusesNewTeamsWithoutStoppingEnrolledTeamsDailyRefresh(t *testing.T) {
	route := newTeamHistoryRoute(t)
	limits := soccerarchive.Limits{MaxEnrolledTeams: 4, ReservedPlayerSlots: 2, MaxRequestsPerRun: 8, MaxRetriesPerTeam: 1, MinRequestInterval: time.Second}
	route.useArchiveLimits(t, limits)
	route.setTeam(4101, seasonSchedule(4101, 77, 7001))
	route.setTeam(4102, seasonSchedule(4102, 78, 7002))
	route.setTeam(4202, seasonSchedule(4202, 79, 7003))
	route.setTeam(4301, seasonSchedule(4301, 80, 7004))
	route.setTeam(4302, seasonSchedule(4302, 80, 7005))
	route.setTeam(4303, seasonSchedule(4303, 80, 7006))

	// Anonymous visitors fill the two slots entered Team IDs may take.
	visitor := newSiteBrowser(t, route.mux)
	for _, teamID := range []int{4301, 4302} {
		if body := lookupTeam(t, visitor, teamID); !strings.Contains(body, fmt.Sprintf("Team %d added to history collection.", teamID)) {
			t.Fatalf("entered Team ID %d was not enrolled: %q", teamID, body)
		}
	}
	refusedManual := lookupTeam(t, visitor, 4303)
	if !strings.Contains(refusedManual, "History collection is full") ||
		!strings.Contains(refusedManual, "Team 4303 was not added to history collection because its reviewed capacity is full.") ||
		!strings.Contains(refusedManual, `value="7106"`) {
		t.Fatalf("third entered Team ID was not refused beside its schedule: %q", refusedManual)
	}

	// The import discovers Craig FC (4101), Old FC (4102) and Taylor FC
	// (4202): the two reserved slots take the first two in Team ID order.
	owner := route.signedIn(t)
	imported := owner.postForm("/soccer/import", url.Values{
		"jwt": {route.jwt}, "history_notice": {"indefinite"},
	})
	if body := imported.Body.String(); imported.Code != http.StatusOK || findSessionCookie(t, imported.Result()) == nil ||
		!strings.Contains(body, "Team 4202 was not added to history collection because its reviewed capacity is full.") ||
		!strings.Contains(body, "Your import and your other teams") {
		t.Fatalf("import at capacity: status %d, body %q", imported.Code, body)
	}

	clock := &readinessClock{now: time.Now().UTC().Add(25 * time.Hour)}
	worker, err := soccerarchive.NewDailyWorker(route.store, route.lpsURL, &http.Client{Timeout: 5 * time.Second}, limits, clock)
	if err != nil {
		t.Fatal(err)
	}
	enrolled := map[int]soccerarchive.RefreshOutcome{4101: "succeeded", 4102: "succeeded", 4301: "succeeded", 4302: "succeeded"}
	for day := 1; day <= 2; day++ {
		runAt := clock.Now()
		report, err := worker.Run(t.Context())
		if err != nil || !report.Complete || report.Requests != 4 || fmt.Sprint(outcomes(report.Results)) != fmt.Sprint(enrolled) {
			t.Fatalf("day %d run: report %+v, err %v; want each of the four enrolled teams refreshed and neither refused team", day, report, err)
		}
		if history := readHistory(t, owner, 1001, 4102, 78); history.Coverage.Status != "fetched" || history.Coverage.FetchedAt == nil ||
			history.Coverage.FetchedAt.Before(runAt) || history.Record.Wins != 1 {
			t.Errorf("day %d: Old FC season 78 after the run = %+v", day, history)
		}
		clock.advance(24 * time.Hour)
	}

	// Taylor still played for the refused team, so the current lookup opens
	// its season, but nothing collected it: no fetch and no refresh record.
	refused := readHistory(t, owner, 1002, 4202, 79)
	if refused.Coverage.Status != "not_fetched" || refused.Coverage.FetchedAt != nil || refused.Refresh != nil || len(refused.Games) != 0 {
		t.Errorf("season of the refused player-linked team = %+v, want uncollected and unenrolled", refused)
	}
	if again := lookupTeam(t, visitor, 4303); !strings.Contains(again, "Team 4303 was not added to history collection") {
		t.Errorf("a later lookup enrolled the refused Team ID at capacity: %q", again)
	}
	if again := lookupTeam(t, visitor, 4301); !strings.Contains(again, "Team 4301 added to history collection.") {
		t.Errorf("an enrolled Team ID lost its place at capacity: %q", again)
	}
}
