package lps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"portfolio/internal/config"
	"portfolio/internal/schedule"
	"portfolio/types"
)

// ScheduleResolver loads players, teams, facilities, and schedule data from LPS.
type ScheduleResolver struct {
	baseURL       string
	httpClient    *http.Client
	jwt           string
	facilityCache map[int]Facility
}

// NewScheduleResolver constructs a resolver with explicit request dependencies.
func NewScheduleResolver(baseURL string, httpClient *http.Client, jwt string) *ScheduleResolver {
	return &ScheduleResolver{
		baseURL:       baseURL,
		httpClient:    httpClient,
		jwt:           jwt,
		facilityCache: make(map[int]Facility),
	}
}

// FetchUserPlayers validates an imported JWT and loads linked players from LPS.
func FetchUserPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string) (UserPlayerDiscovery, error) {
	var discovery UserPlayerDiscovery

	normalizedJWT, err := NormalizeImportedJWT(jwt)
	if err != nil {
		return discovery, NewFetchError(ErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	req, err := newAPIRequest(ctx, baseURL, normalizedJWT, "users", "check")
	if err != nil {
		return discovery, err
	}

	resp, err := doAPIRequest(httpClient, req)
	if err != nil {
		return discovery, NewFetchError(ErrorUpstream, 0, http.StatusBadGateway, "could not reach Let's Play Soccer while loading players: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxLPSResponseBodySize))
	if err != nil {
		return discovery, NewFetchError(ErrorUpstream, 0, http.StatusBadGateway, "could not read the player lookup response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return discovery, NewFetchError(ErrorUnauthorized, 0, resp.StatusCode, "Let's Play Soccer rejected the imported token with status 401")
	}
	if resp.StatusCode == http.StatusForbidden {
		return discovery, NewFetchError(ErrorForbidden, 0, resp.StatusCode, "Let's Play Soccer denied access to the player lookup with status 403")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return discovery, NewFetchError(ErrorUpstream, 0, resp.StatusCode, "Let's Play Soccer returned status %d while loading players", resp.StatusCode)
	}

	discovery, err = DecodeLPSUserPlayers(responseBody)
	if err != nil {
		var fetchErr *FetchError
		if errors.As(err, &fetchErr) {
			return discovery, err
		}
		return discovery, NewFetchError(ErrorUpstream, 0, http.StatusBadGateway, "%v", err)
	}

	return discovery, nil
}

// FetchGamesForPlayers resolves teams for the selected players and merges their schedules.
func FetchGamesForPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string, playerIDs []int) ([]types.Game, error) {
	normalizedJWT, err := NormalizeImportedJWT(jwt)
	if err != nil {
		return nil, NewFetchError(ErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	resolver := NewScheduleResolver(baseURL, httpClient, normalizedJWT)
	teamByID := make(map[int]TeamSummary)
	for _, playerID := range SortedUniqueIDs(playerIDs) {
		playerTeams, err := resolver.FetchPlayerTeams(ctx, playerID)
		if err != nil {
			return nil, err
		}
		for _, team := range playerTeams {
			if team.UTeamID <= 0 {
				continue
			}
			if _, exists := teamByID[team.UTeamID]; !exists {
				teamByID[team.UTeamID] = team
			}
		}
	}

	teamIDs := make([]int, 0, len(teamByID))
	for teamID := range teamByID {
		teamIDs = append(teamIDs, teamID)
	}
	sort.Ints(teamIDs)

	games := make([]types.Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range teamIDs {
		team := teamByID[teamID]
		teamGames, err := resolver.FetchTeamGames(ctx, teamID, &team)
		if err != nil {
			return nil, err
		}
		games = schedule.MergeScheduleGames(games, teamGames, indexByKey)
	}
	schedule.SortScheduleGames(games)
	return games, nil
}

// FetchGamesForTeams loads and merges schedules for the selected team IDs.
func FetchGamesForTeams(ctx context.Context, baseURL string, httpClient *http.Client, teamIDs []int) ([]types.Game, error) {
	resolver := NewScheduleResolver(baseURL, httpClient, "")
	games := make([]types.Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range SortedUniqueIDs(teamIDs) {
		teamGames, err := resolver.FetchTeamGames(ctx, teamID, nil)
		if err != nil {
			return nil, err
		}
		games = schedule.MergeScheduleGames(games, teamGames, indexByKey)
	}
	schedule.SortScheduleGames(games)
	return games, nil
}

// FetchPlayerTeams loads the teams linked to a player.
func (resolver *ScheduleResolver) FetchPlayerTeams(ctx context.Context, playerID int) ([]TeamSummary, error) {
	if playerID <= 0 {
		return nil, NewFetchError(ErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newAPIRequest(ctx, resolver.baseURL, resolver.jwt, "players", strconv.Itoa(playerID), "my_teams")
	if err != nil {
		return nil, err
	}

	resp, err := doAPIRequest(resolver.httpClient, req)
	if err != nil {
		return nil, NewFetchError(ErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading player teams: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxLPSResponseBodySize))
	if err != nil {
		return nil, NewFetchError(ErrorUpstream, playerID, http.StatusBadGateway, "could not read the player teams response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, NewFetchError(ErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, NewFetchError(ErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, NewFetchError(ErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find teams for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, NewFetchError(ErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading player teams", resp.StatusCode)
	}

	var teams []TeamSummary
	if err := json.Unmarshal(responseBody, &teams); err != nil {
		return nil, NewFetchError(ErrorUpstream, playerID, http.StatusBadGateway, "The player teams response format was not recognized.")
	}

	sort.Slice(teams, func(i, j int) bool {
		if teams[i].UTeamID != teams[j].UTeamID {
			return teams[i].UTeamID < teams[j].UTeamID
		}
		return teams[i].TeamName < teams[j].TeamName
	})
	return teams, nil
}

// FetchTeamGames loads, maps, and filters a team's upcoming games.
func (resolver *ScheduleResolver) FetchTeamGames(ctx context.Context, teamID int, selectedTeam *TeamSummary) ([]types.Game, error) {
	response, err := resolver.FetchTeamSchedule(ctx, teamID)
	if err != nil {
		return nil, err
	}

	games := make([]types.Game, 0, len(response.Games))
	for i := range response.Games {
		game, err := resolver.MapTeamScheduleGame(ctx, &response.Games[i], response.Team, selectedTeam)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	schedule.NormalizeScheduleGames(games)
	return schedule.UpcomingScheduleGames(games), nil
}

// FetchTeamSchedule loads the raw team schedule response.
func (resolver *ScheduleResolver) FetchTeamSchedule(ctx context.Context, teamID int) (TeamScheduleResponse, error) {
	var teamSchedule TeamScheduleResponse
	if teamID <= 0 {
		return teamSchedule, NewFetchError(ErrorInvalidTeam, teamID, http.StatusBadRequest, "team ID %d is invalid", teamID)
	}

	req, err := newAPIRequest(ctx, resolver.baseURL, "", "teams", strconv.Itoa(teamID))
	if err != nil {
		return teamSchedule, err
	}

	resp, err := doAPIRequest(resolver.httpClient, req)
	if err != nil {
		return teamSchedule, NewFetchError(ErrorUpstream, teamID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading team schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxLPSResponseBodySize))
	if err != nil {
		return teamSchedule, NewFetchError(ErrorUpstream, teamID, http.StatusBadGateway, "could not read the team schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return teamSchedule, NewFetchError(ErrorInvalidTeam, teamID, resp.StatusCode, "Let's Play Soccer could not find team %d", teamID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return teamSchedule, NewFetchError(ErrorUpstream, teamID, resp.StatusCode, "Let's Play Soccer returned status %d while loading team schedules", resp.StatusCode)
	}

	if err := json.Unmarshal(responseBody, &teamSchedule); err != nil {
		return teamSchedule, NewFetchError(ErrorUpstream, teamID, http.StatusBadGateway, "The team schedule response format was not recognized.")
	}
	return teamSchedule, nil
}

// MapTeamScheduleGame maps a raw team schedule game into the shared game model.
func (resolver *ScheduleResolver) MapTeamScheduleGame(ctx context.Context, rawGame *TeamScheduleGame, responseTeam TeamSummary, selectedTeam *TeamSummary) (types.Game, error) {
	if rawGame == nil {
		return types.Game{}, nil
	}

	var selected TeamSummary
	if selectedTeam != nil {
		selected = *selectedTeam
	}

	facilityID := FirstPositiveInt(rawGame.FacilityID, selected.FacilityID, responseTeam.FacilityID, rawGame.HomeTeam.FacilityID, rawGame.VisitorTeam.FacilityID)
	facilityName := FirstNonEmptyString(rawGame.FacilityName, selected.FacilityName, responseTeam.FacilityName, rawGame.HomeTeam.FacilityName, rawGame.VisitorTeam.FacilityName)
	facility, err := resolver.FetchFacility(ctx, facilityID)
	if err != nil {
		return types.Game{}, err
	}
	if strings.TrimSpace(facility.FacilityName) != "" {
		facilityName = strings.TrimSpace(facility.FacilityName)
	}

	fieldName := strings.TrimSpace(rawGame.FieldName)
	if fieldName == "" && rawGame.Field > 0 {
		fieldName = "Field " + strconv.Itoa(rawGame.Field)
	}

	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	playerTeamName, opponentTeamName, divisionName := resolveSelectedTeamMatchup(rawGame, responseTeam, &selected)
	if playerTeamName == "" {
		playerTeamName = homeName
	}
	if opponentTeamName == "" {
		opponentTeamName = visitorName
		if playerTeamName == visitorName {
			opponentTeamName = homeName
		}
	}

	game := types.Game{
		ID:               IntString(rawGame.UGameID),
		DateTime:         schedule.FormatGameDateTime(schedule.NormalizeLPSScheduleTime(rawGame.SchedGameDateTime)),
		StartAt:          schedule.NormalizeLPSScheduleTime(rawGame.SchedGameDateTime),
		EndAt:            schedule.NormalizeLPSScheduleTime(StringPointerValue(rawGame.SchedGameEndTime)),
		Field:            fieldName,
		Location:         strings.TrimSpace(facilityName),
		Home:             homeName,
		Away:             visitorName,
		Season:           FirstNonEmptyString(IntString(selected.Season), IntString(responseTeam.Season), IntString(rawGame.Season), IntString(rawGame.HomeTeam.Season), IntString(rawGame.VisitorTeam.Season)),
		PlayerTeamName:   playerTeamName,
		OpponentTeamName: opponentTeamName,
		DivisionName:     divisionName,
		FacilityID:       facilityID,
		FacilityName:     strings.TrimSpace(facilityName),
		FacilityAddress:  strings.TrimSpace(facility.Address),
		FacilityCity:     strings.TrimSpace(facility.City),
		FacilityState:    strings.TrimSpace(facility.State),
		FacilityZIP:      strings.TrimSpace(facility.ZIP),
		Result:           strings.TrimSpace(rawGame.Result),
	}

	return game, nil
}

// FetchFacility loads a facility and caches it for the lifetime of the resolver.
func (resolver *ScheduleResolver) FetchFacility(ctx context.Context, facilityID int) (Facility, error) {
	if facilityID <= 0 {
		return Facility{}, nil
	}
	if facility, ok := resolver.facilityCache[facilityID]; ok {
		return facility, nil
	}

	req, err := newAPIRequest(ctx, resolver.baseURL, "", "facilities", strconv.Itoa(facilityID))
	if err != nil {
		return Facility{}, err
	}

	resp, err := doAPIRequest(resolver.httpClient, req)
	if err != nil {
		return Facility{}, NewFetchError(ErrorUpstream, facilityID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading facility %d: %w", facilityID, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, config.MaxLPSResponseBodySize))
	if err != nil {
		return Facility{}, NewFetchError(ErrorUpstream, facilityID, http.StatusBadGateway, "could not read the facility response for facility %d: %w", facilityID, err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return Facility{}, NewFetchError(ErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer could not find facility %d", facilityID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Facility{}, NewFetchError(ErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer returned status %d while loading facility %d", resp.StatusCode, facilityID)
	}

	var facility Facility
	if err := json.Unmarshal(responseBody, &facility); err != nil {
		return Facility{}, NewFetchError(ErrorUpstream, facilityID, http.StatusBadGateway, "The facility response format was not recognized.")
	}
	resolver.facilityCache[facilityID] = facility
	return facility, nil
}

func resolveSelectedTeamMatchup(rawGame *TeamScheduleGame, responseTeam TeamSummary, selectedTeam *TeamSummary) (string, string, string) {
	if rawGame == nil {
		return "", "", ""
	}

	selectedTeamID := responseTeam.UTeamID
	selectedTeamName := strings.TrimSpace(responseTeam.TeamName)
	divisionName := strings.TrimSpace(responseTeam.DivisionName)
	if selectedTeam != nil {
		selectedTeamID = FirstPositiveInt(selectedTeam.UTeamID, responseTeam.UTeamID)
		selectedTeamName = FirstNonEmptyString(selectedTeam.TeamName, responseTeam.TeamName)
		divisionName = FirstNonEmptyString(selectedTeam.DivisionName, responseTeam.DivisionName)
	}
	if selectedTeamID == 0 && rawGame.TeamIDSelected != nil {
		selectedTeamID = *rawGame.TeamIDSelected
	}

	homeID := FirstPositiveInt(rawGame.HomeTeam.UTeamID, rawGame.UTeam1)
	visitorID := FirstPositiveInt(rawGame.VisitorTeam.UTeamID, rawGame.UTeam2)
	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	homeDivision := strings.TrimSpace(rawGame.HomeTeam.DivisionName)
	visitorDivision := strings.TrimSpace(rawGame.VisitorTeam.DivisionName)

	switch {
	case selectedTeamID > 0 && homeID == selectedTeamID:
		return FirstNonEmptyString(selectedTeamName, homeName), visitorName, FirstNonEmptyString(divisionName, homeDivision, visitorDivision)
	case selectedTeamID > 0 && visitorID == selectedTeamID:
		return FirstNonEmptyString(selectedTeamName, visitorName), homeName, FirstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}

	playerTeamName := FirstNonEmptyString(selectedTeamName, homeName)
	if playerTeamName == visitorName {
		return playerTeamName, homeName, FirstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}
	return playerTeamName, visitorName, FirstNonEmptyString(divisionName, homeDivision, visitorDivision)
}
