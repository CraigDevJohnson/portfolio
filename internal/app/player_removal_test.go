package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"portfolio/internal/config"
)

// changeLPS changes what the fake LPS answers from the next request on.
func (route *playerHistoryRoute) changeLPS(change func()) {
	route.mu.Lock()
	defer route.mu.Unlock()
	change()
}

// playerRecords returns every stored record in one player's partition.
func (route *playerHistoryRoute) playerRecords(t *testing.T, playerID int) map[string]map[string]any {
	t.Helper()
	items, err := route.table.Items()
	if err != nil {
		t.Fatalf("decode stored items: %v", err)
	}
	records := make(map[string]map[string]any)
	for key, item := range items {
		if strings.HasPrefix(key, fmt.Sprintf("PLAYER#%d/", playerID)) {
			records[key] = item
		}
	}
	return records
}

// playerRecordKinds counts one player's stored records by kind and owner.
func (route *playerHistoryRoute) playerRecordKinds(t *testing.T, playerID int) map[string]int {
	t.Helper()
	kinds := make(map[string]int)
	for _, record := range route.playerRecords(t, playerID) {
		kind := fmt.Sprint(record["kind"])
		if subject, owned := record["owner_subject"]; owned {
			kind += "/" + fmt.Sprint(subject)
		}
		kinds[kind]++
	}
	return kinds
}

// importAsSecondOwner signs another invited site account in and imports the
// same LPS account through the disclosed dialog, so the shared player
// partitions hold links recorded through two owners.
func (route *playerHistoryRoute) importAsSecondOwner(t *testing.T) {
	t.Helper()
	route.cognito.subject, route.cognito.email = "second-subject", "second@example.com"
	route.app.Config.SiteInvitations["second@example.com"] = []string{"soccer"}
	second := route.signedInOwner(t)
	if imported := route.disclosedImport(t, second); imported.Code != http.StatusOK {
		t.Fatalf("second owner's import status = %d", imported.Code)
	}
	route.cognito.subject, route.cognito.email = "stable-subject", "owner@example.com"
}

// removalRequest opens the owner's Soccer page, finds the removal control
// for playerID, and returns the request its htmx form sends from this
// site's own page.
func (route *playerHistoryRoute) removalRequest(t *testing.T, owner *siteBrowser, playerID int) *http.Request {
	t.Helper()
	page := owner.get("/soccer")
	if page.Code != http.StatusOK {
		t.Fatalf("Soccer page status = %d", page.Code)
	}
	doc := parsePlannerHTML(t, page.Body.String())
	var form *html.Node
	for _, candidate := range plannerElements(doc, plannerAttrIs("hx-post", "/soccer/players/remove")) {
		if len(plannerElements(candidate, plannerAttrIs("value", strconv.Itoa(playerID)))) > 0 {
			if form != nil {
				t.Fatalf("the page offers two removal controls for player %d", playerID)
			}
			form = candidate
		}
	}
	if form == nil {
		t.Fatalf("the imported owner's page offers no removal control for player %d", playerID)
	}
	values := url.Values{}
	for _, input := range plannerElements(form, plannerAttrIs("type", "hidden")) {
		values.Add(soccerHTMLAttribute(input, "name"), soccerHTMLAttribute(input, "value"))
	}
	request := browserForm(siteOrigin, soccerHTMLAttribute(form, "hx-post"), values)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", strings.TrimPrefix(soccerHTMLAttribute(form, "hx-target"), "#"))
	return request
}

// removalFeedback returns the text of the outcome the removal response shows
// in the LPS connection card it replaces.
func removalFeedback(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	doc := parsePlannerHTML(t, response.Body.String())
	card := plannerSingle(t, doc, "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	feedback := plannerElements(card, func(node *html.Node) bool {
		return strings.Contains(" "+soccerHTMLAttribute(node, "class")+" ", " ui-feedback ")
	})
	if len(feedback) != 1 {
		t.Fatalf("removal response card has %d outcome messages, want 1: %q", len(feedback), response.Body.String())
	}
	return plannerText(feedback[0])
}

