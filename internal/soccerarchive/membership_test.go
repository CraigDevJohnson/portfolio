package soccerarchive

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/types"
)

func TestDynamoArchivePersistsOwnerBoundPlayerSeasonEvidenceAndTeamEnrollment(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "soccer-history")
	observedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	discovery := &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "stable-subject", ObservedAt: observedAt,
		Players: []types.LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
			{UPlayerID: 1002, FirstName: "Taylor", LastName: "Johnson", IsMainPlayer: false},
		},
		KnownTeams: []lps.TeamSummary{
			{UTeamID: 4101, TeamName: "Shared FC", Season: 77},
			{UTeamID: 4102, TeamName: "Old FC", Season: 78},
			{UTeamID: 4202, TeamName: "Taylor FC", Season: 79},
		},
		Memberships: []PlayerMembership{
			{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Shared FC", Season: 77}},
			{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4102, TeamName: "Old FC", Season: 78}},
			{PlayerID: 1002, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Shared FC", Season: 77}},
			{PlayerID: 1002, Team: lps.TeamSummary{UTeamID: 4202, TeamName: "Taylor FC", Season: 79}},
		},
	}
	if err := store.SavePlayerDiscovery(context.Background(), discovery); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePlayerDiscovery(context.Background(), discovery); err != nil {
		t.Fatalf("repeated import: %v", err)
	}

	assertArchiveItem(t, backend, "PLAYER#1001/META", map[string]any{
		// The main-player flag is owner-relative, so the shared identity omits it.
		"kind": "player", "player_id": 1001, "first_name": "Craig", "last_name": "Johnson", "is_main_player": nil,
		"observed_at": observedAt.Format(sortableUTCFormat),
	})
	assertArchiveItem(t, backend, "PLAYER#1002/META", map[string]any{
		"kind": "player", "player_id": 1002, "first_name": "Taylor", "is_main_player": nil,
	})
	assertArchiveItem(t, backend, "TEAM#4101/META", map[string]any{
		"kind": "team", "team_id": 4101, "enrollment_source": "player", "season_id": 77, "due_pk": "TEAM_DUE",
	})
	if backend.Len() != 11 { // 2 players, 2 owner links, 4 memberships, 3 teams
		t.Fatalf("durable item count = %d, want 11", backend.Len())
	}
	membershipCount := 0
	items, err := backend.Items()
	if err != nil {
		t.Fatal(err)
	}
	for key, got := range items {
		if _, exists := got["ttl"]; exists {
			t.Errorf("%s inherited session TTL", key)
		}
		for field, value := range got {
			if strings.Contains(strings.ToLower(field), "jwt") || strings.Contains(strings.ToLower(field), "token") || strings.Contains(strings.ToLower(field), "credential") || strings.Contains(fmt.Sprint(value), "eyJ") {
				t.Errorf("%s has credential-like durable field %s", key, field)
			}
		}
		if got["kind"] != "membership" {
			continue
		}
		membershipCount++
		if got["owner_issuer"] != discovery.OwnerIssuer || got["owner_subject"] != discovery.OwnerSubject || got["source"] != "authenticated_player_lookup" || got["observed_at"] != observedAt.Format(sortableUTCFormat) {
			t.Errorf("%s missing owner, source, or observation: %#v", key, got)
		}
	}
	if membershipCount != 4 {
		t.Errorf("stored membership count = %d, want 4", membershipCount)
	}
	for _, candidate := range []struct {
		issuer, subject            string
		playerID, teamID, seasonID int
		want                       bool
	}{
		{discovery.OwnerIssuer, discovery.OwnerSubject, 1001, 4101, 77, true},
		{discovery.OwnerIssuer, discovery.OwnerSubject, 1001, 4102, 78, true},
		{discovery.OwnerIssuer, discovery.OwnerSubject, 1001, 4101, 78, false},
		{discovery.OwnerIssuer, discovery.OwnerSubject, 1002, 4102, 78, false},
		{discovery.OwnerIssuer, "other-subject", 1001, 4101, 77, false},
	} {
		got, err := store.HasPlayerMembership(context.Background(), candidate.issuer, candidate.subject, candidate.playerID, candidate.teamID, candidate.seasonID)
		if err != nil || got != candidate.want {
			t.Errorf("HasPlayerMembership(%q, %q, %d, %d, %d) = %v, %v; want %v", candidate.issuer, candidate.subject, candidate.playerID, candidate.teamID, candidate.seasonID, got, err, candidate.want)
		}
	}

	if err := store.SaveTeamSnapshot(context.Background(), &Snapshot{
		TeamID: 4101, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Shared FC", Season: 77}, FetchedAt: observedAt.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	assertArchiveItem(t, backend, "TEAM#4101/META", map[string]any{"enrollment_source": "player"})
}

func TestDynamoArchiveRejectsUnownedOrUnprovenPlayerEvidence(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "soccer-history")
	base := PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "stable-subject", ObservedAt: time.Now(),
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4101, Season: 77}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: 77}}},
	}
	for _, mutate := range []func(*PlayerDiscovery){
		func(d *PlayerDiscovery) { d.OwnerSubject = "" },
		func(d *PlayerDiscovery) { d.Memberships[0].PlayerID = 1002 },
		func(d *PlayerDiscovery) { d.Memberships[0].Team.Season = 0 },
	} {
		candidate := base
		candidate.Memberships = append([]PlayerMembership(nil), base.Memberships...)
		mutate(&candidate)
		if err := store.SavePlayerDiscovery(context.Background(), &candidate); err == nil {
			t.Fatal("invalid discovery was stored")
		}
		if backend.Len() != 0 {
			t.Fatalf("invalid discovery left %d durable items", backend.Len())
		}
	}
}

