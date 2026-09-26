package soccerarchive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/internal/lps"
)

const sortableUTCFormat = "2006-01-02T15:04:05.000000000Z"

type dynamoAPI interface {
	PutItem(ctx context.Context, input *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// DynamoStore stores source facts in a dedicated, non-TTL DynamoDB table.
type DynamoStore struct {
	api       dynamoAPI
	tableName string
}

// NewDynamoStore prepares the durable archive adapter. Calling it does not
// activate collection or create an AWS resource.
func NewDynamoStore(ctx context.Context, tableName string) (*DynamoStore, error) {
	if strings.TrimSpace(tableName) == "" {
		return nil, errors.New("soccer archive table name is required")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return newDynamoStore(dynamodb.NewFromConfig(cfg), tableName), nil
}

func newDynamoStore(api dynamoAPI, tableName string) *DynamoStore {
	return &DynamoStore{api: api, tableName: tableName}
}

type archiveItem struct {
	PK                string `dynamodbav:"pk"`
	SK                string `dynamodbav:"sk"`
	Kind              string `dynamodbav:"kind"`
	TeamID            int    `dynamodbav:"team_id,omitempty"`
	TeamName          string `dynamodbav:"team_name,omitempty"`
	DivisionName      string `dynamodbav:"division_name,omitempty"`
	SeasonID          int    `dynamodbav:"season_id,omitempty"`
	GameID            int    `dynamodbav:"game_id,omitempty"`
	HomeTeamID        int    `dynamodbav:"home_team_id,omitempty"`
	AwayTeamID        int    `dynamodbav:"away_team_id,omitempty"`
	HomeTeamName      string `dynamodbav:"home_team_name,omitempty"`
	AwayTeamName      string `dynamodbav:"away_team_name,omitempty"`
	FacilityID        int    `dynamodbav:"facility_id,omitempty"`
	FacilityName      string `dynamodbav:"facility_name,omitempty"`
	Address           string `dynamodbav:"address,omitempty"`
	City              string `dynamodbav:"city,omitempty"`
	State             string `dynamodbav:"state,omitempty"`
	ZIP               string `dynamodbav:"zip,omitempty"`
	ScheduledAt       string `dynamodbav:"scheduled_at,omitempty"`
	ScheduledEndAt    string `dynamodbav:"scheduled_end_at,omitempty"`
	FieldID           int    `dynamodbav:"field_id,omitempty"`
	FieldName         string `dynamodbav:"field_name,omitempty"`
	Result            string `dynamodbav:"result,omitempty"`
	EnrollmentSource  string `dynamodbav:"enrollment_source,omitempty"`
	Status            string `dynamodbav:"status,omitempty"`
	ReturnedGameCount int    `dynamodbav:"returned_game_count"`
	SeasonIDs         []int  `dynamodbav:"season_ids,omitempty"`
	RawSourceJSON     string `dynamodbav:"raw_source_json,omitempty"`
	FetchedAt         string `dynamodbav:"fetched_at"`
	DuePK             string `dynamodbav:"due_pk,omitempty"`
	DueSK             string `dynamodbav:"due_sk,omitempty"`
}

// SaveTeamSnapshot upserts stable source IDs. Coverage is written last: a
// failed mid-response write never reports that response as successfully stored.
func (s *DynamoStore) SaveTeamSnapshot(ctx context.Context, snapshot *Snapshot) error {
	if snapshot == nil || snapshot.TeamID <= 0 || snapshot.Team.UTeamID != snapshot.TeamID || snapshot.FetchedAt.IsZero() {
		return errors.New("archive snapshot requires a confirmed team ID and fetch time")
	}
	for i := range snapshot.Games {
		game := &snapshot.Games[i]
		if game.UGameID <= 0 {
			return fmt.Errorf("team %d response contains a game without a stable ID", snapshot.TeamID)
		}
	}

	fetchedAt := snapshot.FetchedAt.UTC().Format(sortableUTCFormat)
	teamJSON, err := json.Marshal(snapshot.Team)
	if err != nil {
		return fmt.Errorf("marshal team %d: %w", snapshot.TeamID, err)
	}
	teamItem := archiveItem{
		PK:               teamKey(snapshot.TeamID),
		SK:               "META",
		Kind:             "team",
		TeamID:           snapshot.TeamID,
		TeamName:         snapshot.Team.TeamName,
		DivisionName:     snapshot.Team.DivisionName,
		SeasonID:         snapshot.Team.Season,
		FacilityID:       snapshot.Team.FacilityID,
		FacilityName:     snapshot.Team.FacilityName,
		EnrollmentSource: "manual",
		RawSourceJSON:    string(teamJSON),
		FetchedAt:        fetchedAt,
		DuePK:            "TEAM_DUE",
		DueSK:            snapshot.FetchedAt.UTC().Add(24*time.Hour).Format(sortableUTCFormat) + "#" + strconv.Itoa(snapshot.TeamID),
	}
	if err := s.put(ctx, &teamItem); err != nil {
		return fmt.Errorf("save team %d: %w", snapshot.TeamID, err)
	}

	facilities := append([]lps.FacilityResponse(nil), snapshot.Facilities...)
	sort.Slice(facilities, func(i, j int) bool { return facilities[i].FacilityID < facilities[j].FacilityID })
	for _, facility := range facilities {
		if facility.FacilityID <= 0 {
			continue
		}
		facilityJSON, err := json.Marshal(facility)
		if err != nil {
			return fmt.Errorf("marshal facility %d: %w", facility.FacilityID, err)
		}
		if err := s.put(ctx, &archiveItem{
			PK:            "FACILITY#" + strconv.Itoa(facility.FacilityID),
			SK:            "META",
			Kind:          "facility",
			FacilityID:    facility.FacilityID,
			FacilityName:  facility.FacilityName,
			Address:       facility.Address,
			City:          facility.City,
			State:         facility.State,
			ZIP:           facility.ZIP,
			RawSourceJSON: string(facilityJSON),
			FetchedAt:     fetchedAt,
		}); err != nil {
			return fmt.Errorf("save facility %d: %w", facility.FacilityID, err)
		}
	}

	games := append([]lps.TeamScheduleGame(nil), snapshot.Games...)
	sort.Slice(games, func(i, j int) bool { return games[i].UGameID < games[j].UGameID })
	seasonCounts := map[int]int{}
	teamGameEdges := map[string]archiveItem{}
	for i := range games {
		game := &games[i]
		seasonID := firstPositive(game.Season, snapshot.Team.Season, game.HomeTeam.Season, game.VisitorTeam.Season)
		seasonCounts[seasonID]++
		gameJSON, err := json.Marshal(game)
		if err != nil {
			return fmt.Errorf("marshal game %d: %w", game.UGameID, err)
		}
		endAt := ""
		if game.SchedGameEndTime != nil {
			endAt = *game.SchedGameEndTime
		}
		if err := s.put(ctx, &archiveItem{
			PK:             "GAME#" + strconv.Itoa(game.UGameID),
			SK:             "META",
			Kind:           "game",
			GameID:         game.UGameID,
			SeasonID:       seasonID,
			HomeTeamID:     firstPositive(game.UTeam1, game.HomeTeam.UTeamID),
			AwayTeamID:     firstPositive(game.UTeam2, game.VisitorTeam.UTeamID),
			HomeTeamName:   game.HomeTeam.TeamName,
			AwayTeamName:   game.VisitorTeam.TeamName,
			FacilityID:     game.FacilityID,
			FacilityName:   game.FacilityName,
			ScheduledAt:    game.SchedGameDateTime,
			ScheduledEndAt: endAt,
			FieldID:        game.Field,
			FieldName:      game.FieldName,
			Result:         game.Result,
			RawSourceJSON:  string(gameJSON),
			FetchedAt:      fetchedAt,
		}); err != nil {
			return fmt.Errorf("save game %d: %w", game.UGameID, err)
		}
		for _, teamID := range []int{snapshot.TeamID, game.UTeam1, game.UTeam2, game.HomeTeam.UTeamID, game.VisitorTeam.UTeamID} {
			if teamID <= 0 || seasonID <= 0 {
				continue
			}
			edge := archiveItem{
				PK:        teamKey(teamID),
				SK:        seasonGameKey(seasonID, game.UGameID),
				Kind:      "team_game",
				TeamID:    teamID,
				SeasonID:  seasonID,
				GameID:    game.UGameID,
				FetchedAt: fetchedAt,
			}
			teamGameEdges[edge.PK+"/"+edge.SK] = edge
		}
	}
	edgeKeys := make([]string, 0, len(teamGameEdges))
	for key := range teamGameEdges {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Strings(edgeKeys)
	for _, key := range edgeKeys {
		edge := teamGameEdges[key]
		if err := s.put(ctx, &edge); err != nil {
			return fmt.Errorf("save team-game edge %s: %w", key, err)
		}
	}

	if snapshot.Team.Season > 0 {
		seasonCounts[snapshot.Team.Season] += 0
	}
	seasonIDs := make([]int, 0, len(seasonCounts))
	for seasonID := range seasonCounts {
		if seasonID > 0 {
			seasonIDs = append(seasonIDs, seasonID)
		}
	}
	sort.Ints(seasonIDs)
	for _, seasonID := range seasonIDs {
		if err := s.put(ctx, &archiveItem{
			PK:                teamKey(snapshot.TeamID),
			SK:                seasonCoverageKey(seasonID),
			Kind:              "coverage",
			TeamID:            snapshot.TeamID,
			SeasonID:          seasonID,
			Status:            "fetched",
			ReturnedGameCount: seasonCounts[seasonID],
			FetchedAt:         fetchedAt,
		}); err != nil {
			return fmt.Errorf("save season %d coverage: %w", seasonID, err)
		}
	}
	return s.put(ctx, &archiveItem{
		PK:                teamKey(snapshot.TeamID),
		SK:                "COVERAGE",
		Kind:              "coverage",
		TeamID:            snapshot.TeamID,
		Status:            "fetched",
		ReturnedGameCount: len(snapshot.Games),
		SeasonIDs:         seasonIDs,
		FetchedAt:         fetchedAt,
	})
}

// ReadTeamSeason returns archived source facts through the team-season index.
// Authorization is intentionally left to the private read contract in #80.
func (s *DynamoStore) ReadTeamSeason(ctx context.Context, teamID, seasonID int) (TeamSeason, error) {
	var history TeamSeason
	if teamID <= 0 || seasonID <= 0 {
		return history, errors.New("positive team and season IDs are required")
	}
	teamItem, err := s.get(ctx, teamKey(teamID), "META")
	if err != nil {
		return history, err
	}
	if teamItem == nil {
		return history, ErrNoArchive
	}
	if err := json.Unmarshal([]byte(teamItem.RawSourceJSON), &history.Team); err != nil {
		return history, fmt.Errorf("decode archived team %d: %w", teamID, err)
	}

	coverageItem, err := s.get(ctx, teamKey(teamID), seasonCoverageKey(seasonID))
	if err != nil {
		return history, err
	}
	history.Coverage.Status = "not_fetched"
	if coverageItem != nil {
		history.Coverage.Status = coverageItem.Status
		history.Coverage.ReturnedGameCount = coverageItem.ReturnedGameCount
		history.Coverage.FetchedAt, err = time.Parse(time.RFC3339Nano, coverageItem.FetchedAt)
		if err != nil {
			return history, fmt.Errorf("decode team %d season %d coverage time: %w", teamID, seasonID, err)
		}
	}

	prefix := fmt.Sprintf("SEASON#%010d#GAME#", seasonID)
	facilityIDs := make(map[int]struct{})
	if history.Team.FacilityID > 0 {
		facilityIDs[history.Team.FacilityID] = struct{}{}
	}
	var cursor map[string]types.AttributeValue
	for {
		page, err := s.api.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     &types.AttributeValueMemberS{Value: teamKey(teamID)},
				":prefix": &types.AttributeValueMemberS{Value: prefix},
			},
			ExclusiveStartKey: cursor,
			ConsistentRead:    aws.Bool(true),
		})
		if err != nil {
			return history, fmt.Errorf("query team %d season %d games: %w", teamID, seasonID, err)
		}
		for _, rawEdge := range page.Items {
			var edge archiveItem
			if err := attributevalue.UnmarshalMap(rawEdge, &edge); err != nil {
				return history, fmt.Errorf("decode team %d game edge: %w", teamID, err)
			}
			if edge.GameID <= 0 {
				continue
			}
			gameItem, err := s.get(ctx, "GAME#"+strconv.Itoa(edge.GameID), "META")
			if err != nil {
				return history, err
			}
			if gameItem == nil {
				return history, fmt.Errorf("archived game %d is missing", edge.GameID)
			}
			// An older lookup edge remains when LPS later reassigns a game.
			// The global game record is authoritative for its current season and
			// team IDs, while an omitted game retains its last known record.
			if gameItem.SeasonID != seasonID ||
				(gameItem.HomeTeamID > 0 && gameItem.AwayTeamID > 0 && gameItem.HomeTeamID != teamID && gameItem.AwayTeamID != teamID) {
				continue
			}
			var game lps.TeamScheduleGame
			if err := json.Unmarshal([]byte(gameItem.RawSourceJSON), &game); err != nil {
				return history, fmt.Errorf("decode archived game %d: %w", edge.GameID, err)
			}
			history.Games = append(history.Games, game)
			if game.FacilityID > 0 {
				facilityIDs[game.FacilityID] = struct{}{}
			}
		}
		if len(page.LastEvaluatedKey) == 0 {
			break
		}
		cursor = page.LastEvaluatedKey
	}

	ids := make([]int, 0, len(facilityIDs))
	for id := range facilityIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		facilityItem, err := s.get(ctx, "FACILITY#"+strconv.Itoa(id), "META")
		if err != nil {
			return history, err
		}
		if facilityItem == nil {
			continue
		}
		var facility lps.FacilityResponse
		if err := json.Unmarshal([]byte(facilityItem.RawSourceJSON), &facility); err != nil {
			return history, fmt.Errorf("decode archived facility %d: %w", id, err)
		}
		history.Facilities = append(history.Facilities, facility)
	}
	return history, nil
}

