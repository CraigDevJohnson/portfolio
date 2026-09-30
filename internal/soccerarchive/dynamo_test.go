package soccerarchive

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
)

func TestDynamoArchiveStoreKeepsOneGameAcrossRepeatedAndPartialLookups(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	firstFetch := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		TeamID: 479691,
		Team: lps.TeamSummary{
			UTeamID: 479691, TeamName: "Boise FC", DivisionName: "Open A", Season: 169, FacilityID: 5,
		},
		Games: []lps.TeamScheduleGame{{
			UGameID: 8001, UTeam1: 479691, UTeam2: 222, Season: 169, FacilityID: 5,
			SchedGameDateTime: "2026-09-26T18:00:00Z", Result: "2-1",
		}},
		Facilities: []lps.FacilityResponse{{FacilityID: 5, FacilityName: "Downtown", Address: "123 Field St", City: "Boise"}},
		FetchedAt:  firstFetch,
	}
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("first SaveTeamSnapshot: %v", err)
	}
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("repeated SaveTeamSnapshot: %v", err)
	}
	firstCount := backend.Len()
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{
		"game_id": 8001, "season_id": 169, "home_team_id": 479691, "away_team_id": 222,
		"facility_id": 5, "result": "2-1", "scheduled_at": "2026-09-26T18:00:00Z",
	})
	assertArchiveItem(t, backend, "TEAM#479691/META", map[string]any{
		"team_id": 479691, "team_name": "Boise FC", "division_name": "Open A", "enrollment_source": "manual",
	})
	assertArchiveItem(t, backend, "FACILITY#5/META", map[string]any{
		"facility_name": "Downtown", "address": "123 Field St",
	})
	assertArchiveItem(t, backend, "TEAM#479691/COVERAGE", map[string]any{
		"returned_game_count": 1, "status": "fetched",
	})
	if backend.Item("TEAM#479691/SEASON#0000000169#GAME#8001") == nil {
		t.Fatal("team-season game lookup edge missing")
	}

	snapshot.FetchedAt = firstFetch.Add(time.Hour)
	snapshot.Games[0].Result = "3-1"
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("corrected SaveTeamSnapshot: %v", err)
	}
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{"result": "3-1"})
	snapshot.FetchedAt = firstFetch.Add(2 * time.Hour)
	snapshot.Games = nil
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("empty SaveTeamSnapshot: %v", err)
	}
	if backend.Len() != firstCount {
		t.Fatalf("item count after repeated and empty lookups = %d, want %d", backend.Len(), firstCount)
	}
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{"result": "3-1"})
	assertArchiveItem(t, backend, "TEAM#479691/COVERAGE", map[string]any{"returned_game_count": 0, "status": "fetched"})

	history, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if history.Team.TeamName != "Boise FC" || history.Coverage.ReturnedGameCount != 0 || len(history.Games) != 1 || history.Games[0].Result != "3-1" || len(history.Facilities) != 1 || history.Facilities[0].Address != "123 Field St" {
		t.Fatalf("read-back history = %#v", history)
	}
	snapshot.FetchedAt = firstFetch.Add(30 * time.Minute)
	snapshot.Games = []lps.TeamScheduleGame{{UGameID: 8001, UTeam1: 479691, UTeam2: 222, Season: 169, Result: "1-1"}}
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("stale SaveTeamSnapshot: %v", err)
	}
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{"result": "3-1"})
	assertArchiveItem(t, backend, "TEAM#479691/COVERAGE", map[string]any{"returned_game_count": 0})
}

func TestDynamoArchiveStoreRejectsOlderFetchWithinTheSameSecond(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	baseTime := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		TeamID:    479691,
		Team:      lps.TeamSummary{UTeamID: 479691, Season: 169},
		Games:     []lps.TeamScheduleGame{{UGameID: 8001, Season: 169, Result: "4-1"}},
		FetchedAt: baseTime.Add(500 * time.Millisecond),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("newer SaveTeamSnapshot: %v", err)
	}
	snapshot.FetchedAt = baseTime
	snapshot.Games[0].Result = "1-1"
	if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("older SaveTeamSnapshot: %v", err)
	}
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{"result": "4-1"})
}

