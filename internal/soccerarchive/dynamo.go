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

// DynamoAPI is the DynamoDB client subset the archive adapter uses.
type DynamoAPI interface {
	PutItem(ctx context.Context, input *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	DeleteItem(ctx context.Context, input *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
}

// DynamoStore stores source facts in a dedicated, non-TTL DynamoDB table.
type DynamoStore struct {
	api       DynamoAPI
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
	return NewDynamoStoreWithAPI(dynamodb.NewFromConfig(cfg), tableName), nil
}

// NewDynamoStoreWithAPI prepares the adapter over an existing DynamoDB client.
func NewDynamoStoreWithAPI(api DynamoAPI, tableName string) *DynamoStore {
	return &DynamoStore{api: api, tableName: tableName}
}

type archiveItem struct {
	PK                string         `dynamodbav:"pk"`
	SK                string         `dynamodbav:"sk"`
	Kind              string         `dynamodbav:"kind"`
	PlayerID          int            `dynamodbav:"player_id,omitempty"`
	FirstName         string         `dynamodbav:"first_name,omitempty"`
	LastName          string         `dynamodbav:"last_name,omitempty"`
	IsMainPlayer      *bool          `dynamodbav:"is_main_player,omitempty"`
	OwnerIssuer       string         `dynamodbav:"owner_issuer,omitempty"`
	OwnerSubject      string         `dynamodbav:"owner_subject,omitempty"`
	Source            string         `dynamodbav:"source,omitempty"`
	ObservedAt        string         `dynamodbav:"observed_at,omitempty"`
	TeamID            int            `dynamodbav:"team_id,omitempty"`
	TeamName          string         `dynamodbav:"team_name,omitempty"`
	DivisionName      string         `dynamodbav:"division_name,omitempty"`
	SeasonID          int            `dynamodbav:"season_id,omitempty"`
	GameID            int            `dynamodbav:"game_id,omitempty"`
	HomeTeamID        int            `dynamodbav:"home_team_id,omitempty"`
	AwayTeamID        int            `dynamodbav:"away_team_id,omitempty"`
	HomeTeamName      string         `dynamodbav:"home_team_name,omitempty"`
	AwayTeamName      string         `dynamodbav:"away_team_name,omitempty"`
	FacilityID        int            `dynamodbav:"facility_id,omitempty"`
	FacilityName      string         `dynamodbav:"facility_name,omitempty"`
	Address           string         `dynamodbav:"address,omitempty"`
	City              string         `dynamodbav:"city,omitempty"`
	State             string         `dynamodbav:"state,omitempty"`
	ZIP               string         `dynamodbav:"zip,omitempty"`
	ScheduledAt       string         `dynamodbav:"scheduled_at,omitempty"`
	ScheduledEndAt    string         `dynamodbav:"scheduled_end_at,omitempty"`
	FieldID           int            `dynamodbav:"field_id,omitempty"`
	FieldName         string         `dynamodbav:"field_name,omitempty"`
	Result            string         `dynamodbav:"result,omitempty"`
	EnrollmentSource  string         `dynamodbav:"enrollment_source,omitempty"`
	Status            CoverageStatus `dynamodbav:"status,omitempty"`
	ReturnedGameCount int            `dynamodbav:"returned_game_count"`
	SeasonIDs         []int          `dynamodbav:"season_ids,omitempty"`
	RawSourceJSON     string         `dynamodbav:"raw_source_json,omitempty"`
	FetchedAt         string         `dynamodbav:"fetched_at"`
	DuePK             string         `dynamodbav:"due_pk,omitempty"`
	DueSK             string         `dynamodbav:"due_sk,omitempty"`
	// Revision counts writes to a game or team enrollment record for
	// conditional merges.
	Revision int `dynamodbav:"revision,omitempty"`
	// The team enrollment record also carries its latest refresh attempt.
	RefreshStatus     RefreshStatus `dynamodbav:"refresh_status,omitempty"`
	AttemptedAt       string        `dynamodbav:"attempted_at,omitempty"`
	FailureKind       lps.ErrorKind `dynamodbav:"failure_kind,omitempty"`
	FailureStatusCode int           `dynamodbav:"failure_status_code,omitempty"`
}

// SaveTeamSnapshot upserts stable source IDs. Coverage and then the team's
// enrollment record, which carries the due-team marker, are written last: a
// failed mid-response write never reports that response as stored and never
// enrolls a team whose snapshot is incomplete.
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
	if err := s.saveFacilities(ctx, snapshot.Facilities, fetchedAt); err != nil {
		return err
	}

	games, err := uniqueSourceGames(snapshot.Games)
	if err != nil {
		return err
	}
	seasonCounts := map[int]int{}
	teamGameEdges := map[string]archiveItem{}
	for i := range games {
		game, seasonID, err := s.saveGame(ctx, &games[i], snapshot.Team.Season, fetchedAt)
		if err != nil {
			return err
		}
		seasonCounts[seasonID]++
		games[i] = game
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
		seasonTeam := teamSummaryForSeason(snapshot.TeamID, &snapshot.Team, games, seasonID)
		if err := s.saveSeasonTeamContext(ctx, snapshot.TeamID, seasonID, &seasonTeam, fetchedAt); err != nil {
			return err
		}
		if err := s.put(ctx, &archiveItem{
			PK:                teamKey(snapshot.TeamID),
			SK:                seasonCoverageKey(seasonID),
			Kind:              "coverage",
			TeamID:            snapshot.TeamID,
			SeasonID:          seasonID,
			Status:            CoverageFetched,
			ReturnedGameCount: seasonCounts[seasonID],
			FetchedAt:         fetchedAt,
		}); err != nil {
			return fmt.Errorf("save season %d coverage: %w", seasonID, err)
		}
	}
	if err := s.put(ctx, &archiveItem{
		PK:                teamKey(snapshot.TeamID),
		SK:                "COVERAGE",
		Kind:              "coverage",
		TeamID:            snapshot.TeamID,
		Status:            CoverageFetched,
		ReturnedGameCount: len(games),
		SeasonIDs:         seasonIDs,
		FetchedAt:         fetchedAt,
	}); err != nil {
		return fmt.Errorf("save team %d coverage: %w", snapshot.TeamID, err)
	}
	return s.saveTeam(ctx, snapshot, fetchedAt)
}

// uniqueSourceGames merges repeated entries for one stable game ID within a
// response, the later entry winning each field it states, and orders the
// result by game ID.
func uniqueSourceGames(source []lps.TeamScheduleGame) ([]lps.TeamScheduleGame, error) {
	byID := make(map[int][]byte, len(source))
	for i := range source {
		game := &source[i]
		payload := game.SourceJSON
		if len(payload) == 0 {
			var err error
			if payload, err = json.Marshal(game); err != nil {
				return nil, fmt.Errorf("marshal game %d: %w", game.UGameID, err)
			}
		}
		if previous, exists := byID[game.UGameID]; exists {
			var err error
			if payload, err = mergeSourceObject(previous, payload); err != nil {
				return nil, fmt.Errorf("merge duplicate game %d: %w", game.UGameID, err)
			}
		}
		byID[game.UGameID] = payload
	}
	ids := make([]int, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	games := make([]lps.TeamScheduleGame, 0, len(ids))
	for _, id := range ids {
		var game lps.TeamScheduleGame
		if err := json.Unmarshal(byID[id], &game); err != nil {
			return nil, fmt.Errorf("decode game %d: %w", id, err)
		}
		games = append(games, game)
	}
	return games, nil
}

// ReadRefreshState returns an enrolled team's latest refresh attempt. A saved
// snapshot or an authenticated player import enrolls a team, so any other
// team is ErrNotEnrolled. A team a player import enrolled has no attempt
// until its first refresh.
func (s *DynamoStore) ReadRefreshState(ctx context.Context, teamID int) (RefreshState, error) {
	if teamID <= 0 {
		return RefreshState{}, ErrNotEnrolled
	}
	team, err := s.get(ctx, teamKey(teamID), "META")
	if err != nil {
		return RefreshState{}, err
	}
	if team == nil {
		return RefreshState{}, ErrNotEnrolled
	}
	state := RefreshState{
		TeamID:              teamID,
		Status:              team.RefreshStatus,
		LastErrorKind:       team.FailureKind,
		LastErrorStatusCode: team.FailureStatusCode,
	}
	if team.AttemptedAt != "" {
		if state.LastAttemptAt, err = time.Parse(sortableUTCFormat, team.AttemptedAt); err != nil {
			return RefreshState{}, fmt.Errorf("decode team %d refresh attempt time: %w", teamID, err)
		}
	}
	if team.DueSK != "" {
		if state.NextDueAt, err = dueTime(team.DueSK); err != nil {
			return RefreshState{}, fmt.Errorf("decode team %d next due time: %w", teamID, err)
		}
	}
	return state, nil
}

// RecordRefreshFailure marks one enrolled team invalid or retryable without
// touching its last successful team, game, facility, or coverage facts. An
// invalid team leaves the due-team index; a retryable one stays due.
func (s *DynamoStore) RecordRefreshFailure(ctx context.Context, failure *RefreshFailure) error {
	if failure == nil {
		return errors.New("refresh failure is required")
	}
	if failure.TeamID <= 0 || failure.AttemptedAt.IsZero() ||
		(failure.Status != RefreshInvalid && failure.Status != RefreshRetryable) ||
		(failure.Status == RefreshRetryable && !failure.NextDueAt.After(failure.AttemptedAt)) ||
		(failure.Status == RefreshInvalid && !failure.NextDueAt.IsZero()) {
		return errors.New("refresh failure requires an enrolled team, attempt time, and valid retry state")
	}
	attemptedAt := failure.AttemptedAt.UTC().Format(sortableUTCFormat)
	for range maxRecordWriteAttempts {
		previous, err := s.get(ctx, teamKey(failure.TeamID), "META")
		if err != nil {
			return err
		}
		if previous == nil {
			return ErrNotEnrolled
		}
		if previous.AttemptedAt >= attemptedAt {
			// A later attempt already recorded the team's refresh state.
			return nil
		}
		record := *previous
		record.Revision = previous.Revision + 1
		record.RefreshStatus = failure.Status
		record.AttemptedAt = attemptedAt
		record.FailureKind = failure.ErrorKind
		record.FailureStatusCode = failure.HTTPStatusCode
		record.DuePK, record.DueSK = "", ""
		if failure.Status == RefreshRetryable {
			record.DuePK, record.DueSK = dueTeamsPK, dueKey(failure.NextDueAt, failure.TeamID)
		}
		written, err := s.putIfUnchanged(ctx, &record, previous)
		if err != nil {
			return fmt.Errorf("record team %d refresh failure: %w", failure.TeamID, err)
		}
		if written {
			return nil
		}
	}
	return fmt.Errorf("record team %d refresh failure: changed by concurrent writes %d times", failure.TeamID, maxRecordWriteAttempts)
}

// mergeSourceObject lays an incoming LPS game over the stored one. Both
// describe the same game, keyed by its stable ID, so an omitted field keeps
// its stored value.
func mergeSourceObject(previous, incoming []byte) ([]byte, error) {
	oldFields, newFields := sourceObject(previous), sourceObject(incoming)
	if oldFields == nil || newFields == nil {
		return nil, errors.New("game source must be a JSON object")
	}
	merged, err := mergeSourceFields(oldFields, newFields)
	if err != nil {
		return nil, err
	}
	return json.Marshal(merged)
}

func mergeSourceFields(oldFields, newFields map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	for key, oldValue := range oldFields {
		// Only an omitted field keeps its old value; an explicit null is an
		// LPS correction and replaces it.
		newValue, present := newFields[key]
		if !present {
			newFields[key] = oldValue
			continue
		}
		// A nested object keeps omitted fields only when both versions name
		// the same LPS entity; a side reassigned to another team replaces the
		// old team whole instead of inheriting its facts.
		oldObject, newObject := sourceObject(oldValue), sourceObject(newValue)
		if oldObject == nil || newObject == nil || !sameSourceEntity(oldObject, newObject) {
			continue
		}
		mergedObject, err := mergeSourceFields(oldObject, newObject)
		if err != nil {
			return nil, err
		}
		if newFields[key], err = json.Marshal(mergedObject); err != nil {
			return nil, err
		}
	}
	return newFields, nil
}

// sourceIdentityKeys are the stable LPS IDs that say which entity a nested
// source object describes, most specific first.
var sourceIdentityKeys = []string{"UTeamID", "UGameID", "FacilityID"}

func sameSourceEntity(oldObject, newObject map[string]json.RawMessage) bool {
	for _, key := range sourceIdentityKeys {
		oldID, newID := sourceID(oldObject[key]), sourceID(newObject[key])
		if oldID == 0 && newID == 0 {
			continue
		}
		return oldID == newID
	}
	return false
}

// sourceObject decodes a JSON object, or returns nil for any other value.
func sourceObject(value []byte) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		return nil
	}
	return fields
}

