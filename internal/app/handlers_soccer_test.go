package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
	"portfolio/types"
)

func TestSoccerImportHandlerStoresCurrentSessionCookie(t *testing.T) {
	app := newTestApp(t)
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		case "/players/1001/my_teams", "/players/1002/my_teams":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	form := url.Values{
		"jwt": {"Bearer " + token},
	}
	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).ImportHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
	}

	sessionCookie := findSessionCookie(t, result)
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

	session := decryptTestSession(t, app, sessionCookie.Value)
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
	app := newTestApp(t)
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
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

	app.Config.LPSAPIBaseURL = server.URL

	importReq := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {"Bearer " + token},
	}.Encode()))
	importReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	importResp := httptest.NewRecorder()

	newTestSoccerHandler(app).ImportHandler(importResp, importReq)

	if importResp.Code != http.StatusOK {
		t.Fatalf("unexpected import status code: got %d want %d", importResp.Code, http.StatusOK)
	}
	if got := requestCounts["/users/check"]; got != 1 {
		t.Fatalf("unexpected /users/check call count: got %d want 1", got)
	}

	sessionCookie := findSessionCookie(t, importResp.Result())
	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}

	session := decryptTestSession(t, app, sessionCookie.Value)
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

	newTestSoccerHandler(app).FetchSchedulesHandler(fetchResp, fetchReq)

	if fetchResp.Code != http.StatusOK {
		t.Fatalf("unexpected fetch status code: got %d want %d", fetchResp.Code, http.StatusOK)
	}
	// my_teams is called once at import (to populate KnownTeams) and once at fetch
	// (to resolve player teams into games) — 2 total per player.
	if got := requestCounts["/players/1669080/my_teams"]; got != 2 {
		t.Fatalf("unexpected player 1669080 my_teams call count: got %d want 2", got)
	}
	if got := requestCounts["/players/1669081/my_teams"]; got != 2 {
		t.Fatalf("unexpected player 1669081 my_teams call count: got %d want 2", got)
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
			jwt:         testutil.TestJWT(t, time.Now().Add(-30*time.Minute)),
			wantMessage: "This JWT has expired.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("did not expect upstream call for %s token validation test", tc.name)
			}))
			defer server.Close()

			app.Config.LPSAPIBaseURL = server.URL

			req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
				"jwt": {tc.jwt},
			}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp := httptest.NewRecorder()

			newTestSoccerHandler(app).ImportHandler(resp, req)

			if resp.Code != http.StatusOK {
				t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
			}
			if strings.Contains(resp.Header().Get("Set-Cookie"), config.LPSSessionCookieName+"=") {
				t.Fatalf("did not expect a session cookie on invalid JWT: %q", resp.Header().Get("Set-Cookie"))
			}
			if !strings.Contains(resp.Body.String(), tc.wantMessage) {
				t.Fatalf("unexpected response body: %q", resp.Body.String())
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
			app := newTestApp(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.serverResponse(w)
			}))
			defer server.Close()

			app.Config.LPSAPIBaseURL = server.URL

			req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
				"jwt": {testutil.TestJWT(t, time.Now().Add(30*time.Minute))},
			}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			resp := httptest.NewRecorder()

			newTestSoccerHandler(app).ImportHandler(resp, req)

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
	app := newTestApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/import", strings.NewReader(url.Values{
		"jwt": {testutil.TestJWT(t, time.Now().Add(30*time.Minute))},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).ImportHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "Could not reach Let&#39;s Play Soccer to look up your players. Try again in a moment.") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestSoccerLogoutHandlerClearsSessionAndRendersUnauthenticatedPanel(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soccer/logout", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:      testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Current browser session",
		Players: []types.LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).LogoutHandler(resp, req)

	result := resp.Result()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
	}
	if got := result.Header.Get("HX-Trigger"); got != "soccer-logout" {
		t.Fatalf("unexpected HX-Trigger header: got %q want %q", got, "soccer-logout")
	}

	assertClearedSessionCookie(t, result)
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
	app := newTestApp(t)

	tests := []struct {
		name      string
		addCookie func(t *testing.T, req *http.Request)
	}{
		{
			name: "expired session",
			addCookie: func(t *testing.T, req *http.Request) {
				addSessionCookie(t, app, req, &types.SessionData{
					JWT:      testutil.TestJWT(t, time.Now().Add(-30*time.Minute)),
					UserName: "Current browser session",
					Players: []types.LPSPlayer{
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

			newTestSoccerHandler(app).SessionHandler(resp, req)

			result := resp.Result()
			if result.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status code: got %d want %d", result.StatusCode, http.StatusOK)
			}

			assertClearedSessionCookie(t, result)
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
