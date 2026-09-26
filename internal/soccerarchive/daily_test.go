package soccerarchive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/types"
)

type fakeDailyClock struct {
	now    time.Time
	sleeps []time.Duration
}

func TestArchiveAdmissionReservesCapacityForPlayerTeamsWithoutEvictingEnrolledTeams(t *testing.T) {
	backend := archivetest.NewTable()
	limits := Limits{MaxEnrolledTeams: 4, ReservedPlayerSlots: 2, MaxRequestsPerRun: 8, MaxRetriesPerTeam: 1, MinRequestInterval: time.Second}
	store, err := NewDynamoStoreWithAPI(backend, "durable-soccer-history", limits)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{101, 102} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: now}); err != nil {
			t.Fatalf("admit manual team %d: %v", teamID, err)
		}
	}
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: 103, Team: lps.TeamSummary{UTeamID: 103, Season: 169}, FetchedAt: now}); !errors.Is(err, ErrAdmissionFull) {
		t.Fatalf("third manual team = %v, want capacity rejection", err)
	}
	for _, teamID := range []int{201, 202} {
		if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
			OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "owner", ObservedAt: now,
			Players:     []types.LPSPlayer{{UPlayerID: 1001}},
			KnownTeams:  []lps.TeamSummary{{UTeamID: teamID, Season: 169}},
			Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}}},
		}); err != nil {
			t.Fatalf("reserved player admission %d: %v", teamID, err)
		}
	}
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "owner", ObservedAt: now,
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 203, Season: 169}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 203, Season: 169}}},
	}); !errors.Is(err, ErrAdmissionFull) {
		t.Fatalf("fifth team = %v, want capacity rejection", err)
	}
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169}, FetchedAt: now.Add(time.Hour)}); err != nil {
		t.Fatalf("existing manual team lost refresh service at capacity: %v", err)
	}
	for _, teamID := range []int{101, 102, 201, 202} {
		if _, err := store.ReadRefreshState(t.Context(), teamID); err != nil {
			t.Errorf("enrolled team %d disappeared: %v", teamID, err)
		}
	}
	if _, err := store.ReadRefreshState(t.Context(), 103); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("rejected manual team was enrolled: %v", err)
	}
	if _, err := store.ReadRefreshState(t.Context(), 203); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("rejected player team was enrolled: %v", err)
	}
}

func TestDailyWorkerRetriesTransientTeamsWithinBudgetAndReportsPartialFailure(t *testing.T) {
	store := newTestStore(t, archivetest.NewTable())
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{101, 202, 303} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: seededAt}); err != nil {
			t.Fatal(err)
		}
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := map[int]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var teamID int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &teamID); err != nil {
			http.NotFound(w, r)
			return
		}
		requests[teamID]++
		if teamID == 101 && requests[teamID] == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if teamID == 202 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":%d,"Season":169},"games":[]}`, teamID)
	}))
	t.Cleanup(server.Close)
	worker, err := NewDailyWorker(store, server.URL, server.Client(), Limits{
		MaxEnrolledTeams: 3, ReservedPlayerSlots: 1, MaxRequestsPerRun: 5, MaxRetriesPerTeam: 1, MinRequestInterval: time.Second,
	}, clock)
	if err != nil {
		t.Fatal(err)
	}
	report, err := worker.Run(t.Context())
	if err != nil || report.Complete || report.PendingDueWork || report.Requests != 5 || len(report.Results) != 3 ||
		report.Results[0].Outcome != RefreshSucceeded || report.Results[1].Outcome != RefreshRetryableFailure || report.Results[2].Outcome != RefreshSucceeded ||
		requests[101] != 2 || requests[202] != 2 || requests[303] != 1 {
		t.Fatalf("bounded retry report = %#v, err %v, requests %#v", report, err, requests)
	}
	state, err := store.ReadRefreshState(t.Context(), 202)
	if err != nil || state.Status != RefreshRetryable || state.LastErrorStatusCode != http.StatusTooManyRequests || !state.NextDueAt.After(clock.Now()) {
		t.Fatalf("transient team was not checkpointed for retry: state %#v, err %v", state, err)
	}
	if len(clock.sleeps) < 4 {
		t.Fatalf("retries or source requests were not paced: %#v", clock.sleeps)
	}
}

