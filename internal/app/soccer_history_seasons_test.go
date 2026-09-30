package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"portfolio/internal/config"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/internal/testutil"
)

// teamSeasonsPath is the private list of one player's proven team seasons.
func teamSeasonsPath(playerID int) string {
	return fmt.Sprintf("/soccer/history/team-seasons?player_id=%d", playerID)
}

// provenTeamSeasons is the list contract as a stats view would decode it.
type provenTeamSeasons struct {
	PlayerID        int  `json:"player_id"`
	CurrentVerified bool `json:"current_verified"`
	TeamSeasons     []struct {
		TeamID      int `json:"team_id"`
		LPSSeasonID int `json:"lps_season_id"`
		Team        struct {
			UTeamID      int    `json:"UTeamID"`
			TeamName     string `json:"team_name"`
			DivisionName string `json:"division_name"`
			Season       int    `json:"Season"`
		} `json:"team"`
		Current bool `json:"current"`
	} `json:"team_seasons"`
}

// summary lists each team season as "team/season name current|former".
func (list *provenTeamSeasons) summary() string {
	entries := make([]string, 0, len(list.TeamSeasons))
	for _, season := range list.TeamSeasons {
		when := "former"
		if season.Current {
			when = "current"
		}
		entries = append(entries, fmt.Sprintf("%d/%d %s %s", season.TeamID, season.LPSSeasonID, season.Team.TeamName, when))
	}
	return strings.Join(entries, ", ")
}

// listTeamSeasons requires an authorized list and decodes its body.
func listTeamSeasons(t *testing.T, browser *siteBrowser, playerID int) provenTeamSeasons {
	t.Helper()
	response := browser.get(teamSeasonsPath(playerID))
	if response.Code != http.StatusOK {
		t.Fatalf("team-season list for player %d: status %d, body %q", playerID, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("team-season list Cache-Control = %q, want private, no-store", got)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("team-season list Content-Type = %q", got)
	}
	var list provenTeamSeasons
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode team-season list: %v: %s", err, response.Body.String())
	}
	if list.PlayerID != playerID {
		t.Errorf("team-season list player = %d, want %d", list.PlayerID, playerID)
	}
	return list
}

// A player's former seasons, which LPS no longer lists, are found from the
// owner's stored proof, beside the seasons LPS lists now, newest LPS season
// first, and each one opens with the per-season read.
func TestSoccerHistoryTeamSeasonsListCurrentAndFormerSeasonsThatTheSeasonReadOpens(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	route.setTeam(4102, oldFCSeason78)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101, 4102); !report.Complete {
		t.Fatalf("refresh: %+v", report)
	}
	// Craig has moved on: LPS now lists only Craig FC's next season, and a
	// team without an LPS season, which proves no season.
	route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":80},{"UTeamID":4300,"team_name":"No Season FC"}]`)

	craig := listTeamSeasons(t, owner, 1001)

	if got, want := craig.summary(), "4101/80 Craig FC current, 4102/78 Old FC former, 4101/77 Craig FC former"; got != want {
		t.Fatalf("Craig's team seasons = %q, want %q", got, want)
	}
	if !craig.CurrentVerified {
		t.Error("Craig's list after LPS answered his current team lookup is not current_verified")
	}
	for _, listed := range craig.TeamSeasons {
		if listed.Team.UTeamID != listed.TeamID || listed.Team.Season != listed.LPSSeasonID {
			t.Errorf("listed team %+v does not match team %d season %d", listed.Team, listed.TeamID, listed.LPSSeasonID)
		}
		history := readHistory(t, owner, 1001, listed.TeamID, listed.LPSSeasonID)
		if history.TeamID != listed.TeamID || history.LPSSeasonID != listed.LPSSeasonID {
			t.Errorf("season read for listed team %d season %d = team %d season %d", listed.TeamID, listed.LPSSeasonID, history.TeamID, history.LPSSeasonID)
		}
	}
	if division := craig.TeamSeasons[2].Team.DivisionName; division != "Open A" {
		t.Errorf("Craig FC season 77 division = %q, want the stored proof's Open A", division)
	}
	if former := readHistory(t, owner, 1001, 4102, 78); former.Record.Wins != 1 || len(former.Games) != 1 {
		t.Errorf("listed former Old FC season = %+v", former)
	}

	// Taylor's seasons are both stored and still listed by LPS.
	taylor := listTeamSeasons(t, owner, 1002)
	if got, want := taylor.summary(), "4202/79 Taylor FC current, 4101/77 Craig FC current"; got != want {
		t.Errorf("Taylor's team seasons = %q, want %q", got, want)
	}
}

