package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/soccerarchive"
	"portfolio/internal/testutil"
)

type recordingTeamArchive struct {
	snapshots []soccerarchive.Snapshot
}

func (archive *recordingTeamArchive) SaveTeamSnapshot(_ context.Context, snapshot *soccerarchive.Snapshot) error {
	archive.snapshots = append(archive.snapshots, *snapshot)
	return nil
}

type recordingPlayerArchive struct {
	recordingTeamArchive

	discoveries []soccerarchive.PlayerDiscovery
}

func (archive *recordingPlayerArchive) SavePlayerDiscovery(_ context.Context, discovery *soccerarchive.PlayerDiscovery) error {
	archive.discoveries = append(archive.discoveries, *discovery)
	return nil
}

func TestSoccerImportDisclosesAndCapturesEveryLinkedPlayersMembership(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(time.Hour))
	requests := map[string]int{}
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		switch r.URL.Path {
		case "/users/check":
			if r.Header.Get("Authorization") != "Bearer "+token {
				t.Errorf("users/check did not receive imported JWT")
			}
			_, _ = fmt.Fprint(w, `{"players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson","is_main_player":false}],"user_players":[{"player_id":1001},{"player_id":1002}]}`)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","Season":77},{"UTeamID":4102,"team_name":"Old FC","Season":78}]`)
		case "/players/1002/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","Season":77},{"UTeamID":4202,"team_name":"Taylor FC","Season":79},{"UTeamID":4300,"team_name":"Unknown Season"}]`)
		case "/teams/4101":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":4101,"team_name":"Craig FC","Season":77},"games":[]}`)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(application, application.Logger, false)
	archive := &recordingPlayerArchive{}
	handler.SetArchiveStore(archive)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	handler.SetArchiveStore(nil)
	offPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie)
	if !strings.Contains(offPage.Body.String(), "History collection is currently off") || strings.Contains(offPage.Body.String(), "are retained indefinitely") {
		t.Fatal("disabled collection page described durable collection as active")
	}
	handler.SetArchiveStore(archive)
	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, ownerCookie)
	for _, notice := range []string{"retained indefinitely", "every linked player", "refresh"} {
		if !strings.Contains(page.Body.String(), notice) {
			t.Errorf("import page omitted %q", notice)
		}
	}
	if len(archive.discoveries) != 0 {
		t.Fatal("opening the import page stored player evidence")
	}

	unauthorized := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}})
	if unauthorized.Code != http.StatusUnauthorized || len(archive.discoveries) != 0 || len(requests) != 0 {
		t.Fatalf("anonymous import collected data: status %d, discoveries %d, LPS requests %#v", unauthorized.Code, len(archive.discoveries), requests)
	}
	application.Config.SiteInvitations["owner@example.com"] = nil
	revoked := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if revoked.Code != http.StatusForbidden || len(archive.discoveries) != 0 || len(requests) != 0 {
		t.Fatalf("revoked import collected data: status %d, discoveries %d, LPS requests %#v", revoked.Code, len(archive.discoveries), requests)
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}

	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "Choose your players") {
		t.Fatalf("granted import failed: status %d, body %q", imported.Code, imported.Body.String())
	}
	if requests["/users/check"] != 1 || requests["/players/1001/my_teams"] != 1 || requests["/players/1002/my_teams"] != 1 || requests["/teams/4101"] != 0 {
		t.Fatalf("import did not discover all players before planner selection: %#v", requests)
	}
	if len(archive.discoveries) != 1 {
		t.Fatalf("stored discoveries = %d, want 1", len(archive.discoveries))
	}
	got := archive.discoveries[0]
	if got.OwnerIssuer != fixture.issuer || got.OwnerSubject != "stable-subject" || len(got.Players) != 2 || len(got.Memberships) != 4 || got.ObservedAt.IsZero() {
		t.Fatalf("owner-bound all-player evidence missing: %#v", got)
	}
	if got.Players[0].UPlayerID != 1001 || !got.Players[0].IsMainPlayer || got.Players[1].UPlayerID != 1002 || got.Players[1].IsMainPlayer {
		t.Fatalf("player identity or flags missing: %#v", got.Players)
	}
	want := map[[3]int]bool{{1001, 4101, 77}: true, {1001, 4102, 78}: true, {1002, 4101, 77}: true, {1002, 4202, 79}: true}
	for _, membership := range got.Memberships {
		key := [3]int{membership.PlayerID, membership.Team.UTeamID, membership.Team.Season}
		if !want[key] {
			t.Errorf("unexpected membership %#v", membership)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing exact membership associations: %#v", want)
	}
	known := map[int]bool{4101: true, 4102: true, 4202: true, 4300: true}
	for _, team := range got.KnownTeams {
		if !known[team.UTeamID] {
			t.Errorf("unexpected enrolled team %#v", team)
		}
		delete(known, team.UTeamID)
	}
	if len(known) != 0 {
		t.Errorf("unselected or seasonless teams were not enrolled: %#v", known)
	}
	if len(archive.snapshots) != 0 {
		t.Fatal("player import selected or fetched planner games")
	}
	sessionCookie := findSessionCookie(t, imported.Result())
	if sessionCookie == nil {
		t.Fatal("import did not retain LPS access")
	}
	session := decryptTestSession(t, application, sessionCookie.Value)
	if session.Workflow.Source != "" || len(session.Workflow.SelectedPlayerIDs) != 0 || len(session.Workflow.SelectedTeamIDs) != 0 {
		t.Fatalf("import preselected planner choices: %#v", session.Workflow)
	}

	manual := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}})
	if manual.Code != http.StatusOK || len(archive.snapshots) != 1 || len(archive.discoveries) != 1 {
		t.Fatalf("manual lookup created membership or failed to archive team: status %d, snapshots %d, discoveries %d", manual.Code, len(archive.snapshots), len(archive.discoveries))
	}
}

func TestSoccerImportDoesNotStorePartialMembershipWhenAPlayerLookupFails(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/check":
			_, _ = fmt.Fprint(w, `{"players":[{"UPlayerID":1001},{"UPlayerID":1002}]}`)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"Season":77}]`)
		case "/players/1002/my_teams":
			http.Error(w, "temporary failure", http.StatusBadGateway)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(application, application.Logger, false)
	archive := &recordingPlayerArchive{}
	handler.SetArchiveStore(archive)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))

	result := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), "Could not look up every linked player") || len(archive.discoveries) != 0 || findSessionCookie(t, result.Result()) != nil {
		t.Fatalf("partial discovery was treated as a complete import: status %d, body %q, writes %d", result.Code, result.Body.String(), len(archive.discoveries))
	}
}
