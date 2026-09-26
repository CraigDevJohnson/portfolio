package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/soccerarchive"
	"portfolio/internal/testutil"
)

type recordingTeamArchive struct {
	snapshots []soccerarchive.Snapshot
}

func (a *recordingTeamArchive) SaveTeamSnapshot(_ context.Context, snapshot *soccerarchive.Snapshot) error {
	a.snapshots = append(a.snapshots, *snapshot)
	return nil
}

func TestManualTeamLookupArchivesSourceFactsAndReportsEnrollment(t *testing.T) {
	app := newTestApp(t)
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":479691,"team_name":"Boise FC","division_name":"Open A","FacilityID":5,"facility_name":"Downtown","Season":169},"games":[{"UGameID":8001,"SchedGameDateTime":%q,"FacilityID":5,"Field":2,"Season":169,"UTeam1":479691,"UTeam2":222,"home_team":{"UTeamID":479691,"team_name":"Boise FC"},"visitor_team":{"UTeamID":222,"team_name":"Away FC"},"result":"2-1"}]}`, future)
		case "/facilities/5":
			_, _ = fmt.Fprint(w, `{"FacilityID":5,"FacilityName":"Downtown","Address":"123 Field St","City":"Boise","State":"ID","ZIP":"83702"}`)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lpsServer.Close()
	app.Config.LPSAPIBaseURL = lpsServer.URL
	archive := &recordingTeamArchive{}
	mux, handler := buildMux(app, app.Logger, false)
	handler.SetArchiveStore(archive)

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{"team_codes": {"479691"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "Boise FC") || !strings.Contains(resp.Body.String(), "Team 479691 added to history collection") {
		t.Fatalf("schedule and enrollment outcome missing: %q", resp.Body.String())
	}
	if len(archive.snapshots) != 1 {
		t.Fatalf("stored snapshots = %d, want 1", len(archive.snapshots))
	}
	got := archive.snapshots[0]
	if got.TeamID != 479691 || got.Team.Season != 169 || got.Team.DivisionName != "Open A" || len(got.Games) != 1 || got.Games[0].UGameID != 8001 || got.Games[0].UTeam1 != 479691 || got.Games[0].UTeam2 != 222 || len(got.Facilities) != 1 || got.Facilities[0].Address != "123 Field St" || got.FetchedAt.IsZero() {
		t.Fatalf("stored source facts = %#v", got)
	}
}

func TestManualTeamLookupDistinguishesAcceptedEmptyScheduleFromInvalidID(t *testing.T) {
	app := newTestApp(t)
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"team_name":"Dormant FC","Season":169},"games":[]}`)
		case "/teams/999999":
			http.NotFound(w, r)
		case "/teams/888888":
			_, _ = fmt.Fprint(w, `{}`)
		default:
			t.Errorf("unexpected LPS request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer lpsServer.Close()
	app.Config.LPSAPIBaseURL = lpsServer.URL
	archive := &recordingTeamArchive{}
	mux, handler := buildMux(app, app.Logger, false)
	handler.SetArchiveStore(archive)

	lookup := func(teamID string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{"team_codes": {teamID}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("team %s HTTP status = %d, want 200", teamID, resp.Code)
		}
		return resp.Body.String()
	}

	accepted := lookup("479691")
	if !strings.Contains(accepted, "accepted the team ID but returned no games") || !strings.Contains(accepted, "added to history collection") {
		t.Fatalf("accepted empty schedule outcome missing: %q", accepted)
	}
	for _, id := range []string{"999999", "888888", "479691,bad"} {
		invalid := lookup(id)
		if !(strings.Contains(invalid, "was not accepted") || strings.Contains(invalid, "were invalid")) || strings.Contains(invalid, "added to history collection") {
			t.Fatalf("invalid team %s outcome: %q", id, invalid)
		}
	}
	if len(archive.snapshots) != 1 || len(archive.snapshots[0].Games) != 0 {
		t.Fatalf("stored snapshots = %#v, want only the accepted empty schedule", archive.snapshots)
	}
}
