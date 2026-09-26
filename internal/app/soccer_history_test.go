package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/testutil"
)

type historyProofKey struct {
	issuer, subject            string
	playerID, teamID, seasonID int
}

type fakeHistoryArchive struct {
	history      soccerarchive.TeamSeason
	proofs       map[historyProofKey]bool
	readErr      error
	reads        int
	refresh      soccerarchive.RefreshState
	refreshErr   error
	refreshReads int
}

func (*fakeHistoryArchive) SaveTeamSnapshot(context.Context, *soccerarchive.Snapshot) error {
	return nil
}

func (archive *fakeHistoryArchive) HasPlayerMembership(_ context.Context, issuer, subject string, playerID, teamID, seasonID int) (bool, error) {
	return archive.proofs[historyProofKey{issuer, subject, playerID, teamID, seasonID}], nil
}

func (archive *fakeHistoryArchive) ReadTeamSeason(context.Context, int, int) (soccerarchive.TeamSeason, error) {
	archive.reads++
	return archive.history, archive.readErr
}

func (archive *fakeHistoryArchive) ReadRefreshState(context.Context, int) (soccerarchive.RefreshState, error) {
	archive.refreshReads++
	return archive.refresh, archive.refreshErr
}

func TestSoccerHistoryReadCalculatesOnlyNumericScoredGamesForProvenTeamSeason(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("LPS call did not use the valid import")
		}
		switch r.URL.Path {
		case "/users/check":
			_, _ = fmt.Fprint(w, `{"players":[{"UPlayerID":1001,"FirstName":"Sam"}],"user_players":[{"player_id":1001}]}`)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Sam FC","Season":77}]`)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(application, application.Logger, false)
	archive := &fakeHistoryArchive{refresh: soccerarchive.RefreshState{TeamID: 4101, Status: soccerarchive.RefreshReady}, history: soccerarchive.TeamSeason{
		Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Sam FC", Season: 77},
		Coverage: soccerarchive.Coverage{
			Status: soccerarchive.CoverageFetched, FetchedAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC), ReturnedGameCount: 9,
		},
		Games: []lps.TeamScheduleGame{
			{UGameID: 1, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2025-01-01T18:00:00Z", Result: "3 - 1"},
			{UGameID: 2, Season: 77, UTeam1: 5, UTeam2: 4101, SchedGameDateTime: "2025-01-02T18:00:00Z", Result: "1 - 2"},
			{UGameID: 3, Season: 77, UTeam1: 5, UTeam2: 4101, SchedGameDateTime: "2025-01-03T18:00:00Z", Result: "4 - 0"},
			{UGameID: 4, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2025-01-04T18:00:00Z", Result: "2 - 2"},
			{UGameID: 5, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2025-01-05T18:00:00Z", Result: "canceled"},
			{UGameID: 6, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2025-01-06T18:00:00Z"},
			{UGameID: 7, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2025-01-07T18:00:00Z", Result: "Final"},
			{UGameID: 8, Season: 77, UTeam1: 4101, UTeam2: 5, SchedGameDateTime: "2099-01-01T18:00:00Z"},
			{UGameID: 9, Season: 77, SchedGameDateTime: "2025-01-09T18:00:00Z", Result: "8 - 0", HomeTeam: lps.TeamSummary{TeamName: "Sam FC"}, VisitorTeam: lps.TeamSummary{TeamName: "Rivals"}},
		},
	}}
	handler.SetArchiveStore(archive)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if imported.Code != http.StatusOK {
		t.Fatalf("import failed: %d: %s", imported.Code, imported.Body.String())
	}
	lpsCookie, guardCookie := findSessionCookie(t, imported.Result()), findImportGuardCookie(imported.Result())
	if lpsCookie == nil || guardCookie == nil {
		t.Fatal("valid LPS import cookies missing")
	}

	response := soccerGrantRequest(mux, http.MethodGet, "/soccer/history?player_id=1001&team_id=4101&season_id=77", nil, ownerCookie, lpsCookie, guardCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("history read = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		PlayerID    int `json:"player_id"`
		TeamID      int `json:"team_id"`
		LPSSeasonID int `json:"lps_season_id"`
		Team        struct {
			TeamName string `json:"team_name"`
		} `json:"team"`
		Coverage struct {
			Status            string    `json:"status"`
			FetchedAt         time.Time `json:"fetched_at"`
			ReturnedGameCount int       `json:"returned_game_count"`
		} `json:"coverage"`
		Refresh struct {
			Status              string    `json:"status"`
			LastAttemptAt       time.Time `json:"last_attempt_at"`
			NextDueAt           time.Time `json:"next_due_at"`
			LastErrorKind       string    `json:"last_error_kind"`
			LastErrorStatusCode int       `json:"last_error_status_code"`
		} `json:"refresh"`
		Record struct {
			Label        string `json:"label"`
			Wins         int    `json:"wins"`
			Losses       int    `json:"losses"`
			Draws        int    `json:"draws"`
			ScoredGames  int    `json:"scored_games"`
			Unclassified int    `json:"unclassified"`
		} `json:"record"`
		Games []struct {
			Game struct {
				ID int `json:"UGameID"`
			} `json:"game"`
			Classification string `json:"classification"`
		} `json:"games"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode history: %v: %s", err, response.Body.String())
	}
	if body.PlayerID != 1001 || body.TeamID != 4101 || body.LPSSeasonID != 77 || body.Team.TeamName != "Sam FC" {
		t.Fatalf("wrong history identity: %+v", body)
	}
	if body.Coverage.Status != "fetched" || !body.Coverage.FetchedAt.Equal(time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)) || body.Coverage.ReturnedGameCount != 9 {
		t.Fatalf("wrong coverage: %+v", body.Coverage)
	}
	if body.Refresh.Status != "ready" || archive.refreshReads != 1 {
		t.Fatalf("wrong refresh state: %+v, reads %d", body.Refresh, archive.refreshReads)
	}
	if body.Record.Label != "Calculated from numeric game scores; not official standings" || body.Record.Wins != 2 || body.Record.Losses != 1 || body.Record.Draws != 1 || body.Record.ScoredGames != 4 || body.Record.Unclassified != 4 {
		t.Fatalf("wrong scored record: %+v", body.Record)
	}
	if len(body.Games) != 8 || body.Games[0].Game.ID != 1 || body.Games[1].Classification != "win" || body.Games[4].Classification != "unclassified" || body.Games[6].Classification != "unclassified" || body.Games[7].Classification != "unclassified" || archive.reads != 1 {
		t.Fatalf("wrong completed games or read count: games %+v, reads %d", body.Games, archive.reads)
	}

	archive.history.Coverage.Status = soccerarchive.CoverageNotFetched
	archive.history.Coverage.ReturnedGameCount = 0
	archive.refresh = soccerarchive.RefreshState{
		TeamID: 4101, Status: soccerarchive.RefreshRetryable,
		LastAttemptAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		NextDueAt:     time.Date(2026, 9, 27, 12, 5, 0, 0, time.UTC),
		LastErrorKind: lps.ErrorUpstream, LastErrorStatusCode: http.StatusServiceUnavailable,
	}
	failed := soccerGrantRequest(mux, http.MethodGet, "/soccer/history?player_id=1001&team_id=4101&season_id=77", nil, ownerCookie, lpsCookie, guardCookie)
	if failed.Code != http.StatusOK || json.Unmarshal(failed.Body.Bytes(), &body) != nil {
		t.Fatalf("failed collection read = %d: %s", failed.Code, failed.Body.String())
	}
	if body.Coverage.Status != "not_fetched" || body.Coverage.ReturnedGameCount != 0 || body.Refresh.Status != "retryable_failure" || body.Refresh.LastErrorKind != "upstream" || body.Refresh.LastErrorStatusCode != http.StatusServiceUnavailable || !body.Refresh.LastAttemptAt.Equal(time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)) || !body.Refresh.NextDueAt.Equal(time.Date(2026, 9, 27, 12, 5, 0, 0, time.UTC)) || len(body.Games) != 8 {
		t.Fatalf("failure hid retained games or collection context: %+v", body)
	}

	archive.history.Games = nil
	archive.history.Coverage = soccerarchive.Coverage{Status: soccerarchive.CoverageFetched, FetchedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)}
	archive.refresh = soccerarchive.RefreshState{TeamID: 4101, Status: soccerarchive.RefreshReady}
	empty := soccerGrantRequest(mux, http.MethodGet, "/soccer/history?player_id=1001&team_id=4101&season_id=77", nil, ownerCookie, lpsCookie, guardCookie)
	if empty.Code != http.StatusOK || json.Unmarshal(empty.Body.Bytes(), &body) != nil {
		t.Fatalf("empty season read = %d: %s", empty.Code, empty.Body.String())
	}
	if body.Coverage.Status != "fetched" || body.Coverage.ReturnedGameCount != 0 || body.Refresh.Status != "ready" || len(body.Games) != 0 {
		t.Fatalf("fetched empty season resembled a failure: %+v", body)
	}
}

func TestSoccerHistoryReadRequiresCurrentOwnerImportAndExactMembership(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	token := testutil.TestJWT(t, time.Now().Add(time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("LPS call did not use the valid import")
		}
		switch r.URL.Path {
		case "/users/check":
			_, _ = fmt.Fprint(w, `{"players":[{"UPlayerID":1001,"FirstName":"Sam"}],"user_players":[{"player_id":1001}]}`)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Sam FC","Season":78},{"UTeamID":4102,"team_name":"Sam FC","Season":77}]`)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, handler := buildMux(application, application.Logger, false)
	archive := &fakeHistoryArchive{
		proofs:  map[historyProofKey]bool{{fixture.issuer, "stable-subject", 1001, 4101, 77}: true},
		history: soccerarchive.TeamSeason{Team: lps.TeamSummary{UTeamID: 4101, Season: 77}, Coverage: soccerarchive.Coverage{Status: soccerarchive.CoverageNotFetched}},
		refresh: soccerarchive.RefreshState{TeamID: 4101, Status: soccerarchive.RefreshReady},
	}
	handler.SetArchiveStore(archive)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	ownerCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	imported := soccerGrantRequest(mux, http.MethodPost, "/soccer/import", url.Values{"jwt": {token}}, ownerCookie)
	if imported.Code != http.StatusOK {
		t.Fatalf("import failed: %d: %s", imported.Code, imported.Body.String())
	}
	lpsCookie, guardCookie := findSessionCookie(t, imported.Result()), findImportGuardCookie(imported.Result())
	if lpsCookie == nil || guardCookie == nil {
		t.Fatal("valid LPS import cookies missing")
	}
	read := func(path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		return soccerGrantRequest(mux, http.MethodGet, path, nil, cookies...)
	}
	const formerSeason = "/soccer/history?player_id=1001&team_id=4101&season_id=77"
	if got := read(formerSeason, lpsCookie, guardCookie); got.Code != http.StatusUnauthorized {
		t.Errorf("signed-out read = %d, want 401", got.Code)
	}
	if got := read(formerSeason, ownerCookie); got.Code != http.StatusUnauthorized {
		t.Errorf("read without LPS import = %d, want 401", got.Code)
	}
	application.Config.SiteInvitations["owner@example.com"] = nil
	if got := read(formerSeason, ownerCookie, lpsCookie, guardCookie); got.Code != http.StatusForbidden {
		t.Errorf("revoked-grant read = %d, want 403", got.Code)
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	fixture.subject = "different-subject"
	otherStateCookie, otherState := beginSiteSignIn(t, mux, "/soccer")
	otherCookie := siteCookie(t, completeSiteSignIn(t, mux, otherStateCookie, otherState))
	if got := read(formerSeason, otherCookie, lpsCookie, guardCookie); got.Code != http.StatusUnauthorized {
		t.Errorf("different-owner read = %d, want 401", got.Code)
	}
	for _, path := range []string{
		"/soccer/history?player_id=1002&team_id=4101&season_id=77",
		"/soccer/history?player_id=1001&team_id=4101&season_id=79",
		"/soccer/history?player_id=1001&team_id=4102&season_id=78",
		"/soccer/history?player_id=1001&team_id=4103&season_id=77",
	} {
		if got := read(path, ownerCookie, lpsCookie, guardCookie); got.Code != http.StatusForbidden {
			t.Errorf("unproven %s = %d, want 403", path, got.Code)
		}
	}
	if archive.reads != 0 {
		t.Fatalf("denied reads accessed stored games %d times", archive.reads)
	}
	former := read(formerSeason, ownerCookie, lpsCookie, guardCookie)
	if former.Code != http.StatusOK || !strings.Contains(former.Body.String(), `"status":"not_fetched"`) || !strings.Contains(former.Body.String(), `"games":[]`) || archive.reads != 1 {
		t.Fatalf("former season with stored proof was unavailable: status %d, body %s, reads %d", former.Code, former.Body.String(), archive.reads)
	}
	archive.readErr = soccerarchive.ErrNoArchive
	archive.refreshErr = soccerarchive.ErrNotEnrolled
	neverFetched := read(formerSeason, ownerCookie, lpsCookie, guardCookie)
	if neverFetched.Code != http.StatusOK || !strings.Contains(neverFetched.Body.String(), `"status":"not_fetched"`) || !strings.Contains(neverFetched.Body.String(), `"refresh":null`) || !strings.Contains(neverFetched.Body.String(), `"games":[]`) {
		t.Fatalf("unfetched authorized season was mistaken for an empty fetch: status %d, body %s", neverFetched.Code, neverFetched.Body.String())
	}

	session := decryptTestSession(t, application, lpsCookie.Value)
	session.JWT = testutil.TestJWT(t, time.Now().Add(-time.Minute))
	expiredCookie := &http.Cookie{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, application, &session)}
	if got := read(formerSeason, ownerCookie, expiredCookie, guardCookie); got.Code != http.StatusUnauthorized || archive.reads != 2 {
		t.Errorf("expired import read = %d, archive reads %d", got.Code, archive.reads)
	}
	signOut := soccerGrantRequest(mux, http.MethodPost, "/sign-out", nil, ownerCookie, lpsCookie, guardCookie)
	if signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out = %d", signOut.Code)
	}
	assertClearedSessionCookie(t, signOut.Result())
}
