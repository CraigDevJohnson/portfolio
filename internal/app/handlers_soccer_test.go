package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
)

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
		if cookie.Name == config.LPSSessionCookieName {
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
		if cookie.Name == config.LPSSessionCookieName {
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
			if strings.Contains(resp.Header().Get("Set-Cookie"), config.LPSSessionCookieName+"=") {
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
			if tt.noCookie && strings.Contains(resp.Header().Get("Set-Cookie"), config.LPSSessionCookieName+"=") {
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
		LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
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
		if cookie.Name == config.LPSSessionCookieName {
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
		LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
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
				req.AddCookie(&http.Cookie{Name: config.LPSSessionCookieName, Value: "not-valid-session-data"})
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
				if cookie.Name == config.LPSSessionCookieName {
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
		LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
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
		LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
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
				LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
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
				if setCookie.Name == config.LPSSessionCookieName && setCookie.MaxAge < 0 {
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
