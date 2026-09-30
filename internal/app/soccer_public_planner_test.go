package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/cmd/web/partials"
	"portfolio/internal/testutil"
	"portfolio/types"
)

// These tests drive the public choice-first ICS planner (#89) through the real
// route assembly from buildMux. A fake Let's Play Soccer API stands in for the
// upstream service; no site session exists, and no imported LPS credential is
// present unless a case adds one explicitly.

const (
	publicSoonGame = `{"UGameID":3030,"SchedGameDateTime":"2099-10-04T09:30:00.000Z","field_name":"Field 3",` +
		`"home_team":{"team_name":"Soon FC"},"visitor_team":{"team_name":"Guests"}}`
	publicSharedGame = `{"UGameID":2020,"SchedGameDateTime":"2099-10-11T10:30:00.000Z","field_name":"Field 1",` +
		`"home_team":{"team_name":"Shared FC"},"visitor_team":{"team_name":"Rivals"}}`
	publicLateGame = `{"UGameID":1010,"SchedGameDateTime":"2099-10-18T09:00:00.000Z","field_name":"Field 2",` +
		`"home_team":{"team_name":"Late FC"},"visitor_team":{"team_name":"Visitors"}}`
	publicNewGame = `{"UGameID":4040,"SchedGameDateTime":"2099-10-25T11:00:00.000Z","field_name":"Field 4",` +
		`"home_team":{"team_name":"Added FC"},"visitor_team":{"team_name":"Latecomers"}}`
	publicPastGame = `{"UGameID":900,"SchedGameDateTime":"2020-09-01T10:00:00.000Z","field_name":"Field 1","result":"2 - 1",` +
		`"home_team":{"team_name":"Past FC"},"visitor_team":{"team_name":"Old Rivals"}}`
)

func publicScheduleJSON(games ...string) string {
	return `{"games":[` + strings.Join(games, ",") + `]}`
}

// newPublicPlannerRoutes returns the production route assembly wired to a fake
// LPS API. The fake fails the test if the public path sends a credential.
func newPublicPlannerRoutes(t *testing.T, schedules func(path string) (int, string)) (http.Handler, *App) {
	t.Helper()
	app := newTestApp(t)
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("public Team ID lookup sent an Authorization header to %s", r.URL.Path)
		}
		status, body := schedules(r.URL.Path)
		if status == 0 {
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(lps.Close)
	app.Config.LPSAPIBaseURL = lps.URL
	mux, _ := buildMux(app, app.Logger, false)
	return mux, app
}

func servePublicPlanner(t *testing.T, routes http.Handler, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if form == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp := httptest.NewRecorder()
	routes.ServeHTTP(resp, req)
	return resp
}

func parsePlannerHTML(t *testing.T, body string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse planner HTML: %v", err)
	}
	return doc
}

func plannerElements(root *html.Node, match func(*html.Node) bool) []*html.Node {
	var found []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && match(node) {
			found = append(found, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func plannerHasAttr(node *html.Node, name string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return true
		}
	}
	return false
}

func plannerAttrIs(name, value string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return plannerHasAttr(node, name) && soccerHTMLAttribute(node, name) == value
	}
}

func plannerText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(builder.String()), " ")
}

type plannerGameRow struct {
	ID      string
	Checked bool
}

func plannerGameRows(root *html.Node, group string) []plannerGameRow {
	inputs := plannerElements(root, func(node *html.Node) bool {
		return node.Data == "input" && plannerHasAttr(node, "data-game-checkbox") && soccerHTMLAttribute(node, "data-game-group") == group
	})
	rows := make([]plannerGameRow, 0, len(inputs))
	for _, input := range inputs {
		rows = append(rows, plannerGameRow{ID: soccerHTMLAttribute(input, "value"), Checked: plannerHasAttr(input, "checked")})
	}
	return rows
}

