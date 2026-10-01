package soccerarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"portfolio/internal/lps"
)

const (
	authenticatedPlayerLookup = "authenticated_player_lookup"
	// playerEnrollment marks a team an authenticated player import found.
	playerEnrollment = "player"
)

// HasPlayerMembership checks one owner-bound authenticated observation by its
// exact player, team, and LPS season key. Team enrollment and names are not proof.
func (s *DynamoStore) HasPlayerMembership(ctx context.Context, issuer, subject string, playerID, teamID, seasonID int) (bool, error) {
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(subject) == "" || playerID <= 0 || teamID <= 0 || seasonID <= 0 {
		return false, nil
	}
	record, err := s.get(ctx, "PLAYER#"+strconv.Itoa(playerID), fmt.Sprintf("%s#TEAM#%010d#SEASON#%010d", ownerEvidencePrefix(issuer, subject), teamID, seasonID))
	if err != nil {
		return false, err
	}
	return record != nil && isOwnerMembership(record, issuer, subject, playerID) && record.TeamID == teamID && record.SeasonID == seasonID, nil
}

// ListPlayerMemberships returns every team season one owner has recorded
// authenticated proof for, for one player, in Team ID then LPS season order.
// It queries only that owner's membership keys in the player's partition, so
// another owner's proof is never read. No proof is an empty list, not an error.
func (s *DynamoStore) ListPlayerMemberships(ctx context.Context, issuer, subject string, playerID int) ([]PlayerMembership, error) {
	memberships := make([]PlayerMembership, 0)
	if strings.TrimSpace(issuer) == "" || strings.TrimSpace(subject) == "" || playerID <= 0 {
		return memberships, nil
	}
	var startKey map[string]types.AttributeValue
	for {
		page, err := s.api.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(s.tableName),
			KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :prefix)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":     &types.AttributeValueMemberS{Value: "PLAYER#" + strconv.Itoa(playerID)},
				":prefix": &types.AttributeValueMemberS{Value: ownerEvidencePrefix(issuer, subject) + "#TEAM#"},
			},
			ConsistentRead:    aws.Bool(true),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("list player %d memberships: %w", playerID, err)
		}
		for _, item := range page.Items {
			var record archiveItem
			if err := attributevalue.UnmarshalMap(item, &record); err != nil {
				return nil, fmt.Errorf("decode player %d membership: %w", playerID, err)
			}
			if !isOwnerMembership(&record, issuer, subject, playerID) || record.TeamID <= 0 || record.SeasonID <= 0 {
				continue
			}
			// The record keeps the lookup's team as LPS returned it; its
			// validated key fields decide which team season it proves.
			team := lps.TeamSummary{TeamName: record.TeamName, DivisionName: record.DivisionName, FacilityID: record.FacilityID, FacilityName: record.FacilityName}
			if record.RawSourceJSON != "" {
				if err := json.Unmarshal([]byte(record.RawSourceJSON), &team); err != nil {
					return nil, fmt.Errorf("decode player %d team %d season %d: %w", playerID, record.TeamID, record.SeasonID, err)
				}
			}
			team.UTeamID, team.Season = record.TeamID, record.SeasonID
			memberships = append(memberships, PlayerMembership{PlayerID: playerID, Team: team})
		}
		if len(page.LastEvaluatedKey) == 0 {
			return memberships, nil
		}
		startKey = page.LastEvaluatedKey
	}
}

// isOwnerMembership reports whether record is one owner's authenticated
// membership observation for playerID.
func isOwnerMembership(record *archiveItem, issuer, subject string, playerID int) bool {
	return record.Kind == "membership" && record.Source == authenticatedPlayerLookup && record.ObservedAt != "" &&
		record.OwnerIssuer == issuer && record.OwnerSubject == subject && record.PlayerID == playerID
}

// playerRemovalRounds bounds how many times a removal lists and deletes the
// player's partition while imports of the same player keep adding to it.
const playerRemovalRounds = 3

// DeletePlayerEvidence removes the complete player partition: the identity,
// every owner link, and every team-season membership, whichever owner
// recorded them. Team, season, game, and facility records live under other
// partitions and are kept. It deletes in reverse key order, so each owner's
// memberships go before that owner's link and the identity goes last; a
// failed delete leaves no membership without its identity and owner link,
// and a retry finishes it. An import of the same player can write while the
// removal runs, so the removal lists the partition again with a consistent
// read and deletes what it finds. It reports success only once that listing
// is empty, and reports an error if the partition still holds records after
// playerRemovalRounds rounds. A later import may recollect the player.
func (s *DynamoStore) DeletePlayerEvidence(ctx context.Context, playerID int) error {
	if playerID <= 0 {
		return errors.New("player removal requires a positive player ID")
	}
	for round := 0; ; round++ {
		keys, err := s.playerEvidenceKeys(ctx, playerID)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		if round == playerRemovalRounds {
			return fmt.Errorf("remove player %d evidence: %d records remain after concurrent imports", playerID, len(keys))
		}
		// Query returns the partition in ascending key order: META, then each
		// OWNER#<hash>#META before that owner's OWNER#<hash>#TEAM#... records.
		slices.Reverse(keys)
		for _, key := range keys {
			if _, err := s.api.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(s.tableName), Key: key}); err != nil {
				return fmt.Errorf("remove player %d evidence: %w", playerID, err)
			}
		}
	}
}

