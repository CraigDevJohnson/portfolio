package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
	"portfolio/types"
)

func TestFetchSchedulesHandlerShowsInvalidPlayerMessage(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"abc"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "selected players were invalid") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestFetchSchedulesHandlerShowsActionable401Message(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:      testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{{
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
	app.Config.LPSAPIBaseURL = server.URL
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if !strings.Contains(resp.Body.String(), "token was rejected") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerReturnsActionableInvalidPlayerError(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"game-1"},
		"player_ids": {"bad-id"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).DownloadICSHandler(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusBadRequest)
	}
	if !strings.Contains(resp.Body.String(), "selected players were invalid") {
		t.Fatalf("unexpected response body: %q", resp.Body.String())
	}
}

func TestDownloadICSHandlerExportsAuthenticatedSchedules(t *testing.T) {
	app := newTestApp(t)
	start := time.Now().Add(24 * time.Hour).Round(time.Second)
	end := start.Add(90 * time.Minute)

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
						"SchedGameDateTime": "` + start.Format(time.RFC3339) + `",
						"schedGameEndTime": "` + end.Format(time.RFC3339) + `",
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
	app.Config.LPSAPIBaseURL = server.URL

	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"888"},
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:      token,
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{{
			UPlayerID: 1001,
			FirstName: "Craig",
			LastName:  "Johnson",
		}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).DownloadICSHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/calendar" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	unfoldedICS := testutil.UnfoldICS(resp.Body.String())
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
	app := newTestApp(t)
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureUnselected := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
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

	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"7001"},
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).DownloadICSHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	if contentType := resp.Header().Get("Content-Type"); contentType != "text/calendar" {
		t.Fatalf("unexpected content type: %q", contentType)
	}
	unfoldedICS := testutil.UnfoldICS(resp.Body.String())
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

