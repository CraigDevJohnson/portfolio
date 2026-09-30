package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
	// logs receives the route's application log.
	logs *runtimeLogs

	mu       sync.Mutex
	requests map[string]int
	// failingPlayer, when set, is a player whose team lookup LPS fails.
	failingPlayer int
	// missingPlayer, when set, is a linked player LPS no longer finds.
	missingPlayer int
	// account, when set, replaces the LPS account /users/check returns.
	account string
	// rejectJWT makes LPS refuse the imported JWT, as it does once the
	// token is revoked.
	rejectJWT bool
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
		failingPlayer, missingPlayer, account, rejectJWT := route.failingPlayer, route.missingPlayer, route.account, route.rejectJWT
		route.mu.Unlock()
		if account == "" {
			account = playerHistoryAccount
		}
		authorized := !rejectJWT && r.Header.Get("Authorization") == "Bearer "+route.jwt
		switch r.URL.Path {
		case "/users/check", "/players/1001/my_teams", "/players/1002/my_teams":
			if !authorized {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		switch r.URL.Path {
		case "/users/check":
			_, _ = fmt.Fprint(w, account)
		case fmt.Sprintf("/players/%d/my_teams", failingPlayer):
			http.Error(w, "temporary failure", http.StatusBadGateway)
		case fmt.Sprintf("/players/%d/my_teams", missingPlayer):
			http.Error(w, "player not found", http.StatusNotFound)
		case "/players/1001/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4102,"team_name":"Old FC","Season":78}]`)
		case "/players/1002/my_teams":
			_, _ = fmt.Fprint(w, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},{"UTeamID":4202,"team_name":"Taylor FC","Season":79},{"UTeamID":4300,"team_name":"Unknown Season"}]`)
		case "/teams/4101":
			// Craig's and Taylor's shared team played Taylor FC in season 77.
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":77},"games":[{"UGameID":7001,"Season":77,"UTeam1":4101,"UTeam2":4202,"home_team":{"UTeamID":4101,"team_name":"Craig FC"},"visitor_team":{"UTeamID":4202,"team_name":"Taylor FC"},"result":"2-1"}]}`)
		case "/teams/4202":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[]}`)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	route.logs = &runtimeLogs{}
	application.Logger = route.logs.logger()
	route.mux, route.handler = buildMux(application, application.Logger, false)
	route.table = archivetest.NewTable()
	route.limitHistory(t, generousArchiveLimits)
	return route
}

// limitHistory rewires the route's archive, over the same table, with the
// given reviewed limits.
func (route *playerHistoryRoute) limitHistory(t *testing.T, limits soccerarchive.Limits) {
	t.Helper()
	store, err := soccerarchive.NewDynamoStoreWithAPI(route.table, "portfolio-lambda-dev-soccer-history", limits)
	if err != nil {
		t.Fatalf("NewDynamoStoreWithAPI: %v", err)
	}
	route.store = store
	route.handler.SetArchiveStore(store)
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

// disclosedImport opens the owner's Soccer page, requires the import dialog
// to disclose indefinite linked-player history before import, and submits
// that dialog's form with the JWT as the owner's browser would.
func (route *playerHistoryRoute) disclosedImport(t *testing.T, owner *siteBrowser) *httptest.ResponseRecorder {
	t.Helper()
	return owner.postForm("/soccer/import", route.disclosedImportForm(t, owner))
}

// disclosedImportForm opens the owner's Soccer page, requires the import
// dialog to disclose indefinite linked-player history, and returns the form
// that dialog submits with the JWT.
func (route *playerHistoryRoute) disclosedImportForm(t *testing.T, owner *siteBrowser) url.Values {
	t.Helper()
	storedBefore := route.table.Len()
	page := owner.get("/soccer")
	if page.Code != http.StatusOK {
		t.Fatalf("Soccer page status = %d", page.Code)
	}
	doc := parsePlannerHTML(t, page.Body.String())
	dialog := plannerSingle(t, doc, "import dialog", plannerAttrIs("role", "dialog"))
	notice := plannerSingle(t, dialog, "history notice", plannerAttrIs("id", "soccer-history-notice"))
	if described := strings.Fields(soccerHTMLAttribute(dialog, "aria-describedby")); !slices.Contains(described, "soccer-history-notice") {
		t.Errorf("import dialog is not described by the history notice: %q", described)
	}
	text := strings.Join(strings.Fields(plannerText(notice)), " ")
	for _, disclosure := range []string{"every player linked", "including players you don't choose", "indefinitely", "refreshing", "JWT stays temporary"} {
		if !strings.Contains(text, disclosure) {
			t.Errorf("history notice %q does not say %q", text, disclosure)
		}
	}
	form := url.Values{"jwt": {route.jwt}}
	for _, input := range plannerElements(plannerSingle(t, dialog, "import form", plannerAttrIs("id", "soccer-login-form")), plannerAttrIs("type", "hidden")) {
		form.Add(soccerHTMLAttribute(input, "name"), soccerHTMLAttribute(input, "value"))
	}
	if route.table.Len() != storedBefore {
		t.Fatal("opening the Soccer page stored linked-player history")
	}
	return form
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
	imported := route.disclosedImport(t, owner)
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

func TestSoccerImportWithoutTheHistoryDisclosureCollectsNothing(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)

	imported := owner.postForm("/soccer/import", url.Values{"jwt": {route.jwt}})

	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), `name="player_ids"`) || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("undisclosed import did not keep the planner import: status %d, body %q", imported.Code, imported.Body.String())
	}
	if route.lpsRequests("/players/1001/my_teams") != 0 || route.lpsRequests("/players/1002/my_teams") != 0 {
		t.Errorf("undisclosed import looked up linked players' teams: %v", route.requests)
	}
	if stored := route.table.Len(); stored != 0 {
		t.Errorf("undisclosed import stored %d durable items", stored)
	}
}

func TestSoccerImportWithoutDurableCollectionMakesNoHistoryClaimAndCollectsNothing(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	// The production route assembly wires no durable archive until the
	// activation review enables it.
	route.handler.SetArchiveStore(nil)
	owner := route.signedInOwner(t)

	page := owner.get("/soccer")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Import access") {
		t.Fatalf("granted Soccer page did not offer import: status %d", page.Code)
	}
	for _, claim := range []string{"soccer-history-notice", `name="history_notice"`, "indefinitely", "History collection"} {
		if strings.Contains(body, claim) {
			t.Errorf("page without durable collection contains %q", claim)
		}
	}

	imported := owner.postForm("/soccer/import", url.Values{"jwt": {route.jwt}, "history_notice": {"indefinite"}})
	if imported.Code != http.StatusOK || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("import without durable collection failed: status %d, body %q", imported.Code, imported.Body.String())
	}
	if route.lpsRequests("/players/1001/my_teams") != 0 || route.lpsRequests("/players/1002/my_teams") != 0 {
		t.Errorf("import without durable collection looked up linked players' teams: %v", route.requests)
	}
}

func TestSoccerImportKeepsEachSiteOwnersPlayerEvidenceSeparate(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	first := route.signedInOwner(t)
	if imported := route.disclosedImport(t, first); imported.Code != http.StatusOK {
		t.Fatalf("first owner's import status = %d", imported.Code)
	}
	firstEvidence := route.memberships(t)["stable-subject"]

	// Another invited site account imports the same LPS account.
	route.cognito.subject, route.cognito.email = "second-subject", "second@example.com"
	route.app.Config.SiteInvitations["second@example.com"] = []string{"soccer"}
	second := route.signedInOwner(t)
	if imported := route.disclosedImport(t, second); imported.Code != http.StatusOK {
		t.Fatalf("second owner's import status = %d", imported.Code)
	}

	byOwner := route.memberships(t)
	if len(byOwner) != 2 {
		t.Fatalf("membership owners = %v, want the two site accounts", byOwner)
	}
	for _, subject := range []string{"stable-subject", "second-subject"} {
		if len(byOwner[subject]) != len(linkedPlayerMemberships) {
			t.Errorf("owner %s memberships = %d, want %d", subject, len(byOwner[subject]), len(linkedPlayerMemberships))
		}
		for triple, item := range byOwner[subject] {
			if !linkedPlayerMemberships[triple] || item["owner_issuer"] != route.cognito.issuer {
				t.Errorf("owner %s membership %+v = %v", subject, triple, item)
			}
		}
	}
	for triple, item := range firstEvidence {
		if fmt.Sprint(byOwner["stable-subject"][triple]) != fmt.Sprint(item) {
			t.Errorf("second owner's import changed the first owner's membership %+v:\nbefore %v\nafter  %v", triple, item, byOwner["stable-subject"][triple])
		}
	}
	links := map[string]int{}
	for _, link := range route.items(t, "player_owner") {
		links[fmt.Sprint(link["owner_subject"])]++
	}
	if links["stable-subject"] != 2 || links["second-subject"] != 2 || len(links) != 2 {
		t.Errorf("player owner links by subject = %v, want both players for each owner", links)
	}
}

func TestSoccerImportCollectsOnlyForTheCurrentGrantedSiteSession(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	disclosed := url.Values{"jwt": {route.jwt}, "history_notice": {"indefinite"}}

	anonymous := newSiteBrowser(t, route.mux).postForm("/soccer/import", disclosed)
	if anonymous.Code != http.StatusUnauthorized {
		t.Errorf("signed-out import status = %d, want 401", anonymous.Code)
	}

	owner := route.signedInOwner(t)
	route.app.Config.SiteInvitations["owner@example.com"] = nil
	revoked := owner.postForm("/soccer/import", disclosed)
	if revoked.Code != http.StatusForbidden {
		t.Errorf("import after the soccer grant was revoked: status %d, want 403", revoked.Code)
	}

	if total := route.lpsRequestTotal(); total != 0 {
		t.Errorf("refused imports reached LPS %d times: %v", total, route.requests)
	}
	if stored := route.table.Len(); stored != 0 {
		t.Errorf("refused imports stored %d durable items", stored)
	}
}

func TestManualTeamIDLookupNeverCreatesPlayerMembership(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)

	// Before any import, a Team ID lookup enrolls the team without any player.
	body := owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4202"}}).Body.String()
	if !strings.Contains(body, "Team 4202 added to history collection.") {
		t.Fatalf("manual lookup did not enroll team 4202: %q", body)
	}
	for _, kind := range []string{"player", "player_owner", "membership"} {
		if found := route.items(t, kind); len(found) != 0 {
			t.Errorf("manual lookup stored %s records: %v", kind, found)
		}
	}

	if imported := route.disclosedImport(t, owner); imported.Code != http.StatusOK {
		t.Fatalf("import status = %d", imported.Code)
	}
	before := route.memberships(t)
	// After the import, the same owner's lookup of Taylor's team neither adds
	// nor refreshes a membership: only the authenticated lookup proves one.
	owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4202"}})
	after := route.memberships(t)
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Errorf("manual lookup changed player memberships:\nbefore %v\nafter  %v", before, after)
	}
	if len(route.items(t, "player")) != 2 {
		t.Errorf("manual lookup changed stored players: %v", route.items(t, "player"))
	}
}

func TestSoccerImportStopsWhenLinkedPlayerHistoryIsIncomplete(t *testing.T) {
	for _, failure := range []struct {
		name    string
		arrange func(route *playerHistoryRoute)
		message string
	}{
		{
			name:    "an unselected player's team lookup fails",
			arrange: func(route *playerHistoryRoute) { route.failingPlayer = 1002 },
			message: "Could not look up every linked player. No player history was saved; try the import again.",
		},
		{
			name: "the durable table is unavailable",
			arrange: func(route *playerHistoryRoute) {
				route.table.FailPut = func(string) error { return fmt.Errorf("table unavailable") }
			},
			message: "Linked-player history could not be saved. Try the import again.",
		},
	} {
		t.Run(failure.name, func(t *testing.T) {
			route := newPlayerHistoryRoute(t)
			failure.arrange(route)
			owner := route.signedInOwner(t)

			imported := route.disclosedImport(t, owner)

			if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), failure.message) || strings.Contains(imported.Body.String(), "data-login-success") {
				t.Fatalf("incomplete history outcome: status %d, body %q", imported.Code, imported.Body.String())
			}
			if findSessionCookie(t, imported.Result()) != nil {
				t.Error("the import was kept although the history it disclosed was not collected")
			}
			if stored := route.table.Len(); stored != 0 {
				t.Errorf("incomplete history left %d durable items", stored)
			}
		})
	}
}

func TestSoccerImportAtHistoryCapacityKeepsTheImportAndEnrollsWhatFits(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	// Player imports may fill both slots; the account's four teams do not
	// fit, so 4101 and 4102 are admitted and 4202 and 4300 are refused.
	route.limitHistory(t, soccerarchive.Limits{MaxEnrolledTeams: 2, ReservedPlayerSlots: 1, MaxRequestsPerRun: 10, MinRequestInterval: time.Second})
	route.logs.includeDefaultLogger(t)
	owner := route.signedInOwner(t)
	route.logs.take(t)

	imported := route.disclosedImport(t, owner)

	body := imported.Body.String()
	if imported.Code != http.StatusOK || !strings.Contains(body, "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("a full history capacity took the import away: status %d, body %q", imported.Code, body)
	}
	if !strings.Contains(body, "History collection is full") ||
		!strings.Contains(body, "Teams 4202 and 4300 were not added to history collection because its reviewed capacity is full.") {
		t.Fatalf("import does not say which teams history collection refused: %q", body)
	}
	teams := route.items(t, "team")
	if len(teams) != 2 || teams["TEAM#4101/META"] == nil || teams["TEAM#4102/META"] == nil {
		t.Fatalf("enrolled teams = %v, want only 4101 and 4102", teams)
	}
	byOwner := route.memberships(t)
	admitted := map[membershipTriple]bool{{1001, 4101, 77}: true, {1001, 4102, 78}: true, {1002, 4101, 77}: true}
	if len(byOwner) != 1 || len(byOwner["stable-subject"]) != len(admitted) {
		t.Fatalf("stored memberships = %v, want the three for admitted teams", byOwner)
	}
	for triple := range byOwner["stable-subject"] {
		if !admitted[triple] {
			t.Errorf("membership %+v names a team history collection refused", triple)
		}
	}
	if players, links := len(route.items(t, "player")), len(route.items(t, "player_owner")); players != 2 || links != 2 {
		t.Errorf("stored %d player identities and %d owner links, want both players", players, links)
	}
	route.assertEvidenceOnlyForEnrolledTeams(t)

	records := route.logs.take(t)
	// Refused player-linked teams are what the admission alarm counts.
	alarmed, manual, errorRecords := admissionRefusalRecords(records)
	if len(alarmed) != 2 || len(manual) != 0 || len(errorRecords) != 0 {
		t.Fatalf("capacity refusal logs = %v, want one alarmed admission warning per refused team and no error", records)
	}
	for i, teamID := range []int{4202, 4300} {
		if rejection := alarmed[i]; rejection["level"] != "WARN" || rejection["team_id"] != float64(teamID) || rejection["source"] != "player" || rejection["limit"] != float64(2) {
			t.Errorf("admission rejection record %d = %v", i, rejection)
		}
	}
}

// assertEvidenceOnlyForEnrolledTeams requires every stored membership to name
// an enrolled team and a player whose identity and owner link are stored, so
// whatever an interrupted save left behind is still consistent evidence.
func (route *playerHistoryRoute) assertEvidenceOnlyForEnrolledTeams(t *testing.T) {
	t.Helper()
	teams := route.items(t, "team")
	players := route.items(t, "player")
	owned := map[string]bool{}
	for _, link := range route.items(t, "player_owner") {
		owned[fmt.Sprintf("%v/%d", link["owner_subject"], intAttribute(link, "player_id"))] = true
	}
	for subject, memberships := range route.memberships(t) {
		for triple := range memberships {
			if team := teams[fmt.Sprintf("TEAM#%d/META", triple.team)]; team["due_pk"] != "TEAM_DUE" {
				t.Errorf("owner %s membership %+v names team %d, which is not enrolled for refresh", subject, triple, triple.team)
			}
			if players[fmt.Sprintf("PLAYER#%d/META", triple.player)] == nil || !owned[fmt.Sprintf("%s/%d", subject, triple.player)] {
				t.Errorf("owner %s membership %+v has no stored player identity and owner link", subject, triple)
			}
		}
	}
}

func TestSoccerImportRetryCompletesLinkedPlayerHistoryAnInterruptedSaveLeftIncomplete(t *testing.T) {
	for _, failure := range []struct {
		name  string
		fails func(key string) bool
	}{
		{
			name:  "a discovered team's enrollment fails",
			fails: func(key string) bool { return key == "TEAM#4202/META" },
		},
		{
			name:  "the last membership fails",
			fails: func(key string) bool { return strings.HasSuffix(key, "#TEAM#0000004202#SEASON#0000000079") },
		},
	} {
		t.Run(failure.name, func(t *testing.T) {
			route := newPlayerHistoryRoute(t)
			route.table.FailPut = func(key string) error {
				if failure.fails(key) {
					return fmt.Errorf("throttled writing %s", key)
				}
				return nil
			}
			owner := route.signedInOwner(t)

			interrupted := route.disclosedImport(t, owner)

			if interrupted.Code != http.StatusOK || !strings.Contains(interrupted.Body.String(), "Linked-player history could not be saved. Try the import again.") {
				t.Fatalf("interrupted save outcome: status %d, body %q", interrupted.Code, interrupted.Body.String())
			}
			if findSessionCookie(t, interrupted.Result()) != nil {
				t.Error("the import was kept although the history it disclosed was not saved in full")
			}
			route.assertEvidenceOnlyForEnrolledTeams(t)

			route.table.FailPut = nil
			retried := route.disclosedImport(t, owner)

			if retried.Code != http.StatusOK || findSessionCookie(t, retried.Result()) == nil {
				t.Fatalf("retried import did not keep the import: status %d, body %q", retried.Code, retried.Body.String())
			}
			byOwner := route.memberships(t)
			if len(byOwner) != 1 || len(byOwner["stable-subject"]) != len(linkedPlayerMemberships) {
				t.Fatalf("memberships after the retry = %v, want the four associations for stable-subject", byOwner)
			}
			for triple := range byOwner["stable-subject"] {
				if !linkedPlayerMemberships[triple] {
					t.Errorf("unexpected membership %+v after the retry", triple)
				}
			}
			if players, links, teams := len(route.items(t, "player")), len(route.items(t, "player_owner")), len(route.items(t, "team")); players != 2 || links != 2 || teams != 4 {
				t.Errorf("after the retry: %d players, %d owner links, %d enrolled teams; want 2, 2, 4", players, links, teams)
			}
			route.assertEvidenceOnlyForEnrolledTeams(t)
		})
	}
}

func TestSoccerImportCollectsTheOtherPlayersWhenLPSNoLongerFindsALinkedPlayer(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	route.missingPlayer = 1002
	owner := route.signedInOwner(t)

	imported := route.disclosedImport(t, owner)

	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("import with a linked player LPS no longer finds: status %d, body %q", imported.Code, imported.Body.String())
	}
	byOwner := route.memberships(t)
	craigOnly := map[membershipTriple]bool{{1001, 4101, 77}: true, {1001, 4102, 78}: true}
	if len(byOwner) != 1 || len(byOwner["stable-subject"]) != len(craigOnly) {
		t.Fatalf("stored memberships = %v, want Craig's two associations", byOwner)
	}
	for triple := range byOwner["stable-subject"] {
		if !craigOnly[triple] {
			t.Errorf("unexpected membership %+v", triple)
		}
	}
	players := route.items(t, "player")
	if len(players) != 2 || players["PLAYER#1002/META"]["first_name"] != "Taylor" {
		t.Errorf("stored player identities = %v, want Craig and Taylor", players)
	}
	links := map[int]bool{}
	for _, link := range route.items(t, "player_owner") {
		links[intAttribute(link, "player_id")] = link["owner_subject"] == "stable-subject"
	}
	if len(links) != 2 || !links[1001] || !links[1002] {
		t.Errorf("owner links by player = %v, want Craig and Taylor for stable-subject", links)
	}
	teams := route.items(t, "team")
	if len(teams) != 2 || teams["TEAM#4101/META"] == nil || teams["TEAM#4102/META"] == nil {
		t.Errorf("enrolled teams = %v, want Craig's 4101 and 4102", teams)
	}
	route.assertEvidenceOnlyForEnrolledTeams(t)
}

// A page on another origin, even a sibling subdomain of the same site, sends
// this site's Lax cookies with a top-level form POST. Only the Soccer page
// itself may submit the import that binds history to the signed-in owner.
func TestSoccerImportCollectsHistoryOnlyFromThisSitesOwnPages(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)
	form := route.disclosedImportForm(t, owner)

	for _, sender := range []struct{ origin, fetchSite string }{
		{origin: "https://dev.example.com", fetchSite: "same-site"},
		{origin: anotherOrigin, fetchSite: "cross-site"},
	} {
		request := browserForm(sender.origin, "/soccer/import", form)
		request.Header.Set("Sec-Fetch-Site", sender.fetchSite)
		refused := owner.do(request)
		if refused.Code != http.StatusForbidden || findSessionCookie(t, refused.Result()) != nil {
			t.Errorf("%s import from %s: status %d, want 403 without imported access", sender.fetchSite, sender.origin, refused.Code)
		}
	}
	if total := route.lpsRequestTotal(); total != 0 {
		t.Errorf("imports from other origins reached LPS %d times: %v", total, route.requests)
	}
	if stored := route.table.Len(); stored != 0 {
		t.Errorf("imports from other origins stored %d durable items", stored)
	}

	accepted := owner.do(browserForm(siteOrigin, "/soccer/import", form))
	if accepted.Code != http.StatusOK || findSessionCookie(t, accepted.Result()) == nil {
		t.Fatalf("same-origin import: status %d, body %q", accepted.Code, accepted.Body.String())
	}
	if collected := len(route.memberships(t)["stable-subject"]); collected != len(linkedPlayerMemberships) {
		t.Errorf("same-origin import stored %d memberships, want %d", collected, len(linkedPlayerMemberships))
	}
}