func TestDynamoArchiveReadFollowsCorrectedGameAssignment(t *testing.T) {
	store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
	first := Snapshot{
		TeamID:    479691,
		Team:      lps.TeamSummary{UTeamID: 479691, Season: 169},
		Games:     []lps.TeamScheduleGame{{UGameID: 8001, UTeam1: 479691, UTeam2: 222, Season: 169, Result: "2-1"}},
		FetchedAt: time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &first); err != nil {
		t.Fatalf("initial SaveTeamSnapshot: %v", err)
	}
	corrected := Snapshot{
		TeamID:    333,
		Team:      lps.TeamSummary{UTeamID: 333, Season: 170},
		Games:     []lps.TeamScheduleGame{{UGameID: 8001, UTeam1: 333, UTeam2: 444, Season: 170, Result: "3-1"}},
		FetchedAt: first.FetchedAt.Add(time.Hour),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &corrected); err != nil {
		t.Fatalf("corrected SaveTeamSnapshot: %v", err)
	}
	oldHistory, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason old team: %v", err)
	}
	if len(oldHistory.Games) != 0 {
		t.Fatalf("old team still shows reassigned game: %#v", oldHistory.Games)
	}
	newHistory, err := store.ReadTeamSeason(context.Background(), 333, 170)
	if err != nil {
		t.Fatalf("ReadTeamSeason new team: %v", err)
	}
	if len(newHistory.Games) != 1 || newHistory.Games[0].Result != "3-1" {
		t.Fatalf("corrected game missing from new team: %#v", newHistory.Games)
	}
}

func TestDynamoArchiveKeepsOmittedGameFieldsAndAppliesExplicitCorrection(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	first := Snapshot{
		TeamID: 479691,
		Team:   lps.TeamSummary{UTeamID: 479691, TeamName: "Boise FC", Season: 169},
		Games: []lps.TeamScheduleGame{{
			UGameID: 8001, Season: 169, UTeam1: 479691, UTeam2: 222,
			SchedGameDateTime: "2026-09-26T18:00:00Z", FacilityID: 5, Result: "2-1",
		}},
		FetchedAt: time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &first); err != nil {
		t.Fatalf("initial SaveTeamSnapshot: %v", err)
	}
	var partial lps.TeamScheduleGame
	if err := json.Unmarshal([]byte(`{"UGameID":8001,"result":"3-1"}`), &partial); err != nil {
		t.Fatalf("decode partial LPS game: %v", err)
	}
	second := Snapshot{TeamID: first.TeamID, Team: first.Team, Games: []lps.TeamScheduleGame{partial}, FetchedAt: first.FetchedAt.Add(time.Hour)}
	if err := store.SaveTeamSnapshot(context.Background(), &second); err != nil {
		t.Fatalf("partial SaveTeamSnapshot: %v", err)
	}
	history, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if len(history.Games) != 1 || history.Games[0].Result != "3-1" || history.Games[0].SchedGameDateTime != "2026-09-26T18:00:00Z" || history.Games[0].UTeam1 != 479691 || history.Games[0].UTeam2 != 222 || history.Games[0].FacilityID != 5 {
		t.Fatalf("partial response erased known game facts: %#v", history.Games)
	}
	assertArchiveItem(t, backend, "GAME#8001/META", map[string]any{
		"result": "3-1", "scheduled_at": "2026-09-26T18:00:00Z", "home_team_id": 479691, "away_team_id": 222,
	})
	var cleared lps.TeamScheduleGame
	if err := json.Unmarshal([]byte(`{"UGameID":8001,"result":""}`), &cleared); err != nil {
		t.Fatalf("decode explicit score correction: %v", err)
	}
	third := Snapshot{TeamID: first.TeamID, Team: first.Team, Games: []lps.TeamScheduleGame{cleared}, FetchedAt: second.FetchedAt.Add(time.Hour)}
	if err := store.SaveTeamSnapshot(context.Background(), &third); err != nil {
		t.Fatalf("explicit clearing SaveTeamSnapshot: %v", err)
	}
	clearedHistory, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason after explicit clearing: %v", err)
	}
	if len(clearedHistory.Games) != 1 || clearedHistory.Games[0].Result != "" || clearedHistory.Games[0].SchedGameDateTime != "2026-09-26T18:00:00Z" {
		t.Fatalf("explicit score clearing did not retain the kickoff: %#v", clearedHistory.Games)
	}
}

