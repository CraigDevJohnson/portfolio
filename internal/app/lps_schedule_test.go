package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/schedule"
)

func TestLPSFetchUpcomingGamesMapsFlexiblePayload(t *testing.T) {
	app := newTestApp(t)
	token := testJWT(t, time.Now().Add(30*time.Minute))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/players/1001/upcoming_games" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"UGameID": 9001,
				"StartDateTime": "2026-01-11T14:55:00-07:00",
				"EndDateTime": "2026-01-11T16:25:00-07:00",
				"FieldName": "3",
				"HomeTeam": "YOUR TEAM",
				"AwayTeam": "OPPONENT A",
				"SeasonID": 168
			}
		]`))
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	games, err := lps.FetchUpcomingGames(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, token, 1001)
	if err != nil {
		t.Fatalf("lpsFetchUpcomingGames returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected games length: %d", len(games))
	}
	if games[0].Home != "YOUR TEAM" || games[0].Away != "OPPONENT A" {
		t.Fatalf("unexpected game teams: %#v", games[0])
	}
	if !strings.Contains(games[0].DateTime, "01/11/26") {
		t.Fatalf("unexpected display datetime: %s", games[0].DateTime)
	}
	if games[0].Location != "Field 3" {
		t.Fatalf("unexpected location: %s", games[0].Location)
	}
}

func TestLPSFetchUpcomingGamesMapsLivePayloadShape(t *testing.T) {
	app := newTestApp(t)
	token := testJWT(t, time.Now().Add(30*time.Minute))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/players/1001/upcoming_games" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"UGameID": 734433,
				"field_name": "Arena 7",
				"Field": "7",
				"SchedGameDateTime": "2026-04-12T19:10:00-06:00",
				"schedGameEndTime": null,
				"facilityName": "LetsPlay North Campus",
				"SeasonID": 244,
				"home_team": {
					"TeamName": "Craig FC"
				},
				"visitor_team": {
					"TeamName": "Visitors United"
				}
			}
		]`))
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	games, err := lps.FetchUpcomingGames(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, token, 1001)
	if err != nil {
		t.Fatalf("lpsFetchUpcomingGames returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected games length: %d", len(games))
	}
	if games[0].ID != "734433" {
		t.Fatalf("unexpected game ID: %s", games[0].ID)
	}
	if games[0].Home != "Craig FC" || games[0].Away != "Visitors United" {
		t.Fatalf("unexpected game teams: %#v", games[0])
	}
	if games[0].StartAt != "2026-04-12T19:10:00-06:00" {
		t.Fatalf("unexpected start time: %s", games[0].StartAt)
	}
	if games[0].EndAt != "" {
		t.Fatalf("expected empty end time for null payload value, got %q", games[0].EndAt)
	}
	if games[0].Field != "Arena 7" {
		t.Fatalf("unexpected field: %s", games[0].Field)
	}
	if games[0].Location != "LetsPlay North Campus" {
		t.Fatalf("unexpected location: %s", games[0].Location)
	}
	if games[0].Season != "244" {
		t.Fatalf("unexpected season: %s", games[0].Season)
	}
	if !strings.Contains(games[0].DateTime, "04/12/26") {
		t.Fatalf("unexpected display datetime: %s", games[0].DateTime)
	}
}

