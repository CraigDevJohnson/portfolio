package soccerarchive

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamotypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/types"
)

// racingTable is an archive table on which another writer acts at a chosen
// moment: after the given read of an item returns, or by canceling the next
// enrollment transactions as DynamoDB does when transactions contend.
type racingTable struct {
	*archivetest.Table
	reads map[string]int
	// afterRead runs once a read of "pk/sk" has returned, with its count.
	afterRead func(key string, read int)
	// cancellations are the reason codes of transactions to cancel, in turn.
	cancellations [][]string
}

func newRacingTable() *racingTable {
	return &racingTable{Table: archivetest.NewTable(), reads: map[string]int{}}
}

func (table *racingTable) GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	output, err := table.Table.GetItem(ctx, input, optFns...)
	key := stringKey(input.Key["pk"]) + "/" + stringKey(input.Key["sk"])
	table.reads[key]++
	if table.afterRead != nil {
		table.afterRead(key, table.reads[key])
	}
	return output, err
}

func (table *racingTable) TransactWriteItems(ctx context.Context, input *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if len(table.cancellations) > 0 {
		codes := table.cancellations[0]
		table.cancellations = table.cancellations[1:]
		reasons := make([]dynamotypes.CancellationReason, len(codes))
		for i, code := range codes {
			reasons[i].Code = aws.String(code)
		}
		return nil, &dynamotypes.TransactionCanceledException{Message: aws.String("Transaction canceled"), CancellationReasons: reasons}
	}
	return table.Table.TransactWriteItems(ctx, input, optFns...)
}

func stringKey(value dynamotypes.AttributeValue) string {
	if s, ok := value.(*dynamotypes.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}

// oneSlot admits a single anonymous team and reserves none for players.
var oneSlot = Limits{MaxEnrolledTeams: 1, ReservedPlayerSlots: 0, MaxRequestsPerRun: 1, MinRequestInterval: time.Second}

func TestEnrollmentThatLosesTheLastSlotToTheSameTeamMergesIntoIt(t *testing.T) {
	table := newRacingTable()
	store, err := NewDynamoStoreWithAPI(table, "durable-soccer-history", oneSlot)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewDynamoStoreWithAPI(table.Table, "durable-soccer-history", oneSlot)
	if err != nil {
		t.Fatal(err)
	}
	fetchedAt := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	snapshot := func(at time.Time) *Snapshot {
		return &Snapshot{
			TeamID: 5, Team: lps.TeamSummary{UTeamID: 5, Season: 169}, FetchedAt: at,
			Games: []lps.TeamScheduleGame{{UGameID: 9005, UTeam1: 5, UTeam2: 6, Season: 169, Result: "1-0"}},
		}
	}
	// Between this save's last read of team 5 and its enrollment, another
	// lookup of team 5 takes the only slot.
	table.afterRead = func(key string, read int) {
		if key == "TEAM#5/META" && read == 2 {
			if err := other.SaveTeamSnapshot(t.Context(), snapshot(fetchedAt.Add(-time.Minute))); err != nil {
				t.Fatalf("concurrent enrollment of team 5: %v", err)
			}
		}
	}

	if err := store.SaveTeamSnapshot(t.Context(), snapshot(fetchedAt)); err != nil {
		t.Fatalf("a team another writer enrolled was refused as over capacity: %v", err)
	}
	if history, err := store.ReadTeamSeason(t.Context(), 5, 169); err != nil || len(history.Games) != 1 || !history.Coverage.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("team 5 history = %#v, err %v", history, err)
	}
	// Team 5 holds the one slot once, so another team is still refused.
	table.afterRead = nil
	if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: 6, Team: lps.TeamSummary{UTeamID: 6}, FetchedAt: fetchedAt}); !errors.Is(err, ErrAdmissionFull) {
		t.Fatalf("second team = %v, want capacity refusal", err)
	}
}

func TestEnrollmentRetriesATransactionConflict(t *testing.T) {
	for _, reasons := range [][]string{
		{"None", "TransactionConflict"},
		{"TransactionConflict", "None"},
	} {
		t.Run(reasons[0]+"/"+reasons[1], func(t *testing.T) {
			table := newRacingTable()
			table.cancellations = [][]string{reasons}
			store, err := NewDynamoStoreWithAPI(table, "durable-soccer-history", oneSlot)
			if err != nil {
				t.Fatal(err)
			}

			if err := store.SaveTeamSnapshot(t.Context(), &Snapshot{TeamID: 5, Team: lps.TeamSummary{UTeamID: 5}, FetchedAt: time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)}); err != nil {
				t.Fatalf("a contended enrollment was not retried: %v", err)
			}
			if _, err := store.ReadRefreshState(t.Context(), 5); err != nil {
				t.Fatalf("team 5 was not enrolled: %v", err)
			}
		})
	}
}

func TestPlayerDiscoveryThatLosesTheLastSlotKeepsEvidenceOnlyForAdmittedTeams(t *testing.T) {
	table := newRacingTable()
	limits := Limits{MaxEnrolledTeams: 2, ReservedPlayerSlots: 1, MaxRequestsPerRun: 2, MinRequestInterval: time.Second}
	store, err := NewDynamoStoreWithAPI(table, "durable-soccer-history", limits)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewDynamoStoreWithAPI(table.Table, "durable-soccer-history", limits)
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC)
	discovery := func(owner string, playerID int, teamIDs ...int) *PlayerDiscovery {
		found := &PlayerDiscovery{OwnerIssuer: "https://issuer.example.com/pool", OwnerSubject: owner, ObservedAt: observedAt, Players: []types.LPSPlayer{{UPlayerID: playerID}}}
		for _, teamID := range teamIDs {
			team := lps.TeamSummary{UTeamID: teamID, Season: 169}
			found.KnownTeams = append(found.KnownTeams, team)
			found.Memberships = append(found.Memberships, PlayerMembership{PlayerID: playerID, Team: team})
		}
		return found
	}
	// After this import enrolls team 301, another import takes the last slot
	// before team 302's enrollment.
	table.afterRead = func(key string, read int) {
		if key == "TEAM#302/META" && read == 1 {
			if err := other.SavePlayerDiscovery(t.Context(), discovery("other-owner", 2002, 999)); err != nil {
				t.Fatalf("concurrent import: %v", err)
			}
		}
	}

	err = store.SavePlayerDiscovery(t.Context(), discovery("owner", 1001, 301, 302))

	var refused *AdmissionError
	if !errors.As(err, &refused) || !errors.Is(err, ErrAdmissionFull) || !slices.Equal(refused.TeamIDs, []int{302}) || refused.Source != "player" || refused.Limit != 2 {
		t.Fatalf("import that lost the last slot = %v, want team 302 refused", err)
	}
	for teamID, want := range map[int]bool{301: true, 302: false, 999: true} {
		if _, err := store.ReadRefreshState(t.Context(), teamID); (err == nil) != want {
			t.Errorf("team %d enrolled = %v, want %v", teamID, err == nil, want)
		}
	}
	items, err := table.Items()
	if err != nil {
		t.Fatal(err)
	}
	memberships := []int{}
	for _, item := range items {
		if item["kind"] == "membership" && item["owner_subject"] == "owner" {
			memberships = append(memberships, int(item["team_id"].(float64)))
		}
	}
	if !slices.Equal(memberships, []int{301}) || items["PLAYER#1001/META"] == nil {
		t.Fatalf("owner memberships = %v, player %v; want only team 301 and the player's identity", memberships, items["PLAYER#1001/META"])
	}
}
