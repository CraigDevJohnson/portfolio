package app

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
	"portfolio/types"
)

// These tests drive the linked-player schedule source (#92) through the real
// route assembly: the invited owner signs in through a fake Cognito, imports
// a JWT that a fake Let's Play Soccer API accepts, and follows the planner
// from its players and teams to an .ics download. Craig (1001) plays for North
// FC (101) and Taylor (1002) for South FC (202); game 2020 is on both
// schedules.

const (
	linkedNorthTeamSchedule = `/teams/101`
	linkedSouthTeamSchedule = `/teams/202`
)

// linkedPlannerWorld is a signed-in, granted browser holding an imported
// LPS session, and the fake LPS it imported from.
type linkedPlannerWorld struct {
	app     *App
	mux     http.Handler
	cognito *fakeSiteCognito
	browser *siteBrowser
	jwt     string
	// teamLookup answers every /players/{id}/my_teams request with this
	// status once set; zero keeps the linked teams.
	teamLookup atomic.Int32
	// taylorLookup answers only Taylor's /players/1002/my_teams request with
	// this status once set, as when LPS withdraws one linked player.
	taylorLookup atomic.Int32
	// noTeams answers /players/{id}/my_teams with no current teams.
	noTeams atomic.Bool
	// bearerCalls counts LPS requests that carried the imported JWT.
	bearerCalls atomic.Int32
}

func newLinkedPlannerWorld(t *testing.T) *linkedPlannerWorld {
	t.Helper()
	world := &linkedPlannerWorld{cognito: newFakeSiteCognito(t), jwt: testutil.TestJWT(t, time.Now().Add(time.Hour))}
	world.app = world.cognito.app(t)
	world.app.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+world.jwt {
			world.bearerCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		teams := map[string]string{
			"/players/1001/my_teams": `[{"UTeamID":101,"team_name":"North FC","Season":77}]`,
			"/players/1002/my_teams": `[{"UTeamID":202,"team_name":"South FC","Season":77}]`,
		}
		switch path := r.URL.Path; {
		case path == "/users/check":
			_, _ = w.Write([]byte(`{"first_name":"Craig","last_name":"Johnson",` +
				`"players":[{"UPlayerID":1001,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},{"UPlayerID":1002,"FirstName":"Taylor","LastName":"Johnson"}],` +
				`"user_players":[{"player_id":1001,"deleted":false},{"player_id":1002,"deleted":false}]}`))
		case teams[path] != "":
			if r.Header.Get("Authorization") != "Bearer "+world.jwt {
				t.Errorf("team discovery for %s omitted the imported LPS token", path)
			}
			status := int(world.teamLookup.Load())
			if taylor := int(world.taylorLookup.Load()); taylor != 0 && path == "/players/1002/my_teams" {
				status = taylor
			}
			if status != 0 {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":"refused"}`))
				return
			}
			if world.noTeams.Load() {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(teams[path]))
		case path == linkedNorthTeamSchedule:
			_, _ = w.Write([]byte(publicScheduleJSON(publicLateGame, publicSharedGame, publicPastGame)))
		case path == linkedSouthTeamSchedule:
			_, _ = w.Write([]byte(publicScheduleJSON(publicSharedGame, publicSoonGame)))
		default:
			t.Errorf("unexpected LPS request %s", path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lps.Close)
	world.app.Config.LPSAPIBaseURL = lps.URL
	world.mux, _ = buildMux(world.app, world.app.Logger, false)
	world.browser = newSiteBrowser(t, world.mux)
	if landing := world.browser.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("owner sign-in status = %d", landing.Code)
	}
	return world
}

// importLinkedPlayers imports the fake LPS account through the real route
// and returns the import response.
func (world *linkedPlannerWorld) importLinkedPlayers(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	imported := world.browser.postForm("/soccer/import", url.Values{"jwt": {world.jwt}})
	if imported.Code != http.StatusOK || !world.browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Fatalf("LPS import: status %d, body %q", imported.Code, imported.Body.String())
	}
	return imported
}