func plannerRowIDs(rows []plannerGameRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func plannerSingle(t *testing.T, root *html.Node, description string, match func(*html.Node) bool) *html.Node {
	t.Helper()
	found := plannerElements(root, match)
	if len(found) != 1 {
		t.Fatalf("%s: found %d elements, want 1", description, len(found))
	}
	return found[0]
}

func icsEvents(ics string) map[string]string {
	events := make(map[string]string)
	for _, block := range strings.Split(ics, "BEGIN:VEVENT")[1:] {
		block = strings.SplitN(block, "END:VEVENT", 2)[0]
		for _, line := range strings.Split(block, "\r\n") {
			if uid, ok := strings.CutPrefix(line, "UID:"); ok {
				events[uid] = block
			}
		}
	}
	return events
}

func TestPublicPlannerRouteOffersOutputChoiceFirstWithoutSession(t *testing.T) {
	routes, _ := newPublicPlannerRoutes(t, func(string) (int, string) { return 0, "" })

	resp := servePublicPlanner(t, routes, http.MethodGet, "/soccer", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /soccer status = %d, want %d", resp.Code, http.StatusOK)
	}
	if cookies := resp.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("viewing the public planner set cookies %v; it must not require a session", cookies)
	}
	doc := parsePlannerHTML(t, resp.Body.String())

	stages := plannerElements(doc, func(node *html.Node) bool { return plannerHasAttr(node, "data-soccer-stage") })
	if len(stages) == 0 || soccerHTMLAttribute(stages[0], "data-soccer-stage") != "calendar-output" {
		t.Fatalf("the first Soccer planner stage must be the calendar output choice")
	}
	if plannerHasAttr(stages[0], "hidden") {
		t.Fatal("the calendar output choice must be visible before any other step")
	}

	options := plannerElements(doc, plannerAttrIs("name", "calendar_output"))
	values := make([]string, 0, len(options))
	for _, option := range options {
		values = append(values, soccerHTMLAttribute(option, "value"))
		if soccerHTMLAttribute(option, "type") != "radio" {
			t.Errorf("calendar output option %q is not a radio control", soccerHTMLAttribute(option, "value"))
		}
		if plannerHasAttr(option, "checked") {
			t.Errorf("calendar output option %q is preselected; the visitor makes the first choice", soccerHTMLAttribute(option, "value"))
		}
	}
	if !slices.Equal(values, []string{"ics", "google"}) {
		t.Fatalf("calendar output options = %v, want [ics google]", values)
	}

	for _, stage := range stages[1:] {
		if !plannerHasAttr(stage, "hidden") {
			t.Errorf("stage %q is visible before an output is chosen", soccerHTMLAttribute(stage, "data-soccer-stage"))
		}
	}
	connections := plannerSingle(t, doc, "Connections panel", plannerAttrIs("id", "soccer-connections"))
	if !plannerHasAttr(connections, "hidden") {
		t.Error("Connections panel is visible before an output is chosen")
	}

	fetchForm := plannerSingle(t, doc, "manual Team ID form", plannerAttrIs("id", "fetch-form"))
	if got := soccerHTMLAttribute(fetchForm, "hx-post"); got != "/soccer/fetch" {
		t.Fatalf("manual Team ID form posts to %q, want /soccer/fetch", got)
	}
	if len(plannerElements(fetchForm, plannerAttrIs("name", "team_codes"))) != 1 {
		t.Fatal("manual Team ID form lacks the team_codes field")
	}
	linkedSource := plannerSingle(t, doc, "linked-player source option", func(node *html.Node) bool {
		return plannerHasAttr(node, "data-soccer-linked-source") && soccerHTMLClassContains(node, "soccer-source-option")
	})
	if !plannerHasAttr(linkedSource, "hidden") {
		t.Error("linked-player source is visible on the public path")
	}
}

