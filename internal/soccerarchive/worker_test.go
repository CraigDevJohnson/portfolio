package soccerarchive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

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
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []int{101, 202, 303} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
			TeamID:    id,
			Team:      lps.TeamSummary{UTeamID: id, Season: 169},
			Games:     []lps.TeamScheduleGame{{UGameID: 8000 + id, UTeam1: id, UTeam2: 404, Season: 169, Result: "1-0"}},
			FetchedAt: seededAt,
		}); err != nil {
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
	if err != nil || invalid.Status != RefreshInvalid || !invalid.NextDueAt.IsZero() || invalid.LastErrorStatusCode != http.StatusNotFound || !invalid.LastAttemptAt.Equal(now) {
		t.Fatalf("invalid team state = %#v, err = %v", invalid, err)
	}
	transient, err := store.ReadRefreshState(t.Context(), 202)
	if err != nil || transient.Status != RefreshRetryable || !transient.NextDueAt.After(now) || transient.LastErrorStatusCode != http.StatusTooManyRequests {
		t.Fatalf("transient team state = %#v, err = %v", transient, err)
	}
	// Neither failure removes stored history; only the invalid ID leaves the
	// due-team index, so later polling skips it.
	for _, id := range []int{101, 202} {
		history, err := store.ReadTeamSeason(t.Context(), id, 169)
		if err != nil || len(history.Games) != 1 || history.Games[0].UGameID != 8000+id || history.Games[0].Result != "1-0" ||
			!history.Coverage.FetchedAt.Equal(seededAt) {
			t.Fatalf("team %d history after failure = %#v, err = %v", id, history, err)
		}
	}
	if _, due := backend.Item("TEAM#101/META")["due_pk"]; due {
		t.Error("invalid team 101 is still in the due-team index")
	}
	assertArchiveItem(t, backend, "TEAM#202/META", map[string]any{"due_pk": "TEAM_DUE"})
	history, err := store.ReadTeamSeason(t.Context(), 303, 169)
	if err != nil || len(history.Games) != 2 || history.Games[0].UGameID != 8303 || history.Games[1].UGameID != 9303 || history.Games[1].Result != "2-0" || history.Coverage.ReturnedGameCount != 1 {
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

func TestRefreshWorkerAppliesCorrectionsWhenLPSNoLongerServesAFacility(t *testing.T) {
	for _, tc := range []struct {
		name string
		// gameFacility is the corrected game's own facility; without one the
		// game takes the team's facility 5.
		gameFacility int
		// served answers each facility LPS still has; any other is gone.
		served     map[int]string
		goneStatus int
		// want is each facility's stored address after the refresh.
		want map[int]string
	}{
		{
			name: "game's own facility is not found", gameFacility: 77, goneStatus: http.StatusNotFound,
			served: map[int]string{5: `{"FacilityID":5,"FacilityName":"New Field","Address":"2 New St"}`},
			want:   map[int]string{5: "2 New St", 77: "7 Gone Rd"},
		},
		{
			name: "game's own facility ID is refused", gameFacility: 77, goneStatus: http.StatusBadRequest,
			served: map[int]string{5: `{"FacilityID":5,"FacilityName":"New Field","Address":"2 New St"}`},
			want:   map[int]string{5: "2 New St", 77: "7 Gone Rd"},
		},
		{
			name: "team facility the game falls back to is not found", goneStatus: http.StatusNotFound,
			want: map[int]string{5: "1 Old St"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := archivetest.NewTable()
			store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
			seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
			if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
				TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169, FacilityID: 5},
				Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "1-0", FacilityID: tc.gameFacility}},
				Facilities: []lps.FacilityResponse{
					{FacilityID: 5, FacilityName: "Old Field", Address: "1 Old St"},
					{FacilityID: 77, FacilityName: "Lost Field", Address: "7 Gone Rd"},
				},
				FetchedAt: seededAt,
			}); err != nil {
				t.Fatalf("enroll team: %v", err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/teams/101" {
					_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":101,"Season":169,"FacilityID":5},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"FacilityID":%d,"SchedGameDateTime":"2026-09-27T19:00:00Z","result":"5-0"}]}`, tc.gameFacility)
					return
				}
				var id int
				if _, err := fmt.Sscanf(r.URL.Path, "/facilities/%d", &id); err == nil && tc.served[id] != "" {
					_, _ = fmt.Fprint(w, tc.served[id])
					return
				}
				w.WriteHeader(tc.goneStatus)
			}))
			defer server.Close()

			refreshedAt := seededAt.Add(24 * time.Hour)
			worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return refreshedAt })
			report := worker.Run(t.Context(), []int{101})

			if !report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshSucceeded {
				t.Fatalf("worker result = %#v", report)
			}
			history, err := store.ReadTeamSeason(t.Context(), 101, 169)
			if err != nil || len(history.Games) != 1 || history.Games[0].Result != "5-0" || history.Games[0].SchedGameDateTime != "2026-09-27T19:00:00Z" ||
				history.Coverage.Status != CoverageFetched || !history.Coverage.FetchedAt.Equal(refreshedAt) || history.Coverage.ReturnedGameCount != 1 {
				t.Fatalf("refreshed history = %#v, err = %v", history, err)
			}
			for id, address := range tc.want {
				assertArchiveItem(t, backend, fmt.Sprintf("FACILITY#%d/META", id), map[string]any{"address": address})
			}
			state, err := store.ReadRefreshState(t.Context(), 101)
			if err != nil || state.Status != RefreshReady || !state.LastAttemptAt.Equal(refreshedAt) || !state.NextDueAt.Equal(refreshedAt.Add(24*time.Hour)) {
				t.Fatalf("refresh state = %#v, err = %v", state, err)
			}
		})
	}
}