// assertLinkedSourceRefreshed requires a response to replace the page's
// linked-player source option out of band with the given state and copy.
func assertLinkedSourceRefreshed(t *testing.T, doc *html.Node, wantState, wantCopy string) {
	t.Helper()
	source := plannerSingle(t, doc, "linked-player source option", plannerAttrIs("id", "soccer-linked-source"))
	if got := soccerHTMLAttribute(source, "hx-swap-oob"); got != "outerHTML" {
		t.Errorf("linked-player source swap = %q, want an out-of-band outerHTML replacement", got)
	}
	if !plannerHasAttr(source, "data-soccer-linked-source") || !soccerHTMLClassContains(source, "soccer-source-option") {
		t.Error("the refreshed linked-player source lost its planner hooks")
	}
	if got := soccerHTMLAttribute(source, "data-source-state"); got != wantState {
		t.Errorf("linked-player source state = %q, want %q", got, wantState)
	}
	if text := plannerText(source); !strings.Contains(text, wantCopy) {
		t.Errorf("linked-player source = %q, want %q", text, wantCopy)
	}
}

// linkedFormValues returns the named form's fields as the browser would submit
// them: hidden inputs, and checked checkboxes.
func linkedFormValues(t *testing.T, doc *html.Node, formID string) url.Values {
	t.Helper()
	form := plannerSingle(t, doc, "form "+formID, plannerAttrIs("id", formID))
	values := url.Values{}
	for _, input := range plannerElements(form, func(node *html.Node) bool { return node.Data == "input" }) {
		name := soccerHTMLAttribute(input, "name")
		switch soccerHTMLAttribute(input, "type") {
		case "hidden":
			values.Add(name, soccerHTMLAttribute(input, "value"))
		case "checkbox":
			if name != "" && plannerHasAttr(input, "checked") {
				values.Add(name, soccerHTMLAttribute(input, "value"))
			}
		}
	}
	return values
}

// linkedOptionLabels returns the visible labels of the named checkboxes.
func linkedOptionLabels(doc *html.Node, name string) []string {
	var labels []string
	for _, input := range plannerElements(doc, plannerAttrIs("name", name)) {
		for label := input.Parent; label != nil; label = label.Parent {
			if label.Type == html.ElementNode && label.Data == "label" {
				labels = append(labels, plannerText(label))
				break
			}
		}
	}
	return labels
}

func linkedSelectedCount(t *testing.T, doc *html.Node) string {
	t.Helper()
	return plannerText(plannerSingle(t, doc, "upcoming selected count", func(node *html.Node) bool {
		return plannerHasAttr(node, "data-selected-count") && soccerHTMLAttribute(node, "data-game-group") == "upcoming-games"
	}))
}

func linkedTeamFingerprint(t *testing.T, doc *html.Node) string {
	t.Helper()
	return soccerHTMLAttribute(plannerSingle(t, doc, "team-set scope", func(node *html.Node) bool {
		return plannerHasAttr(node, "data-team-fingerprint")
	}), "data-team-fingerprint")
}

func TestLinkedPlayerSourceRevealsPlayersAndTheirTeamsForAGrantedImport(t *testing.T) {
	world := newLinkedPlannerWorld(t)

	before := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
	source := plannerSingle(t, before, "linked-player source option", func(node *html.Node) bool {
		return plannerHasAttr(node, "data-soccer-linked-source") && soccerHTMLClassContains(node, "soccer-source-option")
	})
	if hidden := plannerHiddenAncestor(source); hidden != nil || !strings.Contains(plannerText(source), "Use linked players") {
		t.Fatalf("granted visitor without an import lacks the linked-player source: %q", plannerText(source))
	}
	if imports := plannerElements(before, func(node *html.Node) bool {
		return node.Data == "button" && plannerHasAttr(node, "data-open-login-modal")
	}); len(imports) == 0 {
		t.Fatal("granted visitor is not offered an LPS import")
	}

	imported := world.importLinkedPlayers(t)
	// The page stays open through the import, so the import response brings
	// its linked-player source up to date.
	assertLinkedSourceRefreshed(t, parsePlannerHTML(t, imported.Body.String()), "ready", "2 linked player(s) are ready")
	page := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
	players := plannerSingle(t, page, "player stage", plannerAttrIs("data-soccer-stage", "players"))
	if hidden := plannerHiddenAncestor(players); hidden != nil {
		t.Fatalf("player stage is server-rendered hidden inside <%s %v> despite the import", hidden.Data, hidden.Attr)
	}
	if got := linkedOptionLabels(players, "player_ids"); !slices.Equal(got, []string{"Craig Johnson Primary player", "Taylor Johnson"}) {
		t.Fatalf("linked players offered = %q", got)
	}

	discovered := parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", linkedFormValues(t, page, "soccer-player-select-form")).Body.String())
	if got := linkedOptionLabels(discovered, "team_ids"); !slices.Equal(got, []string{"North FC Season 77", "South FC Season 77"}) {
		t.Fatalf("linked teams offered = %q", got)
	}
	if got := linkedFormValues(t, discovered, "soccer-team-select-form")["team_ids"]; !slices.Equal(got, []string{"101", "202"}) {
		t.Fatalf("linked teams checked by default = %v, want both", got)
	}
}