func TestLPSFetchGamesForPlayersResolvesPlayerTeamsAndFacilityDetails(t *testing.T) {
	app := newTestApp(t)
	token := testJWT(t, time.Now().Add(30*time.Minute))
	requestCounts := map[string]int{}
	future := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	expectedStart, ok := schedule.ParseScheduleTime(future)
	if !ok {
		t.Fatalf("schedule.ParseScheduleTime returned false for %q", future)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCounts[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/my_teams":
			if got := r.Header.Get("Authorization"); got != "Bearer "+token {
				t.Fatalf("unexpected authorization header: %s", got)
			}
			_, _ = w.Write([]byte(`[
				{
					"UTeamID": 479393,
					"team_name": "STRUGGLE BUS",
					"division_name": "Coed Over 30 B Sun",
					"FacilityID": 4,
					"facility_name": "Boise",
					"Season": 169
				}
			]`))
		case "/teams/479393":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 3037322,
						"field_name": "Field 2",
						"SchedGameDateTime": %q,
						"schedGameEndTime": null,
						"facilityName": "Boise",
						"result": "7 - 3",
						"Field": 2,
						"FacilityID": 4,
						"UTeam1": 479830,
						"UTeam2": 479393,
						"home_team": {
							"UTeamID": 479830,
							"team_name": "FC CHAIN MAIL",
							"division_name": "Coed Over 30 B Sun",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						},
						"visitor_team": {
							"UTeamID": 479393,
							"team_name": "STRUGGLE BUS",
							"division_name": "Coed Over 30 B Sun",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						}
					}
				],
				"team": {
					"UTeamID": 479393,
					"team_name": "STRUGGLE BUS",
					"division_name": "Coed Over 30 B Sun",
					"FacilityID": 4,
					"facility_name": "Boise",
					"Season": 169
				}
			}`, future)))
		case "/facilities/4":
			_, _ = w.Write([]byte(`{
				"FacilityID": 4,
				"FacilityName": "Boise Indoor Soccer",
				"Address": "11448 W. President Drive",
				"City": "Boise",
				"State": "ID",
				"ZIP": "83713"
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	games, err := lps.FetchGamesForPlayers(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, token, []int{1001})
	if err != nil {
		t.Fatalf("lpsFetchGamesForPlayers returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected games length: got %d want 1", len(games))
	}
	if got := requestCounts["/players/1001/my_teams"]; got != 1 {
		t.Fatalf("unexpected my_teams call count: got %d want 1", got)
	}
	if got := requestCounts["/teams/479393"]; got != 1 {
		t.Fatalf("unexpected team lookup call count: got %d want 1", got)
	}
	if got := requestCounts["/facilities/4"]; got != 1 {
		t.Fatalf("unexpected facility lookup call count: got %d want 1", got)
	}

	game := games[0]
	if game.ID != "3037322" {
		t.Fatalf("unexpected game ID: %q", game.ID)
	}
	if game.PlayerTeamName != "STRUGGLE BUS" {
		t.Fatalf("unexpected player team name: %q", game.PlayerTeamName)
	}
	if game.OpponentTeamName != "FC CHAIN MAIL" {
		t.Fatalf("unexpected opponent team name: %q", game.OpponentTeamName)
	}
	if game.DivisionName != "Coed Over 30 B Sun" {
		t.Fatalf("unexpected division name: %q", game.DivisionName)
	}
	if game.FacilityName != "Boise Indoor Soccer" {
		t.Fatalf("unexpected facility name: %q", game.FacilityName)
	}
	if game.FacilityAddress != "11448 W. President Drive" || game.FacilityCity != "Boise" || game.FacilityState != "ID" || game.FacilityZIP != "83713" {
		t.Fatalf("unexpected facility details: %#v", game)
	}
	if game.Result != "7 - 3" {
		t.Fatalf("unexpected result: %q", game.Result)
	}
	if game.StartAt != expectedStart.Format(time.RFC3339) {
		t.Fatalf("unexpected start time: %q", game.StartAt)
	}
}

func TestLPSFetchGamesForTeamsCachesFacilityLookupsPerRequest(t *testing.T) {
	app := newTestApp(t)
	lookupCounts := map[string]int{}
	futureOne := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookupCounts[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 7001,
						"SchedGameDateTime": %q,
						"field_name": "Field 3",
						"facilityName": "Boise",
						"FacilityID": 4,
						"UTeam1": 479691,
						"UTeam2": 479700,
						"home_team": {
							"UTeamID": 479691,
							"team_name": "UNITED NATIONS",
							"division_name": "Coed F Fri",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						},
						"visitor_team": {
							"UTeamID": 479700,
							"team_name": "GALACTICOS FC",
							"division_name": "Coed F Fri",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						}
					},
					{
						"UGameID": 7002,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"FacilityID": 4,
						"UTeam1": 479691,
						"UTeam2": 479701,
						"home_team": {
							"UTeamID": 479691,
							"team_name": "UNITED NATIONS",
							"division_name": "Coed F Fri",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						},
						"visitor_team": {
							"UTeamID": 479701,
							"team_name": "RIVALS",
							"division_name": "Coed F Fri",
							"FacilityID": 4,
							"facility_name": "Boise",
							"Season": 169
						}
					}
				],
				"team": {
					"UTeamID": 479691,
					"team_name": "UNITED NATIONS",
					"division_name": "Coed F Fri",
					"FacilityID": 4,
					"facility_name": "Boise",
					"Season": 169
				}
			}`, futureOne, futureTwo)))
		case "/facilities/4":
			_, _ = w.Write([]byte(`{
				"FacilityID": 4,
				"FacilityName": "Boise Indoor Soccer",
				"Address": "11448 W. President Drive",
				"City": "Boise",
				"State": "ID",
				"ZIP": "83713"
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	games, err := lps.FetchGamesForTeams(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, []int{479691})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams returned error: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("unexpected games length: got %d want 2", len(games))
	}
	if got := lookupCounts["/facilities/4"]; got != 1 {
		t.Fatalf("expected one cached facility lookup, got %d", got)
	}
	if games[0].FacilityAddress != "11448 W. President Drive" {
		t.Fatalf("unexpected facility address: %q", games[0].FacilityAddress)
	}
	if games[0].PlayerTeamName != "UNITED NATIONS" || games[0].OpponentTeamName != "GALACTICOS FC" {
		t.Fatalf("unexpected team matchup: %#v", games[0])
	}
}