// The list reads only the signed-in owner's stored proof. Neither another
// invited account nor a new identity with the first owner's email, importing
// the same LPS account, finds the seasons only the first owner proved.
func TestSoccerHistoryTeamSeasonsNeverListAnotherSiteOwnersProof(t *testing.T) {
	for _, successor := range []struct {
		name           string
		subject, email string
	}{
		{"another invited account", otherSiteSubject, otherSiteEmail},
		{"a new identity with the same email", "recreated-subject", testSiteEmail},
	} {
		t.Run(successor.name, func(t *testing.T) {
			route := newTeamHistoryRoute(t)
			route.app.Config.SiteInvitations[successor.email] = []string{"soccer"}
			owner := route.signedIn(t)
			route.importLinkedPlayers(t, owner)
			// Craig has left Old FC and Craig FC's season 77, so only the
			// first owner's stored proof holds them.
			route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","Season":80}]`)
			former := listTeamSeasons(t, owner, 1001)
			if got, want := former.summary(), "4101/80 Craig FC current, 4102/78 Old FC former, 4101/77 Craig FC former"; got != want {
				t.Fatalf("first owner's team seasons = %q, want %q", got, want)
			}

			// The successor imports the same LPS account.
			route.cognito.subject, route.cognito.email = successor.subject, successor.email
			other := route.signedIn(t)
			route.importLinkedPlayers(t, other)
			successors := listTeamSeasons(t, other, 1001)
			if got, want := successors.summary(), "4101/80 Craig FC current"; got != want {
				t.Errorf("successor's team seasons = %q, want only the season LPS lists now", got)
			}

			// The successor signs in to the first owner's browser, which
			// drops the first owner's import.
			owner.expireSiteSession()
			if landing := owner.signIn("/soccer"); landing.Code != http.StatusSeeOther {
				t.Fatalf("successor's sign-in to the first owner's browser: status %d", landing.Code)
			}
			assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)

			route.cognito.subject, route.cognito.email = "stable-subject", testSiteEmail
			first := route.signedIn(t)
			route.importLinkedPlayers(t, first)
			again := listTeamSeasons(t, first, 1001)
			if got, want := again.summary(), "4101/80 Craig FC current, 4102/78 Old FC former, 4101/77 Craig FC former"; got != want {
				t.Errorf("first owner's team seasons after signing in again = %q, want %q", got, want)
			}
		})
	}
}

// assertTeamSeasonsDenied requires a list to be refused with status and to
// disclose no team season.
func assertTeamSeasonsDenied(t *testing.T, browser *siteBrowser, status, playerID int) {
	t.Helper()
	response := browser.get(teamSeasonsPath(playerID))
	if response.Code != status || strings.Contains(response.Body.String(), "team_seasons") || strings.Contains(response.Body.String(), "UTeamID") {
		t.Errorf("team-season list for player %d: status %d, body %q; want %d without seasons", playerID, response.Code, response.Body.String(), status)
	}
	if got := response.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("refused team-season list Cache-Control = %q, want no-store", got)
	}
}

// The list is refused exactly as the per-season read is: without a site
// session, without the current soccer grant, without a valid same-owner
// import, and for a player the import does not confirm.
func TestSoccerHistoryTeamSeasonsNeedTheSameAuthorityAsTheSeasonRead(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	route.setPlayerTeams(1001, `[]`)
	listTeamSeasons(t, owner, 1001)

	assertTeamSeasonsDenied(t, newSiteBrowser(t, route.mux), http.StatusUnauthorized, 1001)

	// A site-session timeout withholds the retained import until its owner
	// signs in again.
	owner.expireSiteSession()
	assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)
	owner.signIn("/soccer")
	listTeamSeasons(t, owner, 1001)

	route.app.Config.SiteInvitations[testSiteEmail] = nil
	assertTeamSeasonsDenied(t, owner, http.StatusForbidden, 1001)
	route.app.Config.SiteInvitations[testSiteEmail] = []string{"soccer"}
	listTeamSeasons(t, owner, 1001)

	// A player this import does not link.
	assertTeamSeasonsDenied(t, owner, http.StatusForbidden, 1003)
	if body := owner.get(teamSeasonsPath(1003)).Body.String(); !strings.Contains(body, "Player is not confirmed by this import") {
		t.Errorf("unconfirmed player's list = %q", body)
	}

	// The import's JWT expires.
	session := decryptTestSession(t, route.app, owner.cookieValue(config.LPSSessionCookieName, config.SoccerCookiePath))
	session.JWT = testutil.TestJWT(t, time.Now().Add(-time.Minute))
	soccerURL, _ := url.Parse(siteOrigin + config.SoccerCookiePath)
	owner.jar.SetCookies(soccerURL, []*http.Cookie{{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, route.app, &session), Path: config.SoccerCookiePath}})
	assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)

	// Site sign-out ends the import, so signing in again is not enough.
	route.importLinkedPlayers(t, owner)
	listTeamSeasons(t, owner, 1001)
	if signOut := owner.do(browserForm(siteOrigin, "/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("sign-out status = %d", signOut.Code)
	}
	assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)
	owner.signIn("/soccer")
	assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)
}