func TestPublicPlannerRouteFetchesOrdersSelectsAndDownloadsWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cookies func(t *testing.T, app *App) []*http.Cookie
	}{
		{name: "signed out", cookies: func(*testing.T, *App) []*http.Cookie { return nil }},
		{name: "with imported LPS access", cookies: func(t *testing.T, app *App) []*http.Cookie {
			req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
			addSessionCookie(t, app, req, &types.SessionData{
				JWT:       testutil.TestJWT(t, time.Now().Add(time.Hour)),
				Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Linked", LastName: "Player", IsMainPlayer: true}},
				ExpiresAt: time.Now().Add(time.Hour),
			})
			return req.Cookies()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			routes, app := newPublicPlannerRoutes(t, func(path string) (int, string) {
				switch path {
				case "/teams/101":
					return http.StatusOK, publicScheduleJSON(publicLateGame, publicSharedGame, publicPastGame)
				case "/teams/202":
					return http.StatusOK, publicScheduleJSON(publicSharedGame, publicSoonGame)
				default:
					return 0, ""
				}
			})
			cookies := tc.cookies(t, app)

			fetch := servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"202, 101"}}, cookies...)
			if fetch.Code != http.StatusOK {
				t.Fatalf("POST /soccer/fetch status = %d, want %d", fetch.Code, http.StatusOK)
			}
			doc := parsePlannerHTML(t, fetch.Body.String())

			rows := plannerGameRows(doc, "upcoming-games")
			if got, want := plannerRowIDs(rows), []string{"3030", "2020", "1010"}; !slices.Equal(got, want) {
				t.Fatalf("upcoming rows = %v, want one row per game soonest first %v", got, want)
			}
			for _, row := range rows {
				if !row.Checked {
					t.Errorf("upcoming game %s did not begin selected", row.ID)
				}
			}
			if past := plannerGameRows(doc, "past-results"); len(past) != 0 {
				t.Errorf("manual lookup rendered past rows %v on the ICS path", plannerRowIDs(past))
			}

			count := plannerSingle(t, doc, "upcoming selected count", func(node *html.Node) bool {
				return plannerHasAttr(node, "data-selected-count") && soccerHTMLAttribute(node, "data-game-group") == "upcoming-games"
			})
			if got := plannerText(count); got != "3 games selected" {
				t.Errorf("selected count = %q, want %q", got, "3 games selected")
			}
			selectAll := plannerSingle(t, doc, "upcoming select-all control", func(node *html.Node) bool {
				return node.Data == "input" && plannerHasAttr(node, "data-select-all") && soccerHTMLAttribute(node, "data-game-group") == "upcoming-games"
			})
			if soccerHTMLAttribute(selectAll, "type") != "checkbox" || !plannerHasAttr(selectAll, "checked") {
				t.Error("select-all control is not a checked checkbox when every game is selected")
			}
			if got := soccerHTMLAttribute(selectAll, "aria-label"); got != "Select all upcoming games" {
				t.Errorf("select-all accessible name = %q", got)
			}
			if label := selectAll.Parent; label == nil || label.Data != "label" || !strings.Contains(plannerText(label), "Select all upcoming games") {
				t.Error("select-all control lacks a visible label")
			}

			scope := plannerSingle(t, doc, "team-set scope", func(node *html.Node) bool { return plannerHasAttr(node, "data-team-fingerprint") })
			if got := soccerHTMLAttribute(scope, "data-team-fingerprint"); got != "101-202" {
				t.Errorf("team-set fingerprint = %q, want 101-202", got)
			}

			download := plannerSingle(t, doc, "ICS download button", plannerAttrIs("id", "download-button"))
			if plannerHasAttr(download, "hidden") || soccerHTMLAttribute(download, "data-soccer-output-only") != "ics" {
				t.Error("ICS download action is not the visible ICS-path action")
			}
			for _, googleOnly := range plannerElements(doc, plannerAttrIs("data-soccer-output-only", "google")) {
				if !plannerHasAttr(googleOnly, "hidden") {
					t.Errorf("Google-only control %q is visible on the ICS path", plannerText(googleOnly))
				}
			}

			// The visitor deselects the shared game and downloads the rest.
			selected := servePublicPlanner(t, routes, http.MethodPost, "/soccer/download", url.Values{
				"team_codes": {"202, 101"},
				"selected":   {"3030", "1010"},
			}, cookies...)
			if selected.Code != http.StatusOK {
				t.Fatalf("POST /soccer/download status = %d body %q", selected.Code, selected.Body.String())
			}
			if got := selected.Header().Get("Content-Type"); got != "text/calendar" {
				t.Fatalf("download Content-Type = %q", got)
			}
			if got := selected.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
				t.Fatalf("download Content-Disposition = %q", got)
			}
			ics := testutil.UnfoldICS(selected.Body.String())
			events := icsEvents(ics)
			if len(events) != 2 || strings.Count(ics, "BEGIN:VEVENT") != 2 {
				t.Fatalf("ICS events = %d, want only the two selected games: %q", strings.Count(ics, "BEGIN:VEVENT"), ics)
			}
			for uid, want := range map[string][]string{
				"3030": {"DTSTART;TZID=America/Denver:20991004T093000", "SUMMARY:Soon FC vs Guests - Field 3"},
				"1010": {"DTSTART;TZID=America/Denver:20991018T090000", "SUMMARY:Late FC vs Visitors - Field 2"},
			} {
				event, ok := events[uid]
				if !ok {
					t.Fatalf("ICS lacks selected game %s: %q", uid, ics)
				}
				for _, line := range want {
					if !strings.Contains(event, line+"\r\n") {
						t.Errorf("ICS event %s lacks %q: %q", uid, line, event)
					}
				}
			}

			none := servePublicPlanner(t, routes, http.MethodPost, "/soccer/download", url.Values{"team_codes": {"202, 101"}}, cookies...)
			if none.Code != http.StatusBadRequest {
				t.Fatalf("download with every game deselected status = %d, want %d", none.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestPublicPlannerRouteRefetchKeepsTeamSetScopeAndSelectsNewGames(t *testing.T) {
	var teamRequests atomic.Int32
	routes, _ := newPublicPlannerRoutes(t, func(path string) (int, string) {
		switch path {
		case "/teams/101":
			if teamRequests.Add(1) == 1 {
				return http.StatusOK, publicScheduleJSON(publicSharedGame)
			}
			return http.StatusOK, publicScheduleJSON(publicSharedGame, publicNewGame)
		case "/teams/202":
			return http.StatusOK, publicScheduleJSON(publicSoonGame)
		default:
			return 0, ""
		}
	})

	first := parsePlannerHTML(t, servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"101,202"}}).Body.String())
	refetch := parsePlannerHTML(t, servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"202 101;101"}}).Body.String())

	scopeOf := func(doc *html.Node) string {
		return soccerHTMLAttribute(plannerSingle(t, doc, "team-set scope", func(node *html.Node) bool {
			return plannerHasAttr(node, "data-team-fingerprint")
		}), "data-team-fingerprint")
	}
	if got, again := scopeOf(first), scopeOf(refetch); got != "101-202" || again != got {
		t.Fatalf("team-set scope changed across refetch of the same teams: first %q, refetch %q", got, again)
	}
	if got := plannerRowIDs(plannerGameRows(first, "upcoming-games")); !slices.Equal(got, []string{"3030", "2020"}) {
		t.Fatalf("first fetch rows = %v", got)
	}
	rows := plannerGameRows(refetch, "upcoming-games")
	if got := plannerRowIDs(rows); !slices.Equal(got, []string{"3030", "2020", "4040"}) {
		t.Fatalf("refetch rows = %v, want the newly discovered game after the known ones", got)
	}
	if !rows[2].Checked {
		t.Fatal("newly discovered upcoming game did not begin selected")
	}
}