func TestRefreshWorkerKeepsHistoryRetryableAfterTemporaryOrUnconfirmedResponses(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		lps        http.HandlerFunc
		closeEarly bool
	}{
		{name: "rate limit", lps: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }},
		{name: "server error", lps: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }},
		{name: "network error", lps: func(w http.ResponseWriter, _ *http.Request) {}, closeEarly: true},
		{name: "team schedule refused", lps: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }},
		{name: "response does not confirm the team", lps: func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, `{"team":{},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"result":"9-9"}]}`)
		}},
		{name: "game facility outage", lps: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/teams/101" {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"FacilityID":5,"result":"9-9"}]}`)
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
		{name: "outage of the team facility a game falls back to", lps: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/teams/101" {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169,"FacilityID":5},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"result":"9-9"}]}`)
				return
			}
			w.WriteHeader(http.StatusServiceUnavailable)
		}},
		{name: "game facility rate limit", lps: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/teams/101" {
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"FacilityID":5,"result":"9-9"}]}`)
				return
			}
			w.WriteHeader(http.StatusTooManyRequests)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			backend := archivetest.NewTable()
			store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
			seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
			if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
				TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169},
				Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "2-1"}}, FetchedAt: seededAt,
			}); err != nil {
				t.Fatalf("enroll team: %v", err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/teams/101" {
					requests++
				}
				testCase.lps(w, r)
			}))
			defer server.Close()
			if testCase.closeEarly {
				server.Close()
			}
			attemptedAt := seededAt.Add(time.Hour)
			worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return attemptedAt })
			report := worker.Run(t.Context(), []int{101})
			if report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshRetryableFailure {
				t.Fatalf("worker result = %#v", report)
			}
			state, err := store.ReadRefreshState(t.Context(), 101)
			if err != nil || state.Status != RefreshRetryable || !state.LastAttemptAt.Equal(attemptedAt) || !state.NextDueAt.After(attemptedAt) {
				t.Fatalf("refresh state = %#v, err = %v", state, err)
			}
			assertArchiveItem(t, backend, "TEAM#101/META", map[string]any{"due_pk": "TEAM_DUE"})
			history, err := store.ReadTeamSeason(t.Context(), 101, 169)
			if err != nil || len(history.Games) != 1 || history.Games[0].Result != "2-1" ||
				history.Coverage.Status != CoverageFetched || !history.Coverage.FetchedAt.Equal(seededAt) {
				t.Fatalf("retained history = %#v, err = %v", history, err)
			}
			// The ID stays valid: the next invocation asks LPS again.
			attemptedAt = attemptedAt.Add(time.Hour)
			if report := worker.Run(t.Context(), []int{101}); len(report.Results) != 1 || report.Results[0].Outcome == RefreshSkippedInvalid {
				t.Fatalf("next worker result = %#v", report)
			}
			if !testCase.closeEarly && requests != 2 {
				t.Fatalf("LPS team requests = %d, want 2", requests)
			}
		})
	}
}

func TestRefreshWorkerRecordsAResponseWithAGameWithoutAStableIDAsAnLPSFailure(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169},
		Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "1-0"}}, FetchedAt: seededAt,
	}); err != nil {
		t.Fatalf("enroll team: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[`+
			`{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"result":"7-0"},`+
			`{"UTeam1":101,"UTeam2":303,"Season":169,"result":"2-2"}]}`)
	}))
	defer server.Close()
	attemptedAt := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return attemptedAt })

	report := worker.Run(t.Context(), []int{101})

	if report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshRetryableFailure ||
		!strings.Contains(report.Results[0].Error, "without a stable ID") {
		t.Fatalf("worker result = %#v", report)
	}
	state, err := store.ReadRefreshState(t.Context(), 101)
	want := RefreshState{
		TeamID: 101, Status: RefreshRetryable, LastAttemptAt: attemptedAt, NextDueAt: attemptedAt.Add(15 * time.Minute),
		LastErrorKind: lps.ErrorUpstream, LastErrorStatusCode: http.StatusBadGateway,
	}
	if err != nil || fmt.Sprint(state) != fmt.Sprint(want) {
		t.Fatalf("refresh state = %+v, err = %v\nwant %+v", state, err, want)
	}
	// Nothing from the rejected response is stored.
	history, err := store.ReadTeamSeason(t.Context(), 101, 169)
	if err != nil || len(history.Games) != 1 || history.Games[0].Result != "1-0" || !history.Coverage.FetchedAt.Equal(seededAt) {
		t.Fatalf("history after rejected response = %#v, err = %v", history, err)
	}
}

