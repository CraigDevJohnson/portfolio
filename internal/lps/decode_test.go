package lps

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portfolio/internal/testutil"
)

func TestNormalizeImportedJWTAcceptsBearerPrefix(t *testing.T) {
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))

	got, err := NormalizeImportedJWT("Bearer " + token)
	if err != nil {
		t.Fatalf("normalizeImportedJWT returned error: %v", err)
	}
	if got != token {
		t.Fatalf("normalizeImportedJWT mismatch: got %q want %q", got, token)
	}
}

func TestNormalizeImportedJWTRejectsExpiredToken(t *testing.T) {
	token := testutil.TestJWT(t, time.Now().Add(-30*time.Minute))

	_, err := NormalizeImportedJWT(token)
	if err == nil {
		t.Fatal("expected normalizeImportedJWT to reject expired tokens")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestLPSFetchUserPlayersMapsSuccessfulPayload(t *testing.T) {
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/check" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected authorization header: %s", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"first_name": "Craig",
			"last_name": "Johnson",
			"players": [
				{
					"UPlayerID": 1669080,
					"FirstName": "Craig",
					"LastName": "Johnson",
					"is_main_player": true
				},
				{
					"UPlayerID": 1669081,
					"FirstName": "Deleted",
					"LastName": "Player",
					"is_main_player": false
				}
			],
			"user_players": [
				{"player_id": 1669080, "deleted": false},
				{"player_id": 1669081, "deleted": true}
			]
		}`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	discovery, err := FetchUserPlayers(t.Context(), server.URL, client, token)
	if err != nil {
		t.Fatalf("lpsFetchUserPlayers returned error: %v", err)
	}
	if discovery.UserName != "Craig Johnson" {
		t.Fatalf("unexpected user name: got %q want %q", discovery.UserName, "Craig Johnson")
	}
	players := discovery.Players
	if len(players) != 1 {
		t.Fatalf("unexpected players length: got %d want 1", len(players))
	}
	if players[0].UPlayerID != 1669080 {
		t.Fatalf("unexpected player ID: %d", players[0].UPlayerID)
	}
	if players[0].FirstName != "Craig" || players[0].LastName != "Johnson" {
		t.Fatalf("unexpected player name: %#v", players[0])
	}
	if !players[0].IsMainPlayer {
		t.Fatalf("expected primary player flag to be true: %#v", players[0])
	}
}

func TestLPSFetchUserPlayersClassifiesAuthFailureJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authFailure":true,"error":"You need to sign in or sign up before continuing."}`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := FetchUserPlayers(t.Context(), server.URL, client, testutil.TestJWT(t, time.Now().Add(30*time.Minute)))
	if err == nil {
		t.Fatal("expected fetch error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != ErrorUnauthorized {
		t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, ErrorUnauthorized)
	}
	if fetchErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: got %d want %d", fetchErr.StatusCode, http.StatusUnauthorized)
	}
	if !strings.Contains(fetchErr.Error(), "sign in") {
		t.Fatalf("unexpected error message: %v", fetchErr)
	}
}

func TestLPSFetchUserPlayersClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantKind   ErrorKind
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantKind: ErrorUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, wantKind: ErrorForbidden},
		{name: "upstream outage", statusCode: http.StatusBadGateway, wantKind: ErrorUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := &http.Client{Timeout: 5 * time.Second}
			_, err := FetchUserPlayers(t.Context(), server.URL, client, testutil.TestJWT(t, time.Now().Add(30*time.Minute)))
			if err == nil {
				t.Fatal("expected fetch error")
			}
			var fetchErr *FetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected FetchError, got %T", err)
			}
			if fetchErr.Kind != tt.wantKind {
				t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, tt.wantKind)
			}
			if fetchErr.StatusCode != tt.statusCode {
				t.Fatalf("unexpected status code: got %d want %d", fetchErr.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestLPSFetchUserPlayersRejectsMalformedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"players":`))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := FetchUserPlayers(t.Context(), server.URL, client, testutil.TestJWT(t, time.Now().Add(30*time.Minute)))
	if err == nil {
		t.Fatal("expected fetch error")
	}
	var fetchErr *FetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected FetchError, got %T", err)
	}
	if fetchErr.Kind != ErrorUpstream {
		t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, ErrorUpstream)
	}
	if !strings.Contains(fetchErr.Error(), "response format was not recognized") {
		t.Fatalf("unexpected error message: %v", fetchErr)
	}
}

func TestMapLPSGameNormalizesMislabelledZuluTimestampsToMountainTime(t *testing.T) {
	game := MapLPSGame(map[string]any{
		"UGameID":           5001,
		"SchedGameDateTime": "2026-03-29T17:20:00.000Z",
		"home_team":         map[string]any{"team_name": "Team A"},
		"visitor_team":      map[string]any{"team_name": "Team B"},
	})

	if game.StartAt != "2026-03-29T17:20:00-06:00" {
		t.Fatalf("unexpected normalized start time: %q", game.StartAt)
	}
	if game.DateTime != "Sun 03/29/26 05:20 PM MDT" {
		t.Fatalf("unexpected display datetime: %q", game.DateTime)
	}
}

func TestDecodeLPSGamesAcceptsLambdaEnvelope(t *testing.T) {
	payload := []byte(`{
		"games": [
			{
				"id": "game-1",
				"datetime": "Sun 03/29/26 05:20 PM MDT",
				"start_at": "2026-03-29T17:20:00-06:00",
				"field": "Field 1",
				"home": "Team A",
				"away": "Team B",
				"season": "2026"
			}
		]
	}`)

	games, err := DecodeLPSGames(payload)
	if err != nil {
		t.Fatalf("DecodeLPSGames returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected game count: got %d want 1", len(games))
	}
	if games[0].ID != "game-1" {
		t.Fatalf("unexpected game ID: %q", games[0].ID)
	}
	if games[0].Home != "Team A" || games[0].Away != "Team B" {
		t.Fatalf("unexpected teams: %#v", games[0])
	}
	if games[0].StartAt != "2026-03-29T17:20:00-06:00" {
		t.Fatalf("unexpected start time: %q", games[0].StartAt)
	}
}

func TestDecodeLPSGamesMapsFlexibleEnvelope(t *testing.T) {
	payload := []byte(`{
		"results": [
			{
				"UGameID": 5001,
				"SchedGameDateTime": "2026-03-29T17:20:00.000Z",
				"field_name": "Field 7",
				"facilityName": "LPS Arena",
				"home_team": {"team_name": "Team A"},
				"visitor_team": {"team_name": "Team B"},
				"Season": 2026,
				"result": "Final"
			}
		]
	}`)

	games, err := DecodeLPSGames(payload)
	if err != nil {
		t.Fatalf("DecodeLPSGames returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected game count: got %d want 1", len(games))
	}

	game := games[0]
	if game.ID != "5001" {
		t.Fatalf("unexpected game ID: %q", game.ID)
	}
	if game.Home != "Team A" || game.Away != "Team B" {
		t.Fatalf("unexpected matchup: %#v", game)
	}
	if game.Field != "Field 7" {
		t.Fatalf("unexpected field: %q", game.Field)
	}
	if game.Location != "LPS Arena" {
		t.Fatalf("unexpected location: %q", game.Location)
	}
	if game.StartAt != "2026-03-29T17:20:00-06:00" {
		t.Fatalf("unexpected normalized start time: %q", game.StartAt)
	}
	if game.DateTime != "Sun 03/29/26 05:20 PM MDT" {
		t.Fatalf("unexpected display datetime: %q", game.DateTime)
	}
	if game.Season != "2026" {
		t.Fatalf("unexpected season: %q", game.Season)
	}
	if game.Result != "Final" {
		t.Fatalf("unexpected result: %q", game.Result)
	}
}

func TestExtractGameMapsHandlesNestedEnvelope(t *testing.T) {
	raw := map[string]any{
		"data": map[string]any{
			"items": []any{
				map[string]any{"UGameID": 5001},
			},
		},
	}

	games := ExtractGameMaps(raw)
	if len(games) != 1 {
		t.Fatalf("unexpected game count: got %d want 1", len(games))
	}
	if got := games[0]["UGameID"]; got != 5001 {
		t.Fatalf("unexpected game payload: %#v", games[0])
	}
}
