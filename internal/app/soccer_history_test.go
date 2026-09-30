package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/config"
	"portfolio/internal/lps"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/internal/testutil"
)

// teamHistoryRoute is the real route assembly with fake Cognito site
// sign-in, a fake LPS whose linked players, player teams and team schedules
// a test can change, and the durable archive over an in-memory DynamoDB
// table, as an approved activation would wire it.
type teamHistoryRoute struct {
	cognito *fakeSiteCognito
	app     *App
	mux     http.Handler
	handler *internalsoccer.Handler
	table   *archivetest.Table
	store   *soccerarchive.DynamoStore
	jwt     string
	lpsURL  string

	mu sync.Mutex
	// account is the /users/check response for the imported JWT.
	account string
	// playerTeams is each linked player's /players/{id}/my_teams response;
	// playerTeamFailures answers a player's lookup with that HTTP status
	// instead.
	playerTeams        map[int]string
	playerTeamFailures map[int]int
	// teams is each team's /teams/{id} response; teamFailures answers a
	// team's lookup with that HTTP status instead.
	teams        map[int]string
	teamFailures map[int]int
}

// Craig (1001) plays for Craig FC (4101) in LPS season 77 and played for Old
// FC (4102) in season 78; Taylor (1002) plays for Craig FC in season 77 and
// for Taylor FC (4202) in season 79.
func newTeamHistoryRoute(t *testing.T) *teamHistoryRoute {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	route := &teamHistoryRoute{
		cognito: cognito, app: application, jwt: testutil.TestJWT(t, time.Now().Add(time.Hour)),
		account: playerHistoryAccount,
		playerTeams: map[int]string{
			1001: `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4102,"team_name":"Old FC","Season":78}]`,
			1002: `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4202,"team_name":"Taylor FC","Season":79}]`,
		},
		playerTeamFailures: map[int]int{},
		teams:              map[int]string{},
		teamFailures:       map[int]int{},
	}
	lpsRoutes := http.NewServeMux()
	lpsRoutes.HandleFunc("GET /users/check", func(w http.ResponseWriter, r *http.Request) {
		if route.authorized(w, r) {
			_, _ = fmt.Fprint(w, route.lpsAccount())
		}
	})
	lpsRoutes.HandleFunc("GET /players/{id}/my_teams", func(w http.ResponseWriter, r *http.Request) {
		if !route.authorized(w, r) {
			return
		}
		playerID, _ := strconv.Atoi(r.PathValue("id"))
		route.mu.Lock()
		teams, found := route.playerTeams[playerID]
		failure := route.playerTeamFailures[playerID]
		route.mu.Unlock()
		if failure != 0 {
			http.Error(w, "player teams lookup failed", failure)
			return
		}
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, teams)
	})
	lpsRoutes.HandleFunc("GET /teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		teamID, _ := strconv.Atoi(r.PathValue("id"))
		route.mu.Lock()
		schedule, found := route.teams[teamID]
		failure := route.teamFailures[teamID]
		route.mu.Unlock()
		switch {
		case failure != 0:
			http.Error(w, "team lookup failed", failure)
		case found:
			_, _ = fmt.Fprint(w, schedule)
		default:
			t.Errorf("unexpected LPS team lookup %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	lpsRoutes.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected LPS request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	lpsServer := httptest.NewServer(lpsRoutes)
	t.Cleanup(lpsServer.Close)
	route.lpsURL = lpsServer.URL
	application.Config.LPSAPIBaseURL = lpsServer.URL
	route.mux, route.handler = buildMux(application, application.Logger, false)
	route.table = archivetest.NewTable()
	store, err := soccerarchive.NewDynamoStoreWithAPI(route.table, "portfolio-lambda-dev-soccer-history", generousArchiveLimits)
	if err != nil {
		t.Fatalf("NewDynamoStoreWithAPI: %v", err)
	}
	route.store = store
	route.handler.SetArchiveStore(route.store)
	return route
}

func (route *teamHistoryRoute) authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer "+route.jwt {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (route *teamHistoryRoute) lpsAccount() string {
	route.mu.Lock()
	defer route.mu.Unlock()
	return route.account
}

// setPlayerTeams changes what LPS returns for a player's current teams.
func (route *teamHistoryRoute) setPlayerTeams(playerID int, teams string) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.playerTeams[playerID] = teams
}

// failPlayerTeams makes LPS answer a player's current team lookup with status.
func (route *teamHistoryRoute) failPlayerTeams(playerID, status int) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.playerTeamFailures[playerID] = status
}

// setTeam changes what LPS returns for a team's schedule lookup.
func (route *teamHistoryRoute) setTeam(teamID int, schedule string) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.teams[teamID] = schedule
	delete(route.teamFailures, teamID)
}

