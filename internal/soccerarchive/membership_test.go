package soccerarchive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/types"
)

func TestDynamoArchivePersistsOwnerBoundPlayerSeasonEvidenceAndTeamEnrollment(t *testing.T) {
	backend := archivetest.NewTable()
	store := newTestStore(t, backend)
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
		"kind": "team", "team_id": 4101, "enrollment_source": "player", "season_id": 77,
	})
	if backend.Len() != 12 { // 2 players, 2 owner links, 4 memberships, 3 teams, 1 admission counter
		t.Fatalf("durable item count = %d, want 12", backend.Len())
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
	store := newTestStore(t, backend)
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
	store := newTestStore(t, backend)
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
	store := newTestStore(t, backend)
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
	if err != nil || !state.LastAttemptAt.Equal(refreshedAt) || !state.NextDueAt.Equal(refreshedAt.Add(4*time.Hour)) {
		t.Fatalf("refresh state after the first refresh = %+v, %v", state, err)
	}
	assertArchiveItem(t, backend, "TEAM#4202/META", map[string]any{"enrollment_source": "player", "team_name": "Taylor FC"})
}

func TestPlayerImportKeepsAnEnrolledTeamsRefreshState(t *testing.T) {
	backend := archivetest.NewTable()
	store := newTestStore(t, backend)
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
	store := newTestStore(t, backend)
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

// importingDuringRemoval is the archive table while another site owner
// imports the same player during a removal: before the first delete that
// follows each listing of the partition, it runs that import, at most
// imports times.
type importingDuringRemoval struct {
	*archivetest.Table

	runImport func() error
	imports   int
	listed    bool
}

func (api *importingDuringRemoval) Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	api.listed = true
	return api.Table.Query(ctx, input, optFns...)
}

func (api *importingDuringRemoval) DeleteItem(ctx context.Context, input *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if api.listed && api.imports > 0 {
		api.listed = false
		api.imports--
		if err := api.runImport(); err != nil {
			return nil, fmt.Errorf("concurrent import: %w", err)
		}
	}
	return api.Table.DeleteItem(ctx, input, optFns...)
}

// removalRace stores the first owner's evidence for player 1001, then
// returns a store whose removals race the second owner's import of 1001.
func removalRace(t *testing.T, imports int) (*DynamoStore, *importingDuringRemoval) {
	t.Helper()
	api := &importingDuringRemoval{Table: archivetest.NewTable(), imports: imports}
	store := newTestStore(t, api)
	observedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	discovery := func(subject string, at time.Time, season int) *PlayerDiscovery {
		return &PlayerDiscovery{
			OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: subject, ObservedAt: at,
			Players:     []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Alex"}},
			KnownTeams:  []lps.TeamSummary{{UTeamID: 4101, Season: season}},
			Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, Season: season}}},
		}
	}
	if err := store.SavePlayerDiscovery(t.Context(), discovery("first-subject", observedAt, 77)); err != nil {
		t.Fatal(err)
	}
	api.runImport = func() error {
		return store.SavePlayerDiscovery(t.Context(), discovery("second-subject", observedAt.Add(time.Second), 78))
	}
	return store, api
}

// assertRemovalLeftConsistentEvidence requires a removal either to report
// success with the player's partition empty, or to report failure with every
// remaining membership still beside its player identity and owner link.
func assertRemovalLeftConsistentEvidence(t *testing.T, backend *archivetest.Table, removal error) {
	t.Helper()
	items, err := backend.Items()
	if err != nil {
		t.Fatal(err)
	}
	remaining := make([]string, 0)
	for key := range items {
		if strings.HasPrefix(key, "PLAYER#1001/") {
			remaining = append(remaining, key)
		}
	}
	if removal == nil {
		if len(remaining) != 0 {
			t.Errorf("removal reported success but left the player's records %v", remaining)
		}
		return
	}
	for _, key := range remaining {
		sk := strings.TrimPrefix(key, "PLAYER#1001/")
		owner, _, isMembership := strings.Cut(sk, "#TEAM#")
		if !isMembership {
			continue
		}
		if items["PLAYER#1001/META"] == nil || items["PLAYER#1001/"+owner+"#META"] == nil {
			t.Errorf("failed removal left membership %s without its player identity or owner link: %v", key, remaining)
		}
	}
}