// When LPS denies the player's current team lookup or cannot answer it, the
// list still finds every season the per-season read opens from stored proof,
// each marked former and the list marked unverified, and keeps the import. A
// current season only LPS could prove is neither listed nor opened.
func TestSoccerHistoryTeamSeasonsListStoredProofWhenLPSCannotConfirmCurrentTeams(t *testing.T) {
	for _, refusal := range []struct {
		name            string
		lpsStatus       int
		seasonReadCode  int
		seasonReadError string
	}{
		{"a denied player", http.StatusForbidden, http.StatusForbidden, "Player is not confirmed by this import"},
		{"an unavailable LPS", http.StatusInternalServerError, http.StatusBadGateway, "Current team membership could not be verified"},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			route := newTeamHistoryRoute(t)
			route.setTeam(4102, oldFCSeason78)
			owner := route.signedIn(t)
			route.importLinkedPlayers(t, owner)
			if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4102); !report.Complete {
				t.Fatalf("refresh: %+v", report)
			}
			// LPS would list Craig FC's season 80 now, if it answered.
			route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","Season":80}]`)
			route.failPlayerTeams(1001, refusal.lpsStatus)

			response := owner.get(teamSeasonsPath(1001))
			if rewritten := findSessionCookie(t, response.Result()); rewritten != nil {
				t.Errorf("list after LPS answered %d rewrote the import: %#v", refusal.lpsStatus, rewritten)
			}
			stored := listTeamSeasons(t, owner, 1001)
			if got, want := stored.summary(), "4102/78 Old FC former, 4101/77 Craig FC former"; got != want {
				t.Fatalf("list after LPS answered %d = %q, want %q", refusal.lpsStatus, got, want)
			}
			if stored.CurrentVerified {
				t.Errorf("list after LPS answered %d is current_verified", refusal.lpsStatus)
			}
			for _, listed := range stored.TeamSeasons {
				readHistory(t, owner, 1001, listed.TeamID, listed.LPSSeasonID)
			}
			if former := readHistory(t, owner, 1001, 4102, 78); former.Record.Wins != 1 || len(former.Games) != 1 {
				t.Errorf("listed former Old FC season after LPS answered %d = %+v", refusal.lpsStatus, former)
			}
			assertHistoryDenied(t, owner, refusal.seasonReadCode, 1001, 4101, 80)
			if body := owner.get(historyPath(1001, 4101, 80)).Body.String(); !strings.Contains(body, refusal.seasonReadError) {
				t.Errorf("season read LPS alone could prove after LPS answered %d = %q, want %q", refusal.lpsStatus, body, refusal.seasonReadError)
			}
		})
	}
}

// A token LPS rejects ends the import, as on the per-season read, so the
// proof it stored lists nothing more.
func TestSoccerHistoryTeamSeasonsJudgeARefusedCurrentLookupAsTheSeasonReadDoes(t *testing.T) {
	for _, refusal := range []struct {
		name       string
		lpsStatus  int
		wantStatus int
		wantBody   string
		importEnds bool
	}{
		{"a rejected token ends the import", http.StatusUnauthorized, http.StatusUnauthorized, "import a fresh bearer JWT", true},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			route := newTeamHistoryRoute(t)
			route.setTeam(4102, oldFCSeason78)
			owner := route.signedIn(t)
			route.importLinkedPlayers(t, owner)
			if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4102); !report.Complete {
				t.Fatalf("refresh: %+v", report)
			}
			route.failPlayerTeams(1001, refusal.lpsStatus)

			refused := owner.get(teamSeasonsPath(1001))
			if refused.Code != refusal.wantStatus || !strings.Contains(refused.Body.String(), refusal.wantBody) || strings.Contains(refused.Body.String(), "team_seasons") {
				t.Fatalf("list after LPS answered %d: status %d, body %q; want %d with %q", refusal.lpsStatus, refused.Code, refused.Body.String(), refusal.wantStatus, refusal.wantBody)
			}
			if got := refused.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("refused list Cache-Control = %q, want private, no-store", got)
			}
			if !refusal.importEnds {
				if cleared := findSessionCookie(t, refused.Result()); cleared != nil {
					t.Errorf("list after LPS answered %d rewrote the import: %#v", refusal.lpsStatus, cleared)
				}
				// Stored proof of Old FC's season still opens it.
				if former := readHistory(t, owner, 1001, 4102, 78); former.Record.Wins != 1 {
					t.Errorf("former Old FC season after LPS answered %d = %+v", refusal.lpsStatus, former)
				}
				return
			}
			assertClearedSessionCookie(t, refused.Result())
			assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)
			assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1002)
		})
	}
}