func TestDailyWorkerLeavesBudgetLimitedTeamsDueForNextInvocation(t *testing.T) {
	store := newTestStore(t, archivetest.NewTable())
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{101, 202, 303} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: seededAt}); err != nil {
			t.Fatal(err)
		}
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := map[int]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var teamID int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &teamID); err != nil {
			http.NotFound(w, r)
			return
		}
		requests[teamID]++
		if teamID == 101 && requests[teamID] == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":%d,"Season":169},"games":[]}`, teamID)
	}))
	t.Cleanup(server.Close)
	worker, err := NewDailyWorker(store, server.URL, server.Client(), Limits{
		MaxEnrolledTeams: 3, ReservedPlayerSlots: 1, MaxRequestsPerRun: 2, MaxRetriesPerTeam: 1, MinRequestInterval: time.Second,
	}, clock)
	if err != nil {
		t.Fatal(err)
	}
	first, err := worker.Run(t.Context())
	if err != nil || first.Complete || !first.PendingDueWork || first.Requests != 2 || len(first.Results) != 1 || first.Results[0].Outcome != RefreshSucceeded || requests[202] != 0 {
		t.Fatalf("first budget-limited pass = %#v, err %v, requests %#v", first, err, requests)
	}
	second, err := worker.Run(t.Context())
	if err != nil || !second.Complete || second.PendingDueWork || second.Requests != 2 || len(second.Results) != 2 || requests[101] != 2 || requests[202] != 1 || requests[303] != 1 {
		t.Fatalf("continuation missed work or repeated checkpoint: %#v, err %v, requests %#v", second, err, requests)
	}
}

func TestDailyRuntimeRequiresEveryReviewedNumericLimit(t *testing.T) {
	values := map[string]string{
		"SOCCER_HISTORY_MAX_TEAMS":       "4",
		"SOCCER_HISTORY_PLAYER_RESERVED": "2",
		"SOCCER_HISTORY_MAX_REQUESTS":    "8",
		"SOCCER_HISTORY_MAX_RETRIES":     "1",
		"SOCCER_HISTORY_MIN_INTERVAL_MS": "250",
	}
	lookup := func(key string) string { return values[key] }
	limits, err := LimitsFromEnvironment(lookup)
	if err != nil || limits.MaxEnrolledTeams != 4 || limits.ReservedPlayerSlots != 2 || limits.MaxRequestsPerRun != 8 || limits.MaxRetriesPerTeam != 1 || limits.MinRequestInterval != 250*time.Millisecond {
		t.Fatalf("reviewed limits = %#v, err %v", limits, err)
	}
	for key := range values {
		original := values[key]
		delete(values, key)
		if _, err := LimitsFromEnvironment(lookup); err == nil {
			t.Errorf("missing %s enabled the worker", key)
		}
		values[key] = original
	}
	values["SOCCER_HISTORY_MAX_REQUESTS"] = "0"
	if _, err := LimitsFromEnvironment(lookup); err == nil {
		t.Fatal("zero request ceiling enabled the worker")
	}
}