func TestPublicPlannerRouteExplainsEmptyAndFailedLookups(t *testing.T) {
	routes, _ := newPublicPlannerRoutes(t, func(path string) (int, string) {
		switch path {
		case "/teams/101":
			return http.StatusOK, publicScheduleJSON(publicPastGame)
		case "/teams/404":
			return http.StatusNotFound, `{"message":"not found"}`
		case "/teams/503":
			return http.StatusServiceUnavailable, `{"message":"unavailable"}`
		default:
			return 0, ""
		}
	})

	for _, tc := range []struct {
		name, teamID  string
		wantHeading   string
		wantText      []string
		forbiddenText []string
	}{
		{
			name: "no upcoming games", teamID: "101",
			wantHeading: "No upcoming games found",
			wantText:    []string{"There are no upcoming games for the selected teams."},
		},
		{
			name: "unknown team", teamID: "404",
			wantHeading: "Could not fetch games",
			wantText:    []string{"Team ID 404 was not accepted by Let's Play Soccer.", "Check your Team IDs"},
		},
		{
			name: "LPS unavailable", teamID: "503",
			wantHeading:   "Could not fetch games",
			wantText:      []string{"Could not load schedules from Let's Play Soccer right now.", "Try again in a moment"},
			forbiddenText: []string{"use team IDs manually", "Check your Team IDs"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {tc.teamID}})
			if resp.Code != http.StatusOK {
				t.Fatalf("POST /soccer/fetch status = %d", resp.Code)
			}
			doc := parsePlannerHTML(t, resp.Body.String())
			if rows := plannerGameRows(doc, "upcoming-games"); len(rows) != 0 {
				t.Fatalf("state %q rendered game rows %v", tc.name, plannerRowIDs(rows))
			}
			heading := plannerSingle(t, doc, "result state heading", func(node *html.Node) bool { return node.Data == "h4" })
			if got := plannerText(heading); got != tc.wantHeading {
				t.Fatalf("heading = %q, want %q", got, tc.wantHeading)
			}
			text := plannerText(doc)
			for _, want := range tc.wantText {
				if !strings.Contains(text, want) {
					t.Errorf("state lacks %q: %q", want, text)
				}
			}
			for _, forbidden := range tc.forbiddenText {
				if strings.Contains(text, forbidden) {
					t.Errorf("state gives misleading advice %q: %q", forbidden, text)
				}
			}
		})
	}
}