func TestVerifiedPlayerRemovalErasesThePlayerForEveryOwnerAndKeepsTeamHistory(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)
	if imported := route.disclosedImport(t, owner); imported.Code != http.StatusOK {
		t.Fatalf("owner's import status = %d", imported.Code)
	}
	route.importAsSecondOwner(t)
	// A Team ID lookup stores the shared team's game, which Craig's and
	// Taylor's team played against Taylor FC.
	if body := owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4101"}}).Body.String(); !strings.Contains(body, "Team 4101 added to history collection.") {
		t.Fatalf("Team ID lookup did not store team 4101: %q", body)
	}
	craigBefore := route.playerRecordKinds(t, 1001)
	wantCraig := map[string]int{"player": 1, "player_owner/stable-subject": 1, "player_owner/second-subject": 1, "membership/stable-subject": 2, "membership/second-subject": 2}
	if fmt.Sprint(craigBefore) != fmt.Sprint(wantCraig) {
		t.Fatalf("Craig's records before removal = %v, want %v", craigBefore, wantCraig)
	}
	taylorBefore := route.playerRecords(t, 1002)
	checksBefore := route.lpsRequests("/users/check")

	removed := owner.do(route.removalRequest(t, owner, 1001))

	if removed.Code != http.StatusOK {
		t.Fatalf("verified removal status = %d, body %q", removed.Code, removed.Body.String())
	}
	if route.lpsRequests("/users/check") != checksBefore+1 {
		t.Errorf("removal did not confirm the player with a fresh LPS lookup: %d checks, want %d", route.lpsRequests("/users/check"), checksBefore+1)
	}
	if left := route.playerRecords(t, 1001); len(left) != 0 {
		t.Errorf("Craig's identity, owner links, or memberships remain after removal: %v", left)
	}
	if taylorAfter := route.playerRecords(t, 1002); fmt.Sprint(taylorAfter) != fmt.Sprint(taylorBefore) {
		t.Errorf("removing Craig changed Taylor's records:\nbefore %v\nafter  %v", taylorBefore, taylorAfter)
	}
	for _, fact := range []string{"TEAM#4101/META", "TEAM#4101/COVERAGE", "TEAM#4102/META", "TEAM#4202/META", "GAME#7001/META"} {
		if route.table.Item(fact) == nil {
			t.Errorf("removal deleted the shared fact %s", fact)
		}
	}
	history, err := route.store.ReadTeamSeason(t.Context(), 4101, 77)
	if err != nil || len(history.Games) != 1 || history.Games[0].UGameID != 7001 || history.Games[0].Result != "2-1" {
		t.Errorf("team 4101 season 77 history after removal = %+v, %v; want game 7001 won 2-1", history, err)
	}

	outcome := removalFeedback(t, removed)
	for _, reported := range []string{"Player data removed", "Craig Johnson", "1001", "every site account", "Team and game history stays", "importing again collects the player again"} {
		if !strings.Contains(outcome, reported) {
			t.Errorf("removal outcome %q does not say %q", outcome, reported)
		}
	}
	for _, private := range []string{"second-subject", "second@example.com", "stable-subject", route.cognito.issuer} {
		if strings.Contains(removed.Body.String(), private) {
			t.Errorf("removal response exposes owner information %q", private)
		}
	}
	// The import that proved authority ends, so it cannot recollect Craig.
	assertClearedSessionCookie(t, removed.Result())
	if owner.holdsCookie(config.LPSSessionCookieName, "/soccer") || owner.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
		t.Error("the browser kept its import after removal")
	}
	if trigger := removed.Header().Get("HX-Trigger"); trigger != "soccer-logout" {
		t.Errorf("removal HX-Trigger = %q, want soccer-logout to reset the planner", trigger)
	}
	if page := owner.get("/soccer").Body.String(); strings.Contains(page, importedAccessShown) || strings.Contains(page, `hx-post="/soccer/players/remove"`) {
		t.Error("the Soccer page still shows the import that removal cleared")
	}
	if left := route.playerRecords(t, 1001); len(left) != 0 {
		t.Errorf("the Soccer page recollected Craig after removal: %v", left)
	}

	// A later deliberate, disclosed import collects Craig again, for the
	// importing owner only.
	if reimported := route.disclosedImport(t, owner); reimported.Code != http.StatusOK || findSessionCookie(t, reimported.Result()) == nil {
		t.Fatalf("later import status = %d", reimported.Code)
	}
	craigAfter := route.playerRecordKinds(t, 1001)
	wantRecollected := map[string]int{"player": 1, "player_owner/stable-subject": 1, "membership/stable-subject": 2}
	if fmt.Sprint(craigAfter) != fmt.Sprint(wantRecollected) {
		t.Errorf("Craig's records after a later import = %v, want %v", craigAfter, wantRecollected)
	}
}