func TestDailyWorkerStopsInvalidTeamsAndBacksOffTemporaryFailures(t *testing.T) {
	store := newTestStore(t, archivetest.NewTable())
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{101, 202} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: seededAt}); err != nil {
			t.Fatal(err)
		}
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := map[int]int{}
	recovered := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var teamID int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &teamID); err != nil {
			http.NotFound(w, r)
			return
		}
		requests[teamID]++
		if teamID == 101 {
			http.NotFound(w, r)
			return
		}
		if !recovered {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"team":{"UTeamID":202,"Season":169},"games":[]}`)
	}))
	t.Cleanup(server.Close)
	worker, err := NewDailyWorker(store, server.URL, server.Client(), Limits{
		MaxEnrolledTeams: 2, ReservedPlayerSlots: 1, MaxRequestsPerRun: 2, MaxRetriesPerTeam: 0, MinRequestInterval: time.Second,
	}, clock)
	if err != nil {
		t.Fatal(err)
	}
	first, err := worker.Run(t.Context())
	if err != nil || first.Complete || first.Requests != 2 || len(first.Results) != 2 || first.Results[0].Outcome != RefreshInvalidTeam || first.Results[1].Outcome != RefreshRetryableFailure {
		t.Fatalf("invalid and transient outcomes = %#v, err %v", first, err)
	}
	invalid, err := store.ReadRefreshState(t.Context(), 101)
	if err != nil || invalid.Status != RefreshInvalid || !invalid.NextDueAt.IsZero() {
		t.Fatalf("invalid team remained due: %#v, err %v", invalid, err)
	}
	immediate, err := worker.Run(t.Context())
	if err != nil || !immediate.Complete || immediate.Requests != 0 || requests[101] != 1 || requests[202] != 1 {
		t.Fatalf("backoff or invalid stop failed: %#v, err %v, requests %#v", immediate, err, requests)
	}
	clock.now = clock.now.Add(16 * time.Minute)
	recovered = true
	later, err := worker.Run(t.Context())
	if err != nil || !later.Complete || later.Requests != 1 || len(later.Results) != 1 || later.Results[0].TeamID != 202 || requests[101] != 1 || requests[202] != 2 {
		t.Fatalf("transient recovery repeated invalid team: %#v, err %v, requests %#v", later, err, requests)
	}
}

func TestDailyRequestBudgetIncludesFacilityLookupsAndPreservesDueWork(t *testing.T) {
	store := newTestStore(t, archivetest.NewTable())
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID: 101, Team: lps.TeamSummary{UTeamID: 101, Season: 169},
		Games:     []lps.TeamScheduleGame{{UGameID: 9001, Season: 169, UTeam1: 101, UTeam2: 202, Result: "1-0"}},
		FetchedAt: seededAt,
	}); err != nil {
		t.Fatal(err)
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/teams/101":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169,"FacilityID":5},"games":[{"UGameID":9001,"UTeam1":101,"UTeam2":202,"Season":169,"result":"2-0","FacilityID":5}]}`)
		case "/facilities/5":
			_, _ = fmt.Fprint(w, `{"FacilityID":5,"FacilityName":"Downtown"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	base := Limits{MaxEnrolledTeams: 1, ReservedPlayerSlots: 0, MaxRequestsPerRun: 1, MaxRetriesPerTeam: 0, MinRequestInterval: time.Second}
	limited, err := NewDailyWorker(store, server.URL, server.Client(), base, clock)
	if err != nil {
		t.Fatal(err)
	}
	first, err := limited.Run(t.Context())
	if err != nil || first.Complete || !first.PendingDueWork || first.Requests != 1 || len(first.Results) != 1 || first.Results[0].Outcome != RefreshBudgetExhausted || !slices.Equal(requests, []string{"/teams/101"}) {
		t.Fatalf("facility fanout exceeded request budget: %#v, err %v, requests %#v", first, err, requests)
	}
	history, err := store.ReadTeamSeason(t.Context(), 101, 169)
	if err != nil || len(history.Games) != 1 || history.Games[0].Result != "1-0" {
		t.Fatalf("budget exhaustion changed retained game: %#v, err %v", history, err)
	}
	base.MaxRequestsPerRun = 2
	continued, err := NewDailyWorker(store, server.URL, server.Client(), base, clock)
	if err != nil {
		t.Fatal(err)
	}
	second, err := continued.Run(t.Context())
	if err != nil || !second.Complete || second.Requests != 2 || !slices.Equal(requests, []string{"/teams/101", "/teams/101", "/facilities/5"}) {
		t.Fatalf("due team did not resume within facility budget: %#v, err %v, requests %#v", second, err, requests)
	}
}

func TestDailyWorkerPagesPastFirstDueIndexPage(t *testing.T) {
	backend := archivetest.NewTable()
	backend.PageSize = 25
	store := newTestStore(t, backend)
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for teamID := 101; teamID <= 130; teamID++ {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: seededAt}); err != nil {
			t.Fatal(err)
		}
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := map[int]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var teamID int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &teamID); err != nil {
			http.NotFound(w, r)
			return
		}
		requests[teamID]++
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":%d,"Season":169},"games":[]}`, teamID)
	}))
	t.Cleanup(server.Close)
	worker, err := NewDailyWorker(store, server.URL, server.Client(), Limits{
		MaxEnrolledTeams: 30, ReservedPlayerSlots: 1, MaxRequestsPerRun: 30, MaxRetriesPerTeam: 0, MinRequestInterval: time.Millisecond,
	}, clock)
	if err != nil {
		t.Fatal(err)
	}
	report, err := worker.Run(t.Context())
	if err != nil || !report.Complete || report.Requests != 30 || len(report.Results) != 30 || backend.IndexQueries() < 2 || len(requests) != 30 {
		t.Fatalf("daily pagination missed due work: report %#v, err %v, queries %d, requests %#v", report, err, backend.IndexQueries(), requests)
	}
	for teamID := 101; teamID <= 130; teamID++ {
		if requests[teamID] != 1 {
			t.Errorf("team %d attempted %d times", teamID, requests[teamID])
		}
	}
}

