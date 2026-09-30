package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
	"portfolio/internal/testutil"
)

// playerHistoryRoute is the real route assembly with fake Cognito site
// sign-in, a fake LPS account that links two players, and the durable archive
// over an in-memory DynamoDB table, as an approved activation would wire it.
type playerHistoryRoute struct {
	cognito *fakeSiteCognito
	app     *App
	mux     http.Handler
	handler *internalsoccer.Handler
	table   *archivetest.Table
	store   *soccerarchive.DynamoStore
	jwt     string

	mu       sync.Mutex
	requests map[string]int
	// failingPlayer, when set, is a player whose team lookup LPS fails.
	failingPlayer int
}

// The fake LPS account links Craig (the account's main player) and Taylor.
// Both play for team 4101 in LPS season 77; Craig also played for 4102 in
// season 78, and Taylor plays for 4202 in season 79 and for 4300, which LPS
// returns without a season.
const playerHistoryAccount = `{"first_name":"Craig","last_name":"Johnson",` +
	`"players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},` +
	`{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson","is_main_player":false}],` +
	`"user_players":[{"player_id":1001},{"player_id":1002}]}`

func newPlayerHistoryRoute(t *testing.T) *playerHistoryRoute {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	route := &playerHistoryRoute{cognito: cognito, app: application, jwt: testutil.TestJWT(t, time.Now().Add(time.Hour)), requests: map[string]int{}}
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route.mu.Lock()
		route.requests[r.URL.Path]++
		failingPlayer := route.failingPlayer
		route.mu.Unlock()
		authorized := r.Header.Get("Authorization") == "Bearer "+route.jwt
		switch r.URL.Path {
		case "/users/check", "/players/1001/my_teams", "/players/1002/my_teams":
			if !authorized {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		switch r.URL.Path {
		case "/users/check":
			_, _ = fmt.Fprint(w, playerHistoryAccount)
		case fmt.Sprintf("/players/%d/my_teams", failingPlayer):
			http.Error(w, "temporary failure", http.StatusBadGateway)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4102,"team_name":"Old FC","Season":78}]`)
		case "/players/1002/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4202,"team_name":"Taylor FC","Season":79},{"UTeamID":4300,"team_name":"Unknown Season"}]`)
		case "/teams/4202":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[]}`)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	route.mux, route.handler = buildMux(application, application.Logger, false)
	route.table = archivetest.NewTable()
	route.store = soccerarchive.NewDynamoStoreWithAPI(route.table, "portfolio-lambda-dev-soccer-history")
	route.handler.SetArchiveStore(route.store)
	return route
}

// lpsRequests returns how many requests the fake LPS received for path.
func (route *playerHistoryRoute) lpsRequests(path string) int {
	route.mu.Lock()
	defer route.mu.Unlock()
	return route.requests[path]
}

// lpsRequestTotal returns how many requests the fake LPS received.
func (route *playerHistoryRoute) lpsRequestTotal() int {
	route.mu.Lock()
	defer route.mu.Unlock()
	total := 0
	for _, count := range route.requests {
		total += count
	}
	return total
}

// signedInOwner signs a new browser in through the fake Cognito as whoever
// the fake identity currently names.
func (route *playerHistoryRoute) signedInOwner(t *testing.T) *siteBrowser {
	t.Helper()
	browser := newSiteBrowser(t, route.mux)
	if landing := browser.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("site sign-in status = %d", landing.Code)
	}
	return browser
}

// items returns the stored items of one kind, keyed by "pk/sk".
func (route *playerHistoryRoute) items(t *testing.T, kind string) map[string]map[string]any {
	t.Helper()
	items, err := route.table.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	matched := make(map[string]map[string]any)
	for key, item := range items {
		if item["kind"] == kind {
			matched[key] = item
		}
	}
	return matched
}

// membershipTriple names one player-team-season association.
type membershipTriple struct{ player, team, season int }

// memberships returns each stored membership by owner subject.
func (route *playerHistoryRoute) memberships(t *testing.T) map[string]map[membershipTriple]map[string]any {
	t.Helper()
	byOwner := make(map[string]map[membershipTriple]map[string]any)
	for _, item := range route.items(t, "membership") {
		subject := fmt.Sprint(item["owner_subject"])
		if byOwner[subject] == nil {
			byOwner[subject] = make(map[membershipTriple]map[string]any)
		}
		triple := membershipTriple{player: intAttribute(item, "player_id"), team: intAttribute(item, "team_id"), season: intAttribute(item, "season_id")}
		if _, duplicate := byOwner[subject][triple]; duplicate {
			t.Errorf("owner %s has two membership records for %+v", subject, triple)
		}
		byOwner[subject][triple] = item
	}
	return byOwner
}

func intAttribute(item map[string]any, name string) int {
	value, _ := item[name].(float64)
	return int(value)
}

// observedWithin parses a stored observation time and requires it to fall
// between before and after.
func observedWithin(t *testing.T, key string, item map[string]any, before, after time.Time) {
	t.Helper()
	observed, err := time.Parse(time.RFC3339Nano, fmt.Sprint(item["observed_at"]))
	if err != nil {
		t.Errorf("%s observed_at %q: %v", key, item["observed_at"], err)
		return
	}
	if observed.Before(before.Add(-time.Millisecond)) || observed.After(after) {
		t.Errorf("%s observed_at = %s, want between %s and %s", key, observed, before, after)
	}
}

// the four exact associations the fake LPS account returns.
var linkedPlayerMemberships = map[membershipTriple]bool{
	{1001, 4101, 77}: true, {1001, 4102, 78}: true, {1002, 4101, 77}: true, {1002, 4202, 79}: true,
}

func TestGrantedSoccerImportRecordsEveryLinkedPlayersOwnerBoundTeamSeasons(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)

	before := time.Now()
	imported := owner.postForm("/soccer/import", url.Values{"jwt": {route.jwt}})
	after := time.Now()

	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("granted import did not list linked players: status %d, body %q", imported.Code, imported.Body.String())
	}
	if route.lpsRequests("/users/check") != 1 || route.lpsRequests("/players/1001/my_teams") != 1 || route.lpsRequests("/players/1002/my_teams") != 1 || route.lpsRequestTotal() != 3 {
		t.Fatalf("import did not look up every linked player's teams, and only those: %v", route.requests)
	}

	byOwner := route.memberships(t)
	if len(byOwner) != 1 || len(byOwner["stable-subject"]) != len(linkedPlayerMemberships) {
		t.Fatalf("stored memberships by owner = %v, want the four associations for stable-subject", byOwner)
	}
	for triple, item := range byOwner["stable-subject"] {
		key := fmt.Sprintf("%+v", triple)
		if !linkedPlayerMemberships[triple] {
			t.Errorf("unexpected membership %s", key)
		}
		if item["owner_issuer"] != route.cognito.issuer || item["source"] != "authenticated_player_lookup" {
			t.Errorf("membership %s owner or source = %v/%v", key, item["owner_issuer"], item["source"])
		}
		observedWithin(t, key, item, before, after)
	}
	if craig := byOwner["stable-subject"][membershipTriple{1001, 4101, 77}]; craig["team_name"] != "Craig FC" || craig["division_name"] != "Open A" {
		t.Errorf("membership team facts = %v/%v, want Craig FC in Open A", craig["team_name"], craig["division_name"])
	}

	players := route.items(t, "player")
	if craig, taylor := players["PLAYER#1001/META"], players["PLAYER#1002/META"]; len(players) != 2 ||
		craig["first_name"] != "Craig" || craig["last_name"] != "Johnson" || taylor["first_name"] != "Taylor" || taylor["last_name"] != "Johnson" {
		t.Fatalf("stored player identities = %v", players)
	}
	mainPlayer := map[int]bool{}
	for key, link := range route.items(t, "player_owner") {
		if link["owner_issuer"] != route.cognito.issuer || link["owner_subject"] != "stable-subject" || link["source"] != "authenticated_player_lookup" {
			t.Errorf("owner link %s = %v", key, link)
		}
		observedWithin(t, key, link, before, after)
		main, ok := link["is_main_player"].(bool)
		if !ok {
			t.Errorf("owner link %s has no main-player flag: %v", key, link)
		}
		mainPlayer[intAttribute(link, "player_id")] = main
	}
	if len(mainPlayer) != 2 || !mainPlayer[1001] || mainPlayer[1002] {
		t.Errorf("owner links' main-player flags = %v, want Craig main and Taylor not", mainPlayer)
	}

	teams := route.items(t, "team")
	if len(teams) != 4 {
		t.Errorf("enrolled teams = %d, want the four discovered teams", len(teams))
	}
	for _, teamID := range []int{4101, 4102, 4202, 4300} {
		team := teams[fmt.Sprintf("TEAM#%d/META", teamID)]
		if team["enrollment_source"] != "player" || team["due_pk"] != "TEAM_DUE" {
			t.Errorf("discovered team %d enrollment = %v", teamID, team)
		}
	}

	stored, err := route.table.Items()
	if err != nil {
		t.Fatal(err)
	}
	for key, item := range stored {
		for _, sessionAttribute := range []string{"ttl", "expires_at"} {
			if _, found := item[sessionAttribute]; found {
				t.Errorf("durable item %s carries the import session attribute %q", key, sessionAttribute)
			}
		}
		for attribute, value := range item {
			if strings.Contains(fmt.Sprint(value), route.jwt) {
				t.Errorf("durable item %s attribute %q holds the imported JWT", key, attribute)
			}
		}
	}

	sessionCookie := findSessionCookie(t, imported.Result())
	if sessionCookie == nil {
		t.Fatal("import did not retain LPS access")
	}
	session := decryptTestSession(t, route.app, sessionCookie.Value)
	if len(session.Players) != 2 || session.Workflow.Source != "" || len(session.Workflow.SelectedPlayerIDs) != 0 || len(session.Workflow.SelectedTeamIDs) != 0 {
		t.Fatalf("import selected planner choices: players %d, workflow %+v", len(session.Players), session.Workflow)
	}
	if route.table.Item("TEAM#4101/COVERAGE") != nil {
		t.Error("the import fetched a discovered team's games")
	}
}
