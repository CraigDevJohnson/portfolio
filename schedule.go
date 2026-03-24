package main

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

func mergeGames(base, incoming *Game) Game {
	merged := *base
	if merged.ID == "" {
		merged.ID = incoming.ID
	}
	if merged.DateTime == "" {
		merged.DateTime = incoming.DateTime
	}
	if merged.StartAt == "" {
		merged.StartAt = incoming.StartAt
	}
	if merged.EndAt == "" {
		merged.EndAt = incoming.EndAt
	}
	if merged.Field == "" {
		merged.Field = incoming.Field
	}
	if merged.Location == "" {
		merged.Location = incoming.Location
	}
	if merged.Home == "" {
		merged.Home = incoming.Home
	}
	if merged.Away == "" {
		merged.Away = incoming.Away
	}
	if merged.Season == "" {
		merged.Season = incoming.Season
	}
	if merged.PlayerTeamName == "" {
		merged.PlayerTeamName = incoming.PlayerTeamName
	}
	if merged.OpponentTeamName == "" {
		merged.OpponentTeamName = incoming.OpponentTeamName
	}
	if merged.DivisionName == "" {
		merged.DivisionName = incoming.DivisionName
	}
	if merged.FacilityID == 0 {
		merged.FacilityID = incoming.FacilityID
	}
	if merged.FacilityName == "" {
		merged.FacilityName = incoming.FacilityName
	}
	if merged.FacilityAddress == "" {
		merged.FacilityAddress = incoming.FacilityAddress
	}
	if merged.FacilityCity == "" {
		merged.FacilityCity = incoming.FacilityCity
	}
	if merged.FacilityState == "" {
		merged.FacilityState = incoming.FacilityState
	}
	if merged.FacilityZIP == "" {
		merged.FacilityZIP = incoming.FacilityZIP
	}
	if merged.Result == "" {
		merged.Result = incoming.Result
	}
	if merged.Location == "" && merged.Field != "" {
		merged.Location = "Field " + merged.Field
	}
	if merged.Field == "" && merged.Location != "" {
		merged.Field = merged.Location
	}
	if merged.DateTime == "" {
		merged.DateTime = formatGameDateTime(merged.StartAt)
	}
	if merged.ID == "" {
		merged.ID = fallbackGameID(&merged)
	}
	return merged
}

func stableGameFields(game *Game) string {
	return strings.Join([]string{game.Home, game.Away, game.StartAt, game.DateTime, game.Location, game.Season}, "|")
}

func fallbackGameID(game *Game) string {
	base := stableGameFields(game)
	if strings.ReplaceAll(base, "|", "") == "" {
		return ""
	}
	hash := md5.Sum([]byte(base))
	return hex.EncodeToString(hash[:])
}

func gameKey(game *Game) string {
	if game.ID != "" {
		return game.ID
	}
	return stableGameFields(game)
}

func gameStartTime(game *Game) (time.Time, bool) {
	if parsed, ok := parseScheduleTime(game.StartAt); ok {
		return parsed, true
	}
	return parseScheduleTime(game.DateTime)
}

func normalizeScheduleGames(games []Game) {
	for index := range games {
		if games[index].ID == "" {
			games[index].ID = fallbackGameID(&games[index])
		}
		if games[index].DateTime == "" {
			games[index].DateTime = formatGameDateTime(games[index].StartAt)
		}
		if games[index].Location == "" && games[index].Field != "" {
			games[index].Location = "Field " + games[index].Field
		}
		if games[index].Field == "" && games[index].Location != "" {
			games[index].Field = games[index].Location
		}
		if games[index].PlayerTeamName == "" {
			games[index].PlayerTeamName = games[index].Home
		}
		if games[index].OpponentTeamName == "" {
			games[index].OpponentTeamName = games[index].Away
		}
		if games[index].FacilityName == "" {
			games[index].FacilityName = games[index].Location
		}
	}
}

func mergeScheduleGames(games, incoming []Game, indexByKey map[string]int) []Game {
	for i := range incoming {
		game := &incoming[i]
		key := gameKey(game)
		if existingIndex, exists := indexByKey[key]; exists {
			games[existingIndex] = mergeGames(&games[existingIndex], game)
			continue
		}
		indexByKey[key] = len(games)
		games = append(games, *game)
	}
	return games
}

func sortScheduleGames(games []Game) {
	sort.Slice(games, func(i, j int) bool {
		left, leftOK := gameStartTime(&games[i])
		right, rightOK := gameStartTime(&games[j])
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

func compareScheduleGames(left, right *Game) int {
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
		{left.FacilityName, right.FacilityName},
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

func upcomingScheduleGames(games []Game) []Game {
	filtered := make([]Game, 0, len(games))
	now := time.Now()
	for i := range games {
		start, ok := gameStartTime(&games[i])
		if ok && start.Before(now) {
			continue
		}
		filtered = append(filtered, games[i])
	}
	return filtered
}