func TestLinkedPlayerPlannerFromChoiceThroughICSDownload(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)
	page := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
	teams := parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", linkedFormValues(t, page, "soccer-player-select-form")).Body.String())

	fetched := world.browser.postForm("/soccer/fetch", linkedFormValues(t, teams, "soccer-team-select-form"))
	if fetched.Code != http.StatusOK {
		t.Fatalf("linked-team fetch status = %d", fetched.Code)
	}
	results := parsePlannerHTML(t, fetched.Body.String())
	rows := plannerGameRows(results, "upcoming-games")
	if got := plannerRowIDs(rows); !slices.Equal(got, []string{"3030", "2020", "1010"}) {
		t.Fatalf("linked upcoming rows = %v, want the shared game once and soonest first [3030 2020 1010]", got)
	}
	for _, row := range rows {
		if !row.Checked {
			t.Errorf("linked game %s did not begin selected", row.ID)
		}
	}
	if got := linkedSelectedCount(t, results); got != "3 games selected" {
		t.Errorf("selected count = %q", got)
	}
	if got := linkedTeamFingerprint(t, results); got != "101-202" {
		t.Errorf("team-set scope = %q, want 101-202", got)
	}

	// The visitor deselects the shared game and downloads the rest from the
	// rendered download form.
	download := linkedFormValues(t, results, "upcoming-games-form")
	download["selected"] = slices.DeleteFunc(download["selected"], func(id string) bool { return id == "2020" })
	ics := world.browser.postForm("/soccer/download", download)
	if ics.Code != http.StatusOK || ics.Header().Get("Content-Type") != "text/calendar" {
		t.Fatalf("linked .ics download: status %d, body %q", ics.Code, ics.Body.String())
	}
	events := icsEvents(testutil.UnfoldICS(ics.Body.String()))
	if len(events) != 2 || events["3030"] == "" || events["1010"] == "" {
		t.Fatalf(".ics events = %v, want exactly the selected games 3030 and 1010", slices.Sorted(maps.Keys(events)))
	}
	if world.bearerCalls.Load() == 0 {
		t.Fatal("the linked-player journey never used the imported LPS access")
	}
}

func TestLinkedPlayerSelectionFollowsPlayerAndTeamChanges(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)

	onlyTaylor := parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1002"}}).Body.String())
	if got := linkedOptionLabels(onlyTaylor, "team_ids"); !slices.Equal(got, []string{"South FC Season 77"}) {
		t.Fatalf("teams for Taylor alone = %q, want only South FC", got)
	}
	south := parsePlannerHTML(t, world.browser.postForm("/soccer/fetch", linkedFormValues(t, onlyTaylor, "soccer-team-select-form")).Body.String())
	if got := plannerRowIDs(plannerGameRows(south, "upcoming-games")); !slices.Equal(got, []string{"3030", "2020"}) {
		t.Fatalf("South FC rows = %v, want [3030 2020]", got)
	}
	if count, scope := linkedSelectedCount(t, south), linkedTeamFingerprint(t, south); count != "2 games selected" || scope != "202" {
		t.Fatalf("South FC selection = %q scoped to %q, want 2 games scoped to 202", count, scope)
	}
	// A North FC game is outside the South FC download.
	if outside := world.browser.postForm("/soccer/download", url.Values{"team_codes": {"202"}, "player_ids": {"1002"}, "selected": {"1010"}}); outside.Code != http.StatusBadRequest {
		t.Fatalf("download of a game outside the chosen teams: status %d, want 400", outside.Code)
	}

	both := linkedFormValues(t, parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}}).Body.String()), "soccer-team-select-form")
	first := parsePlannerHTML(t, world.browser.postForm("/soccer/fetch", both).Body.String())
	again := parsePlannerHTML(t, world.browser.postForm("/soccer/fetch", both).Body.String())
	for name, doc := range map[string]*html.Node{"fetch": first, "refetch": again} {
		if count, scope := linkedSelectedCount(t, doc), linkedTeamFingerprint(t, doc); count != "3 games selected" || scope != "101-202" {
			t.Errorf("%s of both teams = %q scoped to %q, want 3 games scoped to 101-202", name, count, scope)
		}
	}
}

