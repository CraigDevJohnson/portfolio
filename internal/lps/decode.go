package lps

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"portfolio/internal/schedule"
	"portfolio/types"
)

// DecodeLPSUserPlayers normalizes the LPS /users/check payload into a filtered discovery result.
func DecodeLPSUserPlayers(payload []byte) (UserPlayerDiscovery, error) {
	var discovery UserPlayerDiscovery

	var envelope UserCheckResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return discovery, errors.New("The player lookup response format was not recognized.")
	}
	if envelope.AuthFailure {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = "Let's Play Soccer rejected the imported token."
		}
		return discovery, NewFetchError(ErrorUnauthorized, 0, http.StatusUnauthorized, "%s", message)
	}

	discovery.UserName = fullName(strings.TrimSpace(envelope.FirstName), strings.TrimSpace(envelope.LastName))
	if discovery.UserName == "" {
		discovery.UserName = "Let's Play Soccer account"
	}
	if len(envelope.Players) == 0 {
		discovery.Players = []types.LPSPlayer{}
		return discovery, nil
	}

	deletedPlayerIDs := make(map[int]struct{})
	for _, userPlayer := range envelope.UserPlayers {
		if userPlayer.Deleted {
			deletedPlayerIDs[userPlayer.PlayerID] = struct{}{}
		}
	}
	if len(deletedPlayerIDs) == 0 {
		discovery.Players = envelope.Players
		return discovery, nil
	}

	players := make([]types.LPSPlayer, 0, len(envelope.Players))
	for _, player := range envelope.Players {
		if _, deleted := deletedPlayerIDs[player.UPlayerID]; deleted {
			continue
		}
		players = append(players, player)
	}

	discovery.Players = players
	return discovery, nil
}

// DecodeLPSGames decodes the flexible LPS games payload shapes into normalized games.
func DecodeLPSGames(payload []byte) ([]types.Game, error) {
	var envelope types.LambdaGamesResponse
	if err := json.Unmarshal(payload, &envelope); err == nil && len(envelope.Games) > 0 {
		return envelope.Games, nil
	}

	var raw any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, errors.New("The schedule response format was not recognized.")
	}

	items := ExtractGameMaps(raw)
	if len(items) == 0 {
		return []types.Game{}, nil
	}

	games := make([]types.Game, 0, len(items))
	for _, item := range items {
		game := MapLPSGame(item)
		if game.ID == "" && game.DateTime == "" && game.Home == "" && game.Away == "" {
			continue
		}
		games = append(games, game)
	}
	return games, nil
}

// ExtractGameMaps pulls game objects out of the common LPS response envelopes.
func ExtractGameMaps(raw any) []map[string]any {
	switch value := raw.(type) {
	case []any:
		games := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if mapped, ok := item.(map[string]any); ok {
				games = append(games, mapped)
			}
		}
		return games
	case map[string]any:
		for _, key := range []string{"games", "upcoming_games", "data", "results", "items"} {
			if nested, ok := value[key]; ok {
				games := ExtractGameMaps(nested)
				if len(games) > 0 {
					return games
				}
			}
		}
		return []map[string]any{value}
	default:
		return nil
	}
}

// MapLPSGame maps a flexible LPS game payload into the shared game model.
func MapLPSGame(raw map[string]any) types.Game {
	startAt := schedule.NormalizeLPSScheduleTime(firstString(raw,
		"start_at", "starts_at", "start_datetime", "StartDateTime", "SchedGameDateTime", "schedGameDateTime", "game_datetime", "datetime", "date_time",
	))
	endAt := schedule.NormalizeLPSScheduleTime(firstString(raw,
		"end_at", "ends_at", "end_datetime", "EndDateTime", "schedGameEndTime", "SchedGameEndTime", "game_end_datetime", "end_time",
	))
	dateTime := schedule.NormalizeLPSScheduleTime(firstString(raw, "display_datetime", "DisplayDateTime", "DateTime", "datetime", "date_time"))
	if dateTime == "" {
		dateTime = schedule.FormatGameDateTime(startAt)
	}

	homeTeam := firstString(raw, "home", "Home", "home_team", "HomeTeam", "home_team_name", "TeamName")
	awayTeam := firstString(raw, "away", "Away", "away_team", "visitor_team", "AwayTeam", "away_team_name", "visitor_team_name", "OpponentName", "opponent_name")
	if homeTeam == "" && awayTeam == "" {
		matchup := firstString(raw, "matchup", "Matchup", "title", "Title")
		if matchup != "" {
			parts := strings.Split(matchup, " vs ")
			if len(parts) == 2 {
				homeTeam = strings.TrimSpace(parts[0])
				awayTeam = strings.TrimSpace(parts[1])
			}
		}
	}

	facility := mapGameFacility(raw)
	location := firstString(raw, "location", "Location", "venue", "Venue", "facility", "Facility", "facilityName")
	if location == "" {
		location = gameFacilityName(facility, "")
	}

	return types.Game{
		ID:               firstString(raw, "id", "ID", "game_id", "GameID", "UGameID"),
		DateTime:         dateTime,
		StartAt:          startAt,
		EndAt:            endAt,
		Field:            firstString(raw, "field_name", "FieldName", "field", "Field"),
		Location:         location,
		Home:             homeTeam,
		Away:             awayTeam,
		Season:           firstString(raw, "season", "Season", "season_id", "SeasonID"),
		PlayerTeamName:   homeTeam,
		OpponentTeamName: awayTeam,
		DivisionName:     firstString(raw, "division_name", "DivisionName"),
		Facility:         facility,
		Result:           firstString(raw, "result", "Result"),
	}
}

func mapGameFacility(raw map[string]any) *types.Facility {
	var nested map[string]any
	if value, ok := raw["facility"].(map[string]any); ok {
		nested = value
	}

	return buildGameFacility(
		firstPositiveInt(firstInt(raw, "FacilityID", "facility_id"), firstInt(nested, "id", "ID", "facility_id", "FacilityID")),
		firstNonEmptyString(firstString(raw, "facilityName", "FacilityName", "facility_name"), firstString(nested, "name", "Name", "facility_name", "FacilityName")),
		firstNonEmptyString(firstString(raw, "Address", "address"), firstString(nested, "address", "Address")),
		firstNonEmptyString(firstString(raw, "City", "city"), firstString(nested, "city", "City")),
		firstNonEmptyString(firstString(raw, "State", "state"), firstString(nested, "state", "State")),
		firstNonEmptyString(firstString(raw, "ZIP", "zip"), firstString(nested, "zip", "ZIP")),
	)
}

func firstString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if converted := anyToString(value); converted != "" {
			return converted
		}
	}
	return ""
}

func firstInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case json.Number:
		return typed.String()
	case map[string]any:
		for _, nestedKey := range []string{"name", "Name", "title", "Title", "value", "Value", "team_name", "TeamName", "display_name", "DisplayName"} {
			if nestedValue, ok := typed[nestedKey]; ok {
				if converted := anyToString(nestedValue); converted != "" {
					return converted
				}
			}
		}
	}
	return ""
}