func (s *DynamoStore) put(ctx context.Context, record *archiveItem) error {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return err
	}
	_, err = s.api.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(fetched_at) OR fetched_at <= :fetched_at"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":fetched_at": &types.AttributeValueMemberS{Value: record.FetchedAt},
		},
	})
	var stale *types.ConditionalCheckFailedException
	if errors.As(err, &stale) {
		return nil
	}
	return err
}

func (s *DynamoStore) get(ctx context.Context, pk, sk string) (*archiveItem, error) {
	item, err := s.api.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: pk},
			"sk": &types.AttributeValueMemberS{Value: sk},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("read %s/%s: %w", pk, sk, err)
	}
	if len(item.Item) == 0 {
		return nil, nil
	}
	var record archiveItem
	if err := attributevalue.UnmarshalMap(item.Item, &record); err != nil {
		return nil, fmt.Errorf("decode %s/%s: %w", pk, sk, err)
	}
	return &record, nil
}

func teamKey(teamID int) string { return "TEAM#" + strconv.Itoa(teamID) }

func seasonGameKey(seasonID, gameID int) string {
	return fmt.Sprintf("SEASON#%010d#GAME#%d", seasonID, gameID)
}

func seasonCoverageKey(seasonID int) string {
	return fmt.Sprintf("SEASON#%010d#COVERAGE", seasonID)
}

func firstPositive(ids ...int) int {
	for _, id := range ids {
		if id > 0 {
			return id
		}
	}
	return 0
}
