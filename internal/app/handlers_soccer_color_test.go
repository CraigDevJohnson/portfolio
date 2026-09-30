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