func sourceID(value json.RawMessage) int64 {
	var id int64
	if err := json.Unmarshal(value, &id); err != nil || id < 0 {
		return 0
	}
	return id
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
	history.Team = lps.TeamSummary{UTeamID: teamID, Season: seasonID}
	seasonTeamItem, err := s.get(ctx, teamKey(teamID), seasonTeamKey(seasonID))
	if err != nil {
		return history, err
	}
	if seasonTeamItem != nil {
		if err := json.Unmarshal([]byte(seasonTeamItem.RawSourceJSON), &history.Team); err != nil {
			return history, fmt.Errorf("decode team %d season %d context: %w", teamID, seasonID, err)
		}
	}

	coverageItem, err := s.get(ctx, teamKey(teamID), seasonCoverageKey(seasonID))
	if err != nil {
		return history, err
	}
	history.Coverage.Status = CoverageNotFetched
	if coverageItem != nil {
		history.Coverage.Status = coverageItem.Status
		history.Coverage.ReturnedGameCount = coverageItem.ReturnedGameCount
		history.Coverage.FetchedAt, err = time.Parse(time.RFC3339Nano, coverageItem.FetchedAt)
		if err != nil {
			return history, fmt.Errorf("decode team %d season %d coverage time: %w", teamID, seasonID, err)
		}
	}

	prefix := fmt.Sprintf("SEASON#%010d#GAME#", seasonID)
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
		}
		if len(page.LastEvaluatedKey) == 0 {
			break
		}
		cursor = page.LastEvaluatedKey
	}
	history.Facilities, err = s.readFacilities(ctx, &history.Team, history.Games)
	if err != nil {
		return history, err
	}
	return history, nil
}