func TestPlayerRemovalIsRefusedWithoutSameOwnerLPSProofOfThePlayer(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)
	withoutImport := route.removalFormFor(1001)
	if refused := owner.do(browserForm(siteOrigin, "/soccer/players/remove", withoutImport)); refused.Code != http.StatusForbidden {
		t.Errorf("removal without an import: status %d, want 403", refused.Code)
	}
	if imported := route.disclosedImport(t, owner); imported.Code != http.StatusOK {
		t.Fatalf("owner's import status = %d", imported.Code)
	}
	route.importAsSecondOwner(t)
	recordsBefore := route.table.Len()

	for _, refusal := range []struct {
		name   string
		send   func() *httptest.ResponseRecorder
		status int
		// lpsLookups is how many fresh LPS player lookups the request makes.
		lpsLookups int
	}{
		{
			name: "signed out",
			send: func() *httptest.ResponseRecorder {
				return newSiteBrowser(t, route.mux).do(route.removalFrom(siteOrigin, 1001))
			},
			status: http.StatusUnauthorized,
		},
		{
			name:   "another site's page",
			send:   func() *httptest.ResponseRecorder { return owner.do(route.removalFrom(anotherOrigin, 1001)) },
			status: http.StatusForbidden,
		},
		{
			name: "a different site owner holding this owner's import",
			send: func() *httptest.ResponseRecorder {
				route.cognito.subject, route.cognito.email = "third-subject", "third@example.com"
				route.app.Config.SiteInvitations["third@example.com"] = []string{"soccer"}
				defer func() { route.cognito.subject, route.cognito.email = "stable-subject", "owner@example.com" }()
				other := route.signedInOwner(t)
				for _, name := range []string{config.LPSSessionCookieName, config.LPSImportGuardCookieName} {
					value := owner.cookieValue(name, "/soccer")
					if value == "" {
						t.Fatalf("the owner holds no %s cookie to hand over", name)
					}
					other.jar.SetCookies(owner.origin.ResolveReference(&url.URL{Path: "/soccer"}), []*http.Cookie{{Name: name, Value: value, Path: "/soccer"}})
				}
				return other.do(route.removalFrom(siteOrigin, 1001))
			},
			status: http.StatusForbidden,
		},
		{
			name:       "a player ID the owner's LPS account does not link",
			send:       func() *httptest.ResponseRecorder { return owner.do(route.removalFrom(siteOrigin, 1003)) },
			status:     http.StatusForbidden,
			lpsLookups: 1,
		},
		{
			name: "a player name instead of an ID",
			send: func() *httptest.ResponseRecorder {
				return owner.do(browserForm(siteOrigin, "/soccer/players/remove", url.Values{"player_id": {"Craig Johnson"}}))
			},
			status: http.StatusBadRequest,
		},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			checks := route.lpsRequests("/users/check")
			refused := refusal.send()
			if refused.Code != refusal.status {
				t.Errorf("status = %d, want %d; body %q", refused.Code, refusal.status, refused.Body.String())
			}
			if lookups := route.lpsRequests("/users/check") - checks; lookups != refusal.lpsLookups {
				t.Errorf("LPS player lookups = %d, want %d", lookups, refusal.lpsLookups)
			}
			if route.table.Len() != recordsBefore || len(route.playerRecords(t, 1001)) == 0 {
				t.Errorf("refused removal changed retained records: %d stored, want %d", route.table.Len(), recordsBefore)
			}
			for _, private := range []string{"second-subject", "second@example.com"} {
				if strings.Contains(refused.Body.String(), private) {
					t.Errorf("refusal exposes owner information %q", private)
				}
			}
		})
	}

	t.Run("LPS no longer links the player", func(t *testing.T) {
		route.changeLPS(func() {
			route.account = `{"players":[{"UPlayerID":1002,"FirstName":"Craig","LastName":"Johnson"}],"user_players":[{"player_id":1002}]}`
		})
		defer route.changeLPS(func() { route.account = "" })
		refused := owner.do(route.removalRequest(t, owner, 1001))
		if refused.Code != http.StatusForbidden || !strings.Contains(removalFeedback(t, refused), "nothing was removed") {
			t.Errorf("removal of a player LPS no longer links: status %d, body %q", refused.Code, refused.Body.String())
		}
		if len(route.playerRecords(t, 1001)) == 0 {
			t.Error("a stale imported player list authorized removal")
		}
	})

	t.Run("the soccer grant is revoked", func(t *testing.T) {
		request := route.removalRequest(t, owner, 1001)
		checks := route.lpsRequests("/users/check")
		route.app.Config.SiteInvitations["owner@example.com"] = nil
		defer func() { route.app.Config.SiteInvitations["owner@example.com"] = []string{"soccer"} }()
		if refused := owner.do(request); refused.Code != http.StatusForbidden || route.lpsRequests("/users/check") != checks {
			t.Errorf("removal after the grant was revoked: status %d, LPS checks %d, want 403 and %d", refused.Code, route.lpsRequests("/users/check"), checks)
		}
		if len(route.playerRecords(t, 1001)) == 0 {
			t.Error("removal ran without the current soccer grant")
		}
	})

	t.Run("LPS rejects the imported JWT", func(t *testing.T) {
		request := route.removalRequest(t, owner, 1001)
		route.changeLPS(func() { route.rejectJWT = true })
		defer route.changeLPS(func() { route.rejectJWT = false })
		refused := owner.do(request)
		if refused.Code != http.StatusForbidden || !strings.Contains(removalFeedback(t, refused), "nothing was removed") {
			t.Errorf("removal with a rejected JWT: status %d, body %q", refused.Code, refused.Body.String())
		}
		if len(route.playerRecords(t, 1001)) == 0 {
			t.Error("a rejected LPS import authorized removal")
		}
		// LPS ended the import, so the browser no longer offers it.
		assertClearedSessionCookie(t, refused.Result())
	})
}

