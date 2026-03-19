package main

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

func TestNormalizeImportedJWTAcceptsBearerPrefix(t *testing.T) {
	token := testJWT(t, time.Now().Add(30*time.Minute))

	got, err := normalizeImportedJWT("Bearer " + token)
	if err != nil {
		t.Fatalf("normalizeImportedJWT returned error: %v", err)
	}
	if got != token {
		t.Fatalf("normalizeImportedJWT mismatch: got %q want %q", got, token)
	}
}

func TestNormalizeImportedJWTRejectsExpiredToken(t *testing.T) {
	token := testJWT(t, time.Now().Add(-30*time.Minute))

	_, err := normalizeImportedJWT(token)
	if err == nil {
		t.Fatal("expected normalizeImportedJWT to reject expired tokens")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestSoccerImportHandlerStoresCurrentSessionCookie(t *testing.T) {
	resetSoccerLoginAttempts(t)

	previousConfig := configData
	token := testJWT(t, time.Now().Add(30*time.Minute))
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
					"UPlayerID": 1001,
					"FirstName": "Craig",
					"LastName": "Johnson",
					"is_main_player": true
				},
				{
					"UPlayerID": 1002,
					"FirstName": "Taylor",
					"LastName": "Johnson",
					"is_main_player": false
				}
			],
			"user_players": [
				{"player_id": 1001, "deleted": false},
				{"player_id": 1002, "deleted": false}
			]
		}`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	form := url.Values{
		"jwt": {"Bearer " + token},
	}
	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	soccerImportHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == lpsSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if !sessionCookie.HttpOnly {
		t.Fatal("expected session cookie to be HttpOnly")
	}
	if sessionCookie.Expires != (time.Time{}) {
		t.Fatalf("expected current-session cookie without expiry, got %v", sessionCookie.Expires)
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected same-site mode: %v", sessionCookie.SameSite)
	}

	session, err := decryptSession(sessionCookie.Value)
	if err != nil {
		t.Fatalf("decryptSession returned error: %v", err)
	}
	if session.JWT == "" {
		t.Fatal("expected imported JWT to be stored in the session")
	}
	if session.UserName != "Craig Johnson" {
		t.Fatalf("unexpected stored user name: got %q want %q", session.UserName, "Craig Johnson")
	}
	if got := []int{session.Players[0].UPlayerID, session.Players[1].UPlayerID}; !reflect.DeepEqual(got, []int{1001, 1002}) {
		t.Fatalf("unexpected stored player IDs: %#v", got)
	}
	if got := session.Players[0]; got.FirstName != "Craig" || got.LastName != "Johnson" || !got.IsMainPlayer {
		t.Fatalf("unexpected primary player: %#v", got)
	}
	if !strings.Contains(resp.Body.String(), "data-login-success") {
		t.Fatalf("expected success feedback in response body, got %q", resp.Body.String())
	}
}

func TestSoccerImportDiscoveryFlowFetchesSchedulesForDiscoveredPlayers(t *testing.T) {
	resetSoccerLoginAttempts(t)

	previousConfig := configData
	token := testJWT(t, time.Now().Add(30*time.Minute))
	requestCounts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCounts[r.URL.Path]++
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("unexpected authorization header for %s: %s", r.URL.Path, got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/check":
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
						"FirstName": "Taylor",
						"LastName": "Johnson",
						"is_main_player": false
					}
				],
				"user_players": [
					{"player_id": 1669080, "deleted": false},
					{"player_id": 1669081, "deleted": false}
				]
			}`))
		case "/players/1669080/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"UGameID": 9001,
					"SchedGameDateTime": "2026-05-01T19:00:00-06:00",
					"field_name": "Arena 1",
					"facilityName": "North Campus",
					"home_team": {"TeamName": "Craig FC"},
					"visitor_team": {"TeamName": "Rivals"},
					"SeasonID": 77
				}
			]`))
		case "/players/1669081/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"UGameID": 9002,
					"SchedGameDateTime": "2026-05-08T20:15:00-06:00",
					"field_name": "Arena 2",
					"facilityName": "South Campus",
					"home_team": {"TeamName": "Taylor FC"},
					"visitor_team": {"TeamName": "Visitors"},
					"SeasonID": 77
				}
			]`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	importReq := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {"Bearer " + token},
	}.Encode()))
	importReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	importResp := httptest.NewRecorder()

	soccerImportHandler(importResp, importReq)

	if importResp.Code != http.StatusOK {
		t.Fatalf("unexpected import status code: got %d want %d", importResp.Code, http.StatusOK)
	}
	if got := requestCounts["/users/check"]; got != 1 {
		t.Fatalf("unexpected /users/check call count: got %d want 1", got)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range importResp.Result().Cookies() {
		if cookie.Name == lpsSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}

	session, err := decryptSession(sessionCookie.Value)
	if err != nil {
		t.Fatalf("decryptSession returned error: %v", err)
	}
	if session.UserName != "Craig Johnson" {
		t.Fatalf("unexpected stored user name: got %q want %q", session.UserName, "Craig Johnson")
	}
	if len(session.Players) != 2 {
		t.Fatalf("unexpected stored players length: got %d want 2", len(session.Players))
	}
	if got := session.Players[0]; got.UPlayerID != 1669080 || got.FirstName != "Craig" || got.LastName != "Johnson" || !got.IsMainPlayer {
		t.Fatalf("unexpected primary player: %#v", got)
	}
	if got := session.Players[1]; got.UPlayerID != 1669081 || got.FirstName != "Taylor" || got.LastName != "Johnson" || got.IsMainPlayer {
		t.Fatalf("unexpected secondary player: %#v", got)
	}

	fetchReq := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"1669080", "1669081"},
	}.Encode()))
	fetchReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	fetchReq.AddCookie(sessionCookie)
	fetchResp := httptest.NewRecorder()

	fetchSchedulesHandler(fetchResp, fetchReq)

	if fetchResp.Code != http.StatusOK {
		t.Fatalf("unexpected fetch status code: got %d want %d", fetchResp.Code, http.StatusOK)
	}
	if got := requestCounts["/players/1669080/upcoming_games"]; got != 1 {
		t.Fatalf("unexpected player 1669080 call count: got %d want 1", got)
	}
	if got := requestCounts["/players/1669081/upcoming_games"]; got != 1 {
		t.Fatalf("unexpected player 1669081 call count: got %d want 1", got)
	}
	if !strings.Contains(fetchResp.Body.String(), "Craig FC") {
		t.Fatalf("expected first discovered player schedule in response body, got %q", fetchResp.Body.String())
	}
	if !strings.Contains(fetchResp.Body.String(), "Taylor FC") {
		t.Fatalf("expected second discovered player schedule in response body, got %q", fetchResp.Body.String())
	}
}