// linkedAccessNotice returns the LPS connection card and the text of the
// notice it shows about imported access that ended.
func linkedAccessNotice(t *testing.T, doc *html.Node) (card *html.Node, notice string) {
	t.Helper()
	card = plannerSingle(t, doc, "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	for _, feedback := range plannerElements(card, func(node *html.Node) bool { return soccerHTMLClassContains(node, "ui-feedback") }) {
		notice += plannerText(feedback)
	}
	return card, notice
}

// assertImportRecovery requires the LPS connection card, which stays visible
// once an output is chosen, to explain the ended import beside a fresh import
// and the manual Team ID path, and no linked player to remain on the page.
func assertImportRecovery(t *testing.T, doc *html.Node, wantNotice string) {
	t.Helper()
	card, notice := linkedAccessNotice(t, doc)
	if !strings.Contains(notice, wantNotice) {
		t.Errorf("LPS card notice = %q, want %q", notice, wantNotice)
	}
	if got := soccerHTMLAttribute(card, "data-connection-state"); got != "disconnected" {
		t.Errorf("LPS card state = %q, want disconnected", got)
	}
	if imports := plannerElements(card, func(node *html.Node) bool {
		return node.Data == "button" && plannerHasAttr(node, "data-open-login-modal") && plannerText(node) == "Import fresh access"
	}); len(imports) != 1 {
		t.Error("LPS card lacks the Import fresh access action")
	}
	if manual := plannerElements(card, func(node *html.Node) bool {
		return node.Data == "a" && soccerHTMLAttribute(node, "href") == "#team_codes"
	}); len(manual) != 1 {
		t.Error("LPS card lacks the manual Team ID path")
	}
	// The site header names Craig Johnson, so Taylor and the player choices
	// show whether a linked player remains.
	playerChoices := plannerElements(doc, func(node *html.Node) bool {
		return soccerHTMLAttribute(node, "name") == "player_ids" && soccerHTMLAttribute(node, "type") == "checkbox"
	})
	if strings.Contains(plannerText(doc), "Taylor Johnson") || len(playerChoices) != 0 {
		t.Error("recovery exposed the linked players")
	}
}

func TestLostLPSAccessDuringTeamDiscoveryOffersRecoveryWithoutPlayers(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)
	world.teamLookup.Store(http.StatusUnauthorized)

	lost := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}})

	if lost.Code != http.StatusOK || !strings.Contains(lost.Header().Get("HX-Trigger"), "soccer-workflow-reset") {
		t.Fatalf("lost LPS access: status %d, HX-Trigger %q", lost.Code, lost.Header().Get("HX-Trigger"))
	}
	if world.browser.holdsCookie(config.LPSSessionCookieName, "/soccer") || world.browser.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
		t.Error("the browser kept imported access LPS rejected")
	}
	doc := parsePlannerHTML(t, lost.Body.String())
	card, _ := linkedAccessNotice(t, doc)
	if soccerHTMLAttribute(card, "hx-swap-oob") != "outerHTML" {
		t.Error("the LPS card is not replaced out of band")
	}
	assertImportRecovery(t, doc, "Your imported Let's Play Soccer token was rejected.")
	assertLinkedSourceRefreshed(t, doc, "locked", "Set up LPS access in Connections above")

	page := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
	if text := plannerText(page); strings.Contains(text, importedAccessShown) || strings.Contains(text, "Taylor Johnson") {
		t.Error("the next page still offered the rejected import")
	}
}