func TestRefreshWorkerStoresAGameSharedByTwoEnrolledTeamsOnceAndRecordsEachFetch(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []int{101, 202} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
			TeamID:    id,
			Team:      lps.TeamSummary{UTeamID: id, Season: 169},
			Games:     []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, SchedGameDateTime: "2026-09-26T18:00:00Z"}},
			FetchedAt: seededAt,
		}); err != nil {
			t.Fatalf("enroll team %d: %v", id, err)
		}
	}
	// Both teams' responses list the same game, now scored and rescheduled.
	sharedGame := `{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"SchedGameDateTime":"2026-09-27T19:00:00Z","result":"2-2"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/101":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":101,"Season":169},"games":[%s]}`, sharedGame)
		case "/teams/202":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":202,"Season":169},"games":[%s]}`, sharedGame)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	refreshedAt := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return refreshedAt })
	report := worker.Run(t.Context(), []int{202, 101})
	if !report.Complete || len(report.Results) != 2 || report.Results[0].Outcome != RefreshSucceeded || report.Results[1].Outcome != RefreshSucceeded {
		t.Fatalf("worker result = %#v", report)
	}

	items, err := backend.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	var gameRecords []string
	for key := range items {
		if strings.HasPrefix(key, "GAME#") {
			gameRecords = append(gameRecords, key)
		}
	}
	if len(gameRecords) != 1 || gameRecords[0] != "GAME#9001/META" {
		t.Fatalf("game records = %v, want only GAME#9001/META", gameRecords)
	}
	assertArchiveItem(t, backend, "GAME#9001/META", map[string]any{
		"result": "2-2", "scheduled_at": "2026-09-27T19:00:00Z", "fetched_at": "2026-09-21T12:00:00.000000000Z",
	})
	for _, id := range []int{101, 202} {
		history, err := store.ReadTeamSeason(t.Context(), id, 169)
		if err != nil || len(history.Games) != 1 || history.Games[0].Result != "2-2" ||
			history.Coverage.Status != CoverageFetched || !history.Coverage.FetchedAt.Equal(refreshedAt) || history.Coverage.ReturnedGameCount != 1 {
			t.Fatalf("team %d history = %#v, err = %v", id, history, err)
		}
		state, err := store.ReadRefreshState(t.Context(), id)
		if err != nil || state.Status != RefreshReady || !state.LastAttemptAt.Equal(refreshedAt) || !state.NextDueAt.Equal(refreshedAt.Add(24*time.Hour)) {
			t.Fatalf("team %d refresh state = %#v, err = %v", id, state, err)
		}
		assertArchiveItem(t, backend, fmt.Sprintf("TEAM#%d/COVERAGE", id), map[string]any{
			"status": "fetched", "returned_game_count": 1, "season_ids": []any{169.0}, "fetched_at": "2026-09-21T12:00:00.000000000Z",
		})
	}
}