// failTeam makes LPS answer a team's schedule lookup with status.
func (route *teamHistoryRoute) failTeam(teamID, status int) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.teamFailures[teamID] = status
}

// signedIn signs a new browser in through the fake Cognito as whoever the
// fake identity currently names.
func (route *teamHistoryRoute) signedIn(t *testing.T) *siteBrowser {
	t.Helper()
	browser := newSiteBrowser(t, route.mux)
	if landing := browser.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("site sign-in status = %d", landing.Code)
	}
	return browser
}

// importLinkedPlayers submits the import dialog's form, which discloses
// indefinite linked-player history, so the import records the linked
// players' team-season memberships under the signed-in owner.
func (route *teamHistoryRoute) importLinkedPlayers(t *testing.T, browser *siteBrowser) {
	t.Helper()
	imported := browser.postForm("/soccer/import", url.Values{
		"jwt":                             {route.jwt},
		partials.SoccerHistoryNoticeField: {partials.SoccerHistoryNoticeIndefinite},
	})
	if imported.Code != http.StatusOK || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("linked-player import: status %d, body %q", imported.Code, imported.Body.String())
	}
}

// refreshTeams runs the on-demand refresh worker against the fake LPS as of at.
func (route *teamHistoryRoute) refreshTeams(t *testing.T, at time.Time, teamIDs ...int) soccerarchive.RefreshReport {
	t.Helper()
	source := lps.NewScheduleResolver(route.lpsURL, route.app.LPSClient, "")
	return soccerarchive.NewRefreshWorker(route.store, source, func() time.Time { return at }).Run(context.Background(), teamIDs)
}

// historyPath is the private history read for one player-team-season.
func historyPath(playerID, teamID, seasonID int) string {
	return fmt.Sprintf("/soccer/history?player_id=%d&team_id=%d&season_id=%d", playerID, teamID, seasonID)
}

// teamSeasonHistory is the read contract as a stats view would decode it.
type teamSeasonHistory struct {
	PlayerID    int `json:"player_id"`
	TeamID      int `json:"team_id"`
	LPSSeasonID int `json:"lps_season_id"`
	Team        struct {
		UTeamID      int    `json:"UTeamID"`
		TeamName     string `json:"team_name"`
		DivisionName string `json:"division_name"`
		Season       int    `json:"Season"`
	} `json:"team"`
	Coverage struct {
		Status            string     `json:"status"`
		FetchedAt         *time.Time `json:"fetched_at"`
		ReturnedGameCount int        `json:"returned_game_count"`
	} `json:"coverage"`
	Refresh *struct {
		Status              string     `json:"status"`
		LastAttemptAt       *time.Time `json:"last_attempt_at"`
		NextDueAt           *time.Time `json:"next_due_at"`
		LastErrorKind       string     `json:"last_error_kind"`
		LastErrorStatusCode int        `json:"last_error_status_code"`
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
			UGameID int    `json:"UGameID"`
			Result  string `json:"result"`
		} `json:"game"`
		Classification string `json:"classification"`
	} `json:"games"`
}

// readHistory requires an authorized read and decodes its body.
func readHistory(t *testing.T, browser *siteBrowser, playerID, teamID, seasonID int) teamSeasonHistory {
	t.Helper()
	response := browser.get(historyPath(playerID, teamID, seasonID))
	if response.Code != http.StatusOK {
		t.Fatalf("history read for player %d team %d season %d: status %d, body %q", playerID, teamID, seasonID, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("history read Cache-Control = %q, want private, no-store", got)
	}
	var history teamSeasonHistory
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode history: %v: %s", err, response.Body.String())
	}
	return history
}

// classifications maps each returned game ID to its classification.
func (history *teamSeasonHistory) classifications() map[int]string {
	byGame := make(map[int]string, len(history.Games))
	for _, game := range history.Games {
		byGame[game.Game.UGameID] = game.Classification
	}
	return byGame
}