func TestLostLPSAccessOnReturnOffersRecoveryWithoutPlayers(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)
	if saved := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}}); saved.Code != http.StatusOK {
		t.Fatalf("player choice status = %d", saved.Code)
	}
	world.teamLookup.Store(http.StatusUnauthorized)

	page := world.browser.get("/soccer")

	assertClearedSessionCookie(t, page.Result())
	assertImportRecovery(t, parsePlannerHTML(t, page.Body.String()), "Your imported Let's Play Soccer token was rejected.")
}

// holdExpiredImport gives the browser the owner's import of both linked
// players, with Taylor chosen, after its JWT expired.
func (world *linkedPlannerWorld) holdExpiredImport(t *testing.T) {
	t.Helper()
	expired := &types.SessionData{
		JWT:          testutil.TestJWT(t, time.Now().Add(-time.Minute)),
		Players:      []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}, {UPlayerID: 1002, FirstName: "Taylor", LastName: "Johnson"}},
		ExpiresAt:    time.Now().Add(-time.Minute),
		OwnerIssuer:  world.cognito.issuer,
		OwnerSubject: world.cognito.subject,
		Workflow:     types.SoccerWorkflowState{Source: "imported", SelectedPlayerIDs: []int{1002}},
	}
	soccer, _ := url.Parse("https://app.example.com/soccer")
	for _, cookie := range importedAccessCookies(t, world.app, expired) {
		cookie.Path = config.SoccerCookiePath
		world.browser.jar.SetCookies(soccer, []*http.Cookie{cookie})
	}
}

const expiredImportNotice = "Your imported Let's Play Soccer token expired."

func TestExpiredImportExplainsRecoveryToItsOwner(t *testing.T) {
	t.Run("returning to the planner", func(t *testing.T) {
		world := newLinkedPlannerWorld(t)
		world.holdExpiredImport(t)

		page := world.browser.get("/soccer")

		assertClearedSessionCookie(t, page.Result())
		assertImportRecovery(t, parsePlannerHTML(t, page.Body.String()), expiredImportNotice)
	})
	t.Run("choosing players", func(t *testing.T) {
		world := newLinkedPlannerWorld(t)
		world.holdExpiredImport(t)

		expired := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1002"}})

		if !strings.Contains(expired.Header().Get("HX-Trigger"), "soccer-workflow-reset") {
			t.Errorf("expired import did not close the private workflow: HX-Trigger %q", expired.Header().Get("HX-Trigger"))
		}
		assertClearedSessionCookie(t, expired.Result())
		assertImportRecovery(t, parsePlannerHTML(t, expired.Body.String()), expiredImportNotice)
		if world.bearerCalls.Load() != 0 {
			t.Error("the expired import was sent to LPS")
		}
	})
	t.Run("fetching chosen teams", func(t *testing.T) {
		world := newLinkedPlannerWorld(t)
		world.holdExpiredImport(t)

		fetched := world.browser.postForm("/soccer/fetch", url.Values{"selection_mode": {"teams"}, "player_ids": {"1002"}, "team_ids": {"202"}})

		if !strings.Contains(fetched.Header().Get("HX-Trigger"), "soccer-workflow-reset") {
			t.Errorf("expired import did not close the private workflow: HX-Trigger %q", fetched.Header().Get("HX-Trigger"))
		}
		// The fetch keeps its team choice, but not the expired import.
		if world.browser.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
			t.Error("the browser kept the expired import's guard")
		}
		if page := world.browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) {
			t.Error("the next page presented the expired import as active")
		}
		doc := parsePlannerHTML(t, fetched.Body.String())
		assertImportRecovery(t, doc, expiredImportNotice)
		// The chosen teams' schedules are public, so they still load.
		if got := plannerRowIDs(plannerGameRows(doc, "upcoming-games")); !slices.Equal(got, []string{"3030", "2020"}) {
			t.Errorf("South FC rows after the import expired = %v, want [3030 2020]", got)
		}
	})
}

