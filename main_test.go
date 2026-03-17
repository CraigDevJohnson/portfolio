package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestEncryptDecryptSessionRoundTrip(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:       []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL:    defaultLPSAPIBaseURL,
		RecaptchaSiteKey: "test-site-key",
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

func TestLPSFetchGamesForPlayersDedupesOverlappingMissingIDGames(t *testing.T) {
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"StartDateTime": "2026-01-11T14:55:00-07:00",
					"FieldName": "3",
					"HomeTeam": "SHARED FC",
					"AwayTeam": "OPPONENT A",
					"SeasonID": 168
				},
				{
					"StartDateTime": "2026-01-11T18:55:00-07:00",
					"FieldName": "4",
					"HomeTeam": "PLAYER ONE FC",
					"AwayTeam": "OPPONENT B",
					"SeasonID": 168
				}
			]`))
		case "/players/1002/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"StartDateTime": "2026-01-11T14:55:00-07:00",
					"FieldName": "3",
					"HomeTeam": "SHARED FC",
					"AwayTeam": "OPPONENT A",
					"SeasonID": 168
				},
				{
					"StartDateTime": "2026-01-11T16:55:00-07:00",
					"FieldName": "5",
					"HomeTeam": "PLAYER TWO FC",
					"AwayTeam": "OPPONENT C",
					"SeasonID": 168
				}
			]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchGamesForPlayers(t.Context(), "api-token", []int{1001, 1002})
	if err != nil {
		t.Fatalf("lpsFetchGamesForPlayers returned error: %v", err)
	}
	if len(games) != 3 {
		t.Fatalf("unexpected deduped games length: got %d want 3", len(games))
	}

	sharedCount := 0
	for _, game := range games {
		if game.Home == "SHARED FC" && game.Away == "OPPONENT A" {
			sharedCount++
			if game.ID == "" {
				t.Fatal("expected shared game to receive a stable fallback ID")
			}
		}
	}
	if sharedCount != 1 {
		t.Fatalf("expected one shared game after dedup, got %d", sharedCount)
	}
}

func TestParseFlexibleTimeUsesLocalTimezoneForTimezoneLessLayouts(t *testing.T) {
	got, ok := parseFlexibleTime("2026-01-11T14:55:00")
	if !ok {
		t.Fatal("parseFlexibleTime returned false")
	}

	want := time.Date(2026, time.January, 11, 14, 55, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time: got %v want %v", got, want)
	}
	if got.Location() != time.Local {
		t.Fatalf("unexpected location: got %v want %v", got.Location(), time.Local)
	}
}

func TestParseFlexibleTimePreservesRFC3339Offsets(t *testing.T) {
	got, ok := parseFlexibleTime("2026-01-11T14:55:00-07:00")
	if !ok {
		t.Fatal("parseFlexibleTime returned false")
	}

	if got.Format(time.RFC3339) != "2026-01-11T14:55:00-07:00" {
		t.Fatalf("unexpected RFC3339 parse result: %s", got.Format(time.RFC3339))
	}
	if _, offset := got.Zone(); offset != -7*60*60 {
		t.Fatalf("unexpected RFC3339 offset: %d", offset)
	}
}

func TestClientIPPrefersTrustedForwardedHeaders(t *testing.T) {
	t.Run("uses cloudflare header from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("CF-Connecting-IP", "198.51.100.24")

		if got := clientIP(req); got != "198.51.100.24" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("ignores cloudflare header on direct connections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "203.0.113.10:443"
		req.Header.Set("CF-Connecting-IP", "198.51.100.24")

		if got := clientIP(req); got != "203.0.113.10" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("uses x forwarded for from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("X-Forwarded-For", "198.51.100.25, 10.0.0.5")

		if got := clientIP(req); got != "198.51.100.25" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})

	t.Run("ignores spoofed x forwarded for on direct connections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/soccer/login", nil)
		req.RemoteAddr = "203.0.113.11:443"
		req.Header.Set("X-Forwarded-For", "198.51.100.26")

		if got := clientIP(req); got != "203.0.113.11" {
			t.Fatalf("unexpected client IP: got %s", got)
		}
	})
}

func TestBuildICSFoldsLongLines(t *testing.T) {
	ics := buildICS([]Game{
		{
			ID:       strings.Repeat("abc123", 8),
			Home:     strings.Repeat("Home Team ", 6),
			Away:     strings.Repeat("Away Team ", 6),
			StartAt:  "2026-01-11T14:55:00-07:00",
			EndAt:    "2026-01-11T16:25:00-07:00",
			Location: strings.Repeat("Championship Field Complex ", 4),
			Season:   strings.Repeat("Spring ", 8),
		},
	})

	if !strings.Contains(ics, "\r\n ") {
		t.Fatalf("expected folded ICS output, got %q", ics)
	}

	for _, line := range strings.Split(ics, "\r\n") {
		if line == "" {
			continue
		}
		if len([]byte(line)) > 75 {
			t.Fatalf("ics line exceeds 75 octets: %d bytes in %q", len([]byte(line)), line)
		}
	}
}

func TestBuildICSFoldsUTF8Lines(t *testing.T) {
	ics := buildICS([]Game{
		{
			ID:       "utf8-game",
			Home:     strings.Repeat("⚽", 20),
			Away:     strings.Repeat("ゴール", 10),
			StartAt:  "2026-01-11T14:55:00-07:00",
			EndAt:    "2026-01-11T16:25:00-07:00",
			Location: strings.Repeat("Équipe ", 12),
		},
	})

	if !utf8.ValidString(ics) {
		t.Fatalf("ics output is not valid UTF-8: %q", ics)
	}

	for _, line := range strings.Split(ics, "\r\n") {
		if line == "" {
			continue
		}
		if len([]byte(line)) > 75 {
			t.Fatalf("ics utf8 line exceeds 75 octets: %d bytes in %q", len([]byte(line)), line)
		}
	}
}

func TestBuildICSSkipsGamesWithUnparseableStartTime(t *testing.T) {
	ics := buildICS([]Game{
		{
			ID:      "good-game",
			Home:    "Team A",
			Away:    "Team B",
			StartAt: "2026-01-11T14:55:00-07:00",
			EndAt:   "2026-01-11T16:25:00-07:00",
		},
		{
			ID:   "bad-game",
			Home: "Team C",
			Away: "Team D",
			// No StartAt — unparseable
		},
	})

	if !strings.Contains(ics, "good-game") {
		t.Fatal("expected good-game in ICS output")
	}
	if strings.Contains(ics, "bad-game") {
		t.Fatal("expected bad-game to be skipped in ICS output")
	}
}

func TestScheduleTimesReturnsFalseForUnparseableStart(t *testing.T) {
	_, _, ok := scheduleTimes(Game{ID: "no-time"})
	if ok {
		t.Fatal("expected scheduleTimes to return false for game with no start time")
	}
}

func TestRateLimiterRejectsAtMaxKeys(t *testing.T) {
	limiter := newLoginRateLimiter(100, time.Hour)
	defer limiter.Close()

	// Fill up to the max key limit
	for i := 0; i < rateLimiterMaxKeys; i++ {
		key := fmt.Sprintf("ip-%d", i)
		if !limiter.Allow(key) {
			t.Fatalf("expected Allow to return true for key %d", i)
		}
	}

	// The next new key should be rejected
	if limiter.Allow("overflow-ip") {
		t.Fatal("expected Allow to return false when max keys exceeded")
	}

	// An existing key should still work
	if !limiter.Allow("ip-0") {
		t.Fatal("expected Allow to return true for existing key")
	}
}

func TestRateLimiterExpiredKeysEvictedAtCapacity(t *testing.T) {
	limiter := newLoginRateLimiter(100, 50*time.Millisecond)
	defer limiter.Close()

	// Fill to capacity
	for i := 0; i < rateLimiterMaxKeys; i++ {
		limiter.Allow(fmt.Sprintf("ip-%d", i))
	}

	// Wait for entries to expire
	time.Sleep(100 * time.Millisecond)

	// New key should succeed because expired entries are swept on demand
	if !limiter.Allow("fresh-ip") {
		t.Fatal("expected Allow to return true after expired entries are swept")
	}
}

func TestRateLimiterCloseIsIdempotent(t *testing.T) {
	limiter := newLoginRateLimiter(5, time.Minute)
	limiter.Close()
	limiter.Close() // must not panic
}

func TestRequestIsHTTPSOnlyTrustsProxiedHeader(t *testing.T) {
	t.Run("trusts X-Forwarded-Proto from trusted proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.RemoteAddr = "127.0.0.1:443"
		req.Header.Set("X-Forwarded-Proto", "https")

		if !requestIsHTTPS(req) {
			t.Fatal("expected requestIsHTTPS to return true for trusted proxy with https proto")
		}
	})

	t.Run("ignores X-Forwarded-Proto from untrusted source", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.RemoteAddr = "203.0.113.10:443"
		req.Header.Set("X-Forwarded-Proto", "https")

		if requestIsHTTPS(req) {
			t.Fatal("expected requestIsHTTPS to return false for untrusted source with spoofed proto")
		}
	})

	t.Run("returns true for direct TLS", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
		req.TLS = &tls.ConnectionState{}

		if !requestIsHTTPS(req) {
			t.Fatal("expected requestIsHTTPS to return true for direct TLS")
		}
	})
}
