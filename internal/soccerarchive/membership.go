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

	"portfolio/internal/lps"
)

const (
	authenticatedPlayerLookup = "authenticated_player_lookup"
	// playerEnrollment marks a team an authenticated player import found.
	playerEnrollment = "player"
)

// SavePlayerDiscovery stores identities, exact owner-bound membership proof,
// and known teams. Stable keys make a repeated import converge on the same
// records. A failure can be retried without multiplying membership edges.
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

	observedAt := discovery.ObservedAt.UTC().Format(sortableUTCFormat)
	ownerPrefix := ownerEvidencePrefix(discovery.OwnerIssuer, discovery.OwnerSubject)
	players := slices.Clone(discovery.Players)
	sort.Slice(players, func(i, j int) bool { return players[i].UPlayerID < players[j].UPlayerID })
	for _, player := range players {
		playerPK := "PLAYER#" + strconv.Itoa(player.UPlayerID)
		mainPlayer := player.IsMainPlayer
		if err := s.put(ctx, &archiveItem{
			PK: playerPK, SK: "META", Kind: "player", PlayerID: player.UPlayerID,
			FirstName: player.FirstName, LastName: player.LastName, IsMainPlayer: &mainPlayer,
			Source: authenticatedPlayerLookup, ObservedAt: observedAt, FetchedAt: observedAt,
		}); err != nil {
			return fmt.Errorf("save player %d: %w", player.UPlayerID, err)
		}
		if err := s.put(ctx, &archiveItem{
			PK: playerPK, SK: ownerPrefix + "#META", Kind: "player_owner", PlayerID: player.UPlayerID,
			OwnerIssuer: discovery.OwnerIssuer, OwnerSubject: discovery.OwnerSubject,
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

	teams := slices.Clone(discovery.KnownTeams)
	sort.Slice(teams, func(i, j int) bool { return teams[i].UTeamID < teams[j].UTeamID })
	for _, team := range teams {
		if err := s.enrollPlayerTeam(ctx, team, discovery.ObservedAt); err != nil {
			return err
		}
	}
	return nil
}

// enrollPlayerTeam enrolls a Team ID an authenticated player lookup returned.
// A team no response has enrolled yet is due for its first refresh at once and
// keeps the lookup's team facts until a team response replaces them; it has no
// fetch or attempt time. An enrolled team keeps its facts, refresh state, and
// due time, and only records that a player import found it.
func (s *DynamoStore) enrollPlayerTeam(ctx context.Context, team lps.TeamSummary, observedAt time.Time) error {
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
		written, err := s.putIfUnchanged(ctx, &record, previous)
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
