package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
	"portfolio/types"
)

type appTestGoogleConnectionStore struct {
	records map[string]internalgoogle.ConnectionRecord
}

func (s *appTestGoogleConnectionStore) Delete(_ context.Context, connectionID string) error {
	delete(s.records, connectionID)
	return nil
}

func (s *appTestGoogleConnectionStore) Get(_ context.Context, connectionID string) (*internalgoogle.ConnectionRecord, error) {
	record, ok := s.records[connectionID]
	if !ok {
		return nil, nil
	}
	clone := record
	return &clone, nil
}

func (s *appTestGoogleConnectionStore) Put(_ context.Context, record *internalgoogle.ConnectionRecord) error {
	s.records[record.ConnectionID] = *record
	return nil
}

func TestSoccerPageRendersAuthPanelOnFirstPaint(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).SoccerPage(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	for _, marker := range []string{`id="soccer-connections"`, `id="soccer-lps-connection"`, `id="soccer-google-connection"`} {
		if !strings.Contains(body, marker) {
			t.Fatalf("expected Soccer connection marker %q in initial page render", marker)
		}
	}
	if strings.Contains(body, `hx-get="/soccer/session"`) {
		t.Fatalf("expected initial soccer page render to avoid HTMX auth bootstrap, got %q", body)
	}
	if strings.Contains(body, "Loading Google Calendar connection") || strings.Contains(body, "Loading import options") {
		t.Fatalf("expected initial soccer page render to avoid loading placeholders, got %q", body)
	}
	if !strings.Contains(body, "Import access") {
		t.Fatalf("expected initial soccer page render to include import controls, got %q", body)
	}
}

func TestSoccerPageRendersImportedPlayersOnFirstPaintWhenSessionExists(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:      testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{{
			UPlayerID:    1001,
			FirstName:    "Craig",
			LastName:     "Johnson",
			IsMainPlayer: true,
		}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).SoccerPage(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "Choose players to continue") {
		t.Fatalf("expected imported player selection in initial page render, got %q", body)
	}
	if !strings.Contains(body, "Craig Johnson") {
		t.Fatalf("expected imported player name in initial page render, got %q", body)
	}
}