func TestPlayerRemovalRacingAnotherOwnersImportLeavesNoOrphanedEvidence(t *testing.T) {
	store, api := removalRace(t, 1)

	removal := store.DeletePlayerEvidence(t.Context(), 1001)

	if api.imports != 0 {
		t.Fatal("the second owner's import did not run during the removal")
	}
	assertRemovalLeftConsistentEvidence(t, api.Table, removal)
}

func TestPlayerRemovalReportsFailureWhileImportsKeepAddingEvidence(t *testing.T) {
	store, api := removalRace(t, 100)

	removal := store.DeletePlayerEvidence(t.Context(), 1001)

	if removal == nil {
		t.Fatal("removal reported success while another owner's imports kept adding the player's records")
	}
	if api.imports == 0 {
		t.Error("removal kept deleting without a bound on its rounds")
	}
}

// recordingQueries is the archive table that records each base-table query
// the store makes, and fails them all with failQuery when it is set.
type recordingQueries struct {
	*archivetest.Table

	queries   []*dynamodb.QueryInput
	failQuery error
}

func (api *recordingQueries) Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	api.queries = append(api.queries, input)
	if api.failQuery != nil {
		return nil, api.failQuery
	}
	return api.Table.Query(ctx, input, optFns...)
}

func TestDynamoArchiveListsOnlyOneOwnersProvenTeamSeasonsForAPlayer(t *testing.T) {
	api := &recordingQueries{Table: archivetest.NewTable()}
	store := newTestStore(t, api)
	const issuer = "https://issuer.example.com/pool"
	observedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	// The first owner's import proves Craig (1001) on Craig FC in season 77
	// and Old FC in season 78, and Taylor (1002) on Craig FC in season 77.
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: issuer, OwnerSubject: "first-subject", ObservedAt: observedAt,
		Players:    []types.LPSPlayer{{UPlayerID: 1001}, {UPlayerID: 1002}},
		KnownTeams: []lps.TeamSummary{{UTeamID: 4101, Season: 77}, {UTeamID: 4102, Season: 78}},
		Memberships: []PlayerMembership{
			{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Craig FC", DivisionName: "Open A", Season: 77}},
			{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4102, TeamName: "Old FC", Color: "navy", FacilityID: 5, FacilityName: "North Field", Season: 78}},
			{PlayerID: 1002, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Craig FC", DivisionName: "Open A", Season: 77}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// A later import by the same owner proves Craig FC's next season.
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: issuer, OwnerSubject: "first-subject", ObservedAt: observedAt.Add(time.Hour),
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4101, Season: 80}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Craig FC", Season: 80}}},
	}); err != nil {
		t.Fatal(err)
	}
	// Another site owner imports Craig with a team the first owner never proved.
	if err := store.SavePlayerDiscovery(t.Context(), &PlayerDiscovery{
		OwnerIssuer: issuer, OwnerSubject: "second-subject", ObservedAt: observedAt.Add(2 * time.Hour),
		Players:     []types.LPSPlayer{{UPlayerID: 1001}},
		KnownTeams:  []lps.TeamSummary{{UTeamID: 4103, Season: 81}},
		Memberships: []PlayerMembership{{PlayerID: 1001, Team: lps.TeamSummary{UTeamID: 4103, TeamName: "Other FC", Season: 81}}},
	}); err != nil {
		t.Fatal(err)
	}
	listed := func(t *testing.T, subject string, playerID int) string {
		t.Helper()
		memberships, err := store.ListPlayerMemberships(t.Context(), issuer, subject, playerID)
		if err != nil {
			t.Fatalf("ListPlayerMemberships(%q, %d): %v", subject, playerID, err)
		}
		if memberships == nil {
			t.Errorf("ListPlayerMemberships(%q, %d) = nil, want a list", subject, playerID)
		}
		got := make([]string, 0, len(memberships))
		for i := range memberships {
			membership := &memberships[i]
			got = append(got, fmt.Sprintf("%d:%d/%d %s|%s%s", membership.PlayerID, membership.Team.UTeamID, membership.Team.Season, membership.Team.TeamName, membership.Team.DivisionName, teamColorAndFacility(&membership.Team)))
		}
		return strings.Join(got, ", ")
	}

	api.queries = nil
	if got, want := listed(t, "first-subject", 1001), "1001:4101/77 Craig FC|Open A, 1001:4101/80 Craig FC|, 1001:4102/78 Old FC| navy 5 North Field"; got != want {
		t.Errorf("first owner's seasons for Craig = %q, want %q", got, want)
	}
	if len(api.queries) != 1 {
		t.Fatalf("listing made %d queries, want one", len(api.queries))
	}
	query := api.queries[0]
	pk, _ := query.ExpressionAttributeValues[":pk"].(*ddbtypes.AttributeValueMemberS)
	prefix, _ := query.ExpressionAttributeValues[":prefix"].(*ddbtypes.AttributeValueMemberS)
	if aws.ToString(query.KeyConditionExpression) != "pk = :pk AND begins_with(sk, :prefix)" || aws.ToString(query.IndexName) != "" ||
		pk == nil || pk.Value != "PLAYER#1001" || prefix == nil || prefix.Value != ownerEvidencePrefix(issuer, "first-subject")+"#TEAM#" {
		t.Errorf("listing query = %q with %v, want the first owner's membership prefix in Craig's partition", aws.ToString(query.KeyConditionExpression), query.ExpressionAttributeValues)
	}

	if got, want := listed(t, "second-subject", 1001), "1001:4103/81 Other FC|"; got != want {
		t.Errorf("second owner's seasons for Craig = %q, want %q", got, want)
	}
	if got, want := listed(t, "first-subject", 1002), "1002:4101/77 Craig FC|Open A"; got != want {
		t.Errorf("first owner's seasons for Taylor = %q, want %q", got, want)
	}
	if got := listed(t, "second-subject", 1002); got != "" {
		t.Errorf("second owner's seasons for Taylor = %q, want none", got)
	}
	if got := listed(t, "first-subject", 1003); got != "" {
		t.Errorf("seasons for a player with no proof = %q, want none", got)
	}

	// The list follows every page of the partition.
	api.PageSize = 1
	if got, want := listed(t, "first-subject", 1001), "1001:4101/77 Craig FC|Open A, 1001:4101/80 Craig FC|, 1001:4102/78 Old FC| navy 5 North Field"; got != want {
		t.Errorf("paged seasons for Craig = %q, want %q", got, want)
	}
	api.PageSize = 0

	// A failed query is an error, never an empty list.
	api.failQuery = errors.New("throttled")
	if memberships, err := store.ListPlayerMemberships(t.Context(), issuer, "first-subject", 1001); err == nil || len(memberships) != 0 {
		t.Errorf("failed listing = %v, %v; want an error", memberships, err)
	}
	api.failQuery = nil

	// Removing Craig removes every owner's proof for him.
	if err := store.DeletePlayerEvidence(t.Context(), 1001); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"first-subject", "second-subject"} {
		if got := listed(t, subject, 1001); got != "" {
			t.Errorf("%s's seasons for removed Craig = %q, want none", subject, got)
		}
	}
	if got, want := listed(t, "first-subject", 1002), "1002:4101/77 Craig FC|Open A"; got != want {
		t.Errorf("Taylor's seasons after Craig's removal = %q, want %q", got, want)
	}
}

// teamColorAndFacility describes a team's color and facility, when it has them.
func teamColorAndFacility(team *lps.TeamSummary) string {
	if team.Color == "" && team.FacilityID == 0 && team.FacilityName == "" {
		return ""
	}
	return fmt.Sprintf(" %s %d %s", team.Color, team.FacilityID, team.FacilityName)
}