func TestRepeatedRefreshWorkerRunsConvergeOnTheSameFacts(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID:    479691,
		Team:      lps.TeamSummary{UTeamID: 479691, TeamName: "Boise FC", Season: 169},
		Games:     []lps.TeamScheduleGame{{UGameID: 8001, UTeam1: 479691, UTeam2: 222, Season: 169, Result: "1-0"}},
		FetchedAt: seededAt,
	}); err != nil {
		t.Fatalf("enroll team: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":479691,"team_name":"Boise FC","Season":169,"FacilityID":5},"games":[`+
				`{"UGameID":8001,"UTeam1":479691,"UTeam2":222,"Season":169,"FacilityID":5,"result":"2-1"},`+
				`{"UGameID":8002,"UTeam1":333,"UTeam2":479691,"Season":169,"FacilityID":5,"result":"0-0"}]}`)
		case "/facilities/5":
			_, _ = fmt.Fprint(w, `{"FacilityID":5,"FacilityName":"Downtown","Address":"123 Field St"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	now := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return now })
	if report := worker.Run(t.Context(), []int{479691}); !report.Complete {
		t.Fatalf("first run = %#v", report)
	}
	first := storedFacts(t, backend)
	// A redelivered invocation, with the team listed twice, runs later.
	now = now.Add(time.Hour)
	if report := worker.Run(t.Context(), []int{479691, 479691}); !report.Complete || len(report.Results) != 1 {
		t.Fatalf("repeated run = %#v", report)
	}
	second := storedFacts(t, backend)

	if len(second) != len(first) {
		t.Fatalf("repeated run stored %d records, want the first run's %d", len(second), len(first))
	}
	for key, facts := range first {
		if fmt.Sprint(second[key]) != fmt.Sprint(facts) {
			t.Errorf("%s changed on a repeated run:\n first  %v\n second %v", key, facts, second[key])
		}
	}
	history, err := store.ReadTeamSeason(t.Context(), 479691, 169)
	if err != nil || len(history.Games) != 2 || history.Games[0].Result != "2-1" || history.Games[1].Result != "0-0" ||
		history.Coverage.ReturnedGameCount != 2 || len(history.Facilities) != 1 || history.Facilities[0].Address != "123 Field St" {
		t.Fatalf("converged history = %#v, err = %v", history, err)
	}
}