// craigFCSeason77 is Craig FC's LPS schedule: every kind of season-77 result,
// one future game, and one game from the next season.
const craigFCSeason77 = `{"team":{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},"games":[
{"UGameID":7001,"Season":77,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"3 - 1"},
{"UGameID":7002,"Season":77,"UTeam1":5002,"UTeam2":4101,"SchedGameDateTime":"2026-01-12T19:00:00Z","result":"1 - 2"},
{"UGameID":7003,"Season":77,"UTeam1":5003,"UTeam2":4101,"SchedGameDateTime":"2026-01-19T19:00:00Z","result":"4 - 0"},
{"UGameID":7004,"Season":77,"UTeam1":4101,"UTeam2":5004,"SchedGameDateTime":"2026-01-26T19:00:00Z","result":"2 - 2"},
{"UGameID":7005,"Season":77,"UTeam1":4101,"UTeam2":5005,"SchedGameDateTime":"2026-02-02T19:00:00Z","result":"canceled"},
{"UGameID":7006,"Season":77,"UTeam1":5006,"UTeam2":4101,"SchedGameDateTime":"2026-02-09T19:00:00Z","result":""},
{"UGameID":7007,"Season":77,"UTeam1":4101,"UTeam2":5007,"SchedGameDateTime":"2026-02-16T19:00:00Z","result":"Final"},
{"UGameID":7008,"Season":77,"UTeam1":4101,"UTeam2":5008,"SchedGameDateTime":"2026-02-23T19:00:00Z","result":"Forfeit"},
{"UGameID":7009,"Season":77,"SchedGameDateTime":"2026-03-02T19:00:00Z","result":"8 - 0","home_team":{"team_name":"Craig FC"},"visitor_team":{"team_name":"Rivals"}},
{"UGameID":7010,"Season":77,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2099-03-09T19:00:00Z","result":""},
{"UGameID":7011,"Season":78,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2026-04-06T19:00:00Z","result":"5 - 0"}]}`

func TestSoccerHistoryReadCountsOnlyNumericScoresFromTheTeamsSide(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	fetchedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if report := route.refreshTeams(t, fetchedAt, 4101); !report.Complete {
		t.Fatalf("refresh of Craig FC: %+v", report)
	}

	history := readHistory(t, owner, 1001, 4101, 77)

	if history.PlayerID != 1001 || history.TeamID != 4101 || history.LPSSeasonID != 77 ||
		history.Team.UTeamID != 4101 || history.Team.TeamName != "Craig FC" || history.Team.DivisionName != "Open A" || history.Team.Season != 77 {
		t.Errorf("history identity = player %d team %d season %d, team %+v", history.PlayerID, history.TeamID, history.LPSSeasonID, history.Team)
	}
	if history.Coverage.Status != "fetched" || history.Coverage.FetchedAt == nil || !history.Coverage.FetchedAt.Equal(fetchedAt) || history.Coverage.ReturnedGameCount != 10 {
		t.Errorf("coverage = %+v, want season 77's ten games fetched at %s", history.Coverage, fetchedAt)
	}
	// A successful fetch keeps the team from being due for the archive's
	// four-hour refresh guard, so the next daily run attempts it (#103).
	if history.Refresh == nil || history.Refresh.Status != "ready" || history.Refresh.LastAttemptAt == nil || !history.Refresh.LastAttemptAt.Equal(fetchedAt) ||
		history.Refresh.NextDueAt == nil || !history.Refresh.NextDueAt.Equal(fetchedAt.Add(4*time.Hour)) || history.Refresh.LastErrorKind != "" {
		t.Errorf("refresh = %+v, want a ready team attempted at %s and due again four hours later", history.Refresh, fetchedAt)
	}
	record := history.Record
	if record.Label != "Calculated from numeric game scores; not official standings" ||
		record.Wins != 2 || record.Losses != 1 || record.Draws != 1 || record.ScoredGames != 4 || record.Unclassified != 5 {
		t.Errorf("record = %+v, want 2-1-1 from four scored games with five unclassified", record)
	}
	want := map[int]string{
		7001: "win", 7002: "win", 7003: "loss", 7004: "draw",
		7005: "unclassified", 7006: "unclassified", 7007: "unclassified", 7008: "unclassified", 7009: "unclassified",
	}
	if got := history.classifications(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("completed games = %v, want %v", got, want)
	}
}

// A completed game is one that has kicked off. A score LPS shows on a game
// dated in the future, or on a game with no readable kickoff, is not a
// completed result, while a past game without a score stays listed.
func TestSoccerHistoryReadListsAndCountsOnlyGamesThatHaveKickedOff(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, `{"team":{"UTeamID":4101,"team_name":"Craig FC","Season":77},"games":[
{"UGameID":7401,"Season":77,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"2 - 1"},
{"UGameID":7402,"Season":77,"UTeam1":5002,"UTeam2":4101,"SchedGameDateTime":"2026-01-12T19:00:00Z","result":""},
{"UGameID":7403,"Season":77,"UTeam1":4101,"UTeam2":5003,"SchedGameDateTime":"2099-01-05T19:00:00Z","result":"3 - 0"},
{"UGameID":7404,"Season":77,"UTeam1":4101,"UTeam2":5004,"SchedGameDateTime":"","result":""},
{"UGameID":7405,"Season":77,"UTeam1":5005,"UTeam2":4101,"SchedGameDateTime":"TBD","result":"0 - 4"}]}`)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101); !report.Complete {
		t.Fatalf("refresh of Craig FC: %+v", report)
	}

	history := readHistory(t, owner, 1001, 4101, 77)

	want := map[int]string{7401: "win", 7402: "unclassified"}
	if got := history.classifications(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("completed games = %v, want %v", got, want)
	}
	if record := history.Record; record.Wins != 1 || record.Losses != 0 || record.Draws != 0 || record.ScoredGames != 1 || record.Unclassified != 1 {
		t.Errorf("record = %+v, want 1-0-0 from one scored game with one unclassified", record)
	}
	if history.Coverage.ReturnedGameCount != 5 {
		t.Errorf("coverage returned %d games, want the five LPS returned for season 77", history.Coverage.ReturnedGameCount)
	}
}