func TestAnotherOwnerIsNotToldAboutAnExpiredImport(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.holdExpiredImport(t)
	world.app.Config.SiteInvitations[otherSiteEmail] = []string{"soccer"}
	world.browser.expireSiteSession()
	world.cognito.subject, world.cognito.email = otherSiteSubject, otherSiteEmail
	world.browser.signIn("/soccer")

	page := world.browser.get("/soccer")

	assertClearedSessionCookie(t, page.Result())
	doc := parsePlannerHTML(t, page.Body.String())
	if _, notice := linkedAccessNotice(t, doc); notice != "" {
		t.Errorf("another owner was told about the previous owner's import: %q", notice)
	}
	if strings.Contains(plannerText(doc), "Taylor Johnson") || len(plannerElements(doc, plannerAttrIs("name", "player_ids"))) != 0 {
		t.Error("another owner saw the previous owner's linked players")
	}
}

func TestTeamDiscoveryThatLPSCannotServeKeepsTheImport(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fail       func(world *linkedPlannerWorld)
		wantText   string
		wantImport bool
	}{
		{
			name:     "LPS unavailable",
			fail:     func(world *linkedPlannerWorld) { world.teamLookup.Store(http.StatusServiceUnavailable) },
			wantText: "Try again in a moment",
		},
		{
			name:       "no current teams",
			fail:       func(world *linkedPlannerWorld) { world.noTeams.Store(true) },
			wantText:   "No current teams were found for the selected linked players.",
			wantImport: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newLinkedPlannerWorld(t)
			world.importLinkedPlayers(t)
			tc.fail(world)

			resp := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}})

			if resp.Code != http.StatusOK || strings.Contains(resp.Header().Get("HX-Trigger"), "soccer-workflow-reset") {
				t.Fatalf("status %d, HX-Trigger %q; the import is still usable", resp.Code, resp.Header().Get("HX-Trigger"))
			}
			if !world.browser.holdsCookie(config.LPSSessionCookieName, "/soccer") || !world.browser.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
				t.Fatal("the browser lost an import LPS did not reject")
			}
			doc := parsePlannerHTML(t, resp.Body.String())
			if text := plannerText(doc); !strings.Contains(text, tc.wantText) {
				t.Errorf("recovery = %q, want %q", text, tc.wantText)
			}
			if len(plannerElements(doc, plannerAttrIs("name", "team_ids"))) != 0 {
				t.Error("recovery offered teams to fetch")
			}
			if manual := plannerElements(doc, plannerAttrIs("href", "#team_codes")); len(manual) != 1 {
				t.Error("recovery lacks the manual Team ID path")
			}
			imports := plannerElements(doc, func(node *html.Node) bool { return plannerHasAttr(node, "data-open-login-modal") })
			if offered := len(imports) != 0; offered != tc.wantImport {
				t.Errorf("fresh import offered = %t, want %t", offered, tc.wantImport)
			}
		})
	}
}

func TestPlannerReturnDuringAnLPSOutageKeepsTheImport(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)
	if saved := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1002"}}); saved.Code != http.StatusOK {
		t.Fatalf("player choice status = %d", saved.Code)
	}
	world.teamLookup.Store(http.StatusServiceUnavailable)

	page := world.browser.get("/soccer")

	if cookie := findSessionCookie(t, page.Result()); cookie != nil && cookie.MaxAge < 0 {
		t.Fatal("an LPS outage cleared the import")
	}
	doc := parsePlannerHTML(t, page.Body.String())
	card, notice := linkedAccessNotice(t, doc)
	if soccerHTMLAttribute(card, "data-connection-state") != "connected" || !strings.Contains(notice, "Try again in a moment") {
		t.Errorf("LPS card = %q with notice %q, want the kept import and a retry explanation", soccerHTMLAttribute(card, "data-connection-state"), notice)
	}
	if got := linkedFormValues(t, doc, "soccer-player-select-form")["player_ids"]; !slices.Equal(got, []string{"1002"}) {
		t.Errorf("saved player choice = %v, want Taylor still chosen for a retry", got)
	}
	if len(plannerElements(doc, plannerAttrIs("name", "team_ids"))) != 0 {
		t.Error("the page offered teams LPS could not confirm")
	}
}

