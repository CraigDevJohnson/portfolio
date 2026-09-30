package app

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/soccerarchive"
)

// testLookupBudget and testImportBudget stand in for the import's LPS lookup
// budget and its whole budget, so a test sees a deadline pass without
// waiting whole seconds.
const (
	testLookupBudget = 200 * time.Millisecond
	testImportBudget = 2 * testLookupBudget
)

// workPastADeadline bounds what a test import does after a deadline cuts its
// work short: in-memory archive writes and rendering the response. It is no
// longer than the lookup budget, so a lookup phase that ran twice its budget
// would fail the elapsed checks.
const workPastADeadline = testLookupBudget

// stalledAnswer is how long a stalled fake LPS or Google path waits before
// it answers anyway, as a slow service eventually would. It is longer than
// any bounded import, so only an unbounded one waits for it.
const stalledAnswer = 3 * time.Second

func TestDisclosedImportSkipsALinkedPlayerLPSHasNotListedByTheLookupDeadline(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	route.handler.HistoryImportLookupBudget = testLookupBudget
	route.stalledPath, route.stallFor = "/players/1002/my_teams", stalledAnswer
	owner := route.signedInOwner(t)
	form := route.disclosedImportForm(t, owner)
	route.logs.take(t)

	started := time.Now()
	imported := owner.postForm("/soccer/import", form)
	elapsed := time.Since(started)

	body := imported.Body.String()
	if imported.Code != http.StatusOK || !strings.Contains(body, "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("a linked player LPS did not list in time took the import away: status %d, body %q", imported.Code, body)
	}
	if elapsed >= testLookupBudget+workPastADeadline {
		t.Errorf("import took %s, want it bounded by the %s lookup budget", elapsed, testLookupBudget)
	}
	title, message := importWarning(t, body)
	if want := "Some player history was not collected"; title != want {
		t.Errorf("import warning title = %q, want %q", title, want)
	}
	if want := "Let's Play Soccer did not list teams for Taylor Johnson in time, so their history was not collected. " +
		"Import again later to collect it. Your import was saved."; message != want {
		t.Errorf("import warning = %q, want %q", message, want)
	}

	byOwner := route.memberships(t)
	craigOnly := map[membershipTriple]bool{{1001, 4101, 77}: true, {1001, 4102, 78}: true}
	if len(byOwner) != 1 || len(byOwner["stable-subject"]) != len(craigOnly) {
		t.Fatalf("stored memberships = %v, want Craig's two associations", byOwner)
	}
	for triple := range byOwner["stable-subject"] {
		if !craigOnly[triple] {
			t.Errorf("membership %+v was stored for a player LPS did not list in time", triple)
		}
	}
	teams := route.items(t, "team")
	if len(teams) != 2 || teams["TEAM#4101/META"] == nil || teams["TEAM#4102/META"] == nil {
		t.Errorf("enrolled teams = %v, want only Craig's 4101 and 4102", teams)
	}
	// Like a linked player LPS no longer finds, the skipped player keeps
	// its identity and owner link without memberships.
	if players, links := len(route.items(t, "player")), len(route.items(t, "player_owner")); players != 2 || links != 2 {
		t.Errorf("stored %d player identities and %d owner links, want both players", players, links)
	}
	route.assertEvidenceOnlyForEnrolledTeams(t)

	skipped := 0
	for _, record := range route.logs.take(t) {
		if record["msg"] == "soccer linked player team lookup missed the import deadline" {
			skipped++
			if record["level"] != "WARN" || record["player_id"] != float64(1002) {
				t.Errorf("skipped player record = %v, want a warning for player 1002", record)
			}
		}
	}
	if skipped != 1 {
		t.Errorf("logged %d skipped-player records, want one for player 1002", skipped)
	}
}