// storedFacts returns every stored record, with its raw LPS source decoded,
// without the attributes that only say when or how often it was written.
func storedFacts(t *testing.T, backend *archivetest.Table) map[string]map[string]any {
	t.Helper()
	items, err := backend.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	for key, item := range items {
		for _, attribute := range []string{"fetched_at", "attempted_at", "due_sk", "revision"} {
			delete(item, attribute)
		}
		if raw, ok := item["raw_source_json"].(string); ok {
			var source any
			if err := json.Unmarshal([]byte(raw), &source); err != nil {
				t.Fatalf("decode %s source: %v", key, err)
			}
			item["raw_source_json"] = source
		}
	}
	return items
}

func TestRefreshWorkerReportsAPartlySavedTeamWithoutDiscardingAnotherTeam(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for _, id := range []int{101, 303} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
			TeamID:    id,
			Team:      lps.TeamSummary{UTeamID: id, Season: 169},
			Games:     []lps.TeamScheduleGame{{UGameID: 8000 + id, UTeam1: id, UTeam2: 404, Season: 169, Result: "1-0"}},
			FetchedAt: seededAt,
		}); err != nil {
			t.Fatalf("enroll team %d: %v", id, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/101":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[{"UGameID":7101,"UTeam1":101,"UTeam2":404,"Season":169,"result":"3-3"},{"UGameID":8101,"UTeam1":101,"UTeam2":404,"Season":169,"result":"4-0"}]}`)
		case "/teams/303":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":303,"Season":169},"games":[{"UGameID":8303,"UTeam1":303,"UTeam2":404,"Season":169,"result":"0-2"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	// Team 101's second game write fails after its first one landed.
	backend.FailPut = func(key string) error {
		if key == "GAME#8101/META" {
			return errors.New("throttled")
		}
		return nil
	}

	refreshedAt := seededAt.Add(24 * time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return refreshedAt })
	report := worker.Run(t.Context(), []int{101, 303})

	if report.Complete || len(report.Results) != 2 || report.Results[0].Outcome != RefreshStoreFailed || report.Results[1].Outcome != RefreshSucceeded {
		t.Fatalf("worker result = %#v", report)
	}
	saved, err := store.ReadTeamSeason(t.Context(), 303, 169)
	if err != nil || len(saved.Games) != 1 || saved.Games[0].Result != "0-2" || !saved.Coverage.FetchedAt.Equal(refreshedAt) {
		t.Fatalf("successful team 303 history = %#v, err = %v", saved, err)
	}
	// The partly saved team keeps its last complete history and refresh
	// record, so it is neither shown as refreshed nor dropped from refresh.
	partial, err := store.ReadTeamSeason(t.Context(), 101, 169)
	if err != nil || len(partial.Games) != 1 || partial.Games[0].UGameID != 8101 || partial.Games[0].Result != "1-0" ||
		!partial.Coverage.FetchedAt.Equal(seededAt) || partial.Coverage.ReturnedGameCount != 1 {
		t.Fatalf("partly saved team 101 history = %#v, err = %v", partial, err)
	}
	state, err := store.ReadRefreshState(t.Context(), 101)
	if err != nil || state.Status != RefreshReady || !state.LastAttemptAt.Equal(seededAt) || !state.NextDueAt.Equal(seededAt.Add(24*time.Hour)) {
		t.Fatalf("partly saved team 101 refresh state = %#v, err = %v", state, err)
	}
}

