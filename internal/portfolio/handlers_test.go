package portfolio

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func TestSkillsHandlerUsesURLFiltersAndHTMXResponseMode(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/skills?category=Cloud+Platforms&proficiency=advanced", nil)
	req.Header.Set("HX-Request", "true")
	res := httptest.NewRecorder()

	SkillsHandler(res, req)

	body := res.Body.String()
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatal("HTMX request returned a full document")
	}
	for _, marker := range []string{`id="skills-filterable"`, `id="skills-filter-controls"`, `hx-swap-oob="outerHTML"`, `5 skills shown`} {
		if !strings.Contains(body, marker) {
			t.Errorf("HTMX response missing %q", marker)
		}
	}
	if strings.Count(body, `data-signal-trail`) != 1 {
		t.Errorf("HTMX Skills fragment trail count = %d, want 1", strings.Count(body, `data-signal-trail`))
	}
	roots := parseFragmentElementRoots(t, body)
	if len(roots) != 2 || nodeID(roots[0]) != "skills-filterable" || nodeID(roots[1]) != "skills-filter-controls" {
		t.Fatalf("HTMX fragment roots = %v, want exactly results then OOB controls", nodeIDs(roots))
	}
	if countDescendantsByAttribute(roots[0], "data-signal-trail") != 1 || findDescendantByAttribute(roots[1], "data-signal-trail") != nil {
		t.Error("HTMX fragment must keep one trail inside results and none inside OOB controls")
	}
	for _, root := range roots {
		if hasAttribute(root, "hx-history-elt") || findDescendantByAttribute(root, "data-skills-showcase") != nil {
			t.Errorf("fragment root %q contains full-page history/showcase structure", nodeID(root))
		}
	}
	if got := res.Header().Values("Vary"); !headerValuesContainAll(got, "HX-Request", "HX-History-Restore-Request") {
		t.Errorf("Vary = %v, want both HTMX response-mode headers", got)
	}
	if got := res.Header().Get("HX-Push-Url"); got != "/skills?category=Cloud+Platforms&proficiency=advanced" {
		t.Errorf("HX-Push-Url = %q, want normalized visible state", got)
	}
}