// playerEvidenceKeys lists the keys of the player's partition in ascending
// key order with a consistent read.
func (s *DynamoStore) playerEvidenceKeys(ctx context.Context, playerID int) ([]map[string]types.AttributeValue, error) {
	playerPK := "PLAYER#" + strconv.Itoa(playerID)
	var startKey map[string]types.AttributeValue
	keys := make([]map[string]types.AttributeValue, 0)
	for {
		page, err := s.api.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(s.tableName),
			KeyConditionExpression:    aws.String("pk = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: playerPK}},
			ProjectionExpression:      aws.String("pk, sk"),
			ConsistentRead:            aws.Bool(true),
			ExclusiveStartKey:         startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("list player %d evidence: %w", playerID, err)
		}
		for _, item := range page.Items {
			var key struct {
				PK string `dynamodbav:"pk"`
				SK string `dynamodbav:"sk"`
			}
			if err := attributevalue.UnmarshalMap(item, &key); err != nil || key.PK != playerPK || key.SK == "" {
				return nil, fmt.Errorf("invalid player %d evidence key", playerID)
			}
			keys = append(keys, map[string]types.AttributeValue{
				"pk": &types.AttributeValueMemberS{Value: key.PK},
				"sk": &types.AttributeValueMemberS{Value: key.SK},
			})
		}
		if len(page.LastEvaluatedKey) == 0 {
			return keys, nil
		}
		startKey = page.LastEvaluatedKey
	}
}

// SavePlayerDiscovery enrolls the known teams, then stores player identities
// and owner links, then exact owner-bound membership proof. A new team takes
// a slot of the reviewed admission capacity, which reserves slots for these
// player-linked teams. A new team that does not fit is refused on its own:
// it is not enrolled and keeps no membership, while the teams already
// enrolled or admitted keep this import's evidence and every player keeps
// its identity and owner link. The refused teams are then returned as an
// *AdmissionError, after everything else is saved. The records are written
// one at a time, so a failed save can leave some of them stored, but never a
// membership whose team is not enrolled or whose player identity and owner
// link are missing. Stable keys make a retry complete the set without
// multiplying membership edges.
func (s *DynamoStore) SavePlayerDiscovery(ctx context.Context, discovery *PlayerDiscovery) error {
	if discovery == nil || strings.TrimSpace(discovery.OwnerIssuer) == "" || strings.TrimSpace(discovery.OwnerSubject) == "" || discovery.ObservedAt.IsZero() || len(discovery.Players) == 0 {
		return errors.New("player discovery requires owner, players, and observation time")
	}
	playerIDs := make(map[int]bool, len(discovery.Players))
	for _, player := range discovery.Players {
		if player.UPlayerID <= 0 {
			return errors.New("player discovery contains an invalid player ID")
		}
		playerIDs[player.UPlayerID] = true
	}
	teamIDs := make(map[int]bool, len(discovery.KnownTeams))
	for _, team := range discovery.KnownTeams {
		if team.UTeamID <= 0 {
			return errors.New("player discovery contains an invalid team ID")
		}
		teamIDs[team.UTeamID] = true
	}
	for _, membership := range discovery.Memberships {
		if !playerIDs[membership.PlayerID] || !teamIDs[membership.Team.UTeamID] || membership.Team.Season <= 0 {
			return errors.New("player discovery contains unproven team-season membership")
		}
	}

	teams := slices.Clone(discovery.KnownTeams)
	sort.Slice(teams, func(i, j int) bool { return teams[i].UTeamID < teams[j].UTeamID })
	// New teams take the slots left in Team ID order; each enrollment takes
	// its slot atomically, so a concurrent import can only refuse more.
	enrolled := make(map[int]bool, len(teams))
	var refused *AdmissionError
	for i := range teams {
		err := s.enrollPlayerTeam(ctx, &teams[i], discovery.ObservedAt)
		var full *AdmissionError
		switch {
		case errors.As(err, &full):
			if refused == nil {
				refused = &AdmissionError{Source: full.Source, Limit: full.Limit}
			}
			refused.TeamIDs = append(refused.TeamIDs, full.TeamIDs...)
		case err != nil:
			return err
		default:
			enrolled[teams[i].UTeamID] = true
		}
	}

	observedAt := discovery.ObservedAt.UTC().Format(sortableUTCFormat)
	ownerPrefix := ownerEvidencePrefix(discovery.OwnerIssuer, discovery.OwnerSubject)
	players := slices.Clone(discovery.Players)
	sort.Slice(players, func(i, j int) bool { return players[i].UPlayerID < players[j].UPlayerID })
	for _, player := range players {
		playerPK := "PLAYER#" + strconv.Itoa(player.UPlayerID)
		if err := s.put(ctx, &archiveItem{
			PK: playerPK, SK: "META", Kind: "player", PlayerID: player.UPlayerID,
			FirstName: player.FirstName, LastName: player.LastName,
			Source: authenticatedPlayerLookup, ObservedAt: observedAt, FetchedAt: observedAt,
		}); err != nil {
			return fmt.Errorf("save player %d: %w", player.UPlayerID, err)
		}
		// The main-player flag describes the player's place in this owner's
		// LPS account, so it belongs to the owner link, not the shared identity.
		mainPlayer := player.IsMainPlayer
		if err := s.put(ctx, &archiveItem{
			PK: playerPK, SK: ownerPrefix + "#META", Kind: "player_owner", PlayerID: player.UPlayerID,
			IsMainPlayer: &mainPlayer, OwnerIssuer: discovery.OwnerIssuer, OwnerSubject: discovery.OwnerSubject,
			Source: authenticatedPlayerLookup, ObservedAt: observedAt, FetchedAt: observedAt,
		}); err != nil {
			return fmt.Errorf("save player %d owner: %w", player.UPlayerID, err)
		}
	}

	memberships := slices.Clone(discovery.Memberships)
	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].PlayerID != memberships[j].PlayerID {
			return memberships[i].PlayerID < memberships[j].PlayerID
		}
		if memberships[i].Team.UTeamID != memberships[j].Team.UTeamID {
			return memberships[i].Team.UTeamID < memberships[j].Team.UTeamID
		}
		return memberships[i].Team.Season < memberships[j].Team.Season
	})
	for _, membership := range memberships {
		if !enrolled[membership.Team.UTeamID] {
			continue
		}
		teamJSON, err := json.Marshal(membership.Team)
		if err != nil {
			return fmt.Errorf("marshal player %d team: %w", membership.PlayerID, err)
		}
		if err := s.put(ctx, &archiveItem{
			PK:   "PLAYER#" + strconv.Itoa(membership.PlayerID),
			SK:   fmt.Sprintf("%s#TEAM#%010d#SEASON#%010d", ownerPrefix, membership.Team.UTeamID, membership.Team.Season),
			Kind: "membership", PlayerID: membership.PlayerID, TeamID: membership.Team.UTeamID,
			TeamName: membership.Team.TeamName, DivisionName: membership.Team.DivisionName, SeasonID: membership.Team.Season,
			FacilityID: membership.Team.FacilityID, FacilityName: membership.Team.FacilityName,
			OwnerIssuer: discovery.OwnerIssuer, OwnerSubject: discovery.OwnerSubject,
			Source: authenticatedPlayerLookup, ObservedAt: observedAt, FetchedAt: observedAt,
			RawSourceJSON: string(teamJSON),
		}); err != nil {
			return fmt.Errorf("save player %d team %d season %d: %w", membership.PlayerID, membership.Team.UTeamID, membership.Team.Season, err)
		}
	}

	if refused != nil {
		return refused
	}
	return nil
}

