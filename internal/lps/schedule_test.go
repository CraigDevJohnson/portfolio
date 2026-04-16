package lps

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"portfolio/internal/schedule"
	"portfolio/internal/testutil"
)

func TestLPSFetchGamesForPlayersResolvesPlayerTeamsAndFacilityDetails(t *testing.T) {
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	requestCounts := map[string]int{}
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
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

	client := &http.Client{Timeout: 5 * time.Second}
	games, err := FetchGamesForPlayers(t.Context(), server.URL, client, token, []int{1001})
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
	if game.Facility == nil {
		t.Fatal("expected facility details to be present")
	}
	if game.Facility.Name != "Boise Indoor Soccer" {
		t.Fatalf("unexpected facility name: %q", game.Facility.Name)
	}
	if game.Facility.Address != "11448 W. President Drive" || game.Facility.City != "Boise" || game.Facility.State != "ID" || game.Facility.ZIP != "83713" {
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
	lookupCounts := map[string]int{}
	futureOne := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
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

	client := &http.Client{Timeout: 5 * time.Second}
	games, err := FetchGamesForTeams(t.Context(), server.URL, client, []int{479691})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams returned error: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("unexpected games length: got %d want 2", len(games))
	}
	if got := lookupCounts["/facilities/4"]; got != 1 {
		t.Fatalf("expected one cached facility lookup, got %d", got)
	}
	if games[0].Facility == nil {
		t.Fatal("expected facility details to be present")
	}
	if games[0].Facility.Address != "11448 W. President Drive" {
		t.Fatalf("unexpected facility address: %q", games[0].Facility.Address)
	}
	if games[0].PlayerTeamName != "UNITED NATIONS" || games[0].OpponentTeamName != "GALACTICOS FC" {
		t.Fatalf("unexpected team matchup: %#v", games[0])
	}
}

func TestLPSFetchGamesForPlayersUsesProvidedJWTWithoutRenormalizing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer not-a-jwt" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := FetchGamesForPlayers(t.Context(), server.URL, client, "not-a-jwt", []int{1001})
	if err == nil {
		t.Fatal("expected upstream authorization error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != ErrorUnauthorized {
		t.Fatalf("unexpected error kind: %s", fetchErr.Kind)
	}
}

func TestLPSFetchGamesForTeamsFiltersPastGamesAndDeduplicates(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-2 * time.Hour))
	sharedFuture := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	uniqueFuture := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))

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

	client := &http.Client{Timeout: 5 * time.Second}
	games, err := FetchGamesForTeams(t.Context(), server.URL, client, []int{479691, 479147})
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

	gamesReversed, err := FetchGamesForTeams(t.Context(), server.URL, client, []int{479147, 479691})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams (reversed teams) returned error: %v", err)
	}
	if !reflect.DeepEqual(games, gamesReversed) {
		t.Fatalf("expected deterministic merge regardless of team order:\noriginal: %#v\nreversed: %#v", games, gamesReversed)
	}
}

func TestLPSFetchAllGamesForTeamsIncludesPastGames(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-2 * time.Hour))
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))

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
						"Season": 169,
						"result": "2 - 1"
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
			}`, past, future)))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	games, err := FetchAllGamesForTeams(t.Context(), server.URL, client, []int{479691})
	if err != nil {
		t.Fatalf("FetchAllGamesForTeams returned error: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("unexpected games length: got %d want 2", len(games))
	}
	if games[0].Home != "Past FC" {
		t.Fatalf("expected past game to be preserved, got %#v", games[0])
	}
	if games[0].Result != "2 - 1" {
		t.Fatalf("expected result text to survive, got %q", games[0].Result)
	}
}