func TestDynamoArchiveSeparatesTwoSiteOwnersOfOneLPSPlayer(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "soccer-history")
	first := &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "first-subject", ObservedAt: time.Now(),
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4101, Season: 77}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: 77}}},
	}
	if err := store.SavePlayerDiscovery(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.OwnerSubject = "second-subject"
	second.ObservedAt = first.ObservedAt.Add(time.Second)
	if err := store.SavePlayerDiscovery(context.Background(), &second); err != nil {
		t.Fatal(err)
	}
	owners := map[string]int{}
	items, err := backend.Items()
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range items {
		if got["kind"] == "membership" {
			owners[got["owner_subject"].(string)]++
		}
	}
	if owners["first-subject"] != 1 || owners["second-subject"] != 1 || len(owners) != 2 {
		t.Fatalf("one site owner replaced another's proof: %#v", owners)
	}
}

// playerEnrollmentLPS serves team 4202's public schedule: one scored game in
// LPS season 79.
func playerEnrollmentLPS(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/4202":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[{"UGameID":9001,"UTeam1":4202,"UTeam2":4999,"Season":79,"result":"2-1"}]}`)
		case "/teams/4101":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRefreshWorkerRefreshesATeamAPlayerImportEnrolled(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	observedAt := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "stable-subject", ObservedAt: observedAt,
		Players:     []types.LPSPlayer{{UPlayerID: 1002, FirstName: "Taylor"}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4202, TeamName: "Taylor FC", Season: 79}},
		Memberships: []PlayerMembership{{PlayerID: 1002, Team: lps.TeamSummary{UTeamID: 4202, TeamName: "Taylor FC", Season: 79}}},
	}); err != nil {
		t.Fatalf("save player discovery: %v", err)
	}

	state, err := store.ReadRefreshState(t.Context(), 4202)
	if err != nil || state.Status != RefreshReady || !state.LastAttemptAt.IsZero() || !state.NextDueAt.Equal(observedAt) {
		t.Fatalf("discovered team refresh state = %+v, %v; want ready, never attempted, due when discovered", state, err)
	}
	unfetched, err := store.ReadTeamSeason(t.Context(), 4202, 79)
	if err != nil || unfetched.Coverage.Status != CoverageNotFetched || len(unfetched.Games) != 0 {
		t.Fatalf("discovered team history before any refresh = %+v, %v; want known but not fetched", unfetched, err)
	}

	server := playerEnrollmentLPS(t)
	refreshedAt := observedAt.Add(time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return refreshedAt })
	report := worker.Run(t.Context(), []int{4202})
	if !report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != RefreshSucceeded {
		t.Fatalf("worker report = %+v", report)
	}
	history, err := store.ReadTeamSeason(t.Context(), 4202, 79)
	if err != nil || history.Coverage.Status != CoverageFetched || len(history.Games) != 1 || history.Games[0].UGameID != 9001 || history.Games[0].Result != "2-1" {
		t.Fatalf("refreshed history = %+v, %v", history, err)
	}
	state, err = store.ReadRefreshState(t.Context(), 4202)
	if err != nil || !state.LastAttemptAt.Equal(refreshedAt) || !state.NextDueAt.Equal(refreshedAt.Add(24*time.Hour)) {
		t.Fatalf("refresh state after the first refresh = %+v, %v", state, err)
	}
	assertArchiveItem(t, backend, "TEAM#4202/META", map[string]any{"enrollment_source": "player", "team_name": "Taylor FC"})
}