func TestSkillsHandlerNormalGETRendersStableProgressiveWorkbench(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/skills?q=terraform", nil)
	res := httptest.NewRecorder()

	SkillsHandler(res, req)

	body := res.Body.String()
	for _, marker := range []string{
		"<!doctype html>",
		`data-layout="skills-workbench"`,
		`class="skills-workbench-history"`,
		`hx-history-elt`,
		`id="skills-filter-controls"`,
		`id="skills-filterable"`,
		`1 skill shown`,
		`value="terraform"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("normal Skills page missing %q", marker)
		}
	}
	if strings.Count(body, `data-signal-trail`) != 1 {
		t.Errorf("normal Skills page trail count = %d, want 1", strings.Count(body, `data-signal-trail`))
	}
	doc := parseHTMLDocument(t, body)
	routeRoot := findDescendant(doc, func(node *html.Node) bool {
		return attributeValue(node, "data-layout") == "skills-workbench" && nodeHasClass(node, "page-kit-page")
	})
	if routeRoot == nil {
		t.Fatal("data-layout is not on the true page-kit-page route root")
	}
	history := findDescendantByClass(doc, "skills-workbench-history")
	if history == nil || !hasAttribute(history, "hx-history-elt") {
		t.Fatal("full page lacks a true hx-history-elt workbench ancestor")
	}
	controls := findDescendantByID(history, "skills-filter-controls")
	results := findDescendantByID(history, "skills-filterable")
	if controls == nil || results == nil {
		t.Fatalf("history ancestor descendants controls=%t results=%t", controls != nil, results != nil)
	}
	if countDescendantsByID(history, "skills-filter-controls") != 1 || countDescendantsByID(history, "skills-filterable") != 1 || countDescendantsByAttribute(history, "data-signal-trail") != 1 {
		t.Fatal("history ancestor does not uniquely contain controls, trail, and results")
	}
	if !nodesAppearInDocumentOrder(history, controls, results) {
		t.Fatal("history ancestor does not contain controls then results in the required order")
	}
	trail := findDescendantByAttribute(results, "data-signal-trail")
	resultHeading := findDescendantByClass(results, "skills-results-heading")
	if trail == nil || resultHeading == nil || !nodesAppearInDocumentOrder(results, trail, resultHeading) {
		t.Fatal("filterable results do not contain the trail before their heading and catalog")
	}
	resultSummary := findDescendantByClass(history, "skills-result-summary")
	if resultSummary == nil || attributeValue(resultSummary, "role") != "status" || attributeValue(resultSummary, "aria-live") != "polite" {
		t.Fatal("workbench lacks its one live result count")
	}
	if countDescendants(history, func(node *html.Node) bool {
		return nodeHasClass(node, "skills-result-summary") && attributeValue(node, "role") == "status" && attributeValue(node, "aria-live") != ""
	}) != 1 {
		t.Fatal("workbench does not have exactly one live result count")
	}
	controlSummary := findDescendantByClass(history, "skills-control-summary")
	if controlSummary == nil || attributeValue(controlSummary, "aria-hidden") != "true" || attributeValue(controlSummary, "aria-live") != "" {
		t.Fatal("OOB control summary must be non-live and hidden from assistive technology")
	}
	for _, panelClass := range []string{"skills-featured-panel", "skills-practices-panel"} {
		panel := findDescendantByClass(history, panelClass)
		if panel == nil {
			t.Errorf("full workbench lacks %s", panelClass)
			continue
		}
		labelID := attributeValue(panel, "aria-labelledby")
		heading := findDescendantByID(panel, labelID)
		if labelID == "" || heading == nil || heading.Data != "h2" {
			t.Errorf("%s aria-labelledby=%q does not resolve to a real h2", panelClass, labelID)
		}
	}
	assertFocusableSkillsControlsHaveStableIDs(t, history)
}

func TestSkillsHandlerHistoryCacheMissRendersFullDocument(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/skills?category=Cloud+Platforms", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-History-Restore-Request", "true")
	res := httptest.NewRecorder()

	SkillsHandler(res, req)

	if body := res.Body.String(); !strings.Contains(body, "<!doctype html>") || strings.Count(body, `data-signal-trail`) != 1 {
		t.Fatalf("history cache miss did not return one-trail full document")
	}
}

func TestSkillsFilteredHandlerUsesExactTwoRootProgressiveFragmentContract(t *testing.T) {
	res := httptest.NewRecorder()
	SkillsFilteredHandler(res, httptest.NewRequest(http.MethodGet, "/skills/filtered?q=terraform", nil))
	roots := parseFragmentElementRoots(t, res.Body.String())
	if len(roots) != 2 || nodeID(roots[0]) != "skills-filterable" || nodeID(roots[1]) != "skills-filter-controls" {
		t.Fatalf("compatibility fragment roots = %v, want results then controls", nodeIDs(roots))
	}
	if attributeValue(roots[1], "hx-swap-oob") != "outerHTML" {
		t.Errorf("controls OOB swap = %q, want outerHTML", attributeValue(roots[1], "hx-swap-oob"))
	}
	if countDescendantsByAttribute(roots[0], "data-signal-trail") != 1 || findDescendantByAttribute(roots[1], "data-signal-trail") != nil {
		t.Error("compatibility fragment must keep one trail inside results and none inside controls")
	}
	for _, root := range roots {
		if hasAttribute(root, "hx-history-elt") || findDescendantByAttribute(root, "data-skills-showcase") != nil {
			t.Error("compatibility fragment includes history/showcase structure")
		}
	}
}

func TestSkillsFragmentPushesNormalizedCanonicalURL(t *testing.T) {
	longQuery := "  cloud & " + strings.Repeat("界", 90) + "  "
	request := httptest.NewRequest(http.MethodGet, "/skills?q="+url.QueryEscape(longQuery)+"&category=Imaginary&proficiency=master", nil)
	request.Header.Set("HX-Request", "true")
	res := httptest.NewRecorder()
	SkillsHandler(res, request)

	got := res.Header().Get("HX-Push-Url")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse HX-Push-Url %q: %v", got, err)
	}
	if parsed.Path != "/skills" || parsed.Query().Get("category") != "" || parsed.Query().Get("proficiency") != "" {
		t.Fatalf("normalized HX-Push-Url = %q, want invalid axes removed", got)
	}
	if utf8.RuneCountInString(parsed.Query().Get("q")) != 80 || parsed.Query().Get("q") != strings.TrimSpace(parsed.Query().Get("q")) {
		t.Fatalf("normalized query in HX-Push-Url = %q, want trimmed 80 runes", parsed.Query().Get("q"))
	}
}

func TestSkillsRenderedControlsAreProgressiveAndDetailIdentityIsStable(t *testing.T) {
	res := httptest.NewRecorder()
	SkillsHandler(res, httptest.NewRequest(http.MethodGet, "/skills?category=Cloud+Platforms&proficiency=advanced&q=cloud", nil))
	body := res.Body.String()
	for _, marker := range []string{
		`<form class="skills-search-form" method="get" action="/skills"`,
		`id="skills-search" type="search" name="q" value="cloud"`,
		`hx-get="/skills" hx-trigger="input changed delay:300ms" hx-include="closest form"`,
		`<input type="hidden" name="category" value="Cloud Platforms">`,
		`<input type="hidden" name="proficiency" value="advanced">`,
		`id="skills-search-submit" type="submit" class="page-kit-action ui-action-secondary skills-search-submit"`,
		`id="skills-category-cloud-platforms" href="/skills?q=cloud&amp;category=Cloud+Platforms&amp;proficiency=advanced"`,
		`aria-current="page"`,
		`hx-target="#skills-filterable" hx-swap="outerHTML" hx-push-url="true" hx-sync="#skills-filterable:replace"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("progressive Skills controls missing %q", marker)
		}
	}
	if strings.Contains(body, `class="ui-action `) {
		t.Error("Skills controls use dead ui-action compatibility utility instead of the canonical page-kit-action primitive")
	}

	detail := httptest.NewRecorder()
	SkillsDetailHandler(detail, httptest.NewRequest(http.MethodGet, "/skills/detail?id=2", nil))
	for _, marker := range []string{
		`id="skill-detail-heading-2"`, `data-skill-detail-heading`, `tabindex="-1"`,
		`id="skill-detail-close-2"`, `class="page-kit-icon-button skill-detail-close"`, `data-close-detail`,
	} {
		if !strings.Contains(detail.Body.String(), marker) {
			t.Errorf("skill detail missing stable focus contract %q", marker)
		}
	}
	detailRoots := parseFragmentElementRoots(t, detail.Body.String())
	heading := findDescendantByID(detailRoots[0], "skill-detail-heading-2")
	closeButton := findDescendantByID(detailRoots[0], "skill-detail-close-2")
	if heading == nil || closeButton == nil || !nodesAppearInDocumentOrder(detailRoots[0], heading, closeButton) {
		t.Fatal("detail Close does not follow its programmatically focusable heading")
	}

	empty := httptest.NewRecorder()
	SkillsHandler(empty, httptest.NewRequest(http.MethodGet, "/skills?proficiency=familiar", nil))
	emptyDoc := parseHTMLDocument(t, empty.Body.String())
	clearLink := findDescendantByID(emptyDoc, "skills-clear-filters")
	if clearLink == nil || !hasAttribute(clearLink, "data-skills-clear") {
		t.Fatal("empty state lacks stable-ID Clear control")
	}
	assertFocusableSkillsControlsHaveStableIDs(t, findDescendantByClass(emptyDoc, "skills-workbench-history"))
}