func TestRefreshStateFollowsTheLatestAttemptWhenWritesInterleave(t *testing.T) {
	seededAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	workerAttempt := seededAt.Add(2 * time.Hour)
	for _, tc := range []struct {
		name      string
		lpsStatus int
		competing func(store *DynamoStore) error
		want      RefreshState
		result    string
	}{
		{
			// A lookup that fetched after the worker's failed attempt
			// lands while the worker records that failure.
			name:      "later lookup succeeds during a failed attempt",
			lpsStatus: http.StatusServiceUnavailable,
			competing: func(store *DynamoStore) error {
				return store.SaveTeamSnapshot(context.Background(), &Snapshot{
					TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169}, FetchedAt: workerAttempt.Add(time.Hour),
					Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "5-5"}},
				})
			},
			want:   RefreshState{TeamID: 101, Status: RefreshReady, LastAttemptAt: workerAttempt.Add(time.Hour), NextDueAt: workerAttempt.Add(25 * time.Hour)},
			result: "5-5",
		},
		{
			// Another invocation's later failure lands while this worker
			// saves the snapshot it fetched first.
			name:      "later attempt fails during a successful refresh",
			lpsStatus: http.StatusOK,
			competing: func(store *DynamoStore) error {
				return store.RecordRefreshFailure(context.Background(), &RefreshFailure{
					TeamID: 101, AttemptedAt: workerAttempt.Add(time.Hour), Status: RefreshRetryable,
					NextDueAt: workerAttempt.Add(2 * time.Hour), ErrorKind: lps.ErrorUpstream, HTTPStatusCode: http.StatusBadGateway,
				})
			},
			want: RefreshState{
				TeamID: 101, Status: RefreshRetryable, LastAttemptAt: workerAttempt.Add(time.Hour), NextDueAt: workerAttempt.Add(2 * time.Hour),
				LastErrorKind: lps.ErrorUpstream, LastErrorStatusCode: http.StatusBadGateway,
			},
			result: "3-0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			table := archivetest.NewTable()
			plain := NewDynamoStoreWithAPI(table, "durable-soccer-history")
			if err := plain.SaveTeamSnapshot(t.Context(), &Snapshot{
				TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169}, FetchedAt: seededAt,
				Games: []lps.TeamScheduleGame{{UGameID: 9001, UTeam1: 101, UTeam2: 202, Season: 169, Result: "1-0"}},
			}); err != nil {
				t.Fatalf("enroll team: %v", err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.lpsStatus != http.StatusOK {
					w.WriteHeader(tc.lpsStatus)
					return
				}
				_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"result":"3-0"}]}`)
			}))
			defer server.Close()
			// The worker's first read of the enrollment record checks its
			// state; the competing write lands right after its second read,
			// the one it updates.
			api := &nthReadAPI{Table: table, watch: "TEAM#101/META", n: 2, competing: func() {
				if err := tc.competing(plain); err != nil {
					t.Errorf("competing write: %v", err)
				}
			}}
			worker := NewRefreshWorker(NewDynamoStoreWithAPI(api, "durable-soccer-history"), lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return workerAttempt })

			report := worker.Run(t.Context(), []int{101})

			if len(report.Results) != 1 || report.Results[0].Outcome == RefreshStoreFailed {
				t.Fatalf("worker result = %#v", report)
			}
			if api.competing != nil {
				t.Fatal("the competing write never ran")
			}
			state, err := plain.ReadRefreshState(t.Context(), 101)
			if err != nil || fmt.Sprint(state) != fmt.Sprint(tc.want) {
				t.Fatalf("refresh state = %+v, err = %v\nwant %+v", state, err, tc.want)
			}
			history, err := plain.ReadTeamSeason(t.Context(), 101, 169)
			if err != nil || len(history.Games) != 1 || history.Games[0].Result != tc.result {
				t.Fatalf("history = %#v, err = %v", history, err)
			}
		})
	}
}

// nthReadAPI runs a competing write once, right after the n-th read of the
// watched item, as a concurrent writer of that record would.
type nthReadAPI struct {
	*archivetest.Table

	watch     string
	n         int
	reads     int
	competing func()
}

func (api *nthReadAPI) GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	output, err := api.Table.GetItem(ctx, input, optFns...)
	var key struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
	}
	if decodeErr := attributevalue.UnmarshalMap(input.Key, &key); decodeErr == nil && key.PK+"/"+key.SK == api.watch {
		api.reads++
		if api.reads == api.n && api.competing != nil {
			competing := api.competing
			api.competing = nil
			competing()
		}
	}
	return output, err
}
