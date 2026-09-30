package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/internal/testutil"
)

// newFakeLPSTeams serves /teams/{id} payloads from the given path map. Each
// payload may contain {past} and {future} placeholders for schedule times.
func newFakeLPSTeams(t *testing.T, payloads map[string]string) *httptest.Server {
	t.Helper()
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, ok := payloads[r.URL.Path]
		if !ok {
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.NewReplacer("{past}", past, "{future}", future).Replace(payload)))
	}))
	t.Cleanup(server.Close)
	return server
}

// fetchSoccerMatchRows posts a team selection to the real /soccer/fetch route
// and returns every rendered match row keyed by its game checkbox value.
func fetchSoccerMatchRows(t *testing.T, mux http.Handler, teamIDs ...string) (rows map[string][]*html.Node, body string) {
	t.Helper()
	form := url.Values{"selection_mode": {"teams"}, "team_ids": teamIDs}
	req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("fetch status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body = resp.Body.String()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse rendered fragment: %v", err)
	}
	return soccerMatchRows(doc), body
}

func soccerMatchRows(doc *html.Node) map[string][]*html.Node {
	rows := make(map[string][]*html.Node)
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "li" && htmlClass(node, "soccer-match-row") {
			id := selectedGameID(node)
			rows[id] = append(rows[id], node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return rows
}

func onlySoccerRow(t *testing.T, rows map[string][]*html.Node, gameID string) *html.Node {
	t.Helper()
	if got := len(rows[gameID]); got != 1 {
		t.Fatalf("game %s rendered %d rows, want exactly one", gameID, got)
	}
	return rows[gameID][0]
}

func TestFetchSchedulesRendersSelectedTeamColorsAndNeutralSharedResult(t *testing.T) {
	app := newTestApp(t)
	server := newFakeLPSTeams(t, map[string]string{
		"/teams/100": `{"team":{"UTeamID":100,"team_name":"Blue FC","Color":"  BlUe  "},"games":[{"UGameID":700,"SchedGameDateTime":"{past}","UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC"},"result":"2 - 1"}]}`,
		"/teams/200": `{"team":{"UTeamID":200,"team_name":"Gold FC","Color":" GoLd "},"games":[{"UGameID":700,"SchedGameDateTime":"{past}","UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC"},"result":"2 - 1"}]}`,
		"/teams/300": `{"team":{"UTeamID":300,"team_name":"Fallback FC","Color":"url(javascript:alert(1))"},"games":[{"UGameID":701,"SchedGameDateTime":"{future}","UTeam1":300,"UTeam2":400,"home_team":{"UTeamID":300,"team_name":"Fallback FC"},"visitor_team":{"UTeamID":400,"team_name":"Visitor One"}},{"UGameID":702,"SchedGameDateTime":"{future}","UTeam1":401,"UTeam2":300,"home_team":{"UTeamID":401,"team_name":"Visitor Two"},"visitor_team":{"UTeamID":300,"team_name":"Fallback FC"}}]}`,
		"/teams/500": `{"team":{"UTeamID":500,"team_name":"Blue FC","Color":"red"},"games":[{"UGameID":703,"SchedGameDateTime":"{future}","UTeam1":600,"UTeam2":700,"home_team":{"UTeamID":600,"team_name":"Blue FC","Color":"green"},"visitor_team":{"UTeamID":700,"team_name":"Other FC","Color":"yellow"}}]}`,
		"/teams/900": `{"team":{"UTeamID":900,"team_name":"Plain FC"},"games":[{"UGameID":901,"SchedGameDateTime":"{future}","UTeam1":900,"UTeam2":910,"home_team":{"UTeamID":900,"team_name":"Plain FC"},"visitor_team":{"UTeamID":910,"team_name":"Visitor Three"}},{"UGameID":902,"SchedGameDateTime":"{future}","UTeam1":911,"UTeam2":900,"home_team":{"UTeamID":911,"team_name":"Visitor Four"},"visitor_team":{"UTeamID":900,"team_name":"Plain FC"}}]}`,
	})
	app.Config.LPSAPIBaseURL = server.URL
	mux, _ := buildMux(app, app.Logger, false)

	fallbackAcrossRefetch := map[string]string{}
	for attempt := range 2 {
		rows, body := fetchSoccerMatchRows(t, mux, "100", "200", "300", "500", "900")
		if strings.Contains(body, "url(javascript:") || strings.Contains(body, "BlUe") || strings.Contains(body, "GoLd") {
			t.Fatal("raw upstream color reached the rendered fragment")
		}
		if len(rows) != 6 {
			t.Fatalf("refetch %d rendered rows for %d games, want one shared, four fallback, and one name-collision game", attempt, len(rows))
		}
		for id, nodes := range rows {
			for _, row := range nodes {
				if style := htmlAttr(row, "style"); style != "" {
					t.Fatalf("game %s row carries inline style %q; colors must come from the closed stylesheet palette", id, style)
				}
			}
		}

		shared := onlySoccerRow(t, rows, "700")
		if htmlAttr(shared, "data-shared-match") == "" {
			t.Fatal("shared game was not marked as a two-team match")
		}
		if home, away := htmlAttr(shared, "data-home-color"), htmlAttr(shared, "data-away-color"); home != "blue" || away != "gold" {
			t.Fatalf("shared game colors = %q/%q, want blue/gold", home, away)
		}
		if text := htmlText(shared); !strings.Contains(text, "Blue FC") || !strings.Contains(text, "Gold FC") || !strings.Contains(text, "Home 2 – Away 1") || strings.Contains(text, "Win (") || strings.Contains(text, "Loss (") {
			t.Fatalf("shared match lacks readable names or neutral score: %q", text)
		}

		// Team 300's color is unusable and team 900 has none; each keeps one
		// fallback whether it plays at home or away, and across refetches.
		for team, games := range map[string][2]string{"300": {"701", "702"}, "900": {"901", "902"}} {
			homeGame, awayGame := onlySoccerRow(t, rows, games[0]), onlySoccerRow(t, rows, games[1])
			fallback := htmlAttr(homeGame, "data-home-color")
			for _, got := range []string{htmlAttr(homeGame, "data-away-color"), htmlAttr(awayGame, "data-home-color"), htmlAttr(awayGame, "data-away-color")} {
				if fallback == "" || got != fallback {
					t.Fatalf("team %s fallback varied across games: %q vs %q", team, fallback, got)
				}
			}
			if htmlAttr(homeGame, "data-shared-match") != "" || htmlAttr(awayGame, "data-shared-match") != "" {
				t.Fatalf("team %s single-team games were marked shared", team)
			}
			if previous := fallbackAcrossRefetch[team]; previous != "" && previous != fallback {
				t.Fatalf("team %s fallback changed on refetch: %q then %q", team, previous, fallback)
			}
			fallbackAcrossRefetch[team] = fallback
		}

		collision := onlySoccerRow(t, rows, "703")
		if htmlAttr(collision, "data-home-color") != "green" || htmlAttr(collision, "data-away-color") != "yellow" || htmlAttr(collision, "data-shared-match") != "" {
			t.Fatalf("explicit team IDs were overridden by a matching name: %v", collision.Attr)
		}
	}
}

func TestFetchSchedulesSharedMatchUsesEachSelectedTeamsOwnColor(t *testing.T) {
	app := newTestApp(t)
	// Team 100's schedule describes selected team 200 with a different nested
	// color. The shared row must still paint team 200 as its own rows do.
	server := newFakeLPSTeams(t, map[string]string{
		"/teams/100": `{"team":{"UTeamID":100,"team_name":"Blue FC","Color":"blue"},"games":[{"UGameID":710,"SchedGameDateTime":"{future}","UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC","Color":"red"}}]}`,
		"/teams/200": `{"team":{"UTeamID":200,"team_name":"Gold FC","Color":"gold"},"games":[{"UGameID":710,"SchedGameDateTime":"{future}","UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC"}},{"UGameID":711,"SchedGameDateTime":"{future}","UTeam1":200,"UTeam2":250,"home_team":{"UTeamID":200,"team_name":"Gold FC"},"visitor_team":{"UTeamID":250,"team_name":"Other FC"}}]}`,
	})
	app.Config.LPSAPIBaseURL = server.URL
	mux, _ := buildMux(app, app.Logger, false)

	rows, _ := fetchSoccerMatchRows(t, mux, "100", "200")
	if own := onlySoccerRow(t, rows, "711"); htmlAttr(own, "data-home-color") != "gold" {
		t.Fatalf("team 200's own row color = %q, want gold", htmlAttr(own, "data-home-color"))
	}
	shared := onlySoccerRow(t, rows, "710")
	if home, away := htmlAttr(shared, "data-home-color"), htmlAttr(shared, "data-away-color"); home != "blue" || away != "gold" {
		t.Fatalf("shared game colors = %q/%q, want each selected team's own blue/gold", home, away)
	}
}

func TestFetchSchedulesKeepsOneColorPerSelectedTeamAcrossRows(t *testing.T) {
	app := newTestApp(t)
	// Team 800 has no team-level color, and LPS names its color on only one of
	// its games. Team 850 has no color in its own schedule, while team 100's
	// schedule nests a color for it in their shared match.
	server := newFakeLPSTeams(t, map[string]string{
		"/teams/100": `{"team":{"UTeamID":100,"team_name":"Blue FC","Color":"blue"},"games":[{"UGameID":860,"SchedGameDateTime":"{future}","UTeam1":100,"UTeam2":850,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":850,"team_name":"Quiet FC","Color":"red"}}]}`,
		"/teams/800": `{"team":{"UTeamID":800,"team_name":"Plain United"},"games":[{"UGameID":801,"SchedGameDateTime":"{future}","UTeam1":800,"UTeam2":820,"home_team":{"UTeamID":800,"team_name":"Plain United","Color":" Green "},"visitor_team":{"UTeamID":820,"team_name":"Visitor Five"}},{"UGameID":802,"SchedGameDateTime":"{future}","UTeam1":821,"UTeam2":800,"home_team":{"UTeamID":821,"team_name":"Visitor Six"},"visitor_team":{"UTeamID":800,"team_name":"Plain United"}}]}`,
		"/teams/850": `{"team":{"UTeamID":850,"team_name":"Quiet FC"},"games":[{"UGameID":860,"SchedGameDateTime":"{future}","UTeam1":100,"UTeam2":850,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":850,"team_name":"Quiet FC"}},{"UGameID":861,"SchedGameDateTime":"{future}","UTeam1":850,"UTeam2":870,"home_team":{"UTeamID":850,"team_name":"Quiet FC"},"visitor_team":{"UTeamID":870,"team_name":"Visitor Seven"}}]}`,
	})
	app.Config.LPSAPIBaseURL = server.URL
	mux, _ := buildMux(app, app.Logger, false)

	rows, _ := fetchSoccerMatchRows(t, mux, "100", "800", "850")
	for _, game := range []string{"801", "802"} {
		row := onlySoccerRow(t, rows, game)
		if home, away := htmlAttr(row, "data-home-color"), htmlAttr(row, "data-away-color"); home != "green" || away != "green" {
			t.Errorf("team 800 game %s colors = %q/%q, want the green LPS names for that team on every row", game, home, away)
		}
	}
	own := htmlAttr(onlySoccerRow(t, rows, "861"), "data-home-color")
	shared := onlySoccerRow(t, rows, "860")
	if away := htmlAttr(shared, "data-away-color"); own == "" || away != own {
		t.Errorf("team 850 shared-match color = %q, want its own rows' fallback %q", away, own)
	}
	if home := htmlAttr(shared, "data-home-color"); home != "blue" {
		t.Errorf("team 100 shared-match color = %q, want blue", home)
	}
}

// The combined preview fixture is the browser-rendered check for team colors:
// it must show a recognized color, a fallback, and shared matches with labels.
func TestSoccerPreviewFixtureRendersTeamColorStates(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, app.Logger, true)
	req := httptest.NewRequest(http.MethodGet, "/__preview/soccer/combined", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200", resp.Code)
	}
	doc, err := html.Parse(strings.NewReader(resp.Body.String()))
	if err != nil {
		t.Fatalf("parse preview page: %v", err)
	}
	rows := soccerMatchRows(doc)

	for _, game := range []struct {
		id, home, away, text string
		shared               bool
	}{
		{id: "preview-upcoming-1", home: "green", away: "orange", shared: true, text: "Pond Mint United vs Campfire Rovers"},
		{id: "preview-past-2", home: "green", away: "orange", shared: true, text: "Home 4 – Away 2"},
		{id: "preview-past-1", home: "green", away: "green", text: "Candle Oat Wanderers vs Pond Mint United"},
	} {
		row := onlySoccerRow(t, rows, game.id)
		if home, away := htmlAttr(row, "data-home-color"), htmlAttr(row, "data-away-color"); home != game.home || away != game.away {
			t.Errorf("%s colors = %q/%q, want %q/%q", game.id, home, away, game.home, game.away)
		}
		if shared := htmlAttr(row, "data-shared-match") != ""; shared != game.shared {
			t.Errorf("%s shared = %t, want %t", game.id, shared, game.shared)
		}
		if text := htmlText(row); !strings.Contains(text, game.text) {
			t.Errorf("%s text = %q, want readable %q", game.id, text, game.text)
		}
	}
	if text := htmlText(onlySoccerRow(t, rows, "preview-past-2")); strings.Contains(text, "Win (") || strings.Contains(text, "Loss (") {
		t.Errorf("shared preview result takes one team's perspective: %q", text)
	}

	fallback := onlySoccerRow(t, rows, "preview-upcoming-2")
	if home, away := htmlAttr(fallback, "data-home-color"), htmlAttr(fallback, "data-away-color"); home == "" || home != away || htmlAttr(fallback, "data-shared-match") != "" {
		t.Errorf("colorless preview team colors = %q/%q shared=%q, want one fallback on both halves", home, away, htmlAttr(fallback, "data-shared-match"))
	}
}

func htmlAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			if attr.Val == "" {
				return "true"
			}
			return attr.Val
		}
	}
	return ""
}

func htmlClass(node *html.Node, class string) bool {
	for _, value := range strings.Fields(htmlAttr(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func selectedGameID(node *html.Node) string {
	if node.Type == html.ElementNode && node.Data == "input" && htmlAttr(node, "name") == "selected" {
		return htmlAttr(node, "value")
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if id := selectedGameID(child); id != "" {
			return id
		}
	}
	return ""
}

func htmlText(node *html.Node) string {
	var text strings.Builder
	var visit func(*html.Node)
	visit = func(child *html.Node) {
		if child.Type == html.TextNode {
			text.WriteString(child.Data)
			text.WriteByte(' ')
		}
		for next := child.FirstChild; next != nil; next = next.NextSibling {
			visit(next)
		}
	}
	visit(node)
	return strings.Join(strings.Fields(text.String()), " ")
}