// maxRecordWriteAttempts bounds how often one game or team enrollment write
// re-reads and merges after concurrent writers change it first.
const maxRecordWriteAttempts = 5

// saveGame merges one returned game into its stable GAME#id record and returns
// the merged game with its season. The write is conditional on the revision it
// read, so a concurrent lookup of a shared game is re-read and merged rather
// than overwritten. The newest fetch wins each field it states; an older
// response only fills fields the stored newer one omits.
func (s *DynamoStore) saveGame(ctx context.Context, sourceGame *lps.TeamScheduleGame, responseSeason int, fetchedAt string) (lps.TeamScheduleGame, int, error) {
	incoming := sourceGame.SourceJSON
	if len(incoming) == 0 {
		var err error
		if incoming, err = json.Marshal(sourceGame); err != nil {
			return lps.TeamScheduleGame{}, 0, fmt.Errorf("marshal game %d: %w", sourceGame.UGameID, err)
		}
	}
	gameKey := "GAME#" + strconv.Itoa(sourceGame.UGameID)
	for range maxRecordWriteAttempts {
		previous, err := s.get(ctx, gameKey, "META")
		if err != nil {
			return lps.TeamScheduleGame{}, 0, err
		}
		gameJSON, itemFetchedAt, revision := incoming, fetchedAt, 1
		if previous != nil {
			revision = previous.Revision + 1
			if previous.FetchedAt > fetchedAt {
				gameJSON, err = mergeSourceObject(incoming, []byte(previous.RawSourceJSON))
				itemFetchedAt = previous.FetchedAt
			} else {
				gameJSON, err = mergeSourceObject([]byte(previous.RawSourceJSON), incoming)
			}
			if err != nil {
				return lps.TeamScheduleGame{}, 0, fmt.Errorf("merge game %d source fields: %w", sourceGame.UGameID, err)
			}
		}
		var game lps.TeamScheduleGame
		if err := json.Unmarshal(gameJSON, &game); err != nil {
			return lps.TeamScheduleGame{}, 0, fmt.Errorf("decode game %d source fields: %w", sourceGame.UGameID, err)
		}
		seasonID := game.SeasonID(responseSeason)
		endAt := ""
		if game.SchedGameEndTime != nil {
			endAt = *game.SchedGameEndTime
		}
		written, err := s.putIfUnchanged(ctx, &archiveItem{
			PK:             gameKey,
			SK:             "META",
			Kind:           "game",
			GameID:         game.UGameID,
			SeasonID:       seasonID,
			HomeTeamID:     game.HomeTeamID(),
			AwayTeamID:     game.AwayTeamID(),
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
			FetchedAt:      itemFetchedAt,
			Revision:       revision,
		}, previous)
		if err != nil {
			return lps.TeamScheduleGame{}, 0, fmt.Errorf("save game %d: %w", game.UGameID, err)
		}
		if written {
			return game, seasonID, nil
		}
	}
	return lps.TeamScheduleGame{}, 0, fmt.Errorf("save game %d: changed by concurrent lookups %d times", sourceGame.UGameID, maxRecordWriteAttempts)
}