// The lookups share one deadline rather than each having its own budget: a
// first player that stalls past it leaves no time to look up the second, so
// LPS is never asked for that player's teams and both are skipped.
func TestDisclosedImportLooksUpNoFurtherPlayerOnceTheSharedDeadlinePasses(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	route.handler.HistoryImportLookupBudget = testLookupBudget
	route.stalledPath, route.stallFor = "/players/1001/my_teams", stalledAnswer
	owner := route.signedInOwner(t)
	form := route.disclosedImportForm(t, owner)
	route.logs.take(t)

	started := time.Now()
	imported := owner.postForm("/soccer/import", form)
	elapsed := time.Since(started)

	body := imported.Body.String()
	if imported.Code != http.StatusOK || !strings.Contains(body, "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("linked players LPS did not list in time took the import away: status %d, body %q", imported.Code, body)
	}
	if elapsed >= testLookupBudget+workPastADeadline {
		t.Errorf("import took %s, want it bounded by the %s lookup budget", elapsed, testLookupBudget)
	}
	if requests := route.lpsRequests("/players/1002/my_teams"); requests != 0 {
		t.Errorf("LPS was asked for Taylor's teams %d times after the shared deadline passed, want none", requests)
	}
	_, message := importWarning(t, body)
	if want := "Let's Play Soccer did not list teams for Craig Johnson and Taylor Johnson in time"; !strings.HasPrefix(message, want) {
		t.Errorf("import warning = %q, want it to name both players", message)
	}
	if byOwner := route.memberships(t); len(byOwner) != 0 {
		t.Errorf("stored memberships = %v, want none", byOwner)
	}
	if teams := route.items(t, "team"); len(teams) != 0 {
		t.Errorf("enrolled teams = %v, want none", teams)
	}
	if players, links := len(route.items(t, "player")), len(route.items(t, "player_owner")); players != 2 || links != 2 {
		t.Errorf("stored %d player identities and %d owner links, want both players", players, links)
	}
	late := map[float64]bool{}
	for _, record := range route.logs.take(t) {
		if record["msg"] == "soccer linked player team lookup missed the import deadline" {
			late[record["player_id"].(float64)] = true
		}
	}
	if len(late) != 2 || !late[1001] || !late[1002] {
		t.Errorf("logged late players %v, want 1001 and 1002", late)
	}
}

func TestDisclosedImportCountsTheAccountLookupAgainstTheLookupDeadline(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	route.handler.HistoryImportLookupBudget = testLookupBudget
	route.stalledPath, route.stallFor = "/users/check", stalledAnswer
	owner := route.signedInOwner(t)
	form := route.disclosedImportForm(t, owner)

	started := time.Now()
	imported := owner.postForm("/soccer/import", form)
	elapsed := time.Since(started)

	body := imported.Body.String()
	if imported.Code != http.StatusOK || !strings.Contains(body, "Could not reach Let&#39;s Play Soccer to look up your players. Try again in a moment.") || strings.Contains(body, "data-login-success") {
		t.Fatalf("import past the deadline for its account lookup: status %d, body %q", imported.Code, body)
	}
	if elapsed >= testLookupBudget+workPastADeadline {
		t.Errorf("import took %s, want it bounded by the %s lookup budget", elapsed, testLookupBudget)
	}
	if findSessionCookie(t, imported.Result()) != nil {
		t.Error("an import whose account LPS did not confirm in time was kept")
	}
	if route.lpsRequests("/players/1001/my_teams") != 0 || route.lpsRequests("/players/1002/my_teams") != 0 {
		t.Errorf("import looked up players' teams without their account: %v", route.requests)
	}
	if stored := route.table.Len(); stored != 0 {
		t.Errorf("import stored %d durable items without its account", stored)
	}
}

// One notice names both kinds of history an import saved without: a player
// LPS did not list in time and a team the reviewed capacity refused.
func TestDisclosedImportNamesALatePlayerAndACapacityRefusalTogether(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	// One slot: Craig's 4101 is admitted and his 4102 refused.
	route.limitHistory(t, soccerarchive.Limits{MaxEnrolledTeams: 1, MaxRequestsPerRun: 10, MinRequestInterval: time.Second})
	route.handler.HistoryImportLookupBudget = testLookupBudget
	route.stalledPath, route.stallFor = "/players/1002/my_teams", stalledAnswer
	owner := route.signedInOwner(t)

	imported := route.disclosedImport(t, owner)

	if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("import status %d, body %q", imported.Code, imported.Body.String())
	}
	title, message := importWarning(t, imported.Body.String())
	if want := "Some player history was not collected"; title != want {
		t.Errorf("import warning title = %q, want %q", title, want)
	}
	if want := "Let's Play Soccer did not list teams for Taylor Johnson in time, so their history was not collected. " +
		"Import again later to collect it. Team 4102 was not added to history collection because its reviewed capacity is full. " +
		"Your import was saved."; message != want {
		t.Errorf("import warning = %q, want %q", message, want)
	}
	if teams := route.items(t, "team"); len(teams) != 1 || teams["TEAM#4101/META"] == nil {
		t.Errorf("enrolled teams = %v, want only 4101", teams)
	}
	route.assertEvidenceOnlyForEnrolledTeams(t)
}