func TestDynamoArchiveAppliesExplicitNullCorrections(t *testing.T) {
	backend := archivetest.NewTable()
	store := NewDynamoStoreWithAPI(backend, "durable-soccer-history")
	team := lps.TeamSummary{UTeamID: 479691, Season: 169}
	firstFetch := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	for i, source := range []string{
		`{"UGameID":8001,"Season":169,"UTeam1":479691,"UTeam2":222,"SchedGameDateTime":"2026-09-26T18:00:00Z","schedGameEndTime":"2026-09-26T19:00:00Z","result":"2-1"}`,
		`{"UGameID":8001,"SchedGameDateTime":"2026-10-03T18:00:00Z","schedGameEndTime":null,"result":null}`,
	} {
		var game lps.TeamScheduleGame
		if err := json.Unmarshal([]byte(source), &game); err != nil {
			t.Fatalf("decode LPS game %d: %v", i, err)
		}
		snapshot := Snapshot{TeamID: 479691, Team: team, Games: []lps.TeamScheduleGame{game}, FetchedAt: firstFetch.Add(time.Duration(i) * time.Hour)}
		if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
			t.Fatalf("SaveTeamSnapshot %d: %v", i, err)
		}
	}

	history, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	if len(history.Games) != 1 {
		t.Fatalf("games = %#v, want the rescheduled game", history.Games)
	}
	game := history.Games[0]
	if game.SchedGameDateTime != "2026-10-03T18:00:00Z" || game.SchedGameEndTime != nil || game.Result != "" || game.UTeam1 != 479691 {
		t.Fatalf("explicit null corrections were not applied: %#v", game)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(game.SourceJSON, &raw); err != nil {
		t.Fatalf("decode stored source: %v", err)
	}
	if string(raw["schedGameEndTime"]) != "null" || string(raw["result"]) != "null" {
		t.Fatalf("stored source end time %s and result %s, want explicit nulls", raw["schedGameEndTime"], raw["result"])
	}
	item := backend.Item("GAME#8001/META")
	for _, attribute := range []string{"scheduled_end_at", "result"} {
		if _, found := item[attribute]; found {
			t.Errorf("GAME#8001/META keeps %s after LPS set it to null", attribute)
		}
	}
}