// putIfUnchanged writes a game or team enrollment record only if it still has
// the revision that was read (or is still absent). It reports false when
// another writer changed the record first.
func (s *DynamoStore) putIfUnchanged(ctx context.Context, record, read *archiveItem) (bool, error) {
	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return false, err
	}
	input := &dynamodb.PutItemInput{
		TableName:           aws.String(s.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	}
	if read != nil {
		input.ConditionExpression = aws.String("#revision = :read_revision")
		input.ExpressionAttributeNames = map[string]string{"#revision": "revision"}
		input.ExpressionAttributeValues = map[string]types.AttributeValue{
			":read_revision": &types.AttributeValueMemberN{Value: strconv.Itoa(read.Revision)},
		}
	}
	_, err = s.api.PutItem(ctx, input)
	var changed *types.ConditionalCheckFailedException
	if errors.As(err, &changed) {
		return false, nil
	}
	return err == nil, err
}

func (s *DynamoStore) saveSeasonTeamContext(ctx context.Context, teamID, seasonID int, responseTeam *lps.TeamSummary, fetchedAt string) error {
	if responseTeam.UTeamID <= 0 {
		return nil
	}
	seasonTeam := *responseTeam
	seasonKey := seasonTeamKey(seasonID)
	previous, err := s.get(ctx, teamKey(teamID), seasonKey)
	if err != nil {
		return err
	}
	if previous != nil {
		var priorTeam lps.TeamSummary
		if err := json.Unmarshal([]byte(previous.RawSourceJSON), &priorTeam); err != nil {
			return fmt.Errorf("decode season %d team context: %w", seasonID, err)
		}
		retainMissingTeamFacts(&seasonTeam, &priorTeam)
	}
	seasonTeamJSON, err := json.Marshal(seasonTeam)
	if err != nil {
		return fmt.Errorf("marshal season %d team context: %w", seasonID, err)
	}
	if err := s.put(ctx, &archiveItem{
		PK:            teamKey(teamID),
		SK:            seasonKey,
		Kind:          "team_season",
		TeamID:        teamID,
		SeasonID:      seasonID,
		TeamName:      seasonTeam.TeamName,
		DivisionName:  seasonTeam.DivisionName,
		FacilityID:    seasonTeam.FacilityID,
		FacilityName:  seasonTeam.FacilityName,
		RawSourceJSON: string(seasonTeamJSON),
		FetchedAt:     fetchedAt,
	}); err != nil {
		return fmt.Errorf("save season %d team context: %w", seasonID, err)
	}
	return nil
}

