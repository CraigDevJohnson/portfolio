package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEncryptDecryptSessionRoundTrip(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: defaultLPSAPIBaseURL,
	}
	defer func() {
		configData = previousConfig
	}()

	expected := SessionData{
		JWT:      "token-value",
		UserID:   42,
		UserName: "Craig Johnson",
		Players: []LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		},
		ExpiresAt: time.Unix(1_773_634_161, 0).UTC(),
	}

	encrypted, err := encryptSession(expected)
	if err != nil {
		t.Fatalf("encryptSession returned error: %v", err)
	}

	actual, err := decryptSession(encrypted)
	if err != nil {
		t.Fatalf("decryptSession returned error: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("decryptSession mismatch: got %#v want %#v", actual, expected)
	}
}

func TestLPSLoginUsesAuthorizationHeaderFallback(t *testing.T) {
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/sign_in" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode login payload: %v", err)
		}
		userPayload, ok := payload["user"].(map[string]any)
		if !ok || userPayload["email"] != "player@example.com" {
			t.Fatalf("unexpected login payload: %#v", payload)
		}

		w.Header().Set("Authorization", "Bearer server-issued-token")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": 55, "first_name": "Craig", "last_name": "Johnson", "players": [{"UPlayerID": 1001, "FirstName": "Craig", "LastName": "Johnson", "is_main_player": true}]}`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	user, err := lpsLogin(t.Context(), "player@example.com", "secret", "captcha-token")
	if err != nil {
		t.Fatalf("lpsLogin returned error: %v", err)
	}
	if user.JWT != "server-issued-token" {
		t.Fatalf("unexpected JWT: %s", user.JWT)
	}
	if len(user.Players) != 1 || user.Players[0].UPlayerID != 1001 {
		t.Fatalf("unexpected player list: %#v", user.Players)
	}
}

func TestLPSFetchUpcomingGamesMapsFlexiblePayload(t *testing.T) {
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/players/1001/upcoming_games" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer api-token" {
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchUpcomingGames(t.Context(), "api-token", 1001)
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
