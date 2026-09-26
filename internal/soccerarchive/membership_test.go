package soccerarchive

import (
	"context"
	"fmt"
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
		"kind": "player", "player_id": 1001, "first_name": "Craig", "last_name": "Johnson", "is_main_player": true,
		"observed_at": observedAt.Format(sortableUTCFormat),
	})
	assertArchiveItem(t, backend, "PLAYER#1002/META", map[string]any{
		"kind": "player", "player_id": 1002, "first_name": "Taylor", "is_main_player": false,
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