// LPS can name a game's sides both in the nested home_team and visitor_team
// objects and in the flat UTeam1 and UTeam2 fields. The schedule and the
// archive take the nested team first, so the record must too.
func TestSoccerHistoryReadTakesEachSideFromTheNestedTeamLikeTheSchedule(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, `{"team":{"UTeamID":4101,"team_name":"Craig FC","Season":77},"games":[
{"UGameID":7101,"Season":77,"UTeam1":4999,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"1 - 0","home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":5001,"team_name":"Rivals"}},
{"UGameID":7102,"Season":77,"UTeam1":4101,"UTeam2":5002,"SchedGameDateTime":"2026-01-12T19:00:00Z","result":"1 - 0","home_team":{"UTeamID":5002,"team_name":"Hosts"},"visitor_team":{"UTeamID":4101,"team_name":"Craig FC"}}]}`)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101); !report.Complete {
		t.Fatalf("refresh of Craig FC: %+v", report)
	}

	history := readHistory(t, owner, 1001, 4101, 77)

	want := map[int]string{7101: "win", 7102: "loss"}
	if got := history.classifications(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("completed games = %v, want %v", got, want)
	}
	if record := history.Record; record.Wins != 1 || record.Losses != 1 || record.Draws != 0 || record.ScoredGames != 2 || record.Unclassified != 0 {
		t.Errorf("record = %+v, want 1-1-0 from two scored games", record)
	}
}