func TestSkillsRealDatasetPreservesApprovedInventoryAndFeaturedSlots(t *testing.T) {
	props := buildSkillsPageProps(SkillsData(), nil).Grid
	if len(props.Categories) != 14 || props.TotalCatalogCount != 74 || len(props.FeaturedSkills) != 16 || len(props.PracticeSkills) != 11 {
		t.Fatalf("Skills inventory = categories:%d catalog:%d featured:%d practices:%d, want 14/74/16/11", len(props.Categories), props.TotalCatalogCount, len(props.FeaturedSkills), len(props.PracticeSkills))
	}
	slots := make(map[string]int)
	for _, item := range props.FeaturedSkills {
		slots[item.Slot]++
	}
	for _, required := range []string{"anchor", "bridge", "signal"} {
		if slots[required] != 1 {
			t.Errorf("featured slot %q count = %d, want 1 real item", required, slots[required])
		}
	}
}

func TestSkillsHandlersNormalizeInvalidAndSearchCaseInsensitively(t *testing.T) {
	tests := []struct {
		name     string
		handler  http.HandlerFunc
		path     string
		markers  []string
		fragment bool
	}{
		{
			name:    "invalid full page axes become inactive",
			handler: SkillsHandler,
			path:    "/skills?category=Imaginary&proficiency=master",
			markers: []string{`data-active-category=""`, `data-active-proficiency=""`, `74 skills shown`},
		},
		{
			name:     "compatibility fragment shares query normalization",
			handler:  SkillsFilteredHandler,
			path:     "/skills/filtered?q=TeRrAfOrM",
			markers:  []string{`id="skills-filterable"`, `id="skills-filter-controls"`, `1 skill shown`, `Terraform`},
			fragment: true,
		},
		{
			name:     "familiar is valid even when empty",
			handler:  SkillsFilteredHandler,
			path:     "/skills/filtered?proficiency=familiar",
			markers:  []string{`data-active-proficiency="familiar"`, `0 skills shown`, `No skills match`},
			fragment: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := httptest.NewRecorder()
			test.handler(res, httptest.NewRequest(http.MethodGet, test.path, nil))
			body := res.Body.String()
			for _, marker := range test.markers {
				if !strings.Contains(body, marker) {
					t.Errorf("response missing %q", marker)
				}
			}
			if strings.Contains(body, "<!DOCTYPE html>") && test.fragment {
				t.Error("compatibility endpoint returned a full document")
			}
			if strings.Count(body, `data-signal-trail`) != 1 && test.fragment {
				t.Errorf("compatibility fragment trail count = %d, want 1", strings.Count(body, `data-signal-trail`))
			}
		})
	}
}