// saveTeam writes the team's enrollment record: its latest team facts, which
// keep a fact the response omits, and a successful refresh attempt that makes
// the team due again a day after this fetch. A refresh failure recorded after
// this fetch keeps its state.
func (s *DynamoStore) saveTeam(ctx context.Context, snapshot *Snapshot, fetchedAt string) error {
	for range maxRecordWriteAttempts {
		previous, err := s.get(ctx, teamKey(snapshot.TeamID), "META")
		if err != nil {
			return err
		}
		if previous != nil && previous.FetchedAt > fetchedAt {
			// A newer response already holds the team facts.
			return nil
		}
		team := snapshot.Team
		teamItem := archiveItem{
			PK:               teamKey(snapshot.TeamID),
			SK:               "META",
			Kind:             "team",
			TeamID:           snapshot.TeamID,
			EnrollmentSource: "manual",
			FetchedAt:        fetchedAt,
			Revision:         1,
			RefreshStatus:    RefreshReady,
			AttemptedAt:      fetchedAt,
			DuePK:            dueTeamsPK,
			DueSK:            dueKey(snapshot.FetchedAt.Add(24*time.Hour), snapshot.TeamID),
		}
		if previous != nil {
			var priorTeam lps.TeamSummary
			if err := json.Unmarshal([]byte(previous.RawSourceJSON), &priorTeam); err != nil {
				return fmt.Errorf("decode team %d context: %w", snapshot.TeamID, err)
			}
			retainMissingTeamFacts(&team, &priorTeam)
			if team.Season == 0 {
				team.Season = priorTeam.Season
			}
			teamItem.Revision = previous.Revision + 1
			if previous.EnrollmentSource != "" {
				teamItem.EnrollmentSource = previous.EnrollmentSource
			}
			if previous.AttemptedAt > fetchedAt {
				teamItem.RefreshStatus, teamItem.AttemptedAt = previous.RefreshStatus, previous.AttemptedAt
				teamItem.FailureKind, teamItem.FailureStatusCode = previous.FailureKind, previous.FailureStatusCode
				teamItem.DuePK, teamItem.DueSK = previous.DuePK, previous.DueSK
			}
		}
		teamJSON, err := json.Marshal(team)
		if err != nil {
			return fmt.Errorf("marshal team %d: %w", snapshot.TeamID, err)
		}
		teamItem.TeamName = team.TeamName
		teamItem.DivisionName = team.DivisionName
		teamItem.SeasonID = team.Season
		teamItem.FacilityID = team.FacilityID
		teamItem.FacilityName = team.FacilityName
		teamItem.RawSourceJSON = string(teamJSON)
		written, err := s.putIfUnchanged(ctx, &teamItem, previous)
		if err != nil {
			return fmt.Errorf("save team %d: %w", snapshot.TeamID, err)
		}
		if written {
			return nil
		}
	}
	return fmt.Errorf("save team %d: changed by concurrent writes %d times", snapshot.TeamID, maxRecordWriteAttempts)
}