func TestSoccerLoginStateCountsUniqueConfirmedTeams(t *testing.T) {
	app := newTestApp(t)
	session := &types.SessionData{
		Players: []types.LPSPlayer{{UPlayerID: 1001}, {UPlayerID: 1002}},
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{1001, 1002},
			SelectedTeamIDs:   []int{4101, 4101, 4102},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	resp := httptest.NewRecorder()

	props := newTestSoccerHandler(app).LoginStateProps(resp, req, session, false)
	if props.ConfirmedTeamCount != 2 {
		t.Fatalf("confirmed team count = %d, want 2 unique teams", props.ConfirmedTeamCount)
	}
}

func TestSoccerPageRestoresWorkflowAndReportsMissingGoogleConnection(t *testing.T) {
	app := newTestApp(t)
	app.Config.GoogleClientID = "configured-client"
	app.Config.GoogleClientSecret = "configured-secret"
	app.Config.GoogleConnectionTableName = "configured-table"
	app.GoogleHandler.SetStore(&appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}})
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/my_teams":
			_, _ = w.Write([]byte(`[{"UTeamID":4101,"team_name":"Craig FC","Season":77}]`))
		case "/teams/4101":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games":[{
					"UGameID":9101,
					"SchedGameDateTime":%q,
					"field_name":"Pitch 4",
					"facilityName":"North Campus",
					"home_team":{"team_name":"Craig FC"},
					"visitor_team":{"team_name":"Rivals"},
					"Season":77
				}]
			}`, future)))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodGet, "/soccer?google=connected", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:       testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}, {UPlayerID: 1002, FirstName: "Taylor", LastName: "Johnson"}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{1001},
			SelectedTeamIDs:   []int{4101},
		},
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).SoccerPage(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	for _, marker := range []string{"Craig Johnson", "Craig FC", "Rivals", "Pitch 4", "Google Calendar connection was not restored"} {
		if !strings.Contains(body, marker) {
			t.Errorf("restored Soccer page missing %q", marker)
		}
	}
	if strings.Contains(body, "Google Calendar connected. Choose a calendar") {
		t.Error("Soccer page showed a successful Google connection flash without a persisted connection")
	}
	for _, value := range []string{"1001", "4101"} {
		if !strings.Contains(body, `value="`+value+`" checked`) {
			t.Errorf("restored Soccer page does not check ID %s", value)
		}
	}
	if strings.Contains(body, `value="1002" checked`) {
		t.Error("restored Soccer page checked an unselected player")
	}
}

func TestSoccerOAuthRoundTripPreservesImportedWorkflowAndRendersConnectedState(t *testing.T) {
	app := newTestApp(t)
	app.Config.GoogleClientID = "configured-client"
	app.Config.GoogleClientSecret = "configured-secret"
	app.Config.GoogleConnectionTableName = "configured-table"
	store := &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	app.GoogleHandler.SetStore(store)

	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/players/1001/my_teams":
			_, _ = w.Write([]byte(`[{"UTeamID":4101,"team_name":"Craig FC","Season":77}]`))
		case "/teams/4101":
			_, _ = w.Write([]byte(fmt.Sprintf(`{"games":[{"UGameID":9101,"SchedGameDateTime":%q,"field_name":"Pitch 4","facilityName":"North Campus","home_team":{"team_name":"Craig FC"},"visitor_team":{"team_name":"Rivals"},"Season":77}]}`, future)))
		default:
			t.Fatalf("unexpected LPS path: %s", r.URL.Path)
		}
	}))
	defer lpsServer.Close()
	app.Config.LPSAPIBaseURL = lpsServer.URL

	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-access","refresh_token":"test-refresh","token_type":"Bearer","expires_in":3600}`))
		case "/calendar/v3/users/me/calendarList":
			if got := r.Header.Get("Authorization"); got != "Bearer test-access" {
				t.Fatalf("unexpected Google authorization header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true}]}`))
		default:
			t.Fatalf("unexpected Google path: %s", r.URL.Path)
		}
	}))
	defer googleServer.Close()
	app.GoogleHandler.OAuthAuthURL = googleServer.URL + "/oauth/auth"
	app.GoogleHandler.OAuthTokenURL = googleServer.URL + "/oauth/token"
	app.GoogleHandler.CalendarAPIBaseURL = googleServer.URL + "/calendar/v3"

	workflow := &types.SessionData{
		JWT:       testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{1001},
			SelectedTeamIDs:   []int{4101},
		},
	}
	encryptedSession := encryptTestSession(t, app, workflow)
	lpsCookie := &http.Cookie{Name: config.LPSSessionCookieName, Value: encryptedSession}

	connectReq := httptest.NewRequest(http.MethodGet, "/soccer/google/connect", nil)
	connectReq.Host = "example.com"
	connectReq.AddCookie(lpsCookie)
	connectResp := httptest.NewRecorder()
	app.GoogleHandler.ConnectHandler(connectResp, connectReq)
	connectResult := connectResp.Result()
	connectLocation, err := connectResult.Location()
	if err != nil {
		t.Fatalf("connect redirect: %v", err)
	}
	stateValue := connectLocation.Query().Get("state")
	if stateValue == "" {
		t.Fatal("connect redirect lacked OAuth state")
	}
	var stateCookie *http.Cookie
	for _, cookie := range connectResult.Cookies() {
		if cookie.Name == config.GoogleOAuthStateCookieName {
			stateCookie = cookie
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("connect response lacked OAuth state cookie")
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/soccer/google/callback?code=auth-code&state="+url.QueryEscape(stateValue), nil)
	callbackReq.Host = "example.com"
	callbackReq.AddCookie(lpsCookie)
	callbackReq.AddCookie(stateCookie)
	callbackResp := httptest.NewRecorder()
	app.GoogleHandler.CallbackHandler(callbackResp, callbackReq)
	callbackResult := callbackResp.Result()
	if location := callbackResp.Header().Get("Location"); location != "/soccer?google=connected" {
		t.Fatalf("callback redirect = %q, want connected Soccer page", location)
	}
	var connectionCookie *http.Cookie
	for _, cookie := range callbackResult.Cookies() {
		if cookie.Name == config.GoogleConnectionCookieName {
			connectionCookie = cookie
		}
		if cookie.Name == config.LPSSessionCookieName && cookie.MaxAge < 0 {
			t.Fatal("Google callback cleared the imported LPS session")
		}
	}
	if connectionCookie == nil {
		t.Fatal("callback response lacked Google connection cookie")
	}

	pageReq := httptest.NewRequest(http.MethodGet, "/soccer?google=connected", nil)
	pageReq.Host = "example.com"
	pageReq.AddCookie(lpsCookie)
	pageReq.AddCookie(connectionCookie)
	pageResp := httptest.NewRecorder()
	newTestSoccerHandler(app).SoccerPage(pageResp, pageReq)
	pageBody := pageResp.Body.String()
	for _, marker := range []string{
		"Google Calendar connected", "Connected to Primary Calendar", "Imported for this session",
		"Craig Johnson", "Craig FC", "Rivals", "Pitch 4", "1 selected", "1 confirmed",
	} {
		if !strings.Contains(pageBody, marker) {
			t.Errorf("connected OAuth return page lacks %q", marker)
		}
	}
	if strings.Contains(pageBody, "Google Calendar connection was not restored") {
		t.Error("connected OAuth return page rendered the missing-connection recovery message")
	}
}

func TestSoccerPageKeepsRestoredChoicesWhenScheduleRefreshFails(t *testing.T) {
	app := newTestApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/players/1001/my_teams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"UTeamID":4101,"team_name":"Craig FC","Season":77}]`))
			return
		}
		if r.URL.Path == "/teams/4101" {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()
	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:       testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{1001},
			SelectedTeamIDs:   []int{4101},
		},
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).SoccerPage(resp, req)

	body := resp.Body.String()
	for _, marker := range []string{"Craig Johnson", "Craig FC", "could not be refreshed"} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(marker)) {
			t.Errorf("restore failure page missing %q", marker)
		}
	}
	if cookie := findSessionCookie(t, resp.Result()); cookie != nil && cookie.MaxAge < 0 {
		t.Fatal("restore failure cleared the workflow session")
	}
}

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
	if got := result.Header.Get("HX-Trigger"); got != "soccer-workflow-reset" {
		t.Fatalf("unexpected import HX-Trigger: got %q want %q", got, "soccer-workflow-reset")
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
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected same-site mode: got %v want Lax for Google OAuth restoration", sessionCookie.SameSite)
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
	firstGameAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	secondGameAt := time.Now().Add(8 * 24 * time.Hour).UTC().Format(time.RFC3339)
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
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 9001,
						"SchedGameDateTime": %q,
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
			}`, firstGameAt)))
		case "/teams/479400":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 9002,
						"SchedGameDateTime": %q,
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
			}`, secondGameAt)))
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
	// Import only discovers players; team lookup happens once when schedules are requested.
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
	if !strings.Contains(resp.Body.String(), "Not imported") {
		t.Fatalf("expected unauthenticated auth panel, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Import access") {
		t.Fatalf("expected unauthenticated import control, got %q", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "Clear import") {
		t.Fatalf("did not expect authenticated controls after logout, got %q", resp.Body.String())
	}
	for _, resetID := range []string{
		"soccer-player-stage-content", "soccer-team-stage-content", "games-container",
	} {
		if !strings.Contains(resp.Body.String(), `id="`+resetID+`" hx-swap-oob="innerHTML"`) {
			t.Errorf("logout response lacks OOB reset for %s", resetID)
		}
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
			body := resp.Body.String()
			lpsStart := strings.Index(body, `id="soccer-lps-connection"`)
			if lpsStart < 0 {
				t.Fatalf("expected /soccer/session to render the auth panel, got %q", body)
			}
			lpsEnd := strings.Index(body[lpsStart:], `>`)
			if lpsEnd < 0 || strings.Contains(body[lpsStart:lpsStart+lpsEnd+1], "hx-swap-oob") {
				t.Fatalf("expected /soccer/session to render the auth panel as primary content, got %q", body)
			}
			if strings.Contains(body, `id="soccer-configuration"`) {
				t.Fatalf("expected /soccer/session to omit the removed duplicate configuration summary, got %q", body)
			}
			if !strings.Contains(body, "Not imported") {
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