func TestSoccerHistoryReadTellsAnEmptySeasonFromMissingOrFailedCollection(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	route.setTeam(4202, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[]}`)
	owner := route.signedIn(t)
	importedAt := time.Now().UTC()
	route.importLinkedPlayers(t, owner)
	fetchedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if report := route.refreshTeams(t, fetchedAt, 4101, 4202); !report.Complete {
		t.Fatalf("first refresh: %+v", report)
	}

	// LPS returned Taylor FC's season with no games.
	empty := readHistory(t, owner, 1002, 4202, 79)
	if empty.Coverage.Status != "fetched" || empty.Coverage.FetchedAt == nil || !empty.Coverage.FetchedAt.Equal(fetchedAt) || empty.Coverage.ReturnedGameCount != 0 ||
		empty.Refresh == nil || empty.Refresh.Status != "ready" || empty.Refresh.LastErrorKind != "" || empty.Games == nil || len(empty.Games) != 0 ||
		empty.Record.ScoredGames != 0 || empty.Record.Unclassified != 0 {
		t.Errorf("fetched empty season = %+v", empty)
	}

	// Old FC is enrolled by the import but no refresh has fetched it yet.
	pending := readHistory(t, owner, 1001, 4102, 78)
	if pending.Coverage.Status != "not_fetched" || pending.Coverage.FetchedAt != nil || pending.Coverage.ReturnedGameCount != 0 ||
		pending.Refresh == nil || pending.Refresh.Status != "ready" || pending.Refresh.LastAttemptAt != nil ||
		pending.Refresh.NextDueAt == nil || pending.Refresh.NextDueAt.Before(importedAt.Add(-time.Second)) || pending.Refresh.NextDueAt.After(time.Now()) ||
		len(pending.Games) != 0 {
		t.Errorf("season awaiting its first collection = %+v", pending)
	}

	// A later refresh of Craig FC fails upstream, and Old FC's Team ID is
	// rejected as invalid.
	failedAt := fetchedAt.Add(24 * time.Hour)
	route.failTeam(4101, http.StatusServiceUnavailable)
	route.failTeam(4102, http.StatusNotFound)
	if report := route.refreshTeams(t, failedAt, 4101, 4102); report.Complete {
		t.Fatalf("failed refresh reported complete: %+v", report)
	}

	stale := readHistory(t, owner, 1001, 4101, 77)
	if stale.Coverage.Status != "fetched" || stale.Coverage.FetchedAt == nil || !stale.Coverage.FetchedAt.Equal(fetchedAt) || stale.Coverage.ReturnedGameCount != 10 {
		t.Errorf("coverage after a failed refresh = %+v, want the earlier fetch kept", stale.Coverage)
	}
	if stale.Refresh == nil || stale.Refresh.Status != "retryable_failure" || stale.Refresh.LastErrorKind != "upstream" || stale.Refresh.LastErrorStatusCode != http.StatusServiceUnavailable ||
		stale.Refresh.LastAttemptAt == nil || !stale.Refresh.LastAttemptAt.Equal(failedAt) ||
		stale.Refresh.NextDueAt == nil || !stale.Refresh.NextDueAt.Equal(failedAt.Add(15*time.Minute)) {
		t.Errorf("refresh after an upstream failure = %+v", stale.Refresh)
	}
	if len(stale.Games) != 9 || stale.Record.Wins != 2 || stale.Record.Losses != 1 || stale.Record.Draws != 1 || stale.Record.Unclassified != 5 {
		t.Errorf("a failed refresh changed the retained games: %d games, record %+v", len(stale.Games), stale.Record)
	}

	invalid := readHistory(t, owner, 1001, 4102, 78)
	if invalid.Coverage.Status != "not_fetched" || invalid.Refresh == nil || invalid.Refresh.Status != "invalid_team" ||
		invalid.Refresh.LastErrorKind != "invalid_team" || invalid.Refresh.LastErrorStatusCode != http.StatusNotFound ||
		invalid.Refresh.LastAttemptAt == nil || !invalid.Refresh.LastAttemptAt.Equal(failedAt) || invalid.Refresh.NextDueAt != nil {
		t.Errorf("season of an invalid Team ID = coverage %+v, refresh %+v", invalid.Coverage, invalid.Refresh)
	}
}

// A season can be partly collected: a later team response no longer returns
// it, or a later response could not be saved in full. Neither may read as a
// current fetch. The season keeps the games and fetch time of the last
// response that returned it, while the refresh record shows the later
// attempt and its outcome.
func TestSoccerHistoryReadTellsAPartlyCollectedSeasonFromACurrentFetch(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	fetchedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if report := route.refreshTeams(t, fetchedAt, 4101); !report.Complete {
		t.Fatalf("first refresh: %+v", report)
	}

	// Craig FC moves on to season 80, and LPS stops returning season 77.
	omittedAt := fetchedAt.Add(24 * time.Hour)
	route.setTeam(4101, `{"team":{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":80},"games":[
{"UGameID":7501,"Season":80,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2099-09-01T19:00:00Z","result":""}]}`)
	if report := route.refreshTeams(t, omittedAt, 4101); !report.Complete {
		t.Fatalf("refresh without season 77: %+v", report)
	}
	omitted := readHistory(t, owner, 1001, 4101, 77)
	if omitted.Coverage.Status != "fetched" || omitted.Coverage.FetchedAt == nil || !omitted.Coverage.FetchedAt.Equal(fetchedAt) || omitted.Coverage.ReturnedGameCount != 10 {
		t.Errorf("coverage of the omitted season = %+v, want the fetch of %s that returned its ten games", omitted.Coverage, fetchedAt)
	}
	if omitted.Refresh == nil || omitted.Refresh.Status != "ready" || omitted.Refresh.LastAttemptAt == nil || !omitted.Refresh.LastAttemptAt.Equal(omittedAt) {
		t.Errorf("refresh after the omission = %+v, want a ready team last attempted at %s", omitted.Refresh, omittedAt)
	}
	if len(omitted.Games) != 9 || omitted.Record.Wins != 2 || omitted.Record.Losses != 1 || omitted.Record.Draws != 1 || omitted.Record.Unclassified != 5 {
		t.Errorf("the omission changed season 77's games: %d games, record %+v", len(omitted.Games), omitted.Record)
	}

	// LPS returns season 77 again with game 7003's score corrected, but the
	// archive cannot write its games.
	failedAt := omittedAt.Add(24 * time.Hour)
	route.setTeam(4101, strings.Replace(craigFCSeason77, `"result":"4 - 0"`, `"result":"0 - 4"`, 1))
	route.table.FailPut = func(key string) error {
		if strings.HasPrefix(key, "GAME#") {
			return fmt.Errorf("throttled")
		}
		return nil
	}
	if report := route.refreshTeams(t, failedAt, 4101); report.Complete || len(report.Results) != 1 || report.Results[0].Outcome != soccerarchive.RefreshStoreFailed {
		t.Fatalf("refresh whose save failed: %+v", report)
	}
	route.table.FailPut = nil
	unsaved := readHistory(t, owner, 1001, 4101, 77)
	if unsaved.Coverage.FetchedAt == nil || !unsaved.Coverage.FetchedAt.Equal(fetchedAt) {
		t.Errorf("coverage after an unsaved refresh = %+v, want the fetch of %s kept", unsaved.Coverage, fetchedAt)
	}
	if unsaved.Refresh == nil || unsaved.Refresh.Status != "retryable_failure" || unsaved.Refresh.LastErrorKind != "store" || unsaved.Refresh.LastErrorStatusCode != 0 ||
		unsaved.Refresh.LastAttemptAt == nil || !unsaved.Refresh.LastAttemptAt.Equal(failedAt) ||
		unsaved.Refresh.NextDueAt == nil || !unsaved.Refresh.NextDueAt.Equal(failedAt.Add(15*time.Minute)) {
		t.Errorf("refresh after an unsaved response = %+v, want a store failure retried 15 minutes after %s", unsaved.Refresh, failedAt)
	}
	if unsaved.Record.Wins != 2 || unsaved.Record.Losses != 1 || unsaved.classifications()[7003] != "loss" {
		t.Errorf("an unsaved correction reached the record: %+v, game 7003 %q", unsaved.Record, unsaved.classifications()[7003])
	}

	// The retry saves the whole response.
	savedAt := failedAt.Add(15 * time.Minute)
	if report := route.refreshTeams(t, savedAt, 4101); !report.Complete {
		t.Fatalf("retried refresh: %+v", report)
	}
	current := readHistory(t, owner, 1001, 4101, 77)
	if current.Coverage.FetchedAt == nil || !current.Coverage.FetchedAt.Equal(savedAt) || current.Coverage.ReturnedGameCount != 10 ||
		current.Refresh == nil || current.Refresh.Status != "ready" || current.Refresh.LastErrorKind != "" {
		t.Errorf("season after the retry = coverage %+v, refresh %+v; want fetched at %s", current.Coverage, current.Refresh, savedAt)
	}
	if record := current.Record; record.Wins != 3 || record.Losses != 0 || record.Draws != 1 || record.ScoredGames != 4 || current.classifications()[7003] != "win" {
		t.Errorf("record after the corrected score was saved = %+v, game 7003 %q; want 3-0-1", record, current.classifications()[7003])
	}
}

// assertHistoryDenied requires a read to be refused with status and to
// disclose no stored game.
func assertHistoryDenied(t *testing.T, browser *siteBrowser, status, playerID, teamID, seasonID int) {
	t.Helper()
	response := browser.get(historyPath(playerID, teamID, seasonID))
	if response.Code != status || strings.Contains(response.Body.String(), "UGameID") {
		t.Errorf("read for player %d team %d season %d: status %d, body %q; want %d without history", playerID, teamID, seasonID, response.Code, response.Body.String(), status)
	}
}

// storedMembershipSeasons lists the seasons of the stored membership proof
// for one player and team, whichever owner holds it.
func (route *teamHistoryRoute) storedMembershipSeasons(t *testing.T, playerID, teamID int) []int {
	t.Helper()
	items, err := route.table.Items()
	if err != nil {
		t.Fatal(err)
	}
	var seasons []int
	for _, item := range items {
		if item["kind"] == "membership" && intAttribute(item, "player_id") == playerID && intAttribute(item, "team_id") == teamID {
			seasons = append(seasons, intAttribute(item, "season_id"))
		}
	}
	return seasons
}

const oldFCSeason78 = `{"team":{"UTeamID":4102,"team_name":"Old FC","Season":78},"games":[
{"UGameID":7201,"Season":78,"UTeam1":4102,"UTeam2":5001,"SchedGameDateTime":"2025-10-06T19:00:00Z","result":"2 - 0"}]}`

func TestSoccerHistoryReadNeedsExactCurrentOrStoredTeamSeasonProof(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	route.setTeam(4102, oldFCSeason78)
	// Another team with Craig FC's name plays on Craig FC's dates in the
	// same LPS season.
	route.setTeam(4103, `{"team":{"UTeamID":4103,"team_name":"Craig FC","division_name":"Open A","Season":77},"games":[
{"UGameID":7301,"Season":77,"UTeam1":4103,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"6 - 0"}]}`)
	owner := route.signedIn(t)
	if lookup := owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4103"}}); !strings.Contains(lookup.Body.String(), "Team 4103 added to history collection.") {
		t.Fatalf("manual Team ID lookup did not archive team 4103: %q", lookup.Body.String())
	}
	route.importLinkedPlayers(t, owner)
	// The same Team ID entered again is saved with the imported workflow.
	owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4103"}})
	if saved := decryptTestSession(t, route.app, owner.cookieValue(config.LPSSessionCookieName, config.SoccerCookiePath)); fmt.Sprint(saved.Workflow.SelectedTeamIDs) != "[4103]" {
		t.Fatalf("imported workflow Team IDs = %v, want [4103]", saved.Workflow.SelectedTeamIDs)
	}
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101, 4102); !report.Complete {
		t.Fatalf("refresh: %+v", report)
	}
	// Craig has moved on: LPS now lists only Craig FC's next season.
	route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":80}]`)

	// Earlier authenticated proof still opens seasons LPS no longer lists.
	if former := readHistory(t, owner, 1001, 4102, 78); former.Record.Wins != 1 || former.Record.ScoredGames != 1 || len(former.Games) != 1 {
		t.Errorf("former Old FC season = %+v", former)
	}
	if former := readHistory(t, owner, 1001, 4101, 77); former.Record.Wins != 2 || len(former.Games) != 9 {
		t.Errorf("former Craig FC season = %+v", former)
	}
	// The current lookup alone proves a season no import has stored.
	if stored := route.storedMembershipSeasons(t, 1001, 4101); fmt.Sprint(stored) != "[77]" {
		t.Fatalf("stored Craig FC seasons for Craig = %v, want only 77", stored)
	}
	if current := readHistory(t, owner, 1001, 4101, 80); current.LPSSeasonID != 80 || current.Coverage.Status != "not_fetched" || len(current.Games) != 0 {
		t.Errorf("current Craig FC season = %+v", current)
	}

	for _, unproven := range []struct {
		name                       string
		playerID, teamID, seasonID int
		reason                     string
	}{
		{"a season the player's team played without the player", 1001, 4101, 79, "Team-season membership is unverified"},
		{"another linked player's former team", 1002, 4102, 78, "Team-season membership is unverified"},
		{"a same-named team on the same dates, entered by Team ID", 1001, 4103, 77, "Team-season membership is unverified"},
		{"a player this import does not link", 1003, 4101, 77, "Player is not confirmed by this import"},
	} {
		t.Run(unproven.name, func(t *testing.T) {
			assertHistoryDenied(t, owner, http.StatusForbidden, unproven.playerID, unproven.teamID, unproven.seasonID)
			if body := owner.get(historyPath(unproven.playerID, unproven.teamID, unproven.seasonID)).Body.String(); !strings.Contains(body, unproven.reason) {
				t.Errorf("denial body = %q, want %q", body, unproven.reason)
			}
		})
	}
	if route.table.Item("GAME#7301/META") == nil || route.table.Item("TEAM#4103/SEASON#0000000077#GAME#7301") == nil {
		t.Error("denying the unproven season removed its stored games")
	}
}

func TestSoccerHistoryReadNeedsTheCurrentImportToLinkThePlayer(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if taylor := readHistory(t, owner, 1002, 4202, 79); taylor.PlayerID != 1002 {
		t.Fatalf("Taylor's season = %+v", taylor)
	}

	// The LPS account no longer links Taylor; the owner imports it again.
	route.mu.Lock()
	route.account = `{"first_name":"Craig","last_name":"Johnson","players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true}],"user_players":[{"player_id":1001}]}`
	route.mu.Unlock()
	route.importLinkedPlayers(t, owner)

	assertHistoryDenied(t, owner, http.StatusForbidden, 1002, 4202, 79)
	if stored := route.storedMembershipSeasons(t, 1002, 4202); fmt.Sprint(stored) != "[79]" {
		t.Errorf("stored Taylor FC proof for Taylor = %v, want season 79 kept", stored)
	}
}

func TestSoccerHistoryReadNeedsTheGrantedSiteSessionAndAValidImport(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4102, oldFCSeason78)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4102); !report.Complete {
		t.Fatalf("refresh: %+v", report)
	}
	route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","Season":80}]`)
	readHistory(t, owner, 1001, 4102, 78)

	assertHistoryDenied(t, newSiteBrowser(t, route.mux), http.StatusUnauthorized, 1001, 4102, 78)

	// A site-session timeout withholds the retained import until its owner
	// signs in again.
	owner.expireSiteSession()
	assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)
	owner.signIn("/soccer")
	readHistory(t, owner, 1001, 4102, 78)

	route.app.Config.SiteInvitations[testSiteEmail] = nil
	assertHistoryDenied(t, owner, http.StatusForbidden, 1001, 4102, 78)
	route.app.Config.SiteInvitations[testSiteEmail] = []string{"soccer"}
	readHistory(t, owner, 1001, 4102, 78)

	// The import's JWT expires.
	session := decryptTestSession(t, route.app, owner.cookieValue(config.LPSSessionCookieName, config.SoccerCookiePath))
	session.JWT = testutil.TestJWT(t, time.Now().Add(-time.Minute))
	soccerURL, _ := url.Parse(siteOrigin + config.SoccerCookiePath)
	owner.jar.SetCookies(soccerURL, []*http.Cookie{{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, route.app, &session), Path: config.SoccerCookiePath}})
	assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)

	// Site sign-out ends the import, so signing in again is not enough.
	route.importLinkedPlayers(t, owner)
	readHistory(t, owner, 1001, 4102, 78)
	if signOut := owner.do(browserForm(siteOrigin, "/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("sign-out status = %d", signOut.Code)
	}
	assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)
	owner.signIn("/soccer")
	assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)
}