func TestPlannerReturnDuringAnLPSOutageKeepsTheSavedSchedule(t *testing.T) {
	world := newLinkedPlannerWorld(t)
	world.importLinkedPlayers(t)
	teams := parsePlannerHTML(t, world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1002"}}).Body.String())
	if fetched := world.browser.postForm("/soccer/fetch", linkedFormValues(t, teams, "soccer-team-select-form")); fetched.Code != http.StatusOK {
		t.Fatalf("South FC fetch status = %d", fetched.Code)
	}
	// LPS stops serving linked teams; the public team schedules still load.
	world.teamLookup.Store(http.StatusServiceUnavailable)

	page := world.browser.get("/soccer")

	if cookie := findSessionCookie(t, page.Result()); cookie != nil && cookie.MaxAge < 0 {
		t.Fatal("an LPS outage cleared the import")
	}
	doc := parsePlannerHTML(t, page.Body.String())
	card, notice := linkedAccessNotice(t, doc)
	if soccerHTMLAttribute(card, "data-connection-state") != "connected" || !strings.Contains(notice, "Try again in a moment") {
		t.Errorf("LPS card = %q with notice %q, want the kept import and a retry explanation", soccerHTMLAttribute(card, "data-connection-state"), notice)
	}
	if got := plannerRowIDs(plannerGameRows(doc, "upcoming-games")); !slices.Equal(got, []string{"3030", "2020"}) {
		t.Errorf("restored South FC rows = %v, want the saved schedule [3030 2020]", got)
	}
}

// teamSelectionNotice returns the text of the notices the team selection
// form shows above its teams.
func teamSelectionNotice(t *testing.T, doc *html.Node) string {
	t.Helper()
	form := plannerSingle(t, doc, "team selection form", plannerAttrIs("id", "soccer-team-select-form"))
	var notice string
	for _, feedback := range plannerElements(form, func(node *html.Node) bool { return soccerHTMLClassContains(node, "ui-feedback") }) {
		notice += plannerText(feedback)
	}
	return notice
}

func TestDiscoveryKeepsTheTeamsOfPlayersLPSStillServes(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			world := newLinkedPlannerWorld(t)
			world.importLinkedPlayers(t)
			world.taylorLookup.Store(int32(status))

			resp := world.browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001", "1002"}})

			if resp.Code != http.StatusOK || strings.Contains(resp.Header().Get("HX-Trigger"), "soccer-workflow-reset") {
				t.Fatalf("status %d, HX-Trigger %q; the import is still usable", resp.Code, resp.Header().Get("HX-Trigger"))
			}
			if !world.browser.holdsCookie(config.LPSSessionCookieName, "/soccer") || !world.browser.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
				t.Fatal("the browser lost an import LPS did not reject")
			}
			doc := parsePlannerHTML(t, resp.Body.String())
			if got := linkedOptionLabels(doc, "team_ids"); !slices.Equal(got, []string{"North FC Season 77"}) {
				t.Fatalf("teams offered = %q, want Craig's North FC", got)
			}
			if notice := teamSelectionNotice(t, doc); !strings.Contains(notice, "Taylor Johnson") {
				t.Errorf("team selection notice = %q, want it to name Taylor Johnson", notice)
			}

			// Both players stay chosen, so the page return asks LPS about
			// Taylor again and must still restore Craig's teams and schedule.
			if fetched := world.browser.postForm("/soccer/fetch", linkedFormValues(t, doc, "soccer-team-select-form")); fetched.Code != http.StatusOK {
				t.Fatalf("North FC fetch status = %d", fetched.Code)
			}
			page := parsePlannerHTML(t, world.browser.get("/soccer").Body.String())
			if got := linkedOptionLabels(page, "team_ids"); !slices.Equal(got, []string{"North FC Season 77"}) {
				t.Errorf("restored teams = %q, want Craig's North FC", got)
			}
			if notice := teamSelectionNotice(t, page); !strings.Contains(notice, "Taylor Johnson") {
				t.Errorf("restored team selection notice = %q, want it to name Taylor Johnson", notice)
			}
			if got := plannerRowIDs(plannerGameRows(page, "upcoming-games")); !slices.Equal(got, []string{"2020", "1010"}) {
				t.Errorf("restored North FC rows = %v, want [2020 1010]", got)
			}
		})
	}
}
