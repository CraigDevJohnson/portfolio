package soccerarchive

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/internal/lps"
)

type inMemoryDynamo struct {
	items map[string]map[string]types.AttributeValue
}

func (d *inMemoryDynamo) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if d.items == nil {
		d.items = make(map[string]map[string]types.AttributeValue)
	}
	var key struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
	}
	if err := attributevalue.UnmarshalMap(input.Item, &key); err != nil {
		return nil, err
	}
	if input.ConditionExpression == nil || *input.ConditionExpression == "" {
		return nil, fmt.Errorf("archive writes must protect newer source facts")
	}
	if previous := d.items[key.PK+"/"+key.SK]; previous != nil {
		previousTime := previous["fetched_at"].(*types.AttributeValueMemberS).Value
		newTime := input.Item["fetched_at"].(*types.AttributeValueMemberS).Value
		if previousTime > newTime {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	d.items[key.PK+"/"+key.SK] = input.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (d *inMemoryDynamo) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	var key struct {
		PK string `dynamodbav:"pk"`
		SK string `dynamodbav:"sk"`
	}
	if err := attributevalue.UnmarshalMap(input.Key, &key); err != nil {
		return nil, err
	}
	return &dynamodb.GetItemOutput{Item: d.items[key.PK+"/"+key.SK]}, nil
}

func (d *inMemoryDynamo) Query(_ context.Context, input *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
	prefix := input.ExpressionAttributeValues[":prefix"].(*types.AttributeValueMemberS).Value
	keys := make([]string, 0)
	for key := range d.items {
		if strings.HasPrefix(key, pk+"/"+prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	items := make([]map[string]types.AttributeValue, 0, len(keys))
	for _, key := range keys {
		items = append(items, d.items[key])
	}
	return &dynamodb.QueryOutput{Items: items}, nil
}

func TestDynamoArchiveStoreKeepsOneGameAcrossRepeatedAndPartialLookups(t *testing.T) {
	backend := &inMemoryDynamo{}
	store := newDynamoStore(backend, "durable-soccer-history")
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
	firstCount := len(backend.items)
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
	if _, ok := backend.items["TEAM#479691/SEASON#0000000169#GAME#8001"]; !ok {
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
	if len(backend.items) != firstCount {
		t.Fatalf("item count after repeated and empty lookups = %d, want %d", len(backend.items), firstCount)
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
	backend := &inMemoryDynamo{}
	store := newDynamoStore(backend, "durable-soccer-history")
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
	store := newDynamoStore(&inMemoryDynamo{}, "durable-soccer-history")
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

func assertArchiveItem(t *testing.T, backend *inMemoryDynamo, key string, expected map[string]any) {
	t.Helper()
	item, ok := backend.items[key]
	if !ok {
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
