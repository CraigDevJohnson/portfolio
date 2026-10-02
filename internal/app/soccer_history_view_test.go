package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/cmd/web/partials"
)

// historyViewPath is the Team history fragment for one player, opening the
// player's newest proven team season.
func historyViewPath(playerID int) string {
	return fmt.Sprintf("/soccer/history/view?player_id=%d", playerID)
}

// historyViewSeasonPath is the Team history fragment opening one team season.
func historyViewSeasonPath(playerID, teamID, seasonID int) string {
	return fmt.Sprintf("/soccer/history/view?player_id=%d&team_id=%d&season_id=%d", playerID, teamID, seasonID)
}

// readHistoryView requires an authorized Team history fragment and parses it.
func readHistoryView(t *testing.T, browser *siteBrowser, path string) *html.Node {
	t.Helper()
	response := browser.get(path)
	if response.Code != http.StatusOK {
		t.Fatalf("%s: status %d, body %q", path, response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("%s Cache-Control = %q, want private, no-store", path, got)
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("%s Content-Type = %q, want HTML", path, got)
	}
	return parsePlannerHTML(t, response.Body.String())
}

func hasHistoryAttr(name string) func(*html.Node) bool {
	return func(node *html.Node) bool { return plannerHasAttr(node, name) }
}

func hasHistoryClass(class string) func(*html.Node) bool {
	return func(node *html.Node) bool {
		return strings.Contains(" "+soccerHTMLAttribute(node, "class")+" ", " "+class+" ")
	}
}

// historyViewSeasons lists the view's team seasons as "team/season
// current|former", in the order shown.
func historyViewSeasons(doc *html.Node) string {
	buttons := plannerElements(doc, hasHistoryAttr("data-soccer-team-history-season"))
	entries := make([]string, 0, len(buttons))
	for _, button := range buttons {
		when := "former"
		if soccerHTMLAttribute(button, "data-current") == "true" {
			when = "current"
		}
		entries = append(entries, soccerHTMLAttribute(button, "data-soccer-team-history-season")+" "+when)
	}
	return strings.Join(entries, ", ")
}

// historyViewOpen names the open team season, or "" when none is open, and
// requires its season button to be the one marked current.
func historyViewOpen(t *testing.T, doc *html.Node) string {
	t.Helper()
	details := plannerElements(doc, hasHistoryAttr("data-soccer-team-history-detail"))
	marked := plannerElements(doc, plannerAttrIs("aria-current", "true"))
	if len(details) == 0 {
		if len(marked) != 0 {
			t.Errorf("no season is open, but %d season buttons are marked current", len(marked))
		}
		return ""
	}
	open := soccerHTMLAttribute(details[0], "data-soccer-team-history-detail")
	if len(details) != 1 || len(marked) != 1 || soccerHTMLAttribute(marked[0], "data-soccer-team-history-season") != open {
		t.Errorf("open season %s: %d details, %d buttons marked current", open, len(details), len(marked))
	}
	return open
}

// historyViewPlayers lists the player buttons, marking the pressed one.
func historyViewPlayers(doc *html.Node) string {
	buttons := plannerElements(doc, hasHistoryAttr("data-soccer-team-history-player"))
	entries := make([]string, 0, len(buttons))
	for _, button := range buttons {
		entry := soccerHTMLAttribute(button, "data-soccer-team-history-player")
		if soccerHTMLAttribute(button, "aria-pressed") == "true" {
			entry += " pressed"
		}
		entries = append(entries, entry)
	}
	return strings.Join(entries, ", ")
}

// historyViewNotices lists the titles of the view's notices and refusals.
func historyViewNotices(doc *html.Node) []string {
	titles := plannerElements(doc, hasHistoryClass("ui-feedback-title"))
	texts := make([]string, 0, len(titles))
	for _, title := range titles {
		texts = append(texts, plannerText(title))
	}
	return texts
}

// historyViewGames lists each game as "id opponent | score | result", in the
// order shown.
func historyViewGames(t *testing.T, doc *html.Node) []string {
	t.Helper()
	rows := plannerElements(doc, hasHistoryAttr("data-soccer-team-history-game"))
	games := make([]string, 0, len(rows))
	for _, row := range rows {
		cell := func(class string) string {
			return plannerText(plannerSingle(t, row, class, hasHistoryClass(class)))
		}
		games = append(games, fmt.Sprintf("%s %s | %s | %s", soccerHTMLAttribute(row, "data-soccer-team-history-game"),
			cell("soccer-team-history-opponent"), cell("soccer-team-history-score"), cell("soccer-team-history-result")))
	}
	return games
}

// craigFCSeason4103 is a different team with Craig FC's name playing on
// Craig FC's dates in the same LPS season, which a visitor can enter by
// Team ID.
const craigFCSeason4103 = `{"team":{"UTeamID":4103,"team_name":"Craig FC","division_name":"Open A","Season":77},"games":[
{"UGameID":7301,"Season":77,"UTeam1":4103,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"6 - 0"}]}`

func TestSoccerHistoryViewListsProvenSeasonsAndOpensTheNewest(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	route.setTeam(4102, oldFCSeason78)
	route.setTeam(4103, craigFCSeason4103)
	owner := route.signedIn(t)
	if lookup := owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4103"}}); !strings.Contains(lookup.Body.String(), "Team 4103 added to history collection.") {
		t.Fatalf("manual Team ID lookup did not archive team 4103: %q", lookup.Body.String())
	}
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101, 4102); !report.Complete {
		t.Fatalf("refresh: %+v", report)
	}
	// Craig has moved on: LPS now lists only Craig FC's next season.
	route.setPlayerTeams(1001, `[{"UTeamID":4101,"team_name":"Craig FC","division_name":"Open A","Season":80}]`)

	view := readHistoryView(t, owner, historyViewPath(1001))

	if got, want := historyViewSeasons(view), "4101/80 current, 4102/78 former, 4101/77 former"; got != want {
		t.Errorf("seasons = %q, want %q", got, want)
	}
	if got := historyViewOpen(t, view); got != "4101/80" {
		t.Errorf("open season = %q, want the newest, 4101/80", got)
	}
	if got := historyViewNotices(view); !slices.Equal(got, []string{"Not collected yet"}) {
		t.Errorf("notices = %q, want Not collected yet", got)
	}
	if got := historyViewPlayers(view); got != "1001 pressed, 1002" {
		t.Errorf("players = %q, want Craig pressed beside Taylor", got)
	}
}

