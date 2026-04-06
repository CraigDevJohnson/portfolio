package lps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"portfolio/internal/schedule"
	"portfolio/types"
)

// ScheduleResolver loads players, teams, facilities, and schedule data from LPS.
type ScheduleResolver struct {
	baseURL       string
	httpClient    *http.Client
	jwt           string
	facilityCache map[int]FacilityResponse
}

// NewScheduleResolver constructs a resolver with explicit request dependencies.
func NewScheduleResolver(baseURL string, httpClient *http.Client, jwt string) *ScheduleResolver {
	return &ScheduleResolver{
		baseURL:       baseURL,
		httpClient:    httpClient,
		jwt:           jwt,
		facilityCache: make(map[int]FacilityResponse),
	}
}

// FetchUserPlayers loads linked players from LPS using a normalized imported JWT.
func FetchUserPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string) (UserPlayerDiscovery, error) {
	var discovery UserPlayerDiscovery

	req, err := newAPIRequest(ctx, baseURL, jwt, "users", "check")
	if err != nil {
		return discovery, err
	}

	responseBody, err := executeAPIRequest(httpClient, req, 0)
	if err != nil {
		return discovery, err
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

// FetchGamesForPlayers resolves teams for the selected players and merges their schedules using a normalized imported JWT.
func FetchGamesForPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string, playerIDs []int) ([]types.Game, error) {
	resolver := NewScheduleResolver(baseURL, httpClient, jwt)
	teamByID := make(map[int]TeamSummary)
	for _, playerID := range sortedUniqueIDs(playerIDs) {
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
	teamLookup := make(map[int]*TeamSummary, len(teamByID))
	for teamID := range teamByID {
		teamIDs = append(teamIDs, teamID)
		team := teamByID[teamID]
		teamLookup[teamID] = &team
	}
	sort.Ints(teamIDs)

	return resolver.mergeTeamSchedules(ctx, teamIDs, teamLookup)
}

// FetchGamesForTeams loads and merges schedules for the selected team IDs.
func FetchGamesForTeams(ctx context.Context, baseURL string, httpClient *http.Client, teamIDs []int) ([]types.Game, error) {
	resolver := NewScheduleResolver(baseURL, httpClient, "")
	return resolver.mergeTeamSchedules(ctx, sortedUniqueIDs(teamIDs), nil)
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

	responseBody, err := executeAPIRequest(resolver.httpClient, req, playerID,
		statusErrorKind{codes: []int{http.StatusBadRequest, http.StatusNotFound}, kind: ErrorInvalidPlayer},
	)
	if err != nil {
		return nil, err
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

	responseBody, err := executeAPIRequest(resolver.httpClient, req, teamID,
		statusErrorKind{codes: []int{http.StatusBadRequest, http.StatusNotFound}, kind: ErrorInvalidTeam},
	)
	if err != nil {
		return teamSchedule, err
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

	facilityID := firstPositiveInt(rawGame.FacilityID, selected.FacilityID, responseTeam.FacilityID, rawGame.HomeTeam.FacilityID, rawGame.VisitorTeam.FacilityID)
	facilityName := firstNonEmptyString(rawGame.FacilityName, selected.FacilityName, responseTeam.FacilityName, rawGame.HomeTeam.FacilityName, rawGame.VisitorTeam.FacilityName)
	facility, err := resolver.FetchFacility(ctx, facilityID)
	if err != nil {
		return types.Game{}, err
	}
	if strings.TrimSpace(facility.FacilityName) != "" {
		facilityName = strings.TrimSpace(facility.FacilityName)
	}
	gameFacility := buildGameFacility(
		facilityID,
		facilityName,
		facility.Address,
		facility.City,
		facility.State,
		facility.ZIP,
	)

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
		ID:               intString(rawGame.UGameID),
		DateTime:         schedule.FormatGameDateTime(schedule.NormalizeLPSScheduleTime(rawGame.SchedGameDateTime)),
		StartAt:          schedule.NormalizeLPSScheduleTime(rawGame.SchedGameDateTime),
		EndAt:            schedule.NormalizeLPSScheduleTime(stringPointerValue(rawGame.SchedGameEndTime)),
		Field:            fieldName,
		Location:         strings.TrimSpace(facilityName),
		Home:             homeName,
		Away:             visitorName,
		Season:           firstNonEmptyString(intString(selected.Season), intString(responseTeam.Season), intString(rawGame.Season), intString(rawGame.HomeTeam.Season), intString(rawGame.VisitorTeam.Season)),
		PlayerTeamName:   playerTeamName,
		OpponentTeamName: opponentTeamName,
		DivisionName:     divisionName,
		Facility:         gameFacility,
		Result:           strings.TrimSpace(rawGame.Result),
	}

	return game, nil
}

// FetchFacility loads a facility and caches it for the lifetime of the resolver.
func (resolver *ScheduleResolver) FetchFacility(ctx context.Context, facilityID int) (FacilityResponse, error) {
	if facilityID <= 0 {
		return FacilityResponse{}, nil
	}
	if facility, ok := resolver.facilityCache[facilityID]; ok {
		return facility, nil
	}

	req, err := newAPIRequest(ctx, resolver.baseURL, "", "facilities", strconv.Itoa(facilityID))
	if err != nil {
		return FacilityResponse{}, err
	}

	responseBody, err := executeAPIRequest(resolver.httpClient, req, facilityID,
		statusErrorKind{codes: []int{http.StatusBadRequest, http.StatusNotFound}, kind: ErrorUpstream},
	)
	if err != nil {
		return FacilityResponse{}, err
	}

	var facility FacilityResponse
	if err := json.Unmarshal(responseBody, &facility); err != nil {
		return FacilityResponse{}, NewFetchError(ErrorUpstream, facilityID, http.StatusBadGateway, "The facility response format was not recognized.")
	}
	resolver.facilityCache[facilityID] = facility
	return facility, nil
}

func (resolver *ScheduleResolver) mergeTeamSchedules(ctx context.Context, teamIDs []int, teamLookup map[int]*TeamSummary) ([]types.Game, error) {
	games := make([]types.Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range teamIDs {
		var selectedTeam *TeamSummary
		if teamLookup != nil {
			selectedTeam = teamLookup[teamID]
		}
		teamGames, err := resolver.FetchTeamGames(ctx, teamID, selectedTeam)
		if err != nil {
			return nil, err
		}
		games = schedule.MergeScheduleGames(games, teamGames, indexByKey)
	}
	schedule.SortScheduleGames(games)
	return games, nil
}

func resolveSelectedTeamMatchup(rawGame *TeamScheduleGame, responseTeam TeamSummary, selectedTeam *TeamSummary) (string, string, string) {
	selectedTeamID := responseTeam.UTeamID
	selectedTeamName := strings.TrimSpace(responseTeam.TeamName)
	divisionName := strings.TrimSpace(responseTeam.DivisionName)
	if selectedTeam != nil {
		selectedTeamID = firstPositiveInt(selectedTeam.UTeamID, responseTeam.UTeamID)
		selectedTeamName = firstNonEmptyString(selectedTeam.TeamName, responseTeam.TeamName)
		divisionName = firstNonEmptyString(selectedTeam.DivisionName, responseTeam.DivisionName)
	}
	if selectedTeamID == 0 && rawGame.TeamIDSelected != nil {
		selectedTeamID = *rawGame.TeamIDSelected
	}

	homeID := firstPositiveInt(rawGame.HomeTeam.UTeamID, rawGame.UTeam1)
	visitorID := firstPositiveInt(rawGame.VisitorTeam.UTeamID, rawGame.UTeam2)
	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	homeDivision := strings.TrimSpace(rawGame.HomeTeam.DivisionName)
	visitorDivision := strings.TrimSpace(rawGame.VisitorTeam.DivisionName)
	homeTeamDivision := firstNonEmptyString(divisionName, homeDivision, visitorDivision)
	visitorTeamDivision := firstNonEmptyString(divisionName, visitorDivision, homeDivision)

	switch {
	case selectedTeamID > 0 && homeID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, homeName), visitorName, homeTeamDivision
	case selectedTeamID > 0 && visitorID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, visitorName), homeName, visitorTeamDivision
	}

	playerTeamName := firstNonEmptyString(selectedTeamName, homeName)
	if playerTeamName == visitorName {
		return playerTeamName, homeName, visitorTeamDivision
	}
	return playerTeamName, visitorName, homeTeamDivision
}