func TestPlayerRemovalKeepsTheImportForARetryWhenTheArchiveFails(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	owner := route.signedInOwner(t)
	if imported := route.disclosedImport(t, owner); imported.Code != http.StatusOK {
		t.Fatalf("owner's import status = %d", imported.Code)
	}
	route.importAsSecondOwner(t)
	request := route.removalRequest(t, owner, 1001)
	// The table fails partway through Craig's partition, after two deletes.
	deletes := 0
	route.table.FailDelete = func(string) error {
		if deletes++; deletes > 2 {
			return errors.New("table unavailable")
		}
		return nil
	}

	failed := owner.do(request)

	if failed.Code != http.StatusServiceUnavailable || !strings.Contains(removalFeedback(t, failed), "Try again") {
		t.Fatalf("failed removal: status %d, body %q", failed.Code, failed.Body.String())
	}
	if cookie := findSessionCookie(t, failed.Result()); cookie != nil && cookie.MaxAge < 0 {
		t.Error("a failed removal cleared the import the retry needs")
	}
	if len(route.playerRecords(t, 1001)) == 0 {
		t.Fatal("the failed delete removed Craig's records")
	}
	// Whatever the interrupted removal left is still consistent evidence:
	// no membership outlives its player identity and owner link.
	route.assertEvidenceOnlyForEnrolledTeams(t)

	route.table.FailDelete = nil
	retried := owner.do(route.removalRequest(t, owner, 1001))
	if retried.Code != http.StatusOK || len(route.playerRecords(t, 1001)) != 0 {
		t.Fatalf("retried removal: status %d, Craig's records %v", retried.Code, route.playerRecords(t, 1001))
	}
}

func TestSoccerPageOffersPlayerRemovalOnlyWithRetainedHistory(t *testing.T) {
	route := newPlayerHistoryRoute(t)
	route.handler.SetArchiveStore(nil)
	owner := route.signedInOwner(t)
	if imported := owner.postForm("/soccer/import", url.Values{"jwt": {route.jwt}}); imported.Code != http.StatusOK {
		t.Fatalf("import without durable collection status = %d", imported.Code)
	}
	if page := owner.get("/soccer").Body.String(); !strings.Contains(page, importedAccessShown) || strings.Contains(page, "/soccer/players/remove") {
		t.Error("a page without retained history offers player removal")
	}
	if refused := owner.do(route.removalFrom(siteOrigin, 1001)); refused.Code != http.StatusServiceUnavailable || route.lpsRequests("/users/check") != 1 {
		t.Errorf("removal without retained history: status %d, LPS checks %d", refused.Code, route.lpsRequests("/users/check"))
	}
}

// removalFormFor is the form a removal control sends for playerID.
func (route *playerHistoryRoute) removalFormFor(playerID int) url.Values {
	return url.Values{"player_id": {strconv.Itoa(playerID)}}
}

// removalFrom is the removal POST a page on origin sends for playerID.
func (route *playerHistoryRoute) removalFrom(origin string, playerID int) *http.Request {
	return browserForm(origin, "/soccer/players/remove", route.removalFormFor(playerID))
}
