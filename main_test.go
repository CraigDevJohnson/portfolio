package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"
)

func unfoldICS(ics string) string {
	return strings.ReplaceAll(ics, "\r\n ", "")
}

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

	encrypted, err := encryptSession(&expected)
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
		if (r.URL.Path == "/users/check" || strings.HasPrefix(r.URL.Path, "/players/")) && r.Header.Get("Authorization") != "Bearer "+token {
			got := r.Header.Get("Authorization")
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
		case "/players/1669080/my_teams":
			_, _ = w.Write([]byte(`[
				{
					"UTeamID": 479393,
					"team_name": "Craig FC",
					"division_name": "Coed One",
					"FacilityID": 4,
					"facility_name": "North Campus",
					"Season": 77
				}
			]`))
		case "/players/1669081/my_teams":
			_, _ = w.Write([]byte(`[
				{
					"UTeamID": 479400,
					"team_name": "Taylor FC",
					"division_name": "Coed Two",
					"FacilityID": 8,
					"facility_name": "South Campus",
					"Season": 77
				}
			]`))
		case "/teams/479393":
			_, _ = w.Write([]byte(`{
				"games": [
					{
						"UGameID": 9001,
						"SchedGameDateTime": "2026-05-01T19:00:00-06:00",
						"field_name": "Arena 1",
						"facilityName": "North Campus",
						"FacilityID": 4,
						"UTeam1": 479393,
						"UTeam2": 479500,
						"home_team": {"UTeamID": 479393, "team_name": "Craig FC", "division_name": "Coed One", "FacilityID": 4, "facility_name": "North Campus", "Season": 77},
						"visitor_team": {"UTeamID": 479500, "team_name": "Rivals", "division_name": "Coed One", "FacilityID": 4, "facility_name": "North Campus", "Season": 77}
					}
				],
				"team": {"UTeamID": 479393, "team_name": "Craig FC", "division_name": "Coed One", "FacilityID": 4, "facility_name": "North Campus", "Season": 77}
			}`))
		case "/teams/479400":
			_, _ = w.Write([]byte(`{
				"games": [
					{
						"UGameID": 9002,
						"SchedGameDateTime": "2026-05-08T20:15:00-06:00",
						"field_name": "Arena 2",
						"facilityName": "South Campus",
						"FacilityID": 8,
						"UTeam1": 479501,
						"UTeam2": 479400,
						"home_team": {"UTeamID": 479501, "team_name": "Visitors", "division_name": "Coed Two", "FacilityID": 8, "facility_name": "South Campus", "Season": 77},
						"visitor_team": {"UTeamID": 479400, "team_name": "Taylor FC", "division_name": "Coed Two", "FacilityID": 8, "facility_name": "South Campus", "Season": 77}
					}
				],
				"team": {"UTeamID": 479400, "team_name": "Taylor FC", "division_name": "Coed Two", "FacilityID": 8, "facility_name": "South Campus", "Season": 77}
			}`))
		case "/facilities/4":
			_, _ = w.Write([]byte(`{"FacilityID": 4, "FacilityName": "North Campus", "Address": "1 North Rd", "City": "Boise", "State": "ID", "ZIP": "83701"}`))
		case "/facilities/8":
			_, _ = w.Write([]byte(`{"FacilityID": 8, "FacilityName": "South Campus", "Address": "2 South Rd", "City": "Boise", "State": "ID", "ZIP": "83702"}`))
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
	if got := requestCounts["/players/1669080/my_teams"]; got != 1 {
		t.Fatalf("unexpected player 1669080 my_teams call count: got %d want 1", got)
	}
	if got := requestCounts["/players/1669081/my_teams"]; got != 1 {
		t.Fatalf("unexpected player 1669081 my_teams call count: got %d want 1", got)
	}
	if got := requestCounts["/teams/479393"]; got != 1 {
		t.Fatalf("unexpected team 479393 call count: got %d want 1", got)
	}
	if got := requestCounts["/teams/479400"]; got != 1 {
		t.Fatalf("unexpected team 479400 call count: got %d want 1", got)
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
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter)
		noCookie       bool
		wantBody       string
	}{
		{
			name: "auth failure",
			serverResponse: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"authFailure":true,"error":"You need to sign in or sign up before continuing."}`))
			},
			noCookie: true,
			wantBody: "The JWT was rejected by Let&#39;s Play Soccer. Copy a fresh bearer token and try again.",
		},
		{
			name: "empty player list",
			serverResponse: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[],"user_players":[]}`))
			},
			noCookie: true,
			wantBody: "No linked players found for this account.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetSoccerLoginAttempts(t)

			previousConfig := configData
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.serverResponse(w)
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
			if tt.noCookie && strings.Contains(resp.Header().Get("Set-Cookie"), lpsSessionCookieName+"=") {
				t.Fatalf("did not expect a session cookie: %q", resp.Header().Get("Set-Cookie"))
			}
			if !strings.Contains(resp.Body.String(), tt.wantBody) {
				t.Fatalf("unexpected response body: %q", resp.Body.String())
			}
		})
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
	addSessionCookie(t, req, &SessionData{
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
				addSessionCookie(t, req, &SessionData{
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
			if strings.Contains(resp.Body.String(), "hx-swap-oob") {
				t.Fatalf("expected /soccer/session to render auth panel as primary content (no OOB swap), got %q", resp.Body.String())
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

func TestLPSFetchGamesForPlayersResolvesPlayerTeamsAndFacilityDetails(t *testing.T) {
	previousConfig := configData
	token := testJWT(t, time.Now().Add(30*time.Minute))
	requestCounts := map[string]int{}
	future := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	expectedStart, ok := parseScheduleTime(future)
	if !ok {
		t.Fatalf("parseScheduleTime returned false for %q", future)
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchGamesForPlayers(t.Context(), token, []int{1001})
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
	previousConfig := configData
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}
	defer func() {
		configData = previousConfig
	}()

	games, err := lpsFetchGamesForTeams(t.Context(), []int{479691})
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
	_, err := lpsFetchGamesForPlayers(t.Context(), "not-a-jwt", []int{1001})
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

func TestLPSFetchGamesForTeamsFiltersPastGamesAndDeduplicates(t *testing.T) {
	previousConfig := configData
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		configData = previousConfig
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

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}

	games, err := lpsFetchGamesForTeams(t.Context(), []int{479691, 479147})
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

	// Verify that merge behavior is deterministic with respect to team order.
	// Reversing the team IDs should yield the same merged, deduplicated schedule.
	gamesReversed, err := lpsFetchGamesForTeams(t.Context(), []int{479147, 479691})
	if err != nil {
		t.Fatalf("lpsFetchGamesForTeams (reversed teams) returned error: %v", err)
	}
	if !reflect.DeepEqual(games, gamesReversed) {
		t.Fatalf("expected deterministic merge regardless of team order:\noriginal: %#v\nreversed: %#v", games, gamesReversed)
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
	if !strings.Contains(resp.Body.String(), "selected players were invalid") {
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
	addSessionCookie(t, req, &SessionData{
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
	if !strings.Contains(resp.Body.String(), "selected players were invalid") {
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
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") && strings.HasPrefix(r.URL.Path, "/players/") {
			t.Fatalf("missing bearer auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/my_teams":
			_, _ = w.Write([]byte(`[
				{
					"UTeamID": 8881,
					"team_name": "Craig FC",
					"division_name": "Premier",
					"FacilityID": 12,
					"facility_name": "North Fieldhouse",
					"Season": 300
				}
			]`))
		case "/teams/8881":
			_, _ = w.Write([]byte(`{
				"games": [
					{
						"UGameID": 888,
						"SchedGameDateTime": "2026-05-15T20:00:00-06:00",
						"schedGameEndTime": "2026-05-15T21:30:00-06:00",
						"facilityName": "North Fieldhouse",
						"field_name": "Pitch 2",
						"FacilityID": 12,
						"UTeam1": 8881,
						"UTeam2": 8882,
						"home_team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"visitor_team": {"UTeamID": 8882, "team_name": "Rivals", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300}
					}
				],
				"team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300}
			}`))
		case "/facilities/12":
			_, _ = w.Write([]byte(`{"FacilityID": 12, "FacilityName": "North Fieldhouse", "Address": "123 Main St", "City": "Boise", "State": "ID", "ZIP": "83709"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	configData.LPSAPIBaseURL = server.URL

	token := testJWT(t, time.Now().Add(30*time.Minute))
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"888"},
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, req, &SessionData{
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
	unfoldedICS := unfoldICS(resp.Body.String())
	if !strings.Contains(unfoldedICS, "UID:888") {
		t.Fatalf("unexpected ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(unfoldedICS, "LOCATION:123 Main St\\, Boise\\, ID\\, 83709") {
		t.Fatalf("expected facility location in ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(unfoldedICS, "SUMMARY:Craig FC vs Rivals - Pitch 2") {
		t.Fatalf("expected canonical summary in ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(unfoldedICS, "STATUS:CONFIRMED") {
		t.Fatalf("expected confirmed status in ICS body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerExportsManualTeamSchedules(t *testing.T) {
	previousConfig := configData
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		configData = previousConfig
		time.Local = previousLocal
	}()

	future := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureUnselected := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/479691":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 7001,
						"SchedGameDateTime": %q,
						"field_name": "Field 3",
						"facilityName": "Boise",
						"FacilityID": 4,
						"home_team": {"team_name": "UNITED NATIONS", "division_name": "Coed F Fri"},
						"visitor_team": {"team_name": "GALACTICOS FC", "division_name": "Coed F Fri"},
						"Season": 169
					},
					{
						"UGameID": 7002,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"FacilityID": 4,
						"home_team": {"team_name": "UNITED NATIONS", "division_name": "Coed F Fri"},
						"visitor_team": {"team_name": "BENCH MOB", "division_name": "Coed F Fri"},
						"Season": 169
					}
				]
			}`, future, futureUnselected)))
		case "/facilities/4":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"FacilityID": 4, "FacilityName": "Boise", "Address": "11448 W. President Drive", "City": "Boise", "State": "ID", "ZIP": "83713"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}

	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"7001"},
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	downloadICSHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/calendar" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	unfoldedICS := unfoldICS(resp.Body.String())
	if !strings.Contains(unfoldedICS, "UID:7001") {
		t.Fatalf("unexpected ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(unfoldedICS, "LOCATION:11448 W. President Drive\\, Boise\\, ID\\, 83713") {
		t.Fatalf("expected facility location in ICS body: %q", resp.Body.String())
	}
	if !strings.Contains(unfoldedICS, "SUMMARY:UNITED NATIONS vs GALACTICOS FC - Field 3") {
		t.Fatalf("expected canonical summary in ICS body: %q", resp.Body.String())
	}
	if strings.Contains(unfoldedICS, "UID:7002") || strings.Contains(unfoldedICS, "BENCH MOB") {
		t.Fatalf("expected unselected games to be excluded from ICS body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerRejectsInvalidTeamSelection(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"7001"},
		"team_codes": {"abc,def"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	downloadICSHandler(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusBadRequest)
	}
	if !strings.Contains(resp.Body.String(), "one or more team IDs were invalid") {
		t.Fatalf("expected invalid team selection message, got %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerClearsSessionOnAuthFailure(t *testing.T) {
	tests := []struct {
		name               string
		statusCode         int
		wantStatus         int
		wantSessionCleared bool
	}{
		{name: "unauthorized clears session", statusCode: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized, wantSessionCleared: true},
		{name: "forbidden preserves session", statusCode: http.StatusForbidden, wantStatus: http.StatusForbidden, wantSessionCleared: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previousConfig := configData
			configData = serverConfig{
				SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
				LPSAPIBaseURL: defaultLPSAPIBaseURL,
			}
			defer func() {
				configData = previousConfig
			}()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()
			configData.LPSAPIBaseURL = server.URL

			token := testJWT(t, time.Now().Add(30*time.Minute))
			req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
				"selected":   {"game-1"},
				"player_ids": {"1001"},
			}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			addSessionCookie(t, req, &SessionData{
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

			if resp.Code != tt.wantStatus {
				t.Fatalf("unexpected status code: got %d want %d", resp.Code, tt.wantStatus)
			}

			var cookieCleared bool
			for _, setCookie := range resp.Result().Cookies() {
				if setCookie.Name == lpsSessionCookieName && setCookie.MaxAge < 0 {
					cookieCleared = true
					break
				}
			}
			if cookieCleared != tt.wantSessionCleared {
				t.Fatalf("session cookie cleared = %v, want %v", cookieCleared, tt.wantSessionCleared)
			}
		})
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

func TestParseFlexibleTimePreservesUTCForZuluTimestamps(t *testing.T) {
	previousLocal := time.Local
	localZone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation returned error: %v", err)
	}
	time.Local = localZone
	defer func() {
		time.Local = previousLocal
	}()

	got, ok := parseFlexibleTime("2026-01-12T01:00:00.000Z")
	if !ok {
		t.Fatal("parseFlexibleTime returned false")
	}

	want := time.Date(2026, time.January, 12, 1, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time: got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("unexpected location: got %v want %v", got.Location(), time.UTC)
	}
}

func TestParseScheduleTimeTreatsMislabelledZuluTimestampsAsMountainWallTime(t *testing.T) {
	got, ok := parseScheduleTime("2026-03-29T17:20:00.000Z")
	if !ok {
		t.Fatal("parseScheduleTime returned false")
	}

	if got.Format(time.RFC3339) != "2026-03-29T17:20:00-06:00" {
		t.Fatalf("unexpected schedule parse result: %s", got.Format(time.RFC3339))
	}
	if got.In(mountainTimeLocation).Format("MST") != "MDT" {
		t.Fatalf("unexpected mountain timezone label: %s", got.In(mountainTimeLocation).Format("MST"))
	}
}

func TestMapLPSGameNormalizesMislabelledZuluTimestampsToMountainTime(t *testing.T) {
	game := mapLPSGame(map[string]any{
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

func TestGoogleEventPayloadTreatsMislabelledZuluTimestampsAsMountainTime(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := googleEventPayload(req, &Game{
		ID:               "zulu-game",
		Home:             "Team A",
		Away:             "Team B",
		PlayerTeamName:   "Team A",
		OpponentTeamName: "Team B",
		Field:            "Field 1",
		StartAt:          "2026-03-29T17:20:00.000Z",
		EndAt:            "2026-03-29T18:50:00.000Z",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.Start.TimeZone != mountainTimeZoneID {
		t.Fatalf("unexpected google start timezone: %q", event.Start.TimeZone)
	}
	if event.End.DateTime != "2026-03-29T18:50:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.End.TimeZone != mountainTimeZoneID {
		t.Fatalf("unexpected google end timezone: %q", event.End.TimeZone)
	}
}

func TestCanonicalGameEventUsesEnrichedScheduleFields(t *testing.T) {
	formatted, ok := canonicalGameEvent(&Game{
		ID:               "3037322",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "FC CHAIN MAIL",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 2",
		Result:           "7 - 3",
		StartAt:          "2026-03-08T12:30:00-06:00",
	})
	if !ok {
		t.Fatal("canonicalGameEvent returned false")
	}

	if formatted.ID != "3037322" {
		t.Fatalf("unexpected canonical game id: %q", formatted.ID)
	}
	if formatted.Summary != "STRUGGLE BUS vs FC CHAIN MAIL - Field 2" {
		t.Fatalf("unexpected canonical summary: %q", formatted.Summary)
	}
	if formatted.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: 7 - 3" {
		t.Fatalf("unexpected canonical description: %q", formatted.Description)
	}
	if formatted.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected canonical location: %q", formatted.Location)
	}
	if formatted.Start.Format(time.RFC3339) != "2026-03-08T12:30:00-06:00" {
		t.Fatalf("unexpected canonical start: %s", formatted.Start.Format(time.RFC3339))
	}
	if formatted.End.Format(time.RFC3339) != "2026-03-08T13:15:00-06:00" {
		t.Fatalf("unexpected canonical end: %s", formatted.End.Format(time.RFC3339))
	}
	if formatted.Status != "confirmed" {
		t.Fatalf("unexpected canonical status: %q", formatted.Status)
	}
}

func TestGoogleEventPayloadUsesCanonicalFormatter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	req.Host = "example.com"
	event, ok := googleEventPayload(req, &Game{
		ID:               "3037322",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "FC CHAIN MAIL",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 2",
		Result:           "7 - 3",
		StartAt:          "2026-03-08T12:30:00-06:00",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.ID != "3037322" {
		t.Fatalf("unexpected google event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs FC CHAIN MAIL - Field 2" {
		t.Fatalf("unexpected google event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: 7 - 3" {
		t.Fatalf("unexpected google event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected google event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-08T12:30:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-08T13:15:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.Status != "confirmed" {
		t.Fatalf("unexpected google event status: %q", event.Status)
	}
	assertGoogleEventHasFortyMinuteReminder(t, event)
	if got := event.ExtendedProperties.Private["game_id"]; got != "3037322" {
		t.Fatalf("unexpected google private game id: %q", got)
	}
	if _, exists := event.ExtendedProperties.Private["portfolio_game_id"]; exists {
		t.Fatalf("legacy portfolio_game_id should not be set: %#v", event.ExtendedProperties.Private)
	}
}

func TestGoogleEventPayloadMirrorsCanonicalFormatterForCancelledGame(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := googleEventPayload(req, &Game{
		ID:               "3042954",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "MANEFESTO",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 1",
		Result:           "canceled",
		StartAt:          "2026-03-29T17:20:00-06:00",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.ID != "3042954" {
		t.Fatalf("unexpected google event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs MANEFESTO - Field 1" {
		t.Fatalf("unexpected google event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing MANEFESTO\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 1\nResult: canceled" {
		t.Fatalf("unexpected google event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected google event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-29T18:05:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.Status != "canceled" {
		t.Fatalf("unexpected google event status: %q", event.Status)
	}
	assertGoogleEventHasFortyMinuteReminder(t, event)
	if got := event.ExtendedProperties.Private["game_id"]; got != "3042954" {
		t.Fatalf("unexpected google private game id: %q", got)
	}
}

func assertGoogleEventHasFortyMinuteReminder(t *testing.T, event googleEvent) {
	t.Helper()

	if event.Reminders == nil || event.Reminders.UseDefault {
		t.Fatalf("expected custom reminders, got %#v", event.Reminders)
	}
	if len(event.Reminders.Overrides) != 1 {
		t.Fatalf("expected one reminder override, got %#v", event.Reminders.Overrides)
	}
	if event.Reminders.Overrides[0].Method != "popup" || event.Reminders.Overrides[0].Minutes != 40 {
		t.Fatalf("unexpected reminder override: %#v", event.Reminders.Overrides[0])
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	var decoded struct {
		Reminders struct {
			Overrides []struct {
				Method  string `json:"method"`
				Minutes int    `json:"minutes"`
			} `json:"overrides"`
			UseDefault bool `json:"useDefault"`
		} `json:"reminders"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if decoded.Reminders.UseDefault {
		t.Fatalf("expected serialized reminders.useDefault=false in payload: %s", string(payload))
	}
	if len(decoded.Reminders.Overrides) != 1 {
		t.Fatalf("expected one serialized reminder override, got %#v in payload: %s", decoded.Reminders.Overrides, string(payload))
	}
	if decoded.Reminders.Overrides[0].Method != "popup" || decoded.Reminders.Overrides[0].Minutes != 40 {
		t.Fatalf("unexpected serialized reminder override: %#v in payload: %s", decoded.Reminders.Overrides[0], string(payload))
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

func TestBuildICSUsesMountainTimezoneForMislabelledZuluTimestamps(t *testing.T) {
	ics := buildICS([]Game{{
		ID:      "mountain-game",
		Home:    "Team A",
		Away:    "Team B",
		StartAt: "2026-03-29T17:20:00.000Z",
		EndAt:   "2026-03-29T18:50:00.000Z",
	}})

	if !strings.Contains(ics, "X-WR-TIMEZONE:America/Denver") {
		t.Fatalf("expected calendar timezone in ICS output, got %q", ics)
	}
	if !strings.Contains(ics, "DTSTART;TZID=America/Denver:20260329T172000") {
		t.Fatalf("expected mountain DTSTART in ICS output, got %q", ics)
	}
	if !strings.Contains(ics, "DTEND;TZID=America/Denver:20260329T185000") {
		t.Fatalf("expected mountain DTEND in ICS output, got %q", ics)
	}
}

func TestBuildICSMirrorsCanonicalFormatterForCancelledGame(t *testing.T) {
	ics := unfoldICS(buildICS([]Game{{
		ID:               "3042954",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "MANEFESTO",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 1",
		Result:           "canceled",
		StartAt:          "2026-03-29T17:20:00-06:00",
	}}))

	expectedLines := []string{
		"UID:3042954",
		"DTSTART;TZID=America/Denver:20260329T172000",
		"DTEND;TZID=America/Denver:20260329T180500",
		"SUMMARY:STRUGGLE BUS vs MANEFESTO - Field 1",
		"DESCRIPTION:STRUGGLE BUS is playing MANEFESTO\\nDivision: Coed Over 30 B Sun\\nFacility: Boise\\nField: Field 1\\nResult: canceled",
		"LOCATION:11448 W. President Drive\\, Boise\\, ID\\, 83713",
		"STATUS:CANCELED",
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(ics, expectedLine) {
			t.Fatalf("expected ICS to contain %q, got %q", expectedLine, ics)
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
	_, _, ok := scheduleTimes(&Game{ID: "no-time"})
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

func testMislabelledLPSZuluTime(at time.Time) string {
	return at.In(mountainTimeLocation).Format("2006-01-02T15:04:05.000") + "Z"
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

func addSessionCookie(t *testing.T, req *http.Request, session *SessionData) {
	t.Helper()
	encrypted, err := encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession returned error: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: lpsSessionCookieName, Value: encrypted})
}

type fakeGoogleConnectionStore struct {
	records map[string]googleConnectionRecord
}

func (store *fakeGoogleConnectionStore) Delete(_ context.Context, connectionID string) error {
	delete(store.records, connectionID)
	return nil
}

func (store *fakeGoogleConnectionStore) Get(_ context.Context, connectionID string) (*googleConnectionRecord, error) {
	record, ok := store.records[connectionID]
	if !ok {
		return nil, nil
	}
	clone := record
	return &clone, nil
}

func (store *fakeGoogleConnectionStore) Put(_ context.Context, record *googleConnectionRecord) error {
	store.records[record.ConnectionID] = *record
	return nil
}

func configureGoogleTestRuntime(t *testing.T, store googleConnectionStore, authURL, tokenURL, apiBaseURL string) {
	t.Helper()

	previousConfig := configData
	previousStore := googleConnections
	previousAuthURL := googleOAuthAuthURL
	previousTokenURL := googleOAuthTokenURL
	previousAPIBaseURL := googleCalendarAPIBaseURL

	configData = serverConfig{
		SessionKey:                []byte("0123456789abcdef0123456789abcdef"),
		LPSAPIBaseURL:             defaultLPSAPIBaseURL,
		GoogleClientID:            "google-client-id",
		GoogleClientSecret:        "google-client-secret",
		GoogleConnectionTableName: "google-connections",
	}
	googleConnections = store
	if authURL != "" {
		googleOAuthAuthURL = authURL
	}
	if tokenURL != "" {
		googleOAuthTokenURL = tokenURL
	}
	if apiBaseURL != "" {
		googleCalendarAPIBaseURL = apiBaseURL
	}

	t.Cleanup(func() {
		configData = previousConfig
		googleConnections = previousStore
		googleOAuthAuthURL = previousAuthURL
		googleOAuthTokenURL = previousTokenURL
		googleCalendarAPIBaseURL = previousAPIBaseURL
	})
}

func TestSoccerGoogleConnectHandlerRedirectsToOAuth(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	configureGoogleTestRuntime(t, store, "https://accounts.example.com/o/oauth2/auth", "", "")

	req := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	req.Host = "example.com"
	resp := httptest.NewRecorder()

	soccerGoogleConnectHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusSeeOther {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusSeeOther)
	}
	location, err := result.Location()
	if err != nil {
		t.Fatalf("result.Location returned error: %v", err)
	}
	if location.Scheme != "https" || location.Host != "accounts.example.com" {
		t.Fatalf("unexpected redirect host: %s", location.String())
	}
	if got := location.Query().Get("redirect_uri"); got != "http://example.com/soccer" {
		t.Fatalf("unexpected redirect_uri: got %q want %q", got, "http://example.com/soccer")
	}
	if location.Query().Get("state") == "" {
		t.Fatal("expected oauth state query parameter")
	}

	var stateCookie *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == googleOAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth state cookie to be set")
	}
	if stateCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected SameSite for state cookie: %v", stateCookie.SameSite)
	}
}

func TestSoccerGoogleCallbackHandlerPersistsConnection(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("token request ParseForm error: %v", err)
			}
			if got := r.FormValue("code"); got != "auth-code" {
				t.Fatalf("unexpected authorization code: %q", got)
			}
			if got := r.FormValue("redirect_uri"); got != "http://example.com/soccer" {
				t.Fatalf("unexpected token redirect_uri: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access-token","refresh_token":"refresh-token","token_type":"Bearer","expires_in":3600}`))
		case "/calendar/v3/users/me/calendarList":
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar","primary":false}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	configureGoogleTestRuntime(t, store, server.URL+"/oauth/auth", server.URL+"/oauth/token", server.URL+"/calendar/v3")

	connectReq := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	connectReq.Host = "example.com"
	connectResp := httptest.NewRecorder()
	soccerGoogleConnectHandler(connectResp, connectReq)

	connectResult := connectResp.Result()
	location, err := connectResult.Location()
	if err != nil {
		t.Fatalf("connect redirect location error: %v", err)
	}
	stateValue := location.Query().Get("state")
	if stateValue == "" {
		t.Fatal("expected oauth state query parameter")
	}

	var stateCookie *http.Cookie
	for _, cookie := range connectResult.Cookies() {
		if cookie.Name == googleOAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("expected oauth state cookie to be set")
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/soccer?code=auth-code&state="+url.QueryEscape(stateValue), nil)
	callbackReq.Host = "example.com"
	callbackReq.AddCookie(stateCookie)
	callbackResp := httptest.NewRecorder()

	soccerHandler(callbackResp, callbackReq)

	callbackResult := callbackResp.Result()
	if callbackResult.StatusCode != http.StatusSeeOther {
		t.Fatalf("unexpected callback status code: got %d want %d", callbackResult.StatusCode, http.StatusSeeOther)
	}
	callbackLocation, err := callbackResult.Location()
	if err != nil {
		t.Fatalf("callback redirect location error: %v", err)
	}
	if got := callbackLocation.String(); got != "/soccer?google=connected" {
		t.Fatalf("unexpected callback redirect: got %q want %q", got, "/soccer?google=connected")
	}

	cookieReq := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	cookieReq.AddCookie(stateCookie)
	storedState, err := getGoogleOAuthStateCookie(cookieReq)
	if err != nil || storedState == nil {
		t.Fatalf("getGoogleOAuthStateCookie returned %v, %v", storedState, err)
	}
	record, ok := store.records[storedState.ConnectionID]
	if !ok {
		t.Fatal("expected persisted google connection record")
	}
	if record.CalendarID != "primary" || record.CalendarSummary != "Primary Calendar" {
		t.Fatalf("unexpected stored calendar selection: %#v", record)
	}
	token, err := decryptGoogleToken(record.TokenCiphertext)
	if err != nil {
		t.Fatalf("decryptGoogleToken returned error: %v", err)
	}
	if token.RefreshToken != "refresh-token" {
		t.Fatalf("unexpected stored refresh token: %#v", token)
	}

	var connectionCookie *http.Cookie
	for _, cookie := range callbackResult.Cookies() {
		if cookie.Name == googleConnectionCookieName {
			connectionCookie = cookie
			break
		}
	}
	if connectionCookie == nil {
		t.Fatal("expected persistent google connection cookie")
	}
}

func TestSoccerGoogleCalendarHandlerUpdatesSelection(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	configureGoogleTestRuntime(t, store, "", "", "")
	tokenCiphertext, err := encryptGoogleToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("encryptGoogleToken returned error: %v", err)
	}
	store.records["connection-1"] = googleConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendar/v3/users/me/calendarList" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar","primary":false}]}`))
	}))
	defer server.Close()

	googleCalendarAPIBaseURL = server.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/calendar", strings.NewReader(url.Values{
		"calendar_id": {"team"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: googleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	soccerGoogleCalendarHandler(resp, req)

	record := store.records["connection-1"]
	if record.CalendarID != "team" || record.CalendarSummary != "Team Calendar" {
		t.Fatalf("unexpected updated calendar selection: %#v", record)
	}
	if !strings.Contains(resp.Body.String(), "Selected calendar: Team Calendar") {
		t.Fatalf("expected selected calendar in response body, got %q", resp.Body.String())
	}
}

func TestGoogleEventMatchesGameIDUsesOnlyCanonicalGameIDFields(t *testing.T) {
	tests := []struct {
		name      string
		event     googleEvent
		configure func(*googleEvent)
		gameID    string
		expected  bool
	}{
		{
			name:     "matches raw google event id",
			event:    googleEvent{ID: "7001"},
			gameID:   "7001",
			expected: true,
		},
		{
			name:  "matches private game id when event id differs",
			event: googleEvent{ID: "legacy-7002"},
			configure: func(event *googleEvent) {
				event.ExtendedProperties.Private = map[string]string{"game_id": "7002"}
			},
			gameID:   "7002",
			expected: true,
		},
		{
			name:   "does not match legacy event without canonical id",
			event:  googleEvent{ID: "legacy-7005"},
			gameID: "7005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.event
			if tt.configure != nil {
				tt.configure(&event)
			}
			got := googleEventMatchesGameID(&event, tt.gameID)
			if got != tt.expected {
				t.Fatalf("googleEventMatchesGameID() = %t, want %t", got, tt.expected)
			}
		})
	}
}

func TestSoccerGoogleAddHandlerAddsUpdatesCancelsAndSkipsByCanonicalGameID(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	configureGoogleTestRuntime(t, store, "", "", "")
	previousConfig := configData
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		configData = previousConfig
		time.Local = previousLocal
	}()
	tokenCiphertext, err := encryptGoogleToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("encryptGoogleToken returned error: %v", err)
	}
	store.records["connection-1"] = googleConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	insertedIDs := make([]string, 0, 2)
	privateLookupIDs := make([]string, 0, 4)
	updatedEvents := map[string]googleEvent{}
	futureOne := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	futureThree := testMislabelledLPSZuluTime(time.Now().Add(72 * time.Hour))
	futureFour := testMislabelledLPSZuluTime(time.Now().Add(96 * time.Hour))
	futureFive := testMislabelledLPSZuluTime(time.Now().Add(120 * time.Hour))
	futureSix := testMislabelledLPSZuluTime(time.Now().Add(144 * time.Hour))
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/teams/479691":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 7001,
						"SchedGameDateTime": %q,
						"field_name": "Field 3",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "GALACTICOS FC"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7002,
						"SchedGameDateTime": %q,
						"field_name": "Field 1",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "CLASSIC XI"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "1 - 0",
						"Season": 169
					},
					{
						"UGameID": 7003,
						"SchedGameDateTime": %q,
						"field_name": "Field 2",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "RED STARS"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7004,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "NIGHT OWLS"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "canceled",
						"Season": 169
					},
					{
						"UGameID": 7005,
						"SchedGameDateTime": %q,
						"field_name": "Field 5",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "OLD GUARD"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7006,
						"SchedGameDateTime": %q,
						"field_name": "Field 6",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "RESERVES"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					}
				]
			}`, futureOne, futureTwo, futureThree, futureFour, futureFive, futureSix)))
		case r.URL.Path == "/facilities/4":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"facilityName":"Boise","Address":"123 Main St","City":"Boise","State":"ID","ZIP":"83702"}`))
		case r.URL.Path == "/calendar/v3/calendars/primary/events" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("io.ReadAll returned error: %v", err)
			}
			var event googleEvent
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			insertedIDs = append(insertedIDs, event.ID)
			if event.ID == "7001" {
				w.WriteHeader(http.StatusCreated)
				return
			}
			if event.ID == "7005" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			t.Fatalf("unexpected insert for event id %q", event.ID)
		case strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/") && r.Method == http.MethodGet:
			eventID := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/")
			switch eventID {
			case "7001", "7002", "7005":
				w.WriteHeader(http.StatusNotFound)
			case "7003":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"7003","status":"canceled","extendedProperties":{"private":{"game_id":"7003"}}}`))
			case "7004":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"7004","status":"confirmed","extendedProperties":{"private":{"game_id":"7004"}}}`))
			default:
				t.Fatalf("unexpected get for event id %q", eventID)
			}
		case r.URL.Path == "/calendar/v3/calendars/primary/events" && r.Method == http.MethodGet:
			privateLookup := r.URL.Query().Get("privateExtendedProperty")
			privateLookupIDs = append(privateLookupIDs, strings.TrimPrefix(privateLookup, "game_id="))
			w.Header().Set("Content-Type", "application/json")
			switch privateLookup {
			case "game_id=7002":
				_, _ = w.Write([]byte(`{"items":[{"id":"legacy-7002","status":"confirmed","extendedProperties":{"private":{"game_id":"7002"}}}]}`))
			case "game_id=7005":
				_, _ = w.Write([]byte(`{"items":[{"id":"legacy-7005","status":"confirmed","extendedProperties":{"private":{"game_id":"legacy-7005"}}}]}`))
			default:
				_, _ = w.Write([]byte(`{"items":[]}`))
			}
		case strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/") && r.Method == http.MethodPut:
			eventID := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("io.ReadAll returned error: %v", err)
			}
			var event googleEvent
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			updatedEvents[eventID] = event
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer apiServer.Close()

	configData.LPSAPIBaseURL = apiServer.URL
	googleCalendarAPIBaseURL = apiServer.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/add", strings.NewReader(url.Values{
		"team_codes": {"479691"},
		"selected":   {"7001", "7002", "7003", "7004", "7005"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: googleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	soccerGoogleAddHandler(resp, req)

	if !strings.Contains(resp.Body.String(), "Added 1 selected game(s) to Google Calendar.") {
		t.Fatalf("expected add success message, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Updated/restored 3 matching game(s).") {
		t.Fatalf("expected update success message, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Skipped 1 game(s) that could not be matched to the same Google game ID.") {
		t.Fatalf("expected skip message, got %q", resp.Body.String())
	}
	if len(insertedIDs) != 2 {
		t.Fatalf("unexpected insert attempt count: got %d want 2", len(insertedIDs))
	}
	if len(updatedEvents) != 3 {
		t.Fatalf("unexpected update count: got %d want 3", len(updatedEvents))
	}
	if updated, ok := updatedEvents["legacy-7002"]; !ok {
		t.Fatalf("expected private game_id match to update legacy-7002: %#v", updatedEvents)
	} else {
		if updated.ID != "legacy-7002" {
			t.Fatalf("expected existing google event id to be preserved, got %q", updated.ID)
		}
		if updated.Status != "confirmed" {
			t.Fatalf("expected private-match update to stay confirmed, got %q", updated.Status)
		}
		if got := updated.ExtendedProperties.Private["game_id"]; got != "7002" {
			t.Fatalf("expected canonical private game id, got %q", got)
		}
		if updated.Summary != "UNITED NATIONS vs CLASSIC XI - Field 1" {
			t.Fatalf("unexpected updated summary: %q", updated.Summary)
		}
		if !strings.Contains(updated.Description, "UNITED NATIONS is playing CLASSIC XI") || !strings.Contains(updated.Description, "Field: Field 1") || !strings.Contains(updated.Description, "Result: 1 - 0") {
			t.Fatalf("unexpected updated description: %q", updated.Description)
		}
		if updated.Location != "123 Main St, Boise, ID, 83702" {
			t.Fatalf("unexpected updated location: %q", updated.Location)
		}
		if updated.Source == nil || updated.Source.Title != "Soccer Schedule" || updated.Source.URL != "http://example.com/soccer" {
			t.Fatalf("unexpected updated source: %#v", updated.Source)
		}
	}
	if updated, ok := updatedEvents["7003"]; !ok {
		t.Fatalf("expected canceled matching event to be restored: %#v", updatedEvents)
	} else if updated.Status != "confirmed" {
		t.Fatalf("expected restored status to be confirmed, got %q", updated.Status)
	}
	if updated, ok := updatedEvents["7004"]; !ok {
		t.Fatalf("expected confirmed matching event to be canceled: %#v", updatedEvents)
	} else if updated.Status != "canceled" {
		t.Fatalf("expected canceled status to be propagated, got %q", updated.Status)
	}
	if _, exists := updatedEvents["legacy-7005"]; exists {
		t.Fatalf("non-matching legacy event should not be mutated: %#v", updatedEvents["legacy-7005"])
	}
	if len(privateLookupIDs) == 0 {
		t.Fatal("expected at least one private game_id lookup")
	}
	if insertedIDs[0] != "7001" || insertedIDs[1] != "7005" {
		t.Fatalf("unexpected insert ids: %#v", insertedIDs)
	}
	for _, lookedUpGameID := range privateLookupIDs {
		if lookedUpGameID == "7006" {
			t.Fatalf("unselected game should not trigger private game_id lookup: %#v", privateLookupIDs)
		}
	}
	if _, exists := updatedEvents["7006"]; exists {
		t.Fatalf("unselected game should not be updated: %#v", updatedEvents["7006"])
	}
}

func TestFetchSchedulesHandlerLoadsManualTeamSchedules(t *testing.T) {
	previousConfig := configData
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		configData = previousConfig
		time.Local = previousLocal
	}()

	past := testMislabelledLPSZuluTime(time.Now().Add(-2 * time.Hour))
	future := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/479691" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"games": [
				{
					"UGameID": 8000,
					"SchedGameDateTime": %q,
					"field_name": "Field 9",
					"facilityName": "Boise",
					"home_team": {"team_name": "Past FC"},
					"visitor_team": {"team_name": "Finished"},
					"Season": 169
				},
				{
					"UGameID": 8001,
					"SchedGameDateTime": %q,
					"field_name": "Field 3",
					"facilityName": "Boise",
					"home_team": {"team_name": "UNITED NATIONS"},
					"visitor_team": {"team_name": "GALACTICOS FC"},
					"Season": 169
				}
			]
		}`, past, future)))
	}))
	defer server.Close()

	configData = serverConfig{
		SessionKey:    previousConfig.SessionKey,
		LPSAPIBaseURL: server.URL,
	}

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	fetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "UNITED NATIONS") || !strings.Contains(body, "GALACTICOS FC") {
		t.Fatalf("expected rendered schedule in response body, got %q", body)
	}
	if strings.Contains(body, "Past FC") {
		t.Fatalf("expected past games to be filtered from rendered schedule, got %q", body)
	}
}
