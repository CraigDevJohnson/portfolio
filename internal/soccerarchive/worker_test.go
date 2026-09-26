package soccerarchive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
)

func TestRefreshWorkerAppliesCorrectionsAndRetainsOmittedGames(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	firstFetch := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID: 479691,
		Team:   lps.TeamSummary{UTeamID: 479691, TeamName: "Boise FC", Season: 169, FacilityID: 5},
		Games: []lps.TeamScheduleGame{
			{UGameID: 8001, UTeam1: 479691, UTeam2: 222, Season: 169, SchedGameDateTime: "2026-09-26T18:00:00Z", Result: "2-1", FacilityID: 5},
			{UGameID: 8002, UTeam1: 479691, UTeam2: 333, Season: 169, Result: "1-1"},
		},
		Facilities: []lps.FacilityResponse{{FacilityID: 5, FacilityName: "Old Field", Address: "1 Old St"}},
		FetchedAt:  firstFetch,
	}); err != nil {
		t.Fatalf("enroll team: %v", err)
	}
	partialResponse := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			if partialResponse {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"Season":169,"FacilityID":5},"games":[{"UGameID":8001,"result":"3-1"}]}`)
			} else {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"team_name":"Boise FC","Season":169,"FacilityID":5},"games":[{"UGameID":8001,"UTeam1":444,"UTeam2":479691,"Season":169,"SchedGameDateTime":"2026-09-27T19:00:00Z","result":"3-1","FacilityID":5},{"UGameID":8001,"result":"3-1"}]}`)
			}
		case "/facilities/5":
			if partialResponse {
				_, _ = fmt.Fprint(w, `{"FacilityID":5,"FacilityName":"New Field"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"FacilityID":5,"FacilityName":"New Field","Address":"2 New St"}`)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := firstFetch.Add(time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return now })
	report := worker.Run(t.Context(), []int{479691, 479691})
	if !report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshSucceeded {
		t.Fatalf("first worker result = %#v", report)
	}
	history, err := store.ReadTeamSeason(t.Context(), 479691, 169)
	if err != nil {
		t.Fatalf("read refreshed history: %v", err)
	}
	if len(history.Games) != 2 || history.Games[0].UGameID != 8001 || history.Games[0].Result != "3-1" || history.Games[0].SchedGameDateTime != "2026-09-27T19:00:00Z" || history.Games[0].UTeam1 != 444 || history.Games[0].UTeam2 != 479691 || history.Games[1].UGameID != 8002 || history.Coverage.ReturnedGameCount != 1 || len(history.Facilities) != 1 || history.Facilities[0].Address != "2 New St" {
		t.Fatalf("refreshed history = %#v", history)
	}
	firstCount := backend.Len()
	partialResponse = true
	now = now.Add(time.Hour)
	report = worker.Run(t.Context(), []int{479691})
	if !report.Complete || backend.Len() != firstCount {
		t.Fatalf("repeat worker result = %#v, item count = %d, want %d", report, backend.Len(), firstCount)
	}
	history, err = store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil || history.Team.TeamName != "Boise FC" || len(history.Games) != 2 || history.Games[0].Result != "3-1" || history.Games[0].UTeam1 != 444 || history.Games[0].UTeam2 != 479691 || len(history.Facilities) != 1 || history.Facilities[0].Address != "2 New St" {
		t.Fatalf("repeat history = %#v, err = %v", history, err)
	}
	assertArchiveItem(t, backend, "TEAM#479691/META", map[string]any{"team_name": "Boise FC"})
}