func (clock *fakeDailyClock) Now() time.Time { return clock.now }

func (clock *fakeDailyClock) Sleep(_ context.Context, delay time.Duration) error {
	clock.sleeps = append(clock.sleeps, delay)
	clock.now = clock.now.Add(delay)
	return nil
}

func TestDailyWorkerRefreshesEveryDueValidTeamAndCheckpointsDuplicateDelivery(t *testing.T) {
	backend := archivetest.NewTable()
	store := newTestStore(t, backend)
	seededAt := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{101, 303, 404} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
			TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: seededAt,
		}); err != nil {
			t.Fatalf("seed manual team %d: %v", teamID, err)
		}
	}
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "owner", ObservedAt: seededAt,
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 202, Season: 169}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 202, Season: 169}}},
	}); err != nil {
		t.Fatalf("seed player team: %v", err)
	}
	initialPlayerState, err := store.ReadRefreshState(t.Context(), 202)
	if err != nil || !initialPlayerState.LastAttemptAt.IsZero() {
		t.Fatalf("new player enrollment was reported as a completed source attempt: %#v, err %v", initialPlayerState, err)
	}
	if err := store.RecordRefreshFailure(t.Context(), &RefreshFailure{
		TeamID: 404, AttemptedAt: seededAt.Add(time.Hour), Status: RefreshInvalid,
		ErrorKind: lps.ErrorInvalidTeam, HTTPStatusCode: http.StatusNotFound,
	}); err != nil {
		t.Fatalf("mark invalid team: %v", err)
	}
	clock := &fakeDailyClock{now: seededAt.Add(25 * time.Hour)}
	requests := []int{}
	requestTimes := []time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var teamID int
		if _, err := fmt.Sscanf(r.URL.Path, "/teams/%d", &teamID); err != nil {
			http.NotFound(w, r)
			return
		}
		requests = append(requests, teamID)
		requestTimes = append(requestTimes, clock.Now())
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":%d,"Season":169},"games":[]}`, teamID)
	}))
	t.Cleanup(server.Close)
	limits := Limits{MaxEnrolledTeams: 8, ReservedPlayerSlots: 2, MaxRequestsPerRun: 8, MaxRetriesPerTeam: 1, MinRequestInterval: 2 * time.Second}
	worker, err := NewDailyWorker(store, server.URL, server.Client(), limits, clock)
	if err != nil {
		t.Fatal(err)
	}
	report, err := worker.Run(t.Context())
	if err != nil {
		t.Fatalf("daily invocation: %v", err)
	}
	if !report.Complete || report.Requests != 3 || len(report.Results) != 3 || !slices.Equal(requests, []int{202, 101, 303}) || backend.IndexQueries() == 0 {
		t.Fatalf("daily report = %#v, requests = %#v", report, requests)
	}
	if len(clock.sleeps) != 2 || clock.sleeps[0] != 2*time.Second || clock.sleeps[1] != 2*time.Second || requestTimes[1].Sub(requestTimes[0]) < 2*time.Second || requestTimes[2].Sub(requestTimes[1]) < 2*time.Second {
		t.Fatalf("requests were not paced: times %#v, sleeps %#v", requestTimes, clock.sleeps)
	}
	for _, teamID := range []int{101, 202, 303} {
		state, err := store.ReadRefreshState(t.Context(), teamID)
		if err != nil || state.Status != RefreshReady || !state.NextDueAt.After(clock.Now()) {
			t.Errorf("team %d was not checkpointed: state %#v, err %v", teamID, state, err)
		}
	}
	second, err := worker.Run(context.Background())
	if err != nil || !second.Complete || second.Requests != 0 || len(second.Results) != 0 || len(requests) != 3 {
		t.Fatalf("duplicate delivery repeated work: report %#v, err %v, requests %#v", second, err, requests)
	}
}
