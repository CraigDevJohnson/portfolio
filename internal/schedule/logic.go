package schedule

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"portfolio/types"
)

const fieldLocationPrefix = "Field "

func normalizeScheduleGame(game *types.Game) {
	// Fill missing derived fields in priority order so downstream sorting, merging,
	// and ICS export all operate on the same normalized shape.
	if game.DateTime == "" {
		game.DateTime = FormatGameDateTime(game.StartAt)
	}
	if game.Location == "" {
		switch {
		case gameFacilityName(game) != "":
			game.Location = gameFacilityName(game)
		case game.Field != "":
			game.Location = fieldLocationPrefix + game.Field
		}
	}
	if game.Field == "" && game.Location != "" {
		game.Field = game.Location
	}
	if game.PlayerTeamName == "" {
		game.PlayerTeamName = game.Home
	}
	if game.OpponentTeamName == "" {
		game.OpponentTeamName = game.Away
	}
	if gameFacilityName(game) == "" && game.Location != "" {
		game.Facility = types.MergeFacility(game.Facility, &types.Facility{Name: game.Location})
	}
	if game.ID == "" {
		game.ID = FallbackGameID(game)
	}
}

// MergeGames combines two representations of the same game into one normalized record.
func MergeGames(base, incoming *types.Game) types.Game {
	merged := *base
	merged.ID = mergeStringValue(merged.ID, incoming.ID)
	merged.DateTime = mergeStringValue(merged.DateTime, incoming.DateTime)
	merged.StartAt = mergeStringValue(merged.StartAt, incoming.StartAt)
	merged.EndAt = mergeStringValue(merged.EndAt, incoming.EndAt)
	merged.Field = mergeStringValue(merged.Field, incoming.Field)
	merged.Location = mergeStringValue(merged.Location, incoming.Location)
	merged.Home = mergeStringValue(merged.Home, incoming.Home)
	merged.Away = mergeStringValue(merged.Away, incoming.Away)
	merged.Season = mergeStringValue(merged.Season, incoming.Season)
	merged.PlayerTeamName = mergeStringValue(merged.PlayerTeamName, incoming.PlayerTeamName)
	merged.OpponentTeamName = mergeStringValue(merged.OpponentTeamName, incoming.OpponentTeamName)
	merged.DivisionName = mergeStringValue(merged.DivisionName, incoming.DivisionName)
	merged.Facility = types.MergeFacility(merged.Facility, incoming.Facility)
	merged.Result = mergeStringValue(merged.Result, incoming.Result)
	normalizeScheduleGame(&merged)
	return merged
}

// StableGameFields returns the fields used to derive a stable fallback identifier.
func StableGameFields(game *types.Game) string {
	return strings.Join([]string{game.Home, game.Away, game.StartAt, game.DateTime, game.Location, game.Season}, "|")
}

// FallbackGameID derives a deterministic identifier when no upstream ID is available.
func FallbackGameID(game *types.Game) string {
	base := StableGameFields(game)
	if strings.ReplaceAll(base, "|", "") == "" {
		return ""
	}
	checksum := md5.Sum([]byte(base))
	return hex.EncodeToString(checksum[:])
}

// GameKey returns the preferred unique key for a game during merge operations.
func GameKey(game *types.Game) string {
	if game.ID != "" {
		return game.ID
	}
	return StableGameFields(game)
}

// GameStartTime returns the best available parsed start time for a game.
func GameStartTime(game *types.Game) (time.Time, bool) {
	if parsed, ok := ParseScheduleTime(game.StartAt); ok {
		return parsed, true
	}
	return ParseScheduleTime(game.DateTime)
}

// NormalizeScheduleGames normalizes each game in place.
func NormalizeScheduleGames(games []types.Game) {
	for index := range games {
		normalizeScheduleGame(&games[index])
	}
}

// MergeScheduleGames merges incoming games into an existing schedule slice.
func MergeScheduleGames(games, incoming []types.Game, indexByKey map[string]int) []types.Game {
	for i := range incoming {
		game := &incoming[i]
		key := GameKey(game)
		if existingIndex, exists := indexByKey[key]; exists {
			games[existingIndex] = MergeGames(&games[existingIndex], game)
			continue
		}
		indexByKey[key] = len(games)
		games = append(games, *game)
	}
	return games
}

// SortScheduleGames sorts games by start time and then stable field ordering.
func SortScheduleGames(games []types.Game) {
	sort.Slice(games, func(i, j int) bool {
		left, leftOK := GameStartTime(&games[i])
		right, rightOK := GameStartTime(&games[j])
		if leftOK && rightOK {
			if !left.Equal(right) {
				return left.Before(right)
			}
			return compareScheduleGames(&games[i], &games[j]) < 0
		}
		if games[i].DateTime != games[j].DateTime {
			return games[i].DateTime < games[j].DateTime
		}
		return compareScheduleGames(&games[i], &games[j]) < 0
	})
}

func compareScheduleGames(left, right *types.Game) int {
	for _, pair := range [][2]string{
		{left.DateTime, right.DateTime},
		{left.StartAt, right.StartAt},
		{left.Home, right.Home},
		{left.Away, right.Away},
		{left.Location, right.Location},
		{left.Field, right.Field},
		{left.Season, right.Season},
		{left.PlayerTeamName, right.PlayerTeamName},
		{left.OpponentTeamName, right.OpponentTeamName},
		{left.DivisionName, right.DivisionName},
		{facilitySortKey(left.Facility), facilitySortKey(right.Facility)},
		{left.Result, right.Result},
		{left.ID, right.ID},
	} {
		if pair[0] == pair[1] {
			continue
		}
		if pair[0] < pair[1] {
			return -1
		}
		return 1
	}
	return 0
}

// UpcomingScheduleGames filters out games that have already started.
func UpcomingScheduleGames(games []types.Game) []types.Game {
	filtered := make([]types.Game, 0, len(games))
	now := time.Now()
	for i := range games {
		start, ok := GameStartTime(&games[i])
		if ok && start.Before(now) {
			continue
		}
		filtered = append(filtered, games[i])
	}
	return filtered
}

func mergeStringValue(base, incoming string) string {
	if base != "" {
		return base
	}
	return incoming
}

func gameFacilityName(game *types.Game) string {
	if game == nil || game.Facility == nil {
		return ""
	}
	return strings.TrimSpace(game.Facility.Name)
}

func facilitySortKey(facility *types.Facility) string {
	if facility == nil {
		return ""
	}

	return strings.Join([]string{
		strings.TrimSpace(facility.Name),
		strings.TrimSpace(facility.Address),
		strings.TrimSpace(facility.City),
		strings.TrimSpace(facility.State),
		strings.TrimSpace(facility.ZIP),
	}, "|")
}