// Without the durable archive there is no list, and a malformed player ID is
// a bad request.
func TestSoccerHistoryTeamSeasonsAreUnavailableWithoutTheDurableArchive(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.handler.SetArchiveStore(nil)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)

	assertTeamSeasonsDenied(t, owner, http.StatusServiceUnavailable, 1001)
	for _, query := range []string{"", "player_id=0", "player_id=x", "player_id=-1001"} {
		if response := owner.get("/soccer/history/team-seasons?" + query); response.Code != http.StatusBadRequest {
			t.Errorf("team-season list with %q: status %d, want 400", query, response.Code)
		}
	}
}

// failingQueries is the archive table whose queries fail, as a DynamoDB
// outage would answer them.
type failingQueries struct{ *archivetest.Table }

func (failingQueries) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, errors.New("simulated DynamoDB query failure")
}

// A membership query that fails is an error, never an empty or current-only
// list, and it keeps the import.
func TestSoccerHistoryTeamSeasonsAreUnavailableWhenTheMembershipQueryFails(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	failing, err := soccerarchive.NewDynamoStoreWithAPI(failingQueries{route.table}, "portfolio-lambda-dev-soccer-history", generousArchiveLimits)
	if err != nil {
		t.Fatalf("NewDynamoStoreWithAPI: %v", err)
	}
	route.handler.SetArchiveStore(failing)

	failed := owner.get(teamSeasonsPath(1001))
	if failed.Code != http.StatusServiceUnavailable || !strings.Contains(failed.Body.String(), "Team history is unavailable") || strings.Contains(failed.Body.String(), "team_seasons") {
		t.Errorf("list after a failed membership query: status %d, body %q; want 503 without seasons", failed.Code, failed.Body.String())
	}
	if got := failed.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("failed list Cache-Control = %q, want private, no-store", got)
	}
	if rewritten := findSessionCookie(t, failed.Result()); rewritten != nil {
		t.Errorf("failed list rewrote the import: %#v", rewritten)
	}

	// The import still lists Craig's seasons once the archive answers again.
	route.handler.SetArchiveStore(route.store)
	recovered := listTeamSeasons(t, owner, 1001)
	if got, want := recovered.summary(), "4102/78 Old FC current, 4101/77 Craig FC current"; got != want {
		t.Errorf("Craig's team seasons after the archive recovered = %q, want %q", got, want)
	}
}

// A player with no stored proof whom LPS lists on no team has an empty list,
// a successful answer distinct from a refusal or an error. Removing a player
// erases the proof, so a later import that LPS lists on no team finds none
// of the player's former seasons.
func TestSoccerHistoryTeamSeasonsListNothingForARemovedPlayerWithoutCurrentTeams(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	// Craig has left every team.
	route.setPlayerTeams(1001, `[]`)
	before := listTeamSeasons(t, owner, 1001)
	if got, want := before.summary(), "4102/78 Old FC former, 4101/77 Craig FC former"; got != want {
		t.Fatalf("Craig's team seasons before removal = %q, want %q", got, want)
	}

	removed := owner.do(browserForm(siteOrigin, "/soccer/players/remove", url.Values{"player_id": {"1001"}}))
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), "Player data removed") {
		t.Fatalf("verified removal: status %d, body %q", removed.Code, removed.Body.String())
	}
	// Removal ends the import that proved authority for it.
	assertTeamSeasonsDenied(t, owner, http.StatusUnauthorized, 1001)

	route.importLinkedPlayers(t, owner)
	empty := owner.get(teamSeasonsPath(1001))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"player_id":1001,"current_verified":true,"team_seasons":[]}` {
		t.Errorf("removed Craig's team seasons: status %d, body %q; want an empty list", empty.Code, empty.Body.String())
	}
	// A player LPS no longer finds has no current seasons either.
	route.mu.Lock()
	delete(route.playerTeams, 1001)
	route.mu.Unlock()
	if got := listTeamSeasons(t, owner, 1001); len(got.TeamSeasons) != 0 {
		t.Errorf("team seasons of a player LPS no longer finds = %q, want none", got.summary())
	}

	taylor := listTeamSeasons(t, owner, 1002)
	if got, want := taylor.summary(), "4202/79 Taylor FC current, 4101/77 Craig FC current"; got != want {
		t.Errorf("Taylor's team seasons after Craig's removal = %q, want %q", got, want)
	}
}