func TestSoccerHistoryViewShowsTheScoredRecordAndCompletedGamesNewestFirst(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	if report := route.refreshTeams(t, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), 4101); !report.Complete {
		t.Fatalf("refresh of Craig FC: %+v", report)
	}

	view := readHistoryView(t, owner, historyViewSeasonPath(1001, 4101, 77))

	if got := historyViewOpen(t, view); got != "4101/77" {
		t.Fatalf("open season = %q, want the requested 4101/77", got)
	}
	record := plannerText(plannerSingle(t, view, "record", hasHistoryAttr("data-soccer-team-history-record")))
	for _, want := range []string{
		"2 W · 1 L · 1 D", "2 wins, 1 loss, 1 draw", "from 4 scored games",
		"Calculated from numeric game scores; not official standings. 5 completed games not counted.",
	} {
		if !strings.Contains(record, want) {
			t.Errorf("record %q lacks %q", record, want)
		}
	}
	if got, want := plannerText(plannerSingle(t, view, "collection status", hasHistoryAttr("data-soccer-team-history-collected"))),
		"Collected Mon 09/28/26 06:00 AM MDT · LPS returned 10 games for this season"; got != want {
		t.Errorf("collection status = %q, want %q", got, want)
	}
	if notices := historyViewNotices(view); len(notices) != 0 {
		t.Errorf("a freshly collected season shows notices %q", notices)
	}
	want := []string{
		"7009 Craig FC vs Rivals | 8 - 0 | Not counted",
		"7008 vs Team 5008 | Forfeit | Not counted",
		"7007 vs Team 5007 | Final | Not counted",
		"7006 at Team 5006 | No score | Not counted",
		"7005 vs Team 5005 | canceled | Not counted",
		"7004 vs Team 5004 | 2–2 | Draw",
		"7003 at Team 5003 | 0–4 | Loss",
		"7002 at Team 5002 | 2–1 | Win",
		"7001 vs Team 5001 | 3–1 | Win",
	}
	if got := historyViewGames(t, view); !slices.Equal(got, want) {
		t.Errorf("games =\n%q\nwant\n%q", got, want)
	}
}