func TestRefreshWorkerReportsInvalidTransientAndSuccessfulTeamsIndependently(t *testing.T) {
	store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []int{101, 202, 303} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: id, Team: lps.TeamSummary{UTeamID: id, Season: 169}, FetchedAt: seededAt}); err != nil {
			t.Fatalf("enroll team %d: %v", id, err)
		}
	}
	requests := map[int]int{}
	team202Status := http.StatusTooManyRequests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &id); err != nil {
			http.NotFound(w, r)
			return
		}
		requests[id]++
		switch id {
		case 101:
			w.WriteHeader(http.StatusNotFound)
		case 202:
			if team202Status == http.StatusOK {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":202,"Season":169},"games":[]}`)
			} else {
				w.WriteHeader(team202Status)
			}
		case 303:
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":303,"Season":169},"games":[{"UGameID":9303,"UTeam1":303,"UTeam2":404,"Season":169,"result":"2-0"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return now })
	report := worker.Run(t.Context(), []int{303, 202, 101, 404})
	if report.Complete || len(report.Results) != 4 ||
		report.Results[0].Outcome != RefreshInvalidTeam ||
		report.Results[1].Outcome != RefreshRetryableFailure ||
		report.Results[2].Outcome != RefreshSucceeded ||
		report.Results[3].Outcome != RefreshNotEnrolled {
		t.Fatalf("mixed worker result = %#v", report)
	}
	if requests[101] != 1 || requests[202] != 1 || requests[303] != 1 || requests[404] != 0 {
		t.Fatalf("LPS request counts = %#v", requests)
	}
	invalid, err := store.ReadRefreshState(t.Context(), 101)
	if err != nil || invalid.Status != RefreshInvalid || !invalid.NextDueAt.IsZero() || invalid.LastErrorStatusCode != http.StatusNotFound {
		t.Fatalf("invalid team state = %#v, err = %v", invalid, err)
	}
	transient, err := store.ReadRefreshState(t.Context(), 202)
	if err != nil || transient.Status != RefreshRetryable || !transient.NextDueAt.After(now) || transient.LastErrorStatusCode != http.StatusTooManyRequests {
		t.Fatalf("transient team state = %#v, err = %v", transient, err)
	}
	history, err := store.ReadTeamSeason(t.Context(), 303, 169)
	if err != nil || len(history.Games) != 1 || history.Games[0].UGameID != 9303 || history.Coverage.ReturnedGameCount != 1 {
		t.Fatalf("successful team's history = %#v, err = %v", history, err)
	}

	team202Status = http.StatusOK
	now = now.Add(time.Hour)
	report = worker.Run(t.Context(), []int{101, 202})
	if report.Complete || len(report.Results) != 2 || report.Results[0].Outcome != RefreshSkippedInvalid || report.Results[1].Outcome != RefreshSucceeded || requests[101] != 1 || requests[202] != 2 {
		t.Fatalf("repeated worker result = %#v, requests = %#v", report, requests)
	}
	recovered, err := store.ReadRefreshState(t.Context(), 202)
	if err != nil || recovered.Status != RefreshReady || recovered.LastErrorStatusCode != 0 {
		t.Fatalf("recovered team state = %#v, err = %v", recovered, err)
	}
}

func TestRefreshWorkerKeepsAnOmittedSeasonsHistoryAndCoverage(t *testing.T) {
	store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID:    101,
		Team:      lps.TeamSummary{UTeamID: 101, TeamName: "Old FC", Season: 169},
		Games:     []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "2-1"}},
		FetchedAt: seededAt,
	}); err != nil {
		t.Fatalf("enroll old season: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/101" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"team_name":"New FC","Season":170},"games":[]}`)
	}))
	defer server.Close()
	refreshedAt := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return refreshedAt })
	if report := worker.Run(t.Context(), []int{101}); !report.Complete {
		t.Fatalf("refresh result = %#v", report)
	}
	// The omitted season keeps its games and still says when LPS last
	// returned it; the refresh does not claim to have fetched it again.
	oldHistory, err := store.ReadTeamSeason(t.Context(), 101, 169)
	if err != nil || len(oldHistory.Games) != 1 || oldHistory.Games[0].UGameID != 9001 ||
		oldHistory.Coverage.Status != CoverageFetched || !oldHistory.Coverage.FetchedAt.Equal(seededAt) || oldHistory.Coverage.ReturnedGameCount != 1 {
		t.Fatalf("old season after omission = %#v, err = %v", oldHistory, err)
	}
	newHistory, err := store.ReadTeamSeason(t.Context(), 101, 170)
	if err != nil || len(newHistory.Games) != 0 || newHistory.Coverage.Status != CoverageFetched ||
		!newHistory.Coverage.FetchedAt.Equal(refreshedAt) || newHistory.Coverage.ReturnedGameCount != 0 {
		t.Fatalf("new empty season = %#v, err = %v", newHistory, err)
	}
}

func TestRefreshWorkerKeepsHistoryRetryableAfterServerAndNetworkFailures(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		closeEarly bool
	}{
		{name: "server error"},
		{name: "network error", closeEarly: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
			seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
			if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
				TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169},
				Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "2-1"}}, FetchedAt: seededAt,
			}); err != nil {
				t.Fatalf("enroll team: %v", err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			if testCase.closeEarly {
				server.Close()
			}
			worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return seededAt.Add(time.Hour) })
			report := worker.Run(t.Context(), []int{101})
			if report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshRetryableFailure {
				t.Fatalf("worker result = %#v", report)
			}
			state, err := store.ReadRefreshState(t.Context(), 101)
			if err != nil || state.Status != RefreshRetryable || !state.NextDueAt.After(seededAt) {
				t.Fatalf("refresh state = %#v, err = %v", state, err)
			}
			history, err := store.ReadTeamSeason(t.Context(), 101, 169)
			if err != nil || len(history.Games) != 1 || history.Games[0].Result != "2-1" || history.Coverage.Status != CoverageFetched {
				t.Fatalf("retained history: games=%d, coverage=%s, err=%v", len(history.Games), history.Coverage.Status, err)
			}
		})
	}
}