func TestLPSFetchGamesForPlayersRejectsMalformedTokenBeforeRequest(t *testing.T) {
	app := newTestApp(t)
	_, err := lps.FetchGamesForPlayers(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, "not-a-jwt", []int{1001})
	if err == nil {
		t.Fatal("expected malformed token error")
	}
	var fetchErr *lps.FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected lps.FetchError, got %T", err)
	}
	if fetchErr.Kind != lps.ErrorMalformedToken {
		t.Fatalf("unexpected error kind: %s", fetchErr.Kind)
	}
}

func TestLPSFetchGamesForTeamsFiltersPastGamesAndDeduplicates(t *testing.T) {
	app := newTestApp(t)
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	past := testMislabelledLPSZuluTime(time.Now().Add(-2 * time.Hour))
	sharedFuture := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	uniqueFuture := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/teams/479691":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 5001,
						"SchedGameDateTime": %q,
						"field_name": "Field 9",
						"facilityName": "Boise",
						"home_team": {"team_name": "Past FC"},
						"visitor_team": {"team_name": "Finished"},
						"Season": 169
					},
					{
						"UGameID": 6000,
						"SchedGameDateTime": %q,
						"home_team": {"team_name": "Shared FC"},
						"visitor_team": {"team_name": "Opponents"},
						"Season": 169
					},
					{
						"UGameID": 6001,
						"SchedGameDateTime": %q,
						"field_name": "Field 2",
						"facilityName": "Boise",
						"home_team": {"team_name": "Team One"},
						"visitor_team": {"team_name": "Visitors"},
						"Season": 169
					}
				]
			}`, past, sharedFuture, uniqueFuture)))
		case "/teams/479147":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 6000,
						"SchedGameDateTime": %q,
						"field_name": "Field 3",
						"facilityName": "Main Complex",
						"home_team": {"team_name": "Shared FC"},
						"visitor_team": {"team_name": "Opponents"},
						"Season": 169
					},
					{
						"UGameID": 6002,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"home_team": {"team_name": "Team Two"},
						"visitor_team": {"team_name": "Rivals"},
						"Season": 169
					}
				]
			}`, sharedFuture, uniqueFuture)))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	games, err := lps.FetchGamesForTeams(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, []int{479691, 479147})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams returned error: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("unexpected deduped games length: got %d want 3", len(games))
	}
	for _, game := range games {
		if game.Home == "Past FC" {
			t.Fatalf("expected past games to be filtered out, got %#v", game)
		}
	}
	if games[0].Home != "Shared FC" || games[0].Field != "Field 3" || games[0].Location != "Main Complex" {
		t.Fatalf("expected shared game to merge richer data, got %#v", games[0])
	}

	gamesReversed, err := lps.FetchGamesForTeams(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, []int{479147, 479691})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams (reversed teams) returned error: %v", err)
	}
	if !reflect.DeepEqual(games, gamesReversed) {
		t.Fatalf("expected deterministic merge regardless of team order:\noriginal: %#v\nreversed: %#v", games, gamesReversed)
	}
}

func TestLPSFetchUpcomingGamesClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		playerID   int
		wantKind   lps.ErrorKind
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, playerID: 1001, wantKind: lps.ErrorUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, playerID: 1001, wantKind: lps.ErrorForbidden},
		{name: "invalid player bad request", statusCode: http.StatusBadRequest, playerID: 999999, wantKind: lps.ErrorInvalidPlayer},
		{name: "invalid player not found", statusCode: http.StatusNotFound, playerID: 999999, wantKind: lps.ErrorInvalidPlayer},
		{name: "upstream outage", statusCode: http.StatusBadGateway, playerID: 1001, wantKind: lps.ErrorUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			app.Config.LPSAPIBaseURL = server.URL

			_, err := lps.FetchUpcomingGames(t.Context(), app.Config.LPSAPIBaseURL, app.LPSClient, testJWT(t, time.Now().Add(30*time.Minute)), tt.playerID)
			if err == nil {
				t.Fatal("expected fetch error")
			}
			var fetchErr *lps.FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected lps.FetchError, got %T", err)
			}
			if fetchErr.Kind != tt.wantKind {
				t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, tt.wantKind)
			}
		})
	}
}