func TestSoccerHistoryViewNamesEachCollectionState(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4101, craigFCSeason77)
	route.setTeam(4202, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[]}`)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	fetchedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if report := route.refreshTeams(t, fetchedAt, 4101, 4202); !report.Complete {
		t.Fatalf("first refresh: %+v", report)
	}
	notices := func(path string) []string {
		t.Helper()
		return historyViewNotices(readHistoryView(t, owner, path))
	}

	if got := notices(historyViewSeasonPath(1002, 4202, 79)); !slices.Equal(got, []string{"No games this season"}) {
		t.Errorf("Taylor FC's empty season notices = %q", got)
	}
	pending := readHistoryView(t, owner, historyViewSeasonPath(1001, 4102, 78))
	if got := historyViewNotices(pending); !slices.Equal(got, []string{"Not collected yet"}) || !strings.Contains(plannerText(pending), "The next refresh is due") {
		t.Errorf("Old FC awaiting collection: notices %q, text %q", got, plannerText(pending))
	}

	// A later refresh of Craig FC fails upstream, and Old FC's Team ID is
	// rejected as invalid.
	route.failTeam(4101, http.StatusServiceUnavailable)
	route.failTeam(4102, http.StatusNotFound)
	if report := route.refreshTeams(t, fetchedAt.Add(24*time.Hour), 4101, 4102); report.Complete {
		t.Fatalf("failed refresh reported complete: %+v", report)
	}
	stale := readHistoryView(t, owner, historyViewSeasonPath(1001, 4101, 77))
	if got := historyViewNotices(stale); !slices.Equal(got, []string{"Latest refresh failed"}) {
		t.Errorf("Craig FC after a failed refresh: notices %q", got)
	}
	if text := plannerText(stale); !strings.Contains(text, "The refresh at Tue 09/29/26 06:00 AM MDT failed: LPS answered HTTP 503. Showing the copy collected Mon 09/28/26 06:00 AM MDT.") {
		t.Errorf("Craig FC's failed refresh is not explained: %q", text)
	}
	if games := historyViewGames(t, stale); len(games) != 9 {
		t.Errorf("a failed refresh hid the kept games: %q", games)
	}
	if got := notices(historyViewSeasonPath(1001, 4102, 78)); !slices.Equal(got, []string{"Not collected yet", "LPS no longer accepts this team"}) {
		t.Errorf("Old FC after LPS rejected it: notices %q", got)
	}

	// Taylor FC's next response lists only games still to come.
	route.setTeam(4202, `{"team":{"UTeamID":4202,"team_name":"Taylor FC","Season":79},"games":[
{"UGameID":7601,"Season":79,"UTeam1":4202,"UTeam2":5001,"SchedGameDateTime":"2099-01-05T19:00:00Z","result":""},
{"UGameID":7602,"Season":79,"UTeam1":5002,"UTeam2":4202,"SchedGameDateTime":"2099-01-12T19:00:00Z","result":""}]}`)
	if report := route.refreshTeams(t, fetchedAt.Add(24*time.Hour), 4202); !report.Complete {
		t.Fatalf("refresh of Taylor FC: %+v", report)
	}
	if got := notices(historyViewSeasonPath(1002, 4202, 79)); !slices.Equal(got, []string{"No completed games yet"}) {
		t.Errorf("Taylor FC with only future games: notices %q", got)
	}

	// A linked player LPS listed on no team at import has no proven season.
	unlisted := newTeamHistoryRoute(t)
	unlisted.setPlayerTeams(1002, `[]`)
	visitor := unlisted.signedIn(t)
	unlisted.importLinkedPlayers(t, visitor)
	taylor := readHistoryView(t, visitor, historyViewPath(1002))
	if got := historyViewNotices(taylor); !slices.Equal(got, []string{"No team seasons yet"}) {
		t.Errorf("Taylor without proof: notices %q", got)
	}
	if seasons, open := historyViewSeasons(taylor), historyViewOpen(t, taylor); seasons != "" || open != "" {
		t.Errorf("Taylor without proof lists %q and opens %q", seasons, open)
	}
}

func TestSoccerHistoryViewRefusesAnUnprovenRequestedSeason(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.setTeam(4103, craigFCSeason4103)
	owner := route.signedIn(t)
	if lookup := owner.postForm("/soccer/fetch", url.Values{"team_codes": {"4103"}}); !strings.Contains(lookup.Body.String(), "Team 4103 added to history collection.") {
		t.Fatalf("manual Team ID lookup did not archive team 4103: %q", lookup.Body.String())
	}
	route.importLinkedPlayers(t, owner)

	for _, unproven := range []struct {
		name                       string
		playerID, teamID, seasonID int
		players                    string
	}{
		{"a season the player's team played without the player", 1001, 4101, 79, "1001 pressed, 1002"},
		{"a same-named team entered by Team ID", 1001, 4103, 77, "1001 pressed, 1002"},
		{"another linked player's former team", 1002, 4102, 78, "1001, 1002 pressed"},
	} {
		t.Run(unproven.name, func(t *testing.T) {
			refused := owner.get(historyViewSeasonPath(unproven.playerID, unproven.teamID, unproven.seasonID))
			body := refused.Body.String()
			if refused.Code != http.StatusForbidden || !strings.Contains(body, "Team-season membership is unverified") {
				t.Fatalf("status %d, body %q; want 403 unverified", refused.Code, body)
			}
			if refused.Header().Get("X-Portal-Fragment-Error") != "true" || refused.Header().Get("Cache-Control") != "private, no-store" {
				t.Errorf("refusal headers = %v", refused.Header())
			}
			// The visitor can still choose a player from the refusal.
			if got := historyViewPlayers(parsePlannerHTML(t, body)); got != unproven.players {
				t.Errorf("refusal players = %q, want %q", got, unproven.players)
			}
			if strings.Contains(body, "data-soccer-team-history-season") || strings.Contains(body, "data-soccer-team-history-game") {
				t.Errorf("the refusal shows team history: %q", body)
			}
		})
	}
}

func TestSoccerHistoryViewKeepsStoredSeasonsWhenLPSCannotConfirmCurrentTeams(t *testing.T) {
	for _, refusal := range []struct {
		name       string
		lpsStatus  int
		wantStatus int
		wantBody   string
	}{
		{"an unavailable LPS keeps the import", http.StatusServiceUnavailable, http.StatusBadGateway, "Current team membership could not be verified"},
		{"a denied player is not confirmed", http.StatusForbidden, http.StatusForbidden, "Player is not confirmed by this import"},
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

			listed := owner.get(historyViewPath(1001))
			if listed.Code != http.StatusOK {
				t.Fatalf("view after LPS answered %d: status %d, body %q", refusal.lpsStatus, listed.Code, listed.Body.String())
			}
			view := parsePlannerHTML(t, listed.Body.String())
			if got, want := historyViewSeasons(view), "4102/78 former, 4101/77 former"; got != want {
				t.Errorf("seasons = %q, want only the stored proof %q", got, want)
			}
			if got := historyViewNotices(view); !slices.Equal(got, []string{"Current teams not confirmed"}) {
				t.Errorf("notices = %q, want Current teams not confirmed", got)
			}
			if got := historyViewOpen(t, view); got != "4102/78" {
				t.Errorf("open season = %q, want the newest stored season 4102/78", got)
			}

			refused := owner.get(historyViewSeasonPath(1001, 4101, 80))
			if refused.Code != refusal.wantStatus || !strings.Contains(refused.Body.String(), refusal.wantBody) || refused.Header().Get("X-Portal-Fragment-Error") != "true" {
				t.Fatalf("season only LPS could prove: status %d, body %q; want %d with %q", refused.Code, refused.Body.String(), refusal.wantStatus, refusal.wantBody)
			}
			for _, response := range []*httptest.ResponseRecorder{listed, refused} {
				if rewritten := findSessionCookie(t, response.Result()); rewritten != nil {
					t.Errorf("the view rewrote the import after LPS answered %d: %#v", refusal.lpsStatus, rewritten)
				}
			}
		})
	}
}

// A page left open after its import ended asks the view for history; the
// answer resets the page's import as every Soccer route does.
func TestSoccerHistoryViewEndsAnImportThatIsGone(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)
	route.expireImportJWT(t, owner)

	ended := owner.get(historyViewPath(1001))

	body := ended.Body.String()
	if ended.Code != http.StatusUnauthorized || !strings.Contains(body, "Imported player access is no longer available in this browser.") {
		t.Fatalf("status %d, body %q; want 401 explaining the ended import", ended.Code, body)
	}
	for header, want := range map[string]string{
		"HX-Trigger": "soccer-workflow-reset", "HX-Reswap": "none", "X-Portal-Fragment-Error": "true", "Cache-Control": "private, no-store",
	} {
		if got := ended.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	card := plannerSingle(t, parsePlannerHTML(t, body), "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	if soccerHTMLAttribute(card, "hx-swap-oob") != "outerHTML" {
		t.Error("the ended import does not replace the LPS card out of band")
	}
	assertClearedSessionCookie(t, ended.Result())
	if guard := findImportGuardCookie(ended.Result()); guard == nil || guard.Value != "" || guard.MaxAge >= 0 {
		t.Errorf("import guard cookie = %#v, want it cleared", guard)
	}
	if strings.Contains(body, "data-soccer-team-history-season") {
		t.Errorf("the ended import shows team seasons: %q", body)
	}
}

// The production route assembly wires no durable archive until the #80
// activation review, so a granted owner with a valid import gets no history.
func TestSoccerHistoryViewIsUnavailableWithoutTheDurableArchive(t *testing.T) {
	route := newTeamHistoryRoute(t)
	route.handler.SetArchiveStore(nil)
	owner := route.signedIn(t)
	route.importLinkedPlayers(t, owner)

	unavailable := owner.get(historyViewPath(1001))
	if body := unavailable.Body.String(); unavailable.Code != http.StatusServiceUnavailable || !strings.Contains(body, "Team history is unavailable") ||
		strings.Contains(body, "data-soccer-team-history-season") || unavailable.Header().Get("X-Portal-Fragment-Error") != "true" {
		t.Errorf("view without the archive: status %d, body %q", unavailable.Code, body)
	}
	for _, query := range []string{"", "player_id=x", "player_id=1001&team_id=4101", "player_id=1001&season_id=77", "player_id=1001&team_id=0&season_id=77"} {
		if response := owner.get("/soccer/history/view?" + query); response.Code != http.StatusBadRequest {
			t.Errorf("view with %q: status %d, want 400", query, response.Code)
		}
	}
}

// historySection returns the Soccer page's Team history section.
func historySection(t *testing.T, doc *html.Node) *html.Node {
	t.Helper()
	return plannerSingle(t, doc, "Team history section", plannerAttrIs("id", "soccer-history"))
}

// requireHiddenHistorySection requires the section to be the hidden, empty
// placeholder.
func requireHiddenHistorySection(t *testing.T, section *html.Node, when string) {
	t.Helper()
	if !plannerHasAttr(section, "hidden") || section.FirstChild != nil {
		t.Errorf("%s: the Team history section is not the hidden, empty placeholder", when)
	}
}

func TestSoccerTeamHistorySectionAppearsOnlyForAGrantedImportWithHistory(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)
	requireHiddenHistorySection(t, historySection(t, parsePlannerHTML(t, owner.get("/soccer").Body.String())), "before the import")

	route.importLinkedPlayers(t, owner)
	page := parsePlannerHTML(t, owner.get("/soccer").Body.String())

	section := historySection(t, page)
	if plannerHasAttr(section, "hidden") || soccerHTMLAttribute(section, "aria-labelledby") != "soccer-team-history-title" {
		t.Fatal("the imported owner's Team history section is hidden or unlabeled")
	}
	if text := plannerText(section); !strings.Contains(text, "Team history") || !strings.Contains(text, "Choose a player to load their team seasons.") {
		t.Errorf("Team history section text = %q", text)
	}
	plannerSingle(t, section, "section heading", plannerAttrIs("id", "soccer-team-history-title"))
	plannerSingle(t, section, "Team history panel", plannerAttrIs("id", "soccer-team-history-panel"))
	plannerSingle(t, section, "Team history loading status", plannerAttrIs("id", "soccer-team-history-loading"))
	buttons := plannerElements(section, hasHistoryAttr("data-soccer-team-history-player"))
	gets := make([]string, 0, len(buttons))
	for _, button := range buttons {
		if soccerHTMLAttribute(button, "aria-pressed") != "false" {
			t.Errorf("player %s starts pressed", soccerHTMLAttribute(button, "data-soccer-team-history-player"))
		}
		gets = append(gets, soccerHTMLAttribute(button, "hx-get"))
	}
	if want := []string{historyViewPath(1001), historyViewPath(1002)}; !slices.Equal(gets, want) {
		t.Fatalf("player buttons load %q, want %q", gets, want)
	}
	// The button's own request opens that player's view.
	readHistoryView(t, owner, gets[0])

	card := plannerSingle(t, page, "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	if link := plannerSingle(t, card, "Team history link", plannerAttrIs("href", "#soccer-history")); plannerText(link) != "View team history" {
		t.Errorf("LPS card link reads %q", plannerText(link))
	}

	// Without the durable archive, as every environment runs until
	// activation, the same import shows no Team history.
	route.handler.SetArchiveStore(nil)
	off := parsePlannerHTML(t, owner.get("/soccer").Body.String())
	requireHiddenHistorySection(t, historySection(t, off), "without the archive")
	if links := plannerElements(off, plannerAttrIs("href", "#soccer-history")); len(links) != 0 {
		t.Error("the LPS card links to Team history without the archive")
	}
}

// The import reveals the section in place, and logout and an ended import
// hide it again, out of band beside the LPS card and planner stages.
func TestSoccerTeamHistorySectionFollowsTheImport(t *testing.T) {
	route := newTeamHistoryRoute(t)
	owner := route.signedIn(t)

	imported := owner.postForm("/soccer/import", url.Values{"jwt": {route.jwt}, partials.SoccerHistoryNoticeField: {partials.SoccerHistoryNoticeIndefinite}})
	revealed := historySection(t, parsePlannerHTML(t, imported.Body.String()))
	if soccerHTMLAttribute(revealed, "hx-swap-oob") != "outerHTML" || plannerHasAttr(revealed, "hidden") {
		t.Fatal("the import does not reveal the Team history section out of band")
	}
	if players := plannerElements(revealed, hasHistoryAttr("data-soccer-team-history-player")); len(players) != 2 {
		t.Errorf("the revealed section offers %d players, want 2", len(players))
	}

	logout := browserForm(siteOrigin, "/soccer/logout", nil)
	logout.Header.Set("HX-Request", "true")
	cleared := historySection(t, parsePlannerHTML(t, owner.do(logout).Body.String()))
	if soccerHTMLAttribute(cleared, "hx-swap-oob") != "outerHTML" {
		t.Error("Clear import does not replace the Team history section out of band")
	}
	requireHiddenHistorySection(t, cleared, "after Clear import")

	route.importLinkedPlayers(t, owner)
	route.expireImportJWT(t, owner)
	ended := historySection(t, parsePlannerHTML(t, owner.get(historyViewPath(1001)).Body.String()))
	if soccerHTMLAttribute(ended, "hx-swap-oob") != "outerHTML" {
		t.Error("an ended import does not replace the Team history section out of band")
	}
	requireHiddenHistorySection(t, ended, "after the import ended")
}
