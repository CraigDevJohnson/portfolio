package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/internal/testutil"
)

func TestFetchSchedulesRendersSelectedTeamColorsAndNeutralSharedResult(t *testing.T) {
	app := newTestApp(t)
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	future := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/teams/100":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":100,"team_name":"Blue FC","Color":"  BlUe  "},"games":[{"UGameID":700,"SchedGameDateTime":%q,"UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC"},"result":"2 - 1"}]}`, past)
		case "/teams/200":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":200,"team_name":"Gold FC","Color":" GoLd "},"games":[{"UGameID":700,"SchedGameDateTime":%q,"UTeam1":100,"UTeam2":200,"home_team":{"UTeamID":100,"team_name":"Blue FC"},"visitor_team":{"UTeamID":200,"team_name":"Gold FC"},"result":"2 - 1"}]}`, past)
		case "/teams/300":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":300,"team_name":"Fallback FC","Color":"url(javascript:alert(1))"},"games":[{"UGameID":701,"SchedGameDateTime":%q,"UTeam1":300,"UTeam2":400,"home_team":{"UTeamID":300,"team_name":"Fallback FC"},"visitor_team":{"UTeamID":400,"team_name":"Visitor One"}},{"UGameID":702,"SchedGameDateTime":%q,"UTeam1":300,"UTeam2":401,"home_team":{"UTeamID":300,"team_name":"Fallback FC"},"visitor_team":{"UTeamID":401,"team_name":"Visitor Two"}}]}`, future, future)
		case "/teams/500":
			_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":500,"team_name":"Blue FC","Color":"red"},"games":[{"UGameID":703,"SchedGameDateTime":%q,"UTeam1":600,"UTeam2":700,"home_team":{"UTeamID":600,"team_name":"Blue FC","Color":"green"},"visitor_team":{"UTeamID":700,"team_name":"Other FC","Color":"yellow"}}]}`, future)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	app.Config.LPSAPIBaseURL = server.URL

	fetch := func() map[string]*html.Node {
		t.Helper()
		form := url.Values{"selection_mode": {"teams"}, "team_ids": {"100", "200", "300", "500"}}
		req := httptest.NewRequest(http.MethodPost, "/soccer/fetch", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp := httptest.NewRecorder()
		newTestSoccerHandler(app).FetchSchedulesHandler(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("fetch status = %d, want 200: %s", resp.Code, resp.Body.String())
		}
		if strings.Contains(resp.Body.String(), "url(javascript:") || strings.Contains(resp.Body.String(), "BlUe") {
			t.Fatal("raw upstream color reached the rendered fragment")
		}
		doc, err := html.Parse(strings.NewReader(resp.Body.String()))
		if err != nil {
			t.Fatalf("parse rendered fragment: %v", err)
		}
		rows := make(map[string]*html.Node)
		var visit func(*html.Node)
		visit = func(node *html.Node) {
			if node.Type == html.ElementNode && node.Data == "li" && htmlClass(node, "soccer-match-row") {
				if id := selectedGameID(node); id != "" {
					rows[id] = node
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(doc)
		return rows
	}

	fallbackAcrossRefetch := ""
	for attempt := range 2 {
		rows := fetch()
		if len(rows) != 4 {
			t.Fatalf("refetch %d rendered %d rows, want one shared, two fallback, and one name-collision game", attempt, len(rows))
		}
		shared := rows["700"]
		if shared == nil || htmlAttr(shared, "data-shared-match") == "" {
			t.Fatal("shared game was not marked as a two-team match")
		}
		if home, away := htmlAttr(shared, "data-home-color"), htmlAttr(shared, "data-away-color"); home != "blue" || away != "gold" {
			t.Fatalf("shared game colors = %q/%q, want blue/gold", home, away)
		}
		if text := htmlText(shared); !strings.Contains(text, "Blue FC") || !strings.Contains(text, "Gold FC") || !strings.Contains(text, "Home 2 – Away 1") || strings.Contains(text, "Win (") || strings.Contains(text, "Loss (") {
			t.Fatalf("shared match lacks readable names or neutral score: %q", text)
		}
		first, second := rows["701"], rows["702"]
		if first == nil || second == nil {
			t.Fatal("fallback games were not rendered")
		}
		fallback := htmlAttr(first, "data-home-color")
		if fallback == "" || fallback != htmlAttr(second, "data-home-color") || fallback != htmlAttr(first, "data-away-color") {
			t.Fatalf("same team ID did not retain one fallback color: %q/%q", fallback, htmlAttr(second, "data-home-color"))
		}
		if fallbackAcrossRefetch != "" && fallback != fallbackAcrossRefetch {
			t.Fatalf("team 300 fallback changed on refetch: %q then %q", fallbackAcrossRefetch, fallback)
		}
		fallbackAcrossRefetch = fallback
		collision := rows["703"]
		if collision == nil || htmlAttr(collision, "data-home-color") != "green" || htmlAttr(collision, "data-away-color") != "yellow" || htmlAttr(collision, "data-shared-match") != "" {
			t.Fatalf("explicit team IDs were overridden by a matching name: %#v", collision)
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