func headerValuesContainAll(values []string, wants ...string) bool {
	joined := "," + strings.ToLower(strings.Join(values, ",")) + ","
	for _, want := range wants {
		if !strings.Contains(joined, strings.ToLower(want)) {
			return false
		}
	}
	return true
}

func parseHTMLDocument(t *testing.T, body string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse HTML document: %v", err)
	}
	return doc
}

func parseFragmentElementRoots(t *testing.T, body string) []*html.Node {
	t.Helper()
	context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(body), context)
	if err != nil {
		t.Fatalf("parse HTML fragment: %v", err)
	}
	var roots []*html.Node
	for _, node := range nodes {
		if node.Type == html.ElementNode {
			roots = append(roots, node)
		}
	}
	return roots
}

func nodeID(node *html.Node) string {
	for _, attribute := range node.Attr {
		if attribute.Key == "id" {
			return attribute.Val
		}
	}
	return ""
}

func nodeIDs(nodes []*html.Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, nodeID(node))
	}
	return ids
}

func hasAttribute(node *html.Node, key string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return true
		}
	}
	return false
}

func attributeValue(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func findDescendantByID(root *html.Node, id string) *html.Node {
	return findDescendant(root, func(node *html.Node) bool { return nodeID(node) == id })
}

func findDescendantByClass(root *html.Node, class string) *html.Node {
	return findDescendant(root, func(node *html.Node) bool {
		return nodeHasClass(node, class)
	})
}

func nodeHasClass(node *html.Node, class string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key == "class" && strings.Contains(" "+attribute.Val+" ", " "+class+" ") {
			return true
		}
	}
	return false
}

func assertFocusableSkillsControlsHaveStableIDs(t *testing.T, root *html.Node) {
	t.Helper()
	if root == nil {
		t.Fatal("missing Skills workbench for focusable-control ID audit")
	}
	seen := make(map[string]bool)
	var visit func(*html.Node, bool)
	visit = func(node *html.Node, hidden bool) {
		hidden = hidden || hasAttribute(node, "hidden")
		if node.Type == html.ElementNode && !hidden {
			focusable := node.Data == "a" || node.Data == "button" || ((node.Data == "input" || node.Data == "select" || node.Data == "textarea") && attributeValue(node, "type") != "hidden")
			if focusable {
				id := nodeID(node)
				if id == "" {
					t.Errorf("focusable Skills <%s> lacks a stable ID", node.Data)
				} else if seen[id] {
					t.Errorf("focusable Skills control ID %q is duplicated", id)
				}
				seen[id] = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, hidden)
		}
	}
	visit(root, false)
}

func findDescendantByAttribute(root *html.Node, key string) *html.Node {
	return findDescendant(root, func(node *html.Node) bool { return hasAttribute(node, key) })
}

func findDescendant(root *html.Node, match func(*html.Node) bool) *html.Node {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match(child) {
			return child
		}
		if found := findDescendant(child, match); found != nil {
			return found
		}
	}
	return nil
}

func countDescendantsByID(root *html.Node, id string) int {
	return countDescendants(root, func(node *html.Node) bool { return nodeID(node) == id })
}

func countDescendantsByAttribute(root *html.Node, key string) int {
	return countDescendants(root, func(node *html.Node) bool { return hasAttribute(node, key) })
}

func countDescendants(root *html.Node, match func(*html.Node) bool) int {
	count := 0
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match(child) {
			count++
		}
		count += countDescendants(child, match)
	}
	return count
}

func nodesAppearInDocumentOrder(root *html.Node, wanted ...*html.Node) bool {
	index := 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if index < len(wanted) && node == wanted[index] {
			index++
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return index == len(wanted)
}
