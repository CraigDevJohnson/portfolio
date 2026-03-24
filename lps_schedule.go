package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type lpsUserPlayerDiscovery struct {
	UserName string
	Players  []LPSPlayer
}

type lpsUserCheckResponse struct {
	AuthFailure bool        `json:"authFailure"`
	Error       string      `json:"error"`
	FirstName   string      `json:"first_name"`
	LastName    string      `json:"last_name"`
	Players     []LPSPlayer `json:"players"`
	UserPlayers []struct {
		PlayerID int  `json:"player_id"`
		Deleted  bool `json:"deleted"`
	} `json:"user_players"`
}

type lpsTeamSummary struct {
	UTeamID      int    `json:"UTeamID"`
	TeamName     string `json:"team_name"`
	DivisionName string `json:"division_name"`
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"facility_name"`
	Season       int    `json:"Season"`
}

type lpsTeamScheduleGame struct {
	UGameID           int            `json:"UGameID"`
	FieldName         string         `json:"field_name"`
	SchedGameDateTime string         `json:"SchedGameDateTime"`
	SchedGameEndTime  *string        `json:"schedGameEndTime"`
	FacilityName      string         `json:"facilityName"`
	Result            string         `json:"result"`
	Field             int            `json:"Field"`
	Season            int            `json:"Season"`
	FacilityID        int            `json:"FacilityID"`
	UTeam1            int            `json:"UTeam1"`
	UTeam2            int            `json:"UTeam2"`
	TeamIDSelected    *int           `json:"team_id_selected"`
	HomeTeam          lpsTeamSummary `json:"home_team"`
	VisitorTeam       lpsTeamSummary `json:"visitor_team"`
}

type lpsTeamScheduleResponse struct {
	Games []lpsTeamScheduleGame `json:"games"`
	Team  lpsTeamSummary        `json:"team"`
}

type lpsFacility struct {
	FacilityID   int    `json:"FacilityID"`
	FacilityName string `json:"FacilityName"`
	Address      string `json:"Address"`
	City         string `json:"City"`
	State        string `json:"State"`
	ZIP          string `json:"ZIP"`
}

type lpsScheduleResolver struct {
	jwt           string
	facilityCache map[int]lpsFacility
}

func jwtExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(claims.Exp, 0)
}

func normalizeImportedJWT(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("Paste the bearer JWT from your Let's Play Soccer browser session.")
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("Paste a single JWT value without extra spaces or line breaks.")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
	}
	for _, segment := range parts[:2] {
		if segment == "" {
			return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
		}
		if _, err := base64.RawURLEncoding.DecodeString(segment); err != nil {
			return "", errors.New("The imported JWT format is not valid base64url data.")
		}
	}

	expiresAt := jwtExpiry(token)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", errors.New("This JWT has expired. Copy a fresh bearer token from letsplaysoccer.com and import it again.")
	}

	return token, nil
}

func importedSessionExpiry(token string) time.Time {
	deadline := time.Now().Add(defaultSessionTTL)
	expiresAt := jwtExpiry(token)
	if expiresAt.IsZero() || expiresAt.After(deadline) {
		return deadline
	}
	return expiresAt
}

func lpsFetchUserPlayers(ctx context.Context, jwt string) (lpsUserPlayerDiscovery, error) {
	var discovery lpsUserPlayerDiscovery

	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "users", "check")
	if err != nil {
		return discovery, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not reach Let's Play Soccer while loading players: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "could not read the player lookup response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return discovery, newLPSFetchError(lpsErrorUnauthorized, 0, resp.StatusCode, "Let's Play Soccer rejected the imported token with status 401")
	}
	if resp.StatusCode == http.StatusForbidden {
		return discovery, newLPSFetchError(lpsErrorForbidden, 0, resp.StatusCode, "Let's Play Soccer denied access to the player lookup with status 403")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, resp.StatusCode, "Let's Play Soccer returned status %d while loading players", resp.StatusCode)
	}

	discovery, err = decodeLPSUserPlayers(responseBody)
	if err != nil {
		var fetchErr *lpsFetchError
		if errors.As(err, &fetchErr) {
			return discovery, err
		}
		return discovery, newLPSFetchError(lpsErrorUpstream, 0, http.StatusBadGateway, "%v", err)
	}

	return discovery, nil
}

func lpsFetchGamesForPlayers(ctx context.Context, jwt string, playerIDs []int) ([]Game, error) {
	normalizedJWT, err := normalizeImportedJWT(jwt)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorMalformedToken, 0, http.StatusUnauthorized, "the imported JWT is malformed: %v", err)
	}

	resolver := newLPSScheduleResolver(normalizedJWT)
	teamByID := make(map[int]lpsTeamSummary)
	for _, playerID := range sortedUniqueIDs(playerIDs) {
		playerTeams, err := resolver.fetchPlayerTeams(ctx, playerID)
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

	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range teamIDs {
		team := teamByID[teamID]
		teamGames, err := resolver.fetchTeamGames(ctx, teamID, &team)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, teamGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

func lpsFetchGamesForTeams(ctx context.Context, teamIDs []int) ([]Game, error) {
	resolver := newLPSScheduleResolver("")
	games := make([]Game, 0)
	indexByKey := make(map[string]int)
	for _, teamID := range sortedUniqueIDs(teamIDs) {
		teamGames, err := resolver.fetchTeamGames(ctx, teamID, nil)
		if err != nil {
			return nil, err
		}
		games = mergeScheduleGames(games, teamGames, indexByKey)
	}
	sortScheduleGames(games)
	return games, nil
}

func newLPSScheduleResolver(jwt string) *lpsScheduleResolver {
	return &lpsScheduleResolver{
		jwt:           jwt,
		facilityCache: make(map[int]lpsFacility),
	}
}

func lpsFetchUpcomingGames(ctx context.Context, normalizedJWT string, playerID int) ([]Game, error) {
	if playerID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, normalizedJWT, "players", strconv.Itoa(playerID), "upcoming_games")
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not read the schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, newLPSFetchError(lpsErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, newLPSFetchError(lpsErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find upcoming games for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading schedules", resp.StatusCode)
	}

	games, err := decodeLPSGames(responseBody)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "%v", err)
	}
	normalizeScheduleGames(games)
	return games, nil
}

func lpsFetchTeamGames(ctx context.Context, teamID int) ([]Game, error) {
	return newLPSScheduleResolver("").fetchTeamGames(ctx, teamID, nil)
}

func (resolver *lpsScheduleResolver) fetchPlayerTeams(ctx context.Context, playerID int) ([]lpsTeamSummary, error) {
	if playerID <= 0 {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, http.StatusBadRequest, "player ID %d is invalid", playerID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, resolver.jwt, "players", strconv.Itoa(playerID), "my_teams")
	if err != nil {
		return nil, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading player teams: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "could not read the player teams response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, newLPSFetchError(lpsErrorUnauthorized, playerID, resp.StatusCode, "Let's Play Soccer rejected the imported token for player %d with status 401", playerID)
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, newLPSFetchError(lpsErrorForbidden, playerID, resp.StatusCode, "Let's Play Soccer denied access to player %d with status 403", playerID)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return nil, newLPSFetchError(lpsErrorInvalidPlayer, playerID, resp.StatusCode, "Let's Play Soccer could not find teams for player %d", playerID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, resp.StatusCode, "Let's Play Soccer returned status %d while loading player teams", resp.StatusCode)
	}

	var teams []lpsTeamSummary
	if err := json.Unmarshal(responseBody, &teams); err != nil {
		return nil, newLPSFetchError(lpsErrorUpstream, playerID, http.StatusBadGateway, "The player teams response format was not recognized.")
	}

	sort.Slice(teams, func(i, j int) bool {
		if teams[i].UTeamID != teams[j].UTeamID {
			return teams[i].UTeamID < teams[j].UTeamID
		}
		return teams[i].TeamName < teams[j].TeamName
	})
	return teams, nil
}

func (resolver *lpsScheduleResolver) fetchTeamGames(ctx context.Context, teamID int, selectedTeam *lpsTeamSummary) ([]Game, error) {
	response, err := resolver.fetchTeamSchedule(ctx, teamID)
	if err != nil {
		return nil, err
	}

	games := make([]Game, 0, len(response.Games))
	for i := range response.Games {
		game, err := resolver.mapTeamScheduleGame(ctx, &response.Games[i], response.Team, selectedTeam)
		if err != nil {
			return nil, err
		}
		games = append(games, game)
	}

	normalizeScheduleGames(games)
	return upcomingScheduleGames(games), nil
}

func (resolver *lpsScheduleResolver) fetchTeamSchedule(ctx context.Context, teamID int) (lpsTeamScheduleResponse, error) {
	var schedule lpsTeamScheduleResponse
	if teamID <= 0 {
		return schedule, newLPSFetchError(lpsErrorInvalidTeam, teamID, http.StatusBadRequest, "team ID %d is invalid", teamID)
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, "", "teams", strconv.Itoa(teamID))
	if err != nil {
		return schedule, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading team schedules: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "could not read the team schedule response: %w", err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return schedule, newLPSFetchError(lpsErrorInvalidTeam, teamID, resp.StatusCode, "Let's Play Soccer could not find team %d", teamID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, resp.StatusCode, "Let's Play Soccer returned status %d while loading team schedules", resp.StatusCode)
	}

	if err := json.Unmarshal(responseBody, &schedule); err != nil {
		return schedule, newLPSFetchError(lpsErrorUpstream, teamID, http.StatusBadGateway, "The team schedule response format was not recognized.")
	}
	return schedule, nil
}

func (resolver *lpsScheduleResolver) mapTeamScheduleGame(ctx context.Context, rawGame *lpsTeamScheduleGame, responseTeam lpsTeamSummary, selectedTeam *lpsTeamSummary) (Game, error) {
	if rawGame == nil {
		return Game{}, nil
	}

	var selected lpsTeamSummary
	if selectedTeam != nil {
		selected = *selectedTeam
	}

	facilityID := firstPositiveInt(rawGame.FacilityID, selected.FacilityID, responseTeam.FacilityID, rawGame.HomeTeam.FacilityID, rawGame.VisitorTeam.FacilityID)
	facilityName := firstNonEmptyString(rawGame.FacilityName, selected.FacilityName, responseTeam.FacilityName, rawGame.HomeTeam.FacilityName, rawGame.VisitorTeam.FacilityName)
	facility, err := resolver.fetchFacility(ctx, facilityID)
	if err != nil {
		return Game{}, err
	}
	if strings.TrimSpace(facility.FacilityName) != "" {
		facilityName = strings.TrimSpace(facility.FacilityName)
	}

	fieldName := strings.TrimSpace(rawGame.FieldName)
	if fieldName == "" && rawGame.Field > 0 {
		fieldName = fmt.Sprintf("Field %d", rawGame.Field)
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

	game := Game{
		ID:               intString(rawGame.UGameID),
		DateTime:         formatGameDateTime(normalizeLPSScheduleTime(rawGame.SchedGameDateTime)),
		StartAt:          normalizeLPSScheduleTime(rawGame.SchedGameDateTime),
		EndAt:            normalizeLPSScheduleTime(stringPointerValue(rawGame.SchedGameEndTime)),
		Field:            fieldName,
		Location:         strings.TrimSpace(facilityName),
		Home:             homeName,
		Away:             visitorName,
		Season:           firstNonEmptyString(intString(selected.Season), intString(responseTeam.Season), intString(rawGame.Season), intString(rawGame.HomeTeam.Season), intString(rawGame.VisitorTeam.Season)),
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

func (resolver *lpsScheduleResolver) fetchFacility(ctx context.Context, facilityID int) (lpsFacility, error) {
	if facilityID <= 0 {
		return lpsFacility{}, nil
	}
	if facility, ok := resolver.facilityCache[facilityID]; ok {
		return facility, nil
	}

	req, err := newLPSAPIRequest(ctx, http.MethodGet, "", "facilities", strconv.Itoa(facilityID))
	if err != nil {
		return lpsFacility{}, err
	}

	resp, err := doLPSAPIRequest(req)
	if err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "could not reach Let's Play Soccer while loading facility %d: %w", facilityID, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxLPSResponseBodySize))
	if err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "could not read the facility response for facility %d: %w", facilityID, err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer could not find facility %d", facilityID)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, resp.StatusCode, "Let's Play Soccer returned status %d while loading facility %d", resp.StatusCode, facilityID)
	}

	var facility lpsFacility
	if err := json.Unmarshal(responseBody, &facility); err != nil {
		return lpsFacility{}, newLPSFetchError(lpsErrorUpstream, facilityID, http.StatusBadGateway, "The facility response format was not recognized.")
	}
	resolver.facilityCache[facilityID] = facility
	return facility, nil
}

func resolveSelectedTeamMatchup(rawGame *lpsTeamScheduleGame, responseTeam lpsTeamSummary, selectedTeam *lpsTeamSummary) (string, string, string) {
	if rawGame == nil {
		return "", "", ""
	}

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

	switch {
	case selectedTeamID > 0 && homeID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, homeName), visitorName, firstNonEmptyString(divisionName, homeDivision, visitorDivision)
	case selectedTeamID > 0 && visitorID == selectedTeamID:
		return firstNonEmptyString(selectedTeamName, visitorName), homeName, firstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}

	playerTeamName := firstNonEmptyString(selectedTeamName, homeName)
	if playerTeamName == visitorName {
		return playerTeamName, homeName, firstNonEmptyString(divisionName, visitorDivision, homeDivision)
	}
	return playerTeamName, visitorName, firstNonEmptyString(divisionName, homeDivision, visitorDivision)
}