// An import that collects no history keeps only the LPS client's timeout, as
// before the lookup budget existed: an account lookup slower than the budget
// still completes it.
func TestImportThatCollectsNoHistoryIsNotBoundByTheLookupBudget(t *testing.T) {
	for _, variant := range []struct {
		name    string
		arrange func(route *playerHistoryRoute)
		form    func(route *playerHistoryRoute) url.Values
	}{
		{
			name:    "the visitor did not submit the disclosed import",
			arrange: func(*playerHistoryRoute) {},
			form:    func(route *playerHistoryRoute) url.Values { return url.Values{"jwt": {route.jwt}} },
		},
		{
			name:    "durable collection is not wired",
			arrange: func(route *playerHistoryRoute) { route.handler.SetArchiveStore(nil) },
			form: func(route *playerHistoryRoute) url.Values {
				return url.Values{"jwt": {route.jwt}, "history_notice": {"indefinite"}}
			},
		},
	} {
		t.Run(variant.name, func(t *testing.T) {
			route := newPlayerHistoryRoute(t)
			variant.arrange(route)
			route.handler.HistoryImportLookupBudget = testLookupBudget
			route.stalledPath, route.stallFor = "/users/check", 2*testLookupBudget
			owner := route.signedInOwner(t)

			imported := owner.postForm("/soccer/import", variant.form(route))

			if imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
				t.Fatalf("import without history collection was cut short by the lookup budget: status %d, body %q", imported.Code, imported.Body.String())
			}
			if route.lpsRequests("/players/1001/my_teams") != 0 || route.lpsRequests("/players/1002/my_teams") != 0 || route.table.Len() != 0 {
				t.Errorf("import without history collection looked up teams or stored history: %v, %d items", route.requests, route.table.Len())
			}
		})
	}
}

// Rendering an import's response checks the owner's Google connection. That
// check runs within what remains of the import's budget, so a slow Google
// cannot hold a saved import past it, and a check cut short keeps the
// connection.
func TestDisclosedImportChecksGoogleWithinTheImportBudget(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)
	google, _ := wireFakeGoogleAccount(t, route.app)
	completeGoogleConsent(t, owner)
	form := route.disclosedImportForm(t, owner)
	route.handler.HistoryImportLookupBudget = testLookupBudget
	route.handler.HistoryImportBudget = testImportBudget
	google.stallCalendarList(stalledAnswer)
	checksBefore := google.calendarListRequests()

	started := time.Now()
	imported := owner.postForm("/soccer/import", form)
	elapsed := time.Since(started)

	body := imported.Body.String()
	if imported.Code != http.StatusOK || !strings.Contains(body, "data-login-success") || findSessionCookie(t, imported.Result()) == nil {
		t.Fatalf("a slow Google check took the import away: status %d, body %q", imported.Code, body)
	}
	if google.calendarListRequests() == checksBefore {
		t.Fatal("the import never checked Google's calendars, so this test no longer exercises a slow check")
	}
	if elapsed >= testImportBudget+workPastADeadline {
		t.Errorf("import took %s, want it bounded by the %s import budget", elapsed, testImportBudget)
	}
	if byOwner := route.memberships(t); len(byOwner["stable-subject"]) != len(linkedPlayerMemberships) {
		t.Errorf("stored memberships = %v, want every linked player's history", byOwner)
	}

	google.stallCalendarList(0)
	page := owner.get("/soccer").Body.String()
	if !strings.Contains(page, calendarAccount) || strings.Contains(page, "Not connected") {
		t.Error("a Google check the import budget cut short removed the owner's connection")
	}
}

// importWarning returns the title and message of the one warning an import
// response shows on the LPS connection card.
func importWarning(t *testing.T, body string) (title, message string) {
	t.Helper()
	card := plannerSingle(t, parsePlannerHTML(t, body), "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	warning := plannerSingle(t, card, "import warning", plannerAttrIs("class", "ui-feedback ui-feedback-warning"))
	return plannerText(plannerSingle(t, warning, "warning title", plannerAttrIs("class", "ui-feedback-title"))),
		plannerText(plannerSingle(t, warning, "warning message", plannerAttrIs("class", "ui-feedback-message")))
}