func TestPublicPlannerRouteRestoresPastOnlyScheduleAsEmptyICSOutput(t *testing.T) {
	routes, app := newPublicPlannerRoutes(t, func(path string) (int, string) {
		if path == "/teams/101" {
			return http.StatusOK, publicScheduleJSON(publicPastGame)
		}
		return 0, ""
	})
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		Workflow: types.SoccerWorkflowState{Source: "manual", SelectedTeamIDs: []int{101}},
	})

	resp := servePublicPlanner(t, routes, http.MethodGet, "/soccer", nil, req.Cookies()...)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /soccer status = %d", resp.Code)
	}
	doc := parsePlannerHTML(t, resp.Body.String())
	review := plannerSingle(t, doc, "review stage", plannerAttrIs("data-soccer-stage", "review"))
	if got := soccerHTMLAttribute(review, "data-soccer-results-ready"); got != "ready" {
		t.Fatalf("restored review stage readiness = %q, want ready", got)
	}
	icsPanels := plannerElements(doc, func(node *html.Node) bool {
		return soccerHTMLAttribute(node, "data-soccer-output-only") == "ics" && soccerHTMLClassContains(node, "games-empty-panel")
	})
	if len(icsPanels) != 1 || plannerHasAttr(icsPanels[0], "hidden") {
		t.Fatalf("restored past-only schedule lacks a visible ICS empty panel")
	}
	if got := plannerText(icsPanels[0]); !strings.Contains(got, "No upcoming games to download") {
		t.Fatalf("ICS empty panel = %q", got)
	}
	if rows := plannerGameRows(doc, "upcoming-games"); len(rows) != 0 {
		t.Fatalf("past-only schedule rendered downloadable rows %v", plannerRowIDs(rows))
	}

	// Past results and their Sync action belong to Google mode only.
	pastForm := plannerSingle(t, doc, "past results form", plannerAttrIs("id", "past-results-form"))
	if gate := plannerOutputGate(pastForm); gate == nil || soccerHTMLAttribute(gate, "data-soccer-output-only") != "google" || !plannerHasAttr(gate, "hidden") {
		t.Error("past results are not inside a hidden Google-only section on the ICS path")
	}
	assertPastResultControlsGoogleOnly(t, doc)
	for _, googleOnly := range plannerElements(doc, plannerAttrIs("data-soccer-output-only", "google")) {
		if !plannerHasAttr(googleOnly, "hidden") {
			t.Errorf("Google-only element %q is visible on the ICS path", plannerText(googleOnly))
		}
	}
}

func TestPublicPlannerKeepsResultSyncInGoogleModeOnly(t *testing.T) {
	// A connected Google account renders the Sync action; it must still sit
	// behind the Google-only gate so the ICS path never offers result sync.
	props := soccerPresentationTestTableProps()
	doc := parsePlannerHTML(t, renderSoccerTestComponent(t, partials.SoccerTableFragment(props)))
	syncActions := plannerElements(doc, func(node *html.Node) bool {
		return plannerHasAttr(node, "data-game-action") && soccerHTMLAttribute(node, "data-game-group") == "past-results"
	})
	if len(syncActions) == 0 {
		t.Fatal("connected schedule lacks a result Sync action")
	}
	assertPastResultControlsGoogleOnly(t, doc)
}

// assertPastResultControlsGoogleOnly requires every past-result control,
// including Sync, to sit inside a hidden Google-only section.
func assertPastResultControlsGoogleOnly(t *testing.T, doc *html.Node) {
	t.Helper()
	controls := plannerElements(doc, func(node *html.Node) bool {
		return soccerHTMLAttribute(node, "data-game-group") == "past-results"
	})
	if len(controls) == 0 {
		t.Fatal("schedule lacks past-result controls")
	}
	for _, control := range controls {
		if gate := plannerOutputGate(control); gate == nil || soccerHTMLAttribute(gate, "data-soccer-output-only") != "google" || !plannerHasAttr(gate, "hidden") {
			t.Errorf("past-result control %q is reachable on the ICS path", plannerText(control))
		}
	}
}