func TestSoccerImportHandlerRejectsMalformedJWT(t *testing.T) {
	tests := []struct {
		name        string
		jwt         string
		wantMessage string
	}{
		{
			name:        "malformed token",
			jwt:         "not-a-jwt",
			wantMessage: "three dot-separated sections",
		},
		{
			name:        "expired token",
			jwt:         testJWT(t, time.Now().Add(-30*time.Minute)),
			wantMessage: "This JWT has expired.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetSoccerLoginAttempts(t)

			previousConfig := configData
			apiCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiCalls++
				t.Fatalf("did not expect upstream call for %s token validation test", tc.name)
			}))
			defer server.Close()

			configData = serverConfig{
				SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
				LPSAPIBaseURL: server.URL,
			}
			defer func() {
				configData = previousConfig
			}()

			req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
				"jwt": {tc.jwt},
			}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp := httptest.NewRecorder()

			soccerImportHandler(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
			}
			if strings.Contains(resp.Header().Get("Set-Cookie"), lpsSessionCookieName+"=") {
				t.Fatalf("did not expect a session cookie on invalid JWT: %q", resp.Header().Get("Set-Cookie"))
			}
			if !strings.Contains(resp.Body.String(), tc.wantMessage) {
				t.Fatalf("unexpected response body: %q", resp.Body.String())
			}
			if apiCalls != 0 {
				t.Fatalf("unexpected upstream API calls: got %d want 0", apiCalls)
			}
		})
	}
}

