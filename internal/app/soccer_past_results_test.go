package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

func TestGoogleModeReviewsScoredPastGamesFromTeamIDsAndLinkedPlayersWithoutWriting(t *testing.T) {
	identity := newFakeSiteCognito(t)
	application := identity.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	application.Config.GoogleClientID = "google-client"
	application.Config.GoogleClientSecret = "google-secret"
	application.Config.GoogleConnectionTableName = "connections"
	application.GoogleHandler.SetStore(&appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}})
	var googleCalls atomic.Int32
	googleAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleCalls.Add(1)
		http.Error(w, "unexpected Google request", http.StatusInternalServerError)
	}))
	t.Cleanup(googleAPI.Close)
	application.GoogleHandler.CalendarAPIBaseURL = googleAPI.URL
	application.GoogleHandler.OAuthTokenURL = googleAPI.URL

	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	upcoming := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	recent := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	old := testutil.MislabelledLPSZuluTime(time.Now().Add(-900 * 24 * time.Hour))
	unscored := testutil.MislabelledLPSZuluTime(time.Now().Add(-48 * time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/check":
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson"}],"user_players":[{"player_id":1001,"deleted":false},{"player_id":1002,"deleted":false}]}`))
		case "/players/1001/my_teams":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("linked team lookup omitted imported JWT")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":101,"team_name":"North FC","Season":77}]`))
		case "/players/1002/my_teams":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("linked team lookup omitted imported JWT")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":202,"team_name":"South FC","Season":77}]`))
		case "/teams/101":
			_, _ = fmt.Fprintf(w, `{"games":[
				{"UGameID":704,"SchedGameDateTime":%q,"home_team":{"team_name":"North FC"},"visitor_team":{"team_name":"Guests"}},
				{"UGameID":702,"SchedGameDateTime":%q,"Result":"1-0","home_team":{"team_name":"Old FC"},"visitor_team":{"team_name":"Rivals"}},
				{"UGameID":703,"SchedGameDateTime":%q,"home_team":{"team_name":"No Score FC"},"visitor_team":{"team_name":"Rivals"}},
				{"UGameID":701,"SchedGameDateTime":%q,"Result":"3-1","home_team":{"team_name":"Recent FC"},"visitor_team":{"team_name":"Rivals"}}
			]}`, upcoming, old, unscored, recent)
		case "/teams/202":
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":701,"SchedGameDateTime":%q,"Result":"3-1","home_team":{"team_name":"Recent FC"},"visitor_team":{"team_name":"Rivals"}}]}`, recent)
		default:
			t.Errorf("unexpected LPS path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	siteSession := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	manual := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"101,202"}}, siteSession)
	assertPastResultReview(t, manual, "manual Team IDs")
	if googleCalls.Load() != 0 {
		t.Fatal("reviewing Team ID results contacted Google Calendar")
	}

	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, siteSession)
	importCookie := findSessionCookie(t, imported.Result())
	if imported.Code != http.StatusOK || importCookie == nil {
		t.Fatalf("linked-player import status = %d", imported.Code)
	}
	discovered := soccerGrantRequest(mux, http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}}, siteSession, importCookie)
	discoverCookie := findSessionCookie(t, discovered.Result())
	if discovered.Code != http.StatusOK || discoverCookie == nil || !strings.Contains(discovered.Body.String(), "North FC") || !strings.Contains(discovered.Body.String(), "South FC") {
		t.Fatalf("linked team discovery failed: status %d", discovered.Code)
	}
	linked := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"selection_mode": {"teams"}, "player_ids": {"1001", "1002"}, "team_ids": {"101", "202"}}, siteSession, discoverCookie)
	assertPastResultReview(t, linked, "linked players")
	if googleCalls.Load() != 0 {
		t.Fatal("reviewing linked-player results contacted Google Calendar")
	}

	sync := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"101,202"}, "selected": {"701"}}, siteSession)
	if sync.Code != http.StatusOK || !strings.Contains(sync.Body.String(), "Connect Google Calendar before syncing results") || googleCalls.Load() != 0 {
		t.Fatalf("Sync without a connection reached Google: status %d, calls %d", sync.Code, googleCalls.Load())
	}
	ics := soccerGrantRequest(mux, http.MethodPost, "/soccer/download", url.Values{"team_codes": {"101,202"}, "selected": {"701"}}, siteSession)
	if ics.Code != http.StatusBadRequest || strings.Contains(ics.Body.String(), "BEGIN:VEVENT") {
		t.Fatalf("ICS exported a past result: status %d", ics.Code)
	}
}

func assertPastResultReview(t *testing.T, response *httptest.ResponseRecorder, source string) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("%s fetch status = %d", source, response.Code)
	}
	body := response.Body.String()
	newest := strings.Index(body, `value="701"`)
	oldest := strings.Index(body, `value="702"`)
	if newest < 0 || oldest <= newest || strings.Count(body, `value="701"`) != 1 || strings.Contains(body, `value="703"`) {
		t.Fatalf("%s past results were missing, duplicated, out of order, or included an unscored game", source)
	}
	if !strings.Contains(body, `data-game-group="past-results"`) || !strings.Contains(body, "2 games selected") || !strings.Contains(body, "Select all past results") {
		t.Fatalf("%s past results lack selected count or select-all controls", source)
	}
	if !strings.Contains(body, `data-soccer-output-only="google" hidden`) {
		t.Fatalf("%s past results are not hidden until Google mode is chosen", source)
	}
}