func TestPlayerImportKeepsAnEnrolledTeamsRefreshState(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	enrolledAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for _, teamID := range []int{4101, 4202} {
		if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 79}, FetchedAt: enrolledAt}); err != nil {
			t.Fatalf("enroll team %d from a Team ID lookup: %v", teamID, err)
		}
	}
	server := playerEnrollmentLPS(t)
	invalidAt := enrolledAt.Add(time.Hour)
	worker := NewRefreshWorker(store, lps.NewScheduleResolver(server.URL, server.Client(), ""), func() time.Time { return invalidAt })
	if report := worker.Run(t.Context(), []int{4101}); report.Results[0].Outcome != RefreshInvalidTeam {
		t.Fatalf("team 4101 refresh = %+v, want invalid", report)
	}
	scheduled, err := store.ReadRefreshState(t.Context(), 4202)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "stable-subject", ObservedAt: invalidAt.Add(time.Hour),
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4101, TeamName: "Craig FC", Season: 80}, {UTeamID: 4202, TeamName: "Taylor FC", Season: 80}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: 80}}},
	}); err != nil {
		t.Fatalf("save player discovery: %v", err)
	}

	if report := worker.Run(t.Context(), []int{4101}); report.Results[0].Outcome != RefreshSkippedInvalid {
		t.Errorf("a player import revived the invalid team 4101: %+v", report)
	}
	if state, err := store.ReadRefreshState(t.Context(), 4202); err != nil || state != scheduled {
		t.Errorf("team 4202 refresh state after a player import = %+v, %v; want unchanged %+v", state, err, scheduled)
	}
	for _, teamID := range []int{4101, 4202} {
		// The team facts still come from the team's own response.
		assertArchiveItem(t, backend, fmt.Sprintf("TEAM#%d/META", teamID), map[string]any{"enrollment_source": "player", "season_id": 79, "team_name": nil})
	}
}

func TestDynamoArchiveRemovesOnePlayerGloballyAndRetainsSharedFacts(t *testing.T) {
	backend := archivetest.NewTable()
	// Two items per page, so the player's partition spans several pages.
	backend.PageSize = 2
	store := NewDynamoStoreWithAPI(backend, "soccer-history")
	observedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	first := &PlayerDiscovery{
		OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: "first-subject", ObservedAt: observedAt,
		Players:    []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Alex"}, {UPlayerID: 1002, FirstName: "Taylor"}},
		KnownTeams: []lps.TeamSummary{{UTeamID: 4101, Season: 77}},
		Memberships: []PlayerMembership{
			{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: 77}},
			{PlayerID: 1002, Team: lps.TeamSummary{UTeamID: 4101, Season: 77}},
		},
	}
	if err := store.SavePlayerDiscovery(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := *first
	second.OwnerSubject = "second-subject"
	second.ObservedAt = observedAt.Add(time.Second)
	second.Players = []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Alex"}}
	second.Memberships = []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: 78}}}
	if err := store.SavePlayerDiscovery(t.Context(), &second); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{
		TeamID: 4101, Team: lps.TeamSummary{UTeamID: 4101, Season: 77},
		Games:      []lps.TeamScheduleGame{{UGameID: 7001, UTeam1: 4101, UTeam2: 4201, Season: 77, FacilityID: 5}},
		Facilities: []lps.FacilityResponse{{FacilityID: 5, FacilityName: "Shared Field"}}, FetchedAt: observedAt.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeletePlayerEvidence(t.Context(), 1001); err != nil {
		t.Fatal(err)
	}
	stored, err := backend.Items()
	if err != nil {
		t.Fatal(err)
	}
	for key := range stored {
		if strings.HasPrefix(key, "PLAYER#1001/") {
			t.Errorf("removed player's personal record remains: %s", key)
		}
	}
	for _, key := range []string{"PLAYER#1002/META", "TEAM#4101/META", "GAME#7001/META", "FACILITY#5/META"} {
		if stored[key] == nil {
			t.Errorf("shared or another player's fact was removed: %s", key)
		}
	}

	first.ObservedAt = observedAt.Add(2 * time.Minute)
	first.Players = []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Alex"}}
	first.Memberships = first.Memberships[:1]
	if err := store.SavePlayerDiscovery(t.Context(), first); err != nil {
		t.Fatalf("later deliberate import did not recollect player: %v", err)
	}
	if backend.Item("PLAYER#1001/META") == nil {
		t.Fatal("later valid import did not restore player identity")
	}
}