func TestSoccerHistoryReadNeverUsesAnotherSiteOwnersImportOrProof(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4102, oldFCSeason78)
	route.app.Config.SiteInvitations[otherSiteEmail] = []string{"soccer"}
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4102); !report.Complete {
		t.Fatalf("refresh: %+v", report)
	}
	route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","Season":80}]`)

	// Another invited account imports the same LPS account after Craig left
	// Old FC. Only the first owner holds proof of that season.
	route.cognito.subject, route.cognito.email = otherSiteSubject, otherSiteEmail
	other := route.signedIn(t)
	route.importLinkedPlayers(t, other)
	assertHistoryDenied(t, other, http.StatusForbidden, 1001, 4102, 78)
	readHistory(t, other, 1001, 4101, 80)

	// The other account signs in to the first owner's browser.
	owner.expireSiteSession()
	owner.signIn("/soccer")
	assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)
	if owner.holdsCookie(config.LPSSessionCookieName, config.SoccerCookiePath) {
		t.Error("the browser kept the first owner's import for another account")
	}

	route.cognito.subject, route.cognito.email = "stable-subject", testSiteEmail
	first := route.signedIn(t)
	route.importLinkedPlayers(t, first)
	readHistory(t, first, 1001, 4102, 78)
}

// A player LPS no longer finds has no current team-seasons, so only earlier
// proof opens a season; any other season is unverified rather than a failed
// lookup to retry.
func TestSoccerHistoryReadTreatsAPlayerLPSNoLongerFindsAsHavingNoCurrentSeasons(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	route.mu.Lock()
	delete(route.playerTeams, 1002)
	route.mu.Unlock()

	readHistory(t, owner, 1002, 4202, 79)
	assertHistoryDenied(t, owner, http.StatusForbidden, 1002, 4202, 80)
}