func TestDynamoArchiveDoesNotMixTeamFactsAfterASideIsReassigned(t *testing.T) {
	store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
	firstFetch := time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC)
	for i, response := range []struct {
		team lps.TeamSummary
		game string
	}{
		{
			team: lps.TeamSummary{UTeamID: 1, Season: 169},
			game: `{"UGameID":8001,"Season":169,"UTeam1":1,"UTeam2":2,"home_team":{"UTeamID":1,"team_name":"Old Home","division_name":"Open A","FacilityID":5,"facility_name":"Old Park","Season":169},"visitor_team":{"UTeamID":2,"team_name":"Away FC","Season":169}}`,
		},
		{
			team: lps.TeamSummary{UTeamID: 3},
			game: `{"UGameID":8001,"UTeam1":3,"home_team":{"UTeamID":3,"team_name":"New Home","Season":169}}`,
		},
	} {
		var game lps.TeamScheduleGame
		if err := json.Unmarshal([]byte(response.game), &game); err != nil {
			t.Fatalf("decode LPS game %d: %v", i, err)
		}
		snapshot := Snapshot{TeamID: response.team.UTeamID, Team: response.team, Games: []lps.TeamScheduleGame{game}, FetchedAt: firstFetch.Add(time.Duration(i) * time.Hour)}
		if err := store.SaveTeamSnapshot(context.Background(), &snapshot); err != nil {
			t.Fatalf("SaveTeamSnapshot %d: %v", i, err)
		}
	}

	history, err := store.ReadTeamSeason(context.Background(), 3, 169)
	if err != nil {
		t.Fatalf("ReadTeamSeason: %v", err)
	}
	wantHome := lps.TeamSummary{UTeamID: 3, TeamName: "New Home", Season: 169}
	if len(history.Games) != 1 || history.Games[0].HomeTeam != wantHome || history.Games[0].VisitorTeam.TeamName != "Away FC" {
		t.Fatalf("reassigned game = %#v, want home side %#v with no facts from team 1", history.Games, wantHome)
	}
	if history.Team != wantHome {
		t.Fatalf("team 3 season context = %#v, want %#v", history.Team, wantHome)
	}
}

func TestDynamoArchiveReadsSeasonSpecificTeamAndFacilityContext(t *testing.T) {
	store := NewDynamoStoreWithAPI(archivetest.NewTable(), "durable-soccer-history")
	first := Snapshot{
		TeamID: 479691,
		Team: lps.TeamSummary{
			UTeamID: 479691, TeamName: "Old FC", DivisionName: "Open A", Season: 169, FacilityID: 5,
		},
		Facilities: []lps.FacilityResponse{{FacilityID: 5, FacilityName: "Old Field", Address: "1 Old St"}},
		FetchedAt:  time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &first); err != nil {
		t.Fatalf("old season SaveTeamSnapshot: %v", err)
	}
	second := Snapshot{
		TeamID: 479691,
		Team: lps.TeamSummary{
			UTeamID: 479691, TeamName: "New FC", DivisionName: "Open B", Season: 170, FacilityID: 6,
		},
		Facilities: []lps.FacilityResponse{{FacilityID: 6, FacilityName: "New Field", Address: "2 New St"}},
		FetchedAt:  first.FetchedAt.Add(time.Hour),
	}
	if err := store.SaveTeamSnapshot(context.Background(), &second); err != nil {
		t.Fatalf("new season SaveTeamSnapshot: %v", err)
	}
	oldHistory, err := store.ReadTeamSeason(context.Background(), 479691, 169)
	if err != nil {
		t.Fatalf("old ReadTeamSeason: %v", err)
	}
	if oldHistory.Team.TeamName != "Old FC" || oldHistory.Team.DivisionName != "Open A" || oldHistory.Team.FacilityID != 5 || len(oldHistory.Facilities) != 1 || oldHistory.Facilities[0].Address != "1 Old St" {
		t.Fatalf("old season context was replaced: %#v", oldHistory)
	}
	newHistory, err := store.ReadTeamSeason(context.Background(), 479691, 170)
	if err != nil {
		t.Fatalf("new ReadTeamSeason: %v", err)
	}
	if newHistory.Team.TeamName != "New FC" || newHistory.Team.DivisionName != "Open B" || newHistory.Team.FacilityID != 6 {
		t.Fatalf("new season context missing: %#v", newHistory)
	}
}

func assertArchiveItem(t *testing.T, backend *archivetest.Table, key string, expected map[string]any) {
	t.Helper()
	item := backend.Item(key)
	if item == nil {
		t.Fatalf("archive item %s missing", key)
	}
	var got map[string]any
	if err := attributevalue.UnmarshalMap(item, &got); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	for field, want := range expected {
		if fmt.Sprint(got[field]) != fmt.Sprint(want) {
			t.Errorf("%s %s = %#v, want %#v", key, field, got[field], want)
		}
	}
	if _, found := got["ttl"]; found {
		t.Errorf("%s unexpectedly has a session TTL", key)
	}
}