func TestSoccerImportHandlerShowsActionableUsersCheckAuthFailure(t *testing.T) {
	resetSoccerLoginAttempts(t)

	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authFailure":true,"error":"You need to sign in or sign up before continuing."}`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {testJWT(t, time.Now().Add(30*time.Minute))},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	soccerImportHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if strings.Contains(resp.Header().Get("Set-Cookie"), lpsSessionCookieName+"=") {
		t.Fatalf("did not expect a session cookie on auth failure: %q", resp.Header().Get("Set-Cookie"))
	}
	if !strings.Contains(resp.Body.String(), "The JWT was rejected by Let&#39;s Play Soccer. Copy a fresh bearer token and try again.") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestSoccerImportHandlerShowsActionableUsersCheckUpstreamError(t *testing.T) {
	resetSoccerLoginAttempts(t)

	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {testJWT(t, time.Now().Add(30*time.Minute))},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	soccerImportHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "Could not reach Let&#39;s Play Soccer to look up your players. Try again in a moment.") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestSoccerImportHandlerShowsEmptyPlayerListMessage(t *testing.T) {
	resetSoccerLoginAttempts(t)

	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[],"user_players":[]}`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {testJWT(t, time.Now().Add(30*time.Minute))},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	soccerImportHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if strings.Contains(resp.Header().Get("Set-Cookie"), lpsSessionCookieName+"=") {
		t.Fatalf("did not expect a session cookie on empty player discovery: %q", resp.Header().Get("Set-Cookie"))
	}
	if !strings.Contains(resp.Body.String(), "No linked players found for this account.") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestSoccerLogoutHandlerClearsSessionAndRendersUnauthenticatedPanel(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: defaultLPSAPIBaseURL,
	}
	defer func() {
		configData = previousConfig
	}()

	req := httptest.NewRequest(http.MethodPost, "/soccer/logout", nil)
	addSessionCookie(t, req, SessionData{
		JWT:      testJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Current browser session",
		Players: []LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	soccerLogoutHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
	}
	if got := result.Header.Get("HX-Trigger"); got != "soccer-logout" {
		t.Fatalf("unexpected HX-Trigger header: got %q want %q", got, "soccer-logout")
	}

	var sessionCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == lpsSessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected cleared session cookie to be set")
	}
	if sessionCookie.Value != "" {
		t.Fatalf("expected cleared session cookie value to be empty, got %q", sessionCookie.Value)
	}
	if sessionCookie.MaxAge >= 0 {
		t.Fatalf("expected cleared session cookie max-age to be negative, got %d", sessionCookie.MaxAge)
	}
	if !sessionCookie.Expires.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected cleared session cookie expiry to be Unix epoch, got %v", sessionCookie.Expires)
	}
	if !strings.Contains(resp.Body.String(), "No import active") {
		t.Fatalf("expected unauthenticated auth panel, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Import access") {
		t.Fatalf("expected unauthenticated import control, got %q", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "Clear import") {
		t.Fatalf("did not expect authenticated controls after logout, got %q", resp.Body.String())
	}
}

func TestSoccerSessionHandlerClearsExpiredOrInvalidSessionAndRendersUnauthenticatedState(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: defaultLPSAPIBaseURL,
	}
	defer func() {
		configData = previousConfig
	}()

	tests := []struct {
		name      string
		addCookie func(t *testing.T, req *http.Request)
	}{
		{
			name: "expired session",
			addCookie: func(t *testing.T, req *http.Request) {
				addSessionCookie(t, req, SessionData{
					JWT:      testJWT(t, time.Now().Add(-30*time.Minute)),
					UserName: "Current browser session",
					Players: []LPSPlayer{
						{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
					},
					ExpiresAt: time.Now().Add(-5 * time.Minute),
				})
			},
		},
		{
			name: "invalid session payload",
			addCookie: func(t *testing.T, req *http.Request) {
				req.AddCookie(&http.Cookie{Name: lpsSessionCookieName, Value: "not-valid-session-data"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/soccer/session", nil)
			tc.addCookie(t, req)
			resp := httptest.NewRecorder()

			soccerSessionHandler(resp, req)

			result := resp.Result()
			if result.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
			}

			var sessionCookie *http.Cookie
			for _, cookie := range result.Cookies() {
				if cookie.Name == lpsSessionCookieName {
					sessionCookie = cookie
					break
				}
			}
			if sessionCookie == nil {
				t.Fatal("expected cleared session cookie to be set")
			}
			if sessionCookie.Value != "" {
				t.Fatalf("expected cleared session cookie value to be empty, got %q", sessionCookie.Value)
			}
			if sessionCookie.MaxAge >= 0 {
				t.Fatalf("expected cleared session cookie max-age to be negative, got %d", sessionCookie.MaxAge)
			}
			if !sessionCookie.Expires.Equal(time.Unix(0, 0)) {
				t.Fatalf("expected cleared session cookie expiry to be Unix epoch, got %v", sessionCookie.Expires)
			}
			if !strings.Contains(resp.Body.String(), "hx-swap-oob=\"outerHTML\"") {
				t.Fatalf("expected session refresh to swap auth panel out-of-band, got %q", resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), "No import active") {
				t.Fatalf("expected unauthenticated auth panel, got %q", resp.Body.String())
			}
			if !strings.Contains(resp.Body.String(), "Import access") {
				t.Fatalf("expected unauthenticated import control, got %q", resp.Body.String())
			}
			if strings.Contains(resp.Body.String(), "Clear import") {
				t.Fatalf("did not expect authenticated controls after invalid session, got %q", resp.Body.String())
			}
		})
	}
}

func TestLPSFetchUpcomingGamesMapsFlexiblePayload(t *testing.T) {
	previousConfig := configData
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchUpcomingGames(t.Context(), token, 1001)
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
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/players/1001/upcoming_games" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testJWT(t, time.Now().Add(30*time.Minute)) {
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

	token := testJWT(t, time.Now().Add(30*time.Minute))
	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchUpcomingGames(t.Context(), token, 1001)
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

func TestLPSFetchGamesForPlayersDedupesOverlappingMissingIDGames(t *testing.T) {
	previousConfig := configData
	token := testJWT(t, time.Now().Add(30*time.Minute))
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

	games, err := lpsFetchGamesForPlayers(t.Context(), token, []int{1001, 1002})
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

func TestLPSFetchGamesForPlayersMergesDuplicateGamesAcrossPlayers(t *testing.T) {
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"UGameID": 555,
					"SchedGameDateTime": "2026-05-01T19:00:00-06:00",
					"home_team": {"TeamName": "Shared FC"},
					"visitor_team": {"TeamName": "Opponents"},
					"SeasonID": 77
				}
			]`))
		case "/players/1002/upcoming_games":
			_, _ = w.Write([]byte(`[
				{
					"UGameID": 555,
					"SchedGameDateTime": "2026-05-01T19:00:00-06:00",
					"field_name": "Field 9",
					"facilityName": "Main Complex",
					"home_team": {"TeamName": "Shared FC"},
					"visitor_team": {"TeamName": "Opponents"},
					"SeasonID": 77
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

	games, err := lpsFetchGamesForPlayers(t.Context(), testJWT(t, time.Now().Add(30*time.Minute)), []int{1001, 1002})
	if err != nil {
		t.Fatalf("lpsFetchGamesForPlayers returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("unexpected deduped games length: got %d want 1", len(games))
	}
	if games[0].Field != "Field 9" {
		t.Fatalf("expected merged field, got %q", games[0].Field)
	}
	if games[0].Location != "Main Complex" {
		t.Fatalf("expected merged location, got %q", games[0].Location)
	}
}

func TestLPSFetchUpcomingGamesRejectsMalformedTokenBeforeRequest(t *testing.T) {
	_, err := lpsFetchUpcomingGames(t.Context(), "not-a-jwt", 1001)
	if err == nil {
		t.Fatal("expected malformed token error")
	}
	var fetchErr *lpsFetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected lpsFetchError, got %T", err)
	}
	if fetchErr.Kind != lpsErrorMalformedToken {
		t.Fatalf("unexpected error kind: %s", fetchErr.Kind)
	}
}

func TestLPSFetchUserPlayersMapsSuccessfulPayload(t *testing.T) {
	previousConfig := configData
	token := testJWT(t, time.Now().Add(30*time.Minute))
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	discovery, err := lpsFetchUserPlayers(t.Context(), token)
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
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authFailure":true,"error":"You need to sign in or sign up before continuing."}`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	_, err := lpsFetchUserPlayers(t.Context(), testJWT(t, time.Now().Add(30*time.Minute)))
	if err == nil {
		t.Fatal("expected fetch error")
	}
	var fetchErr *lpsFetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected lpsFetchError, got %T", err)
	}
	if fetchErr.Kind != lpsErrorUnauthorized {
		t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, lpsErrorUnauthorized)
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
		wantKind   lpsErrorKind
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantKind: lpsErrorUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, wantKind: lpsErrorForbidden},
		{name: "upstream outage", statusCode: http.StatusBadGateway, wantKind: lpsErrorUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousConfig := configData
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			configData = serverConfig{
				SessionKey:    previousConfig.SessionKey,
				LPSAPIBaseURL: server.URL,
			}
			defer func() {
				configData = previousConfig
			}()

			_, err := lpsFetchUserPlayers(t.Context(), testJWT(t, time.Now().Add(30*time.Minute)))
			if err == nil {
				t.Fatal("expected fetch error")
			}
			var fetchErr *lpsFetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected lpsFetchError, got %T", err)
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
	previousConfig := configData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"players":`))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	_, err := lpsFetchUserPlayers(t.Context(), testJWT(t, time.Now().Add(30*time.Minute)))
	if err == nil {
		t.Fatal("expected fetch error")
	}
	var fetchErr *lpsFetchError
	if !errors.As(err, &fetchErr) {
		t.Fatalf("expected lpsFetchError, got %T", err)
	}
	if fetchErr.Kind != lpsErrorUpstream {
		t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, lpsErrorUpstream)
	}
	if !strings.Contains(fetchErr.Error(), "response format was not recognized") {
		t.Fatalf("unexpected error message: %v", fetchErr)
	}
}

func TestLPSFetchUpcomingGamesClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		playerID   int
		wantKind   lpsErrorKind
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, playerID: 1001, wantKind: lpsErrorUnauthorized},
		{name: "forbidden", statusCode: http.StatusForbidden, playerID: 1001, wantKind: lpsErrorForbidden},
		{name: "invalid player bad request", statusCode: http.StatusBadRequest, playerID: 999999, wantKind: lpsErrorInvalidPlayer},
		{name: "invalid player not found", statusCode: http.StatusNotFound, playerID: 999999, wantKind: lpsErrorInvalidPlayer},
		{name: "upstream outage", statusCode: http.StatusBadGateway, playerID: 1001, wantKind: lpsErrorUpstream},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousConfig := configData
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			configData = serverConfig{
				SessionKey:    previousConfig.SessionKey,
				LPSAPIBaseURL: server.URL,
			}
			defer func() {
				configData = previousConfig
			}()

			_, err := lpsFetchUpcomingGames(t.Context(), testJWT(t, time.Now().Add(30*time.Minute)), tt.playerID)
			if err == nil {
				t.Fatal("expected fetch error")
			}
			var fetchErr *lpsFetchError
			if !errors.As(err, &fetchErr) {
				t.Fatalf("expected lpsFetchError, got %T", err)
			}
			if fetchErr.Kind != tt.wantKind {
				t.Fatalf("unexpected error kind: got %s want %s", fetchErr.Kind, tt.wantKind)
			}
		})
	}
}

func TestFetchSchedulesHandlerShowsInvalidPlayerMessage(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"abc"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	fetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "selected player IDs were invalid") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestFetchSchedulesHandlerShowsActionable401Message(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: defaultLPSAPIBaseURL,
	}
	defer func() {
		configData = previousConfig
	}()

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, req, SessionData{
		JWT:      testJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Craig Johnson",
		Players: []LPSPlayer{{
			UPlayerID: 1001,
			FirstName: "Craig",
			LastName:  "Johnson",
		}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	configData.LPSAPIBaseURL = server.URL
	resp := httptest.NewRecorder()

	fetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "token was rejected") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerReturnsActionableInvalidPlayerError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"game-1"},
		"player_ids": {"bad-id"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	downloadICSHandler(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusBadRequest)
	}
	if !strings.Contains(resp.Body.String(), "selected player IDs were invalid") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerExportsAuthenticatedSchedules(t *testing.T) {
	previousConfig := configData
	configData = serverConfig{
		SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL: defaultLPSAPIBaseURL,
	}
	defer func() {
		configData = previousConfig
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Fatalf("missing bearer auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"UGameID": 888,
				"SchedGameDateTime": "2026-05-15T20:00:00-06:00",
				"schedGameEndTime": "2026-05-15T21:30:00-06:00",
				"facilityName": "North Fieldhouse",
				"field_name": "Pitch 2",
				"home_team": {"TeamName": "Craig FC"},
				"visitor_team": {"TeamName": "Rivals"},
				"SeasonID": 300
			}
		]`))
	}))
	defer server.Close()
	configData.LPSAPIBaseURL = server.URL

	token := testJWT(t, time.Now().Add(30*time.Minute))
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"888"},
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, req, SessionData{
		JWT:      token,
		UserName: "Craig Johnson",
		Players: []LPSPlayer{{
			UPlayerID: 1001,
			FirstName: "Craig",
			LastName:  "Johnson",
		}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	downloadICSHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/calendar" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	if !strings.Contains(resp.Body.String(), "UID:888@craigdevjohnson.com") {
		t.Fatalf("unexpected ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "LOCATION:North Fieldhouse") {
		t.Fatalf("expected facility location in ICS body: %q", resp.Body.String())
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

func testJWT(t *testing.T, expiresAt time.Time) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"exp": expiresAt.Unix()})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	payloadToken := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString([]byte("signature"))
	return strings.Join([]string{header, payloadToken, signature}, ".")
}

func resetSoccerLoginAttempts(t *testing.T) {
	t.Helper()

	previousLimiter := soccerLoginAttempts
	soccerLoginAttempts = newLoginRateLimiter(5, time.Minute)
	t.Cleanup(func() {
		soccerLoginAttempts.Close()
		soccerLoginAttempts = previousLimiter
	})
}

func addSessionCookie(t *testing.T, req *http.Request, session SessionData) {
	t.Helper()
	encrypted, err := encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: lpsSessionCookieName, Value: encrypted})
}