// A current lookup LPS refuses is judged as every other Soccer route judges
// it: a rejected token ends the import, so the proof it stored opens nothing
// more; a player LPS denies is not confirmed by the import; and an
// unavailable LPS says nothing about the import, which stays usable.
func TestSoccerHistoryReadJudgesARefusedCurrentLookupAsOtherSoccerRoutesDo(t *testing.T) {
	for _, refusal := range []struct {
		name       string
		lpsStatus  int
		wantStatus int
		wantBody   string
		importEnds bool
	}{
		{"a rejected token ends the import", http.StatusUnauthorized, http.StatusUnauthorized, "import a fresh bearer JWT", true},
		{"a denied player is not confirmed", http.StatusForbidden, http.StatusForbidden, "Player is not confirmed by this import", false},
		{"an unavailable LPS keeps the import", http.StatusInternalServerError, http.StatusBadGateway, "Current team membership could not be verified", false},
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

			// No stored proof covers Craig FC's season 80, so LPS is asked.
			refused := owner.get(historyPath(1001, 4101, 80))
			if refused.Code != refusal.wantStatus || !strings.Contains(refused.Body.String(), refusal.wantBody) {
				t.Fatalf("read after LPS answered %d: status %d, body %q; want %d with %q", refusal.lpsStatus, refused.Code, refused.Body.String(), refusal.wantStatus, refusal.wantBody)
			}
			if got := refused.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("refused read Cache-Control = %q, want private, no-store", got)
			}
			if !refusal.importEnds {
				if cleared := findSessionCookie(t, refused.Result()); cleared != nil {
					t.Errorf("read after LPS answered %d rewrote the import: %#v", refusal.lpsStatus, cleared)
				}
				// Stored proof of Old FC's season still opens it.
				if former := readHistory(t, owner, 1001, 4102, 78); former.Record.Wins != 1 || len(former.Games) != 1 {
					t.Errorf("former Old FC season after LPS answered %d = %+v", refusal.lpsStatus, former)
				}
				return
			}
			assertClearedSessionCookie(t, refused.Result())
			if guard := findImportGuardCookie(refused.Result()); guard == nil || guard.Value != "" || guard.MaxAge >= 0 {
				t.Errorf("rejected import's guard cookie = %#v, want it cleared", guard)
			}
			// The import is gone, so its stored proof opens nothing more.
			assertHistoryDenied(t, owner, http.StatusUnauthorized, 1001, 4102, 78)
			assertHistoryDenied(t, owner, http.StatusUnauthorized, 1002, 4202, 79)
		})
	}
}

// The production route assembly wires no durable archive until the #80
// activation review, so a granted owner with a valid import gets no history.
func TestSoccerHistoryReadIsUnavailableWithoutTheDurableArchive(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.handler.SetArchiveStore(nil)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)

	assertHistoryDenied(t, owner, http.StatusServiceUnavailable, 1001, 4101, 77)
	for _, query := range []string{"player_id=1001&team_id=4101", "player_id=1001&team_id=0&season_id=77", "player_id=x&team_id=4101&season_id=77"} {
		if response := owner.get("/soccer/history?" + query); response.Code != http.StatusBadRequest {
			t.Errorf("history read with %q: status %d, want 400", query, response.Code)
		}
	}
}
