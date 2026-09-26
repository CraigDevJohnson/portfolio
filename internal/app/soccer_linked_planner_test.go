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

	"portfolio/internal/testutil"
)

func TestGrantedLinkedPlayerPlannerFromOutputChoiceThroughICS(t *testing.T) {
	identity := newFakeSiteCognito(t)
	application := identity.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(30*time.Minute))
	soon := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	middle := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	late := testutil.MislabelledLPSZuluTime(time.Now().Add(72 * time.Hour))
	var denyPlayerTeams atomic.Bool
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/check":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("import omitted the LPS bearer token")
			}
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson"}],"user_players":[{"player_id":1001,"deleted":false},{"player_id":1002,"deleted":false}]}`))
		case "/players/1001/my_teams":
			if denyPlayerTeams.Load() {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("team discovery omitted the LPS bearer token")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":101,"team_name":"North FC","Season":77}]`))
		case "/players/1002/my_teams":
			if denyPlayerTeams.Load() {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("team discovery omitted the LPS bearer token")
			}
			_, _ = w.Write([]byte(`[{"UTeamID":202,"team_name":"South FC","Season":77}]`))
		case "/teams/101":
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":3030,"SchedGameDateTime":%q,"home_team":{"team_name":"North FC"},"visitor_team":{"team_name":"Guests"}},{"UGameID":2020,"SchedGameDateTime":%q,"home_team":{"team_name":"Shared FC"},"visitor_team":{"team_name":"Rivals"}}]}`, late, middle)
		case "/teams/202":
			_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":2020,"SchedGameDateTime":%q,"home_team":{"team_name":"Shared FC"},"visitor_team":{"team_name":"Rivals"}},{"UGameID":1010,"SchedGameDateTime":%q,"home_team":{"team_name":"South FC"},"visitor_team":{"team_name":"Guests"}}]}`, middle, soon)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lps.Close)
	application.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	siteSession := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	choice := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteSession)
	if choice.Code != http.StatusOK || !strings.Contains(choice.Body.String(), "Import access") || !strings.Contains(choice.Body.String(), "Use linked players") {
		t.Fatalf("granted visitor cannot reach linked-player import after output choice: status %d", choice.Code)
	}
	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, siteSession)
	importCookie := findSessionCookie(t, imported.Result())
	// The import's guard cookie must accompany its session, as #91 requires.
	guard := findImportGuardCookie(imported.Result())
	if imported.Code != http.StatusOK || importCookie == nil || guard == nil {
		t.Fatalf("LPS import failed: status %d", imported.Code)
	}
	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteSession, importCookie, guard)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Craig Johnson") || !strings.Contains(page.Body.String(), "Taylor Johnson") {
		t.Fatalf("linked players were not revealed after import: status %d", page.Code)
	}

	discovered := soccerGrantRequest(mux, http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}}, siteSession, importCookie, guard)
	if discovered.Code != http.StatusOK || !strings.Contains(discovered.Body.String(), "North FC") || !strings.Contains(discovered.Body.String(), "South FC") {
		t.Fatalf("current linked teams were not available: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	discoverCookie := findSessionCookie(t, discovered.Result())
	if discoverCookie == nil {
		t.Fatal("player choice did not persist")
	}
	selectedTeams := url.Values{"selection_mode": {"teams"}, "player_ids": {"1001", "1002"}, "team_ids": {"101", "202"}}
	fetched := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", selectedTeams, siteSession, discoverCookie, guard)
	if fetched.Code != http.StatusOK {
		t.Fatalf("linked-team fetch status = %d, body %q", fetched.Code, fetched.Body.String())
	}
	body := fetched.Body.String()
	positions := []int{strings.Index(body, `value="1010"`), strings.Index(body, `value="2020"`), strings.Index(body, `value="3030"`)}
	if positions[0] < 0 || positions[1] <= positions[0] || positions[2] <= positions[1] || strings.Count(body, `value="2020"`) != 1 || !strings.Contains(body, "3 games selected") {
		t.Fatalf("linked games were not unique, chronological, and selected: positions %v", positions)
	}
	if !strings.Contains(body, `data-team-fingerprint="101-202"`) || !strings.Contains(body, "Select all upcoming games") {
		t.Fatal("linked games lack a stable team selection and select-all control")
	}
	fetchCookie := findSessionCookie(t, fetched.Result())
	if fetchCookie == nil {
		t.Fatal("selected teams did not persist")
	}
	selectedICS := soccerGrantRequest(mux, http.MethodPost, "/soccer/download", url.Values{"team_codes": {"101,202"}, "player_ids": {"1001", "1002"}, "selected": {"1010"}}, siteSession, fetchCookie, guard)
	if selectedICS.Code != http.StatusOK || selectedICS.Header().Get("Content-Type") != "text/calendar" {
		t.Fatalf("linked ICS download failed: status %d", selectedICS.Code)
	}
	ics := testutil.UnfoldICS(selectedICS.Body.String())
	if strings.Count(ics, "BEGIN:VEVENT") != 1 || !strings.Contains(ics, "UID:1010") || strings.Contains(ics, "UID:2020") {
		t.Fatal("ICS did not preserve the selected game only")
	}

	denyPlayerTeams.Store(true)
	lost := soccerGrantRequest(mux, http.MethodPost, "/soccer/discover-teams", url.Values{"player_ids": {"1001"}}, siteSession, fetchCookie, guard)
	cleared := findSessionCookie(t, lost.Result())
	if lost.Code != http.StatusOK || !strings.Contains(lost.Body.String(), "token was rejected") || !strings.Contains(lost.Body.String(), "Import fresh access") || cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("lost LPS access did not clear import and offer recovery: status %d, body %q", lost.Code, lost.Body.String())
	}
	if strings.Contains(lost.Body.String(), "Craig Johnson") || strings.Contains(lost.Body.String(), "Taylor Johnson") {
		t.Fatal("lost access response exposed linked players")
	}
	stalePage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteSession, fetchCookie, guard)
	clearedPageCookie := findSessionCookie(t, stalePage.Result())
	if stalePage.Code != http.StatusOK || !strings.Contains(stalePage.Body.String(), "token was rejected") || strings.Contains(stalePage.Body.String(), "Imported in this browser") || clearedPageCookie == nil || clearedPageCookie.Value != "" {
		t.Fatalf("restored planner kept rejected LPS access: status %d", stalePage.Code)
	}

	public := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"101"}})
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "North FC") {
		t.Fatalf("public Team ID path regressed: status %d", public.Code)
	}
}