// plannerOutputGate returns the nearest ancestor-or-self that limits node to
// one calendar output.
func plannerOutputGate(node *html.Node) *html.Node {
	for current := node; current != nil; current = current.Parent {
		if current.Type == html.ElementNode && plannerHasAttr(current, "data-soccer-output-only") {
			return current
		}
	}
	return nil
}

// importedLPSCookies returns a valid imported LPS session cookie so a case can
// prove the public Team ID path leaves that private credential untouched.
func importedLPSCookies(t *testing.T, app *App) []*http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	addSessionCookie(t, app, req, &types.SessionData{
		JWT:       testutil.TestJWT(t, time.Now().Add(time.Hour)),
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Linked", LastName: "Player", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(time.Hour),
	})
	return req.Cookies()
}

func assertImportedAccessKept(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == "lps_session" && (cookie.MaxAge < 0 || cookie.Value == "") {
			t.Errorf("public Team ID request cleared the imported LPS session: %v", cookie)
		}
	}
	if trigger := resp.Header().Get("HX-Trigger"); strings.Contains(trigger, "soccer-workflow-reset") {
		t.Errorf("public Team ID request reset the workflow with HX-Trigger %q", trigger)
	}
}

func TestPublicPlannerRouteExplainsRefusedTeamLookupWithoutTouchingImportedAccess(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		for _, imported := range []bool{false, true} {
			name := fmt.Sprintf("LPS %d imported=%t", status, imported)
			t.Run(name, func(t *testing.T) {
				routes, app := newPublicPlannerRoutes(t, func(path string) (int, string) {
					if path == "/teams/479691" {
						return status, `{"message":"refused"}`
					}
					return 0, ""
				})
				var cookies []*http.Cookie
				if imported {
					cookies = importedLPSCookies(t, app)
				}

				fetch := servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"479691"}}, cookies...)
				if fetch.Code != http.StatusOK {
					t.Fatalf("POST /soccer/fetch status = %d", fetch.Code)
				}
				assertImportedAccessKept(t, fetch)
				doc := parsePlannerHTML(t, fetch.Body.String())
				heading := plannerSingle(t, doc, "result state heading", func(node *html.Node) bool { return node.Data == "h4" })
				if got := plannerText(heading); got != "Could not fetch games" {
					t.Fatalf("heading = %q, want Could not fetch games", got)
				}
				text := plannerText(doc)
				for _, want := range []string{
					"Let's Play Soccer would not share the schedule for team ID 479691.",
					"Check that the team appears on the Let's Play Soccer Team Schedules page",
				} {
					if !strings.Contains(text, want) {
						t.Errorf("refused lookup lacks %q: %q", want, text)
					}
				}
				for _, forbidden := range []string{"token", "discovered player", "imported players"} {
					if strings.Contains(text, forbidden) {
						t.Errorf("refused public lookup blames private access with %q: %q", forbidden, text)
					}
				}

				download := servePublicPlanner(t, routes, http.MethodPost, "/soccer/download", url.Values{
					"team_codes": {"479691"},
					"selected":   {"1"},
				}, cookies...)
				if download.Code != http.StatusBadGateway {
					t.Fatalf("POST /soccer/download status = %d, want %d", download.Code, http.StatusBadGateway)
				}
				assertImportedAccessKept(t, download)
				if body := download.Body.String(); strings.Contains(body, "token") || !strings.Contains(body, "team ID 479691") {
					t.Errorf("refused download message = %q", body)
				}
			})
		}
	}
}

func TestPublicPlannerRouteRejectedTeamDownloadKeepsImportedAccess(t *testing.T) {
	routes, app := newPublicPlannerRoutes(t, func(path string) (int, string) {
		if path == "/teams/101" {
			return http.StatusNotFound, `{"message":"not found"}`
		}
		return 0, ""
	})

	resp := servePublicPlanner(t, routes, http.MethodPost, "/soccer/download", url.Values{
		"team_codes": {"101"},
		"selected":   {"1"},
	}, importedLPSCookies(t, app)...)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("POST /soccer/download status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
	if body := resp.Body.String(); !strings.Contains(body, "team ID 101 was not accepted") {
		t.Errorf("rejected team download message = %q", body)
	}
	assertImportedAccessKept(t, resp)
}
