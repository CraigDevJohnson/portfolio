package partials

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

	"portfolio/types"
)

func TestPortalStatePresentationCoversEC2Lifecycle(t *testing.T) {
	tests := []struct {
		state PortalState
		class string
		label string
	}{
		{PortalStatePending, "portal-state-pending", "Pending"},
		{PortalStateRunning, "portal-state-running", "Running"},
		{PortalStateStopping, "portal-state-stopping", "Stopping"},
		{PortalStateStopped, "portal-state-stopped", "Stopped"},
		{PortalStateShuttingDown, "portal-state-shutting-down", "Shutting down"},
		{PortalStateTerminated, "portal-state-terminated", "Terminated"},
	}

	for _, test := range tests {
		t.Run(string(test.state), func(t *testing.T) {
			view := portalStatePresentation(string(test.state))
			if view.Class != test.class || view.Label != test.label || strings.TrimSpace(view.Description) == "" {
				t.Fatalf("portalStatePresentation(%q) = %#v, want class %q, label %q, and a description", test.state, view, test.class, test.label)
			}
		})
	}
}

func TestPortalStatePresentationFailsClosedForUnknownInput(t *testing.T) {
	for _, state := range []string{"", "rebooting", " running ", `running\" onclick=\"alert(1)`} {
		view := portalStatePresentation(state)
		if view.Class != "portal-state-unknown" || view.Label != "Unknown" || strings.TrimSpace(view.Description) == "" {
			t.Errorf("portalStatePresentation(%q) = %#v, want the complete neutral unknown presentation", state, view)
		}
	}
}

func TestPortalActionAvailabilityMatchesLifecycleSafetyMatrix(t *testing.T) {
	tests := []struct {
		state   string
		enabled map[string]bool
	}{
		{string(PortalStatePending), map[string]bool{}},
		{string(PortalStateRunning), map[string]bool{"stop": true, "restart": true}},
		{string(PortalStateStopping), map[string]bool{}},
		{string(PortalStateStopped), map[string]bool{"start": true}},
		{string(PortalStateShuttingDown), map[string]bool{}},
		{string(PortalStateTerminated), map[string]bool{}},
		{"unknown", map[string]bool{}},
	}

	for _, test := range tests {
		for _, action := range []string{"start", "stop", "restart"} {
			wantDisabled := !test.enabled[action]
			if got := portalActionDisabled(test.state, action); got != wantDisabled {
				t.Errorf("portalActionDisabled(%q, %q) = %t, want %t", test.state, action, got, wantDisabled)
			}
		}
	}
}

func TestInstanceRowRendersOneResponsiveSemanticRowPair(t *testing.T) {
	const instanceID = "i-0f1e2d3c4b5a69788"
	markup := portalTestRender(t, InstanceRow(types.InstanceSummary{
		ID:           instanceID,
		Name:         "Portfolio web",
		State:        string(PortalStateRunning),
		InstanceType: "t3.small",
		AZ:           "us-east-1a",
	}))
	document := portalTestParseTableRows(t, markup)
	tbody := portalTestFind(document, func(node *html.Node) bool { return node.Data == "tbody" })
	rows := portalTestElementChildren(tbody)
	if len(rows) != 2 {
		t.Fatalf("InstanceRow rendered %d table rows, want one primary row followed by one detail row", len(rows))
	}
	primary, detail := rows[0], rows[1]
	if portalTestAttribute(primary, "id") != "instance-"+instanceID || !portalTestHasClass(primary, "portal-instance-row") {
		t.Fatalf("first row is not the stable primary instance row: %s", markup)
	}
	if !portalTestHasClass(detail, "portal-instance-detail-row") || portalTestAttribute(detail, "data-portal-instance-detail") != instanceID {
		t.Fatalf("second row is not the adjacent detail row for %s: %s", instanceID, markup)
	}

	cells := portalTestElementChildren(primary)
	wantLabels := []string{"ID", "Name", "State", "Type", "AZ", "Actions", "Metrics", "Logs"}
	if len(cells) != len(wantLabels) {
		t.Fatalf("primary row cell count = %d, want %d", len(cells), len(wantLabels))
	}
	for index, want := range wantLabels {
		if got := portalTestAttribute(cells[index], "data-label"); got != want {
			t.Errorf("cell %d data-label = %q, want %q", index, got, want)
		}
	}

	state := portalTestFind(primary, func(node *html.Node) bool { return portalTestHasClass(node, "portal-state") })
	if state == nil || portalTestAttribute(state, "data-portal-state-raw") != string(PortalStateRunning) || !portalTestHasClass(state, "portal-state-running") {
		t.Fatalf("state does not retain raw diagnostics and normalized class: %s", markup)
	}
	if got := strings.TrimSpace(portalTestText(state)); !strings.HasPrefix(got, "Running") {
		t.Errorf("visible normalized state label = %q, want Running", got)
	}
	description := portalTestFind(state, func(node *html.Node) bool { return portalTestHasClass(node, "portal-state-description") })
	if description == nil || !portalTestHasClass(description, "sr-only") || strings.TrimSpace(portalTestText(description)) == "" {
		t.Errorf("normalized state description is not exposed accessibly: %s", markup)
	}

	for _, kind := range []string{"metrics", "logs"} {
		control := portalTestFind(primary, func(node *html.Node) bool {
			return node.Data == "button" && portalTestAttribute(node, "aria-controls") == kind+"-"+instanceID
		})
		if control == nil || !portalTestHasAttribute(control, "data-portal-detail-control") || portalTestAttribute(control, "aria-expanded") != "false" {
			t.Errorf("%s control lacks the initial disclosure contract: %s", kind, markup)
		}
		target := portalTestFind(detail, func(node *html.Node) bool { return portalTestAttribute(node, "id") == kind+"-"+instanceID })
		if target == nil || portalTestAttribute(target, "aria-busy") != "false" || portalTestAttribute(target, "data-portal-detail-kind") != kind {
			t.Errorf("%s target lacks the initial busy/detail contract: %s", kind, markup)
		}
	}

	seenIDs := make(map[string]bool)
	portalTestWalk(document, func(node *html.Node) {
		id := portalTestAttribute(node, "id")
		if id == "" {
			return
		}
		if seenIDs[id] {
			t.Errorf("InstanceRow rendered duplicate id %q", id)
		}
		seenIDs[id] = true
	})
	if strings.Contains(markup, "data-signal-trail") {
		t.Fatal("instance fragment unexpectedly renders a full-page signal trail")
	}
}

