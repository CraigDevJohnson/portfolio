package lps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

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

// TeamScheduleSource retains the raw facts alongside rendered schedule games.
type TeamScheduleSource struct {
	TeamID     int
	Response   TeamScheduleResponse
	Facilities []FacilityResponse
	FetchedAt  time.Time
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

// FetchGamesForPlayers resolves teams for the selected players and returns upcoming merged games.
func FetchGamesForPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string, playerIDs []int) ([]types.Game, error) {
	games, err := FetchAllGamesForPlayers(ctx, baseURL, httpClient, jwt, playerIDs)
	if err != nil {
		return nil, err
	}
	return schedule.UpcomingScheduleGames(games), nil
}

// FetchAllGamesForPlayers resolves teams for the selected players and merges all schedule games.
func FetchAllGamesForPlayers(ctx context.Context, baseURL string, httpClient *http.Client, jwt string, playerIDs []int) ([]types.Game, error) {
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

// FetchGamesForTeams loads and merges upcoming schedules for the selected team IDs.
func FetchGamesForTeams(ctx context.Context, baseURL string, httpClient *http.Client, teamIDs []int) ([]types.Game, error) {
	games, err := FetchAllGamesForTeams(ctx, baseURL, httpClient, teamIDs)
	if err != nil {
		return nil, err
	}
	return schedule.UpcomingScheduleGames(games), nil
}

// FetchAllGamesForTeams loads and merges all schedules for the selected team IDs.
func FetchAllGamesForTeams(ctx context.Context, baseURL string, httpClient *http.Client, teamIDs []int) ([]types.Game, error) {
	resolver := NewScheduleResolver(baseURL, httpClient, "")
	return resolver.mergeTeamSchedules(ctx, sortedUniqueIDs(teamIDs), nil)
}

// FetchAllGamesForTeamsWithSource returns the same normalized games and each
// team's raw LPS response without fetching a team twice.
func FetchAllGamesForTeamsWithSource(ctx context.Context, baseURL string, httpClient *http.Client, teamIDs []int) ([]types.Game, []TeamScheduleSource, error) {
	resolver := NewScheduleResolver(baseURL, httpClient, "")
	teamIDs = sortedUniqueIDs(teamIDs)
	sources := make([]TeamScheduleSource, 0, len(teamIDs))
	schedules := make([]TeamScheduleResponse, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		response, err := resolver.FetchTeamSchedule(ctx, teamID)
		if err != nil {
			return nil, nil, err
		}
		// The caller decides whether Response.Team confirms teamID, so the source
		// keeps the response exactly as LPS sent it; the games still belong to the
		// visitor's usual schedule either way, mapped from the schedule's own copy.
		sources = append(sources, TeamScheduleSource{TeamID: teamID, Response: response, FetchedAt: time.Now().UTC()})
		schedules = append(schedules, selectedTeamSchedule(response, teamID))
	}
	teamGames, err := resolver.mapSelectedTeamSchedules(ctx, teamIDs, nil, schedules)
	if err != nil {
		return nil, nil, err
	}
	for i := range sources {
		if err := resolver.fetchSourceFacilities(ctx, &sources[i], teamGames[i]); err != nil {
			return nil, nil, err
		}
	}
	return mergeTeamGames(teamGames), sources, nil
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

// FetchTeamGames loads, maps, and normalizes a team's schedule games.
func (resolver *ScheduleResolver) FetchTeamGames(ctx context.Context, teamID int, selectedTeam *TeamSummary) ([]types.Game, error) {
	response, err := resolver.fetchSelectedTeamSchedule(ctx, teamID)
	if err != nil {
		return nil, err
	}
	colors := selectedTeamColors([]int{teamID}, map[int]*TeamSummary{teamID: selectedTeam}, []TeamScheduleResponse{response})
	return resolver.mapTeamGames(ctx, &response, selectedTeam, colors)
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
		statusErrorKind{codes: []int{http.StatusUnauthorized, http.StatusForbidden}, kind: ErrorTeamRefused},
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
// teamColors holds the approved color resolved for each fetched Team ID; a side
// whose team was not fetched uses the color LPS nests on this game.
func (resolver *ScheduleResolver) MapTeamScheduleGame(ctx context.Context, rawGame *TeamScheduleGame, responseTeam, selectedTeam *TeamSummary, teamColors map[int]string) (types.Game, error) {
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
	gameFacility := types.NewFacilityDetails(
		facilityID,
		facilityName,
		facility.Address,
		facility.City,
		facility.State,
		facility.ZIP,
	)
	endAt := ""
	if rawGame.SchedGameEndTime != nil {
		endAt = strings.TrimSpace(*rawGame.SchedGameEndTime)
	}

	fieldName := strings.TrimSpace(rawGame.FieldName)
	if fieldName == "" && rawGame.Field > 0 {
		fieldName = "Field " + strconv.Itoa(rawGame.Field)
	}

	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	homeTeamID := firstPositiveInt(rawGame.HomeTeam.UTeamID, rawGame.UTeam1)
	awayTeamID := firstPositiveInt(rawGame.VisitorTeam.UTeamID, rawGame.UTeam2)
	selectedTeamID := firstPositiveInt(selected.UTeamID, responseTeam.UTeamID)
	if selectedTeamID <= 0 && rawGame.TeamIDSelected != nil {
		selectedTeamID = *rawGame.TeamIDSelected
	}
	selectedTeamName := strings.TrimSpace(firstNonEmptyString(selected.TeamName, responseTeam.TeamName))
	selectedColor := firstApprovedTeamColor(selected.Color, responseTeam.Color)
	// Identify the selected team's side once; colors and the result perspective
	// both follow this answer.
	homeSelected, awaySelected := selectedMatchSides(selectedTeamID, selectedTeamName, homeTeamID, awayTeamID, homeName, visitorName)
	homeColor := firstNonEmptyString(teamColors[homeTeamID], approvedTeamColor(rawGame.HomeTeam.Color))
	awayColor := firstNonEmptyString(teamColors[awayTeamID], approvedTeamColor(rawGame.VisitorTeam.Color))
	if homeSelected {
		homeTeamID = firstPositiveInt(homeTeamID, selectedTeamID)
		homeColor = selectedColor
	}
	if awaySelected {
		awayTeamID = firstPositiveInt(awayTeamID, selectedTeamID)
		awayColor = selectedColor
	}
	playerTeamName, opponentTeamName, divisionName := resolveSelectedTeamMatchup(rawGame, responseTeam, &selected, homeSelected, awaySelected)
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
		EndAt:            schedule.NormalizeLPSScheduleTime(endAt),
		Field:            fieldName,
		Location:         strings.TrimSpace(facilityName),
		Home:             homeName,
		Away:             visitorName,
		HomeTeam:         types.TeamAppearance{ID: homeTeamID, Color: homeColor, Selected: homeSelected},
		AwayTeam:         types.TeamAppearance{ID: awayTeamID, Color: awayColor, Selected: awaySelected},
		ScheduleTeam:     types.TeamAppearance{ID: selectedTeamID, Color: selectedColor},
		Season:           firstNonEmptyString(intString(selected.Season), intString(responseTeam.Season), intString(rawGame.Season), intString(rawGame.HomeTeam.Season), intString(rawGame.VisitorTeam.Season)),
		PlayerTeamName:   playerTeamName,
		OpponentTeamName: opponentTeamName,
		DivisionName:     divisionName,
		Facility:         gameFacility,
		Result:           strings.TrimSpace(rawGame.Result),
	}

	return game, nil
}

// selectedTeamColors resolves one display color per fetched Team ID, so every
// row agrees on a team's color. For each team the first approved color wins
// from: its own team-level colors, colors LPS nests for it on its own games,
// then colors nested for it in the other fetched schedules, in Team ID order.
// A team with none is absent and receives its Team ID fallback in the view.
func selectedTeamColors(teamIDs []int, teamLookup map[int]*TeamSummary, schedules []TeamScheduleResponse) map[int]string {
	colors := make(map[int]string, len(teamIDs))
	for i, teamID := range teamIDs {
		candidates := []string{schedules[i].Team.Color}
		if selected := teamLookup[teamID]; selected != nil {
			candidates = append([]string{selected.Color}, candidates...)
		}
		candidates = append(candidates, nestedTeamColors(teamID, &schedules[i])...)
		if color := firstApprovedTeamColor(candidates...); color != "" {
			colors[teamID] = color
		}
	}
	for i, teamID := range teamIDs {
		if colors[teamID] != "" {
			continue
		}
		var candidates []string
		for j := range schedules {
			if j != i {
				candidates = append(candidates, nestedTeamColors(teamID, &schedules[j])...)
			}
		}
		if color := firstApprovedTeamColor(candidates...); color != "" {
			colors[teamID] = color
		}
	}
	return colors
}

// nestedTeamColors lists the colors a schedule nests on game sides whose Team
// ID is teamID.
func nestedTeamColors(teamID int, response *TeamScheduleResponse) []string {
	var colors []string
	for i := range response.Games {
		game := &response.Games[i]
		if firstPositiveInt(game.HomeTeam.UTeamID, game.UTeam1) == teamID {
			colors = append(colors, game.HomeTeam.Color)
		}
		if firstPositiveInt(game.VisitorTeam.UTeamID, game.UTeam2) == teamID {
			colors = append(colors, game.VisitorTeam.Color)
		}
	}
	return colors
}

func selectedMatchSides(selectedID int, selectedName string, homeID, awayID int, homeName, awayName string) (home, away bool) {
	switch {
	case selectedID > 0 && homeID == selectedID:
		return true, false
	case selectedID > 0 && awayID == selectedID:
		return false, true
	case selectedName != "" && (selectedID <= 0 || homeID <= 0) && strings.EqualFold(selectedName, homeName):
		return true, false
	case selectedName != "" && (selectedID <= 0 || awayID <= 0) && strings.EqualFold(selectedName, awayName):
		return false, true
	case selectedName == "" && homeID <= 0 && awayID <= 0:
		// Nothing identifies either side, so keep the historical home default.
		// A known name that matches neither side is not guessed.
		return true, false
	default:
		return false, false
	}
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

// fetchSelectedTeamSchedule loads a selected team's schedule and makes sure its
// team summary carries the requested Team ID.
func (resolver *ScheduleResolver) fetchSelectedTeamSchedule(ctx context.Context, teamID int) (TeamScheduleResponse, error) {
	response, err := resolver.FetchTeamSchedule(ctx, teamID)
	if err != nil {
		return response, err
	}
	return selectedTeamSchedule(response, teamID), nil
}

// selectedTeamSchedule returns a copy of a selected team's schedule whose team
// summary carries the requested Team ID.
func selectedTeamSchedule(response TeamScheduleResponse, teamID int) TeamScheduleResponse {
	if response.Team.UTeamID <= 0 {
		response.Team.UTeamID = teamID
	}
	return response
}

// mapTeamGames maps one selected team's schedule, painting every fetched team
// with the one color resolved for its Team ID.
func (resolver *ScheduleResolver) mapTeamGames(ctx context.Context, response *TeamScheduleResponse, selectedTeam *TeamSummary, teamColors map[int]string) ([]types.Game, error) {
	response.Team.Color = teamColors[response.Team.UTeamID]
	games := make([]types.Game, 0, len(response.Games))
	for i := range response.Games {
		game, err := resolver.MapTeamScheduleGame(ctx, &response.Games[i], &response.Team, selectedTeam, teamColors)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	schedule.NormalizeScheduleGames(games)
	return games, nil
}

// fetchSourceFacilities records the facilities behind a team's mapped games,
// plus a confirmed team's own facility, on its archive source.
func (resolver *ScheduleResolver) fetchSourceFacilities(ctx context.Context, source *TeamScheduleSource, games []types.Game) error {
	// Mapping already fetched (and cached) each game's facility, so those
	// lookups fail the request exactly as they do on the ordinary path.
	facilityIDs := make(map[int]struct{})
	for i := range games {
		game := &games[i]
		if game.Facility != nil && game.Facility.ID > 0 {
			facilityIDs[game.Facility.ID] = struct{}{}
		}
	}
	for facilityID := range facilityIDs {
		facility, err := resolver.FetchFacility(ctx, facilityID)
		if err != nil {
			return err
		}
		source.Facilities = append(source.Facilities, facility)
	}
	// A confirmed team's own facility is archive-only context that the
	// visitor's schedule does not use: a failed lookup leaves it out instead
	// of taking the schedule away.
	if team := source.Response.Team; team.UTeamID == source.TeamID && team.FacilityID > 0 {
		if _, fetched := facilityIDs[team.FacilityID]; !fetched {
			if facility, err := resolver.FetchFacility(ctx, team.FacilityID); err == nil {
				source.Facilities = append(source.Facilities, facility)
			}
		}
	}
	sort.Slice(source.Facilities, func(i, j int) bool { return source.Facilities[i].FacilityID < source.Facilities[j].FacilityID })
	return nil
}

// mergeTeamSchedules fetches every selected schedule before mapping any row,
// so a team's color can come from another fetched schedule, then merges the
// mapped games into one deduplicated, sorted list.
func (resolver *ScheduleResolver) mergeTeamSchedules(ctx context.Context, teamIDs []int, teamLookup map[int]*TeamSummary) ([]types.Game, error) {
	schedules := make([]TeamScheduleResponse, 0, len(teamIDs))
	for _, teamID := range teamIDs {
		response, err := resolver.fetchSelectedTeamSchedule(ctx, teamID)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, response)
	}
	teamGames, err := resolver.mapSelectedTeamSchedules(ctx, teamIDs, teamLookup, schedules)
	if err != nil {
		return nil, err
	}
	return mergeTeamGames(teamGames), nil
}

// mapSelectedTeamSchedules maps each fetched schedule, in teamIDs order, with
// one color resolved per selected Team ID.
func (resolver *ScheduleResolver) mapSelectedTeamSchedules(ctx context.Context, teamIDs []int, teamLookup map[int]*TeamSummary, schedules []TeamScheduleResponse) ([][]types.Game, error) {
	colors := selectedTeamColors(teamIDs, teamLookup, schedules)
	teamGames := make([][]types.Game, len(teamIDs))
	for i, teamID := range teamIDs {
		games, err := resolver.mapTeamGames(ctx, &schedules[i], teamLookup[teamID], colors)
		if err != nil {
			return nil, err
		}
		teamGames[i] = games
	}
	return teamGames, nil
}

// mergeTeamGames merges each team's mapped games into one deduplicated,
// sorted list.
func mergeTeamGames(teamGames [][]types.Game) []types.Game {
	games := make([]types.Game, 0)
	indexByKey := make(map[string]int)
	for _, mapped := range teamGames {
		games = schedule.MergeScheduleGames(games, mapped, indexByKey)
	}
	schedule.SortScheduleGames(games)
	return games
}

// resolveSelectedTeamMatchup names the selected team, its opponent, and its
// division from the sides selectedMatchSides identified. When neither side was
// identified, the selected team's name stands in for calendar labels and the
// other side is treated as the opponent.
func resolveSelectedTeamMatchup(rawGame *TeamScheduleGame, responseTeam, selectedTeam *TeamSummary, homeSelected, awaySelected bool) (string, string, string) {
	selectedTeamName := strings.TrimSpace(responseTeam.TeamName)
	divisionName := strings.TrimSpace(responseTeam.DivisionName)
	if selectedTeam != nil {
		selectedTeamName = firstNonEmptyString(selectedTeam.TeamName, responseTeam.TeamName)
		divisionName = firstNonEmptyString(selectedTeam.DivisionName, responseTeam.DivisionName)
	}

	homeName := strings.TrimSpace(rawGame.HomeTeam.TeamName)
	visitorName := strings.TrimSpace(rawGame.VisitorTeam.TeamName)
	homeDivision := strings.TrimSpace(rawGame.HomeTeam.DivisionName)
	visitorDivision := strings.TrimSpace(rawGame.VisitorTeam.DivisionName)
	homeTeamDivision := firstNonEmptyString(divisionName, homeDivision, visitorDivision)
	visitorTeamDivision := firstNonEmptyString(divisionName, visitorDivision, homeDivision)

	switch {
	case homeSelected:
		return firstNonEmptyString(selectedTeamName, homeName), visitorName, homeTeamDivision
	case awaySelected:
		return firstNonEmptyString(selectedTeamName, visitorName), homeName, visitorTeamDivision
	}

	playerTeamName := firstNonEmptyString(selectedTeamName, homeName)
	if playerTeamName == visitorName {
		return playerTeamName, homeName, visitorTeamDivision
	}
	return playerTeamName, visitorName, homeTeamDivision
}