func TestResolveSyncResultsGamesIncludesPastManualTeamGamesWithResults(t *testing.T) {
	app := newTestApp(t)
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	pastWithResult := testutil.MislabelledLPSZuluTime(time.Now().Add(-48 * time.Hour))
	futureWithResult := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))

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
						"Season": 169,
						"result": "4 - 2"
					},
					{
						"UGameID": 7002,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"FacilityID": 4,
						"home_team": {"team_name": "UNITED NATIONS", "division_name": "Coed F Fri"},
						"visitor_team": {"team_name": "BENCH MOB", "division_name": "Coed F Fri"},
						"Season": 169,
						"result": "1 - 0"
					}
				]
			}`, pastWithResult, futureWithResult)))
		case "/facilities/4":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"FacilityID": 4, "FacilityName": "Boise", "Address": "11448 W. President Drive", "City": "Boise", "State": "ID", "ZIP": "83713"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/sync-results", strings.NewReader(url.Values{
		"team_codes": {"479691"},
		"selected":   {"7001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm returned error: %v", err)
	}
	resp := httptest.NewRecorder()

	_, games, message, ok := newTestSoccerHandler(app).ResolveSyncResultsGames(resp, req)
	if !ok {
		t.Fatalf("ResolveSyncResultsGames returned not ok with message %q", message)
	}
	if len(games) != 1 {
		t.Fatalf("expected one past result game, got %d (%#v)", len(games), games)
	}
	if games[0].ID != "7001" {
		t.Fatalf("unexpected synced game id: got %q want %q", games[0].ID, "7001")
	}
}

func TestResolveSyncResultsGamesRequiresSelectedPastResults(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soccer/google/sync-results", strings.NewReader(url.Values{
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm returned error: %v", err)
	}
	resp := httptest.NewRecorder()

	_, games, message, ok := newTestSoccerHandler(app).ResolveSyncResultsGames(resp, req)
	if ok {
		t.Fatalf("ResolveSyncResultsGames returned ok with games %#v", games)
	}
	if message != "Select at least one past result to sync." {
		t.Fatalf("unexpected message: got %q", message)
	}
}

func TestDownloadICSHandlerRejectsInvalidTeamSelection(t *testing.T) {
	app := newTestApp(t)
	req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
		"selected":   {"7001"},
		"team_codes": {"abc,def"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).DownloadICSHandler(resp, req)

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
			app := newTestApp(t)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()
			app.Config.LPSAPIBaseURL = server.URL

			token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
			req := httptest.NewRequest(http.MethodPost, "/soccer/download", strings.NewReader(url.Values{
				"selected":   {"game-1"},
				"player_ids": {"1001"},
			}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			addSessionCookie(t, app, req, &types.SessionData{
				JWT:      token,
				UserName: "Craig Johnson",
				Players: []types.LPSPlayer{{
					UPlayerID: 1001,
					FirstName: "Craig",
					LastName:  "Johnson",
				}},
				ExpiresAt: time.Now().Add(30 * time.Minute),
			})
			resp := httptest.NewRecorder()

			newTestSoccerHandler(app).DownloadICSHandler(resp, req)

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
	app := newTestApp(t)
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-2 * time.Hour))
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
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

	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)

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
	if strings.Contains(body, "Sync Past Results") || strings.Contains(body, "Add Selected to Google Calendar") {
		t.Fatalf("expected disconnected Google actions to be hidden from rendered schedule, got %q", body)
	}
	if !strings.Contains(body, "Google Calendar add is unavailable in this environment.") {
		t.Fatalf("expected rendered schedule to explain unavailable Google add state, got %q", body)
	}
	if !strings.Contains(body, "Download Selected (.ics)") {
		t.Fatalf("expected rendered schedule to preserve ICS download action, got %q", body)
	}
}

func TestFetchSchedulesHandlerSplitsUpcomingGamesAndPastResults(t *testing.T) {
	app := newTestApp(t)
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	olderPast := testutil.MislabelledLPSZuluTime(time.Now().Add(-72 * time.Hour))
	recentPast := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	soonUpcoming := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	laterUpcoming := testutil.MislabelledLPSZuluTime(time.Now().Add(72 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/players/1001/my_teams":
			w.Header().Set("Content-Type", "application/json")
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
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 8801,
						"SchedGameDateTime": %q,
						"field_name": "Pitch 2",
						"facilityName": "North Fieldhouse",
						"FacilityID": 12,
						"home_team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"visitor_team": {"UTeamID": 8882, "team_name": "Older Result FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"result": "4 - 2"
					},
					{
						"UGameID": 8802,
						"SchedGameDateTime": %q,
						"field_name": "Pitch 3",
						"facilityName": "North Fieldhouse",
						"FacilityID": 12,
						"home_team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"visitor_team": {"UTeamID": 8884, "team_name": "Recent Result FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"result": "3 - 1"
					},
					{
						"UGameID": 8803,
						"SchedGameDateTime": %q,
						"field_name": "Pitch 4",
						"facilityName": "North Fieldhouse",
						"FacilityID": 12,
						"home_team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"visitor_team": {"UTeamID": 8883, "team_name": "Soon Match FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300}
					},
					{
						"UGameID": 8804,
						"SchedGameDateTime": %q,
						"field_name": "Pitch 5",
						"facilityName": "North Fieldhouse",
						"FacilityID": 12,
						"home_team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300},
						"visitor_team": {"UTeamID": 8885, "team_name": "Later Match FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300}
					}
				],
				"team": {"UTeamID": 8881, "team_name": "Craig FC", "division_name": "Premier", "FacilityID": 12, "facility_name": "North Fieldhouse", "Season": 300}
			}`, olderPast, recentPast, soonUpcoming, laterUpcoming)))
		case "/facilities/12":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"FacilityID": 12, "FacilityName": "North Fieldhouse", "Address": "123 Main St", "City": "Boise", "State": "ID", "ZIP": "83709"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"player_ids": {"1001"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:      testutil.TestJWT(t, time.Now().Add(30*time.Minute)),
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{{
			UPlayerID: 1001,
			FirstName: "Craig",
			LastName:  "Johnson",
		}},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	if strings.Count(body, `<table class="games-table">`) != 2 {
		t.Fatalf("expected two rendered schedule tables, got body %q", body)
	}
	if !strings.Contains(body, "Upcoming games") {
		t.Fatalf("expected upcoming games heading, got %q", body)
	}
	if !strings.Contains(body, "Past results") {
		t.Fatalf("expected past results heading, got %q", body)
	}
	if !strings.Contains(body, "Select all upcoming games") {
		t.Fatalf("expected upcoming select-all label, got %q", body)
	}
	if !strings.Contains(body, "Select all past results") {
		t.Fatalf("expected past select-all label, got %q", body)
	}
	if strings.Contains(body, "Select all games") {
		t.Fatalf("expected global select-all label to be removed, got %q", body)
	}
	if !strings.Contains(body, "Soon Match FC") || !strings.Contains(body, "Later Match FC") {
		t.Fatalf("expected upcoming game to render, got %q", body)
	}
	if !strings.Contains(body, "Recent Result FC") || !strings.Contains(body, "Older Result FC") {
		t.Fatalf("expected past result to render, got %q", body)
	}
	if strings.Index(body, "Upcoming games") > strings.Index(body, "Past results") {
		t.Fatalf("expected upcoming section to render before past results, got %q", body)
	}
	if strings.Index(body, "Soon Match FC") > strings.Index(body, "Later Match FC") {
		t.Fatalf("expected upcoming games to render soonest first, got %q", body)
	}
	if strings.Index(body, "Recent Result FC") > strings.Index(body, "Older Result FC") {
		t.Fatalf("expected past results to render newest first, got %q", body)
	}
}

func TestFetchSchedulesHandlerShowsConnectGoogleCTAWhenAvailableButDisconnected(t *testing.T) {
	app := newTestApp(t)
	app.Config.GoogleClientID = "client-id"
	app.Config.GoogleClientSecret = "client-secret"
	app.Config.GoogleConnectionTableName = "connections-table"

	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		time.Local = previousLocal
	}()

	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/479691" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"games": [
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
		}`, future)))
	}))
	defer server.Close()

	app.Config.LPSAPIBaseURL = server.URL

	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(url.Values{
		"team_codes": {"479691"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()

	newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusOK)
	}
	body := resp.Body.String()
	if !strings.Contains(body, "Connect Google Calendar") {
		t.Fatalf("expected rendered schedule to show connect Google CTA, got %q", body)
	}
	if strings.Contains(body, "Google Calendar add is unavailable in this environment.") {
		t.Fatalf("expected rendered schedule to avoid unavailable message when Google is configured, got %q", body)
	}
	if strings.Contains(body, "Sync Past Results") || strings.Contains(body, "Add Selected to Google Calendar") {
		t.Fatalf("expected disconnected Google actions to stay hidden until connected, got %q", body)
	}
}