func TestInstanceRowDisablesEveryActionForUnknownState(t *testing.T) {
	markup := portalTestRender(t, InstanceRow(types.InstanceSummary{
		ID:           "i-0123456789abcdef0",
		Name:         "Unexpected state",
		State:        "rebooting",
		InstanceType: "t3.nano",
		AZ:           "us-east-1z",
	}))
	document := portalTestParseTableRows(t, markup)
	state := portalTestFind(document, func(node *html.Node) bool { return portalTestHasClass(node, "portal-state") })
	if state == nil || !portalTestHasClass(state, "portal-state-unknown") || !strings.HasPrefix(strings.TrimSpace(portalTestText(state)), "Unknown") {
		t.Fatalf("unknown raw state did not render the neutral label: %s", markup)
	}

	actionCount := 0
	portalTestWalk(document, func(node *html.Node) {
		if node.Data == "button" && portalTestAttribute(node, "hx-post") != "" {
			actionCount++
			if !portalTestHasAttribute(node, "disabled") || portalTestAttribute(node, "aria-disabled") != "true" {
				t.Errorf("unknown-state action is not disabled: %s", portalTestAttribute(node, "hx-post"))
			}
		}
	})
	if actionCount != 3 {
		t.Fatalf("unknown row action count = %d, want 3", actionCount)
	}
}

func portalTestRender(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render portal component: %v", err)
	}
	return output.String()
}

func portalTestParseTableRows(t *testing.T, markup string) *html.Node {
	t.Helper()
	document, err := html.Parse(strings.NewReader("<table><tbody>" + markup + "</tbody></table>"))
	if err != nil {
		t.Fatalf("parse portal row markup: %v", err)
	}
	return document
}

func portalTestFind(node *html.Node, predicate func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if predicate(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if match := portalTestFind(child, predicate); match != nil {
			return match
		}
	}
	return nil
}

func portalTestWalk(node *html.Node, visit func(*html.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		portalTestWalk(child, visit)
	}
}

func portalTestElementChildren(node *html.Node) []*html.Node {
	var children []*html.Node
	if node == nil {
		return children
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			children = append(children, child)
		}
	}
	return children
}

func portalTestAttribute(node *html.Node, key string) string {
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func portalTestHasAttribute(node *html.Node, key string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return true
		}
	}
	return false
}

func portalTestHasClass(node *html.Node, class string) bool {
	for _, candidate := range strings.Fields(portalTestAttribute(node, "class")) {
		if candidate == class {
			return true
		}
	}
	return false
}

func portalTestText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var text strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(portalTestText(child))
	}
	return text.String()
}