func (s *DynamoStore) saveFacilities(ctx context.Context, source []lps.FacilityResponse, fetchedAt string) error {
	facilities := append([]lps.FacilityResponse(nil), source...)
	sort.Slice(facilities, func(i, j int) bool { return facilities[i].FacilityID < facilities[j].FacilityID })
	for i := range facilities {
		facility := &facilities[i]
		if facility.FacilityID <= 0 {
			continue
		}
		previous, err := s.get(ctx, "FACILITY#"+strconv.Itoa(facility.FacilityID), "META")
		if err != nil {
			return err
		}
		if previous != nil {
			var priorFacility lps.FacilityResponse
			if err := json.Unmarshal([]byte(previous.RawSourceJSON), &priorFacility); err != nil {
				return fmt.Errorf("decode facility %d context: %w", facility.FacilityID, err)
			}
			retainMissingFacilityFacts(facility, &priorFacility)
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
	return nil
}

// retainMissingFacilityFacts fills the facility facts current lacks from previous.
func retainMissingFacilityFacts(current, previous *lps.FacilityResponse) {
	if current.FacilityName == "" {
		current.FacilityName = previous.FacilityName
	}
	if current.Address == "" {
		current.Address = previous.Address
	}
	if current.City == "" {
		current.City = previous.City
	}
	if current.State == "" {
		current.State = previous.State
	}
	if current.ZIP == "" {
		current.ZIP = previous.ZIP
	}
}

func (s *DynamoStore) readFacilities(ctx context.Context, team *lps.TeamSummary, games []lps.TeamScheduleGame) ([]lps.FacilityResponse, error) {
	facilityIDs := make(map[int]struct{})
	if team.FacilityID > 0 {
		facilityIDs[team.FacilityID] = struct{}{}
	}
	for i := range games {
		if games[i].FacilityID > 0 {
			facilityIDs[games[i].FacilityID] = struct{}{}
		}
	}
	ids := make([]int, 0, len(facilityIDs))
	for id := range facilityIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	facilities := make([]lps.FacilityResponse, 0, len(ids))
	for _, id := range ids {
		facilityItem, err := s.get(ctx, "FACILITY#"+strconv.Itoa(id), "META")
		if err != nil {
			return nil, err
		}
		if facilityItem == nil {
			continue
		}
		var facility lps.FacilityResponse
		if err := json.Unmarshal([]byte(facilityItem.RawSourceJSON), &facility); err != nil {
			return nil, fmt.Errorf("decode archived facility %d: %w", id, err)
		}
		facilities = append(facilities, facility)
	}
	return facilities, nil
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

// dueTeamsPK partitions the due-team index; its sort key orders teams by
// their next refresh time.
const dueTeamsPK = "TEAM_DUE"

func dueKey(dueAt time.Time, teamID int) string {
	return dueAt.UTC().Format(sortableUTCFormat) + "#" + strconv.Itoa(teamID)
}

func dueTime(dueSK string) (time.Time, error) {
	value, _, found := strings.Cut(dueSK, "#")
	if !found {
		return time.Time{}, errors.New("team refresh due key is missing its Team ID")
	}
	return time.Parse(sortableUTCFormat, value)
}

func seasonGameKey(seasonID, gameID int) string {
	return fmt.Sprintf("SEASON#%010d#GAME#%d", seasonID, gameID)
}

func seasonCoverageKey(seasonID int) string {
	return fmt.Sprintf("SEASON#%010d#COVERAGE", seasonID)
}

func seasonTeamKey(seasonID int) string {
	return fmt.Sprintf("SEASON#%010d#META", seasonID)
}

func teamSummaryForSeason(teamID int, responseTeam *lps.TeamSummary, games []lps.TeamScheduleGame, seasonID int) lps.TeamSummary {
	if responseTeam.UTeamID == teamID && responseTeam.Season == seasonID {
		return *responseTeam
	}
	for i := range games {
		game := &games[i]
		if game.HomeTeam.UTeamID == teamID && game.HomeTeam.Season == seasonID {
			return game.HomeTeam
		}
		if game.VisitorTeam.UTeamID == teamID && game.VisitorTeam.Season == seasonID {
			return game.VisitorTeam
		}
	}
	return lps.TeamSummary{}
}

// retainMissingTeamFacts fills the team facts current lacks from previous.
func retainMissingTeamFacts(current, previous *lps.TeamSummary) {
	if current.TeamName == "" {
		current.TeamName = previous.TeamName
	}
	if current.DivisionName == "" {
		current.DivisionName = previous.DivisionName
	}
	if current.FacilityID == 0 {
		current.FacilityID = previous.FacilityID
	}
	if current.FacilityName == "" {
		current.FacilityName = previous.FacilityName
	}
}