// enrollPlayerTeam enrolls a Team ID an authenticated player lookup returned.
// A team no response has enrolled yet is due for its first refresh at once and
// keeps the lookup's team facts until a team response replaces them; it has no
// fetch or attempt time. An enrolled team keeps its facts, refresh state, and
// due time, and only records that a player import found it.
func (s *DynamoStore) enrollPlayerTeam(ctx context.Context, team *lps.TeamSummary, observedAt time.Time) error {
	for range maxRecordWriteAttempts {
		previous, err := s.get(ctx, teamKey(team.UTeamID), "META")
		if err != nil {
			return err
		}
		if previous != nil && previous.EnrollmentSource == playerEnrollment {
			return nil
		}
		var record archiveItem
		if previous != nil {
			record = *previous
			record.Revision = previous.Revision + 1
		} else {
			teamJSON, err := json.Marshal(team)
			if err != nil {
				return fmt.Errorf("marshal enrolled team %d: %w", team.UTeamID, err)
			}
			record = archiveItem{
				PK: teamKey(team.UTeamID), SK: "META", Kind: "team", TeamID: team.UTeamID,
				TeamName: team.TeamName, DivisionName: team.DivisionName, SeasonID: team.Season,
				FacilityID: team.FacilityID, FacilityName: team.FacilityName, RawSourceJSON: string(teamJSON),
				Revision: 1, RefreshStatus: RefreshReady, DuePK: dueTeamsPK, DueSK: dueKey(observedAt, team.UTeamID),
			}
		}
		record.EnrollmentSource = playerEnrollment
		var written bool
		if previous == nil {
			written, err = s.enrollNew(ctx, &record, playerEnrollment)
		} else {
			written, err = s.putIfUnchanged(ctx, &record, previous)
		}
		if errors.Is(err, ErrAdmissionFull) {
			return err
		}
		if err != nil {
			return fmt.Errorf("enroll player team %d: %w", team.UTeamID, err)
		}
		if written {
			return nil
		}
	}
	return fmt.Errorf("enroll player team %d: changed by concurrent writes %d times", team.UTeamID, maxRecordWriteAttempts)
}

func ownerEvidencePrefix(issuer, subject string) string {
	sum := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return "OWNER#" + hex.EncodeToString(sum[:])
}
