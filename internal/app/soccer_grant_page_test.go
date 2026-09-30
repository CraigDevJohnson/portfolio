package app

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSoccerPageOffersSiteSignInForPrivateActionsOnlyWhereSignInWorks(t *testing.T) {
	unconfigured := newTestApp(t)
	mux, _ := buildMux(unconfigured, unconfigured.Logger, false)
	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("Soccer page status = %d", page.Code)
	}
	body := page.Body.String()
	if strings.Contains(body, `href="/sign-in`) {
		t.Error("Soccer page linked to site sign-in although this environment cannot start it")
	}
	if !strings.Contains(body, "site sign-in is not available here") {
		t.Error("Soccer page did not explain why private actions are unavailable without site sign-in")
	}

	configured := newTestApp(t)
	enableTestSiteIdentity(configured, map[string][]string{testSiteEmail: {"soccer"}})
	mux, _ = buildMux(configured, configured.Logger, false)
	body = soccerGrantRequest(mux, http.MethodGet, "/soccer", nil).Body.String()
	if !strings.Contains(body, `<a class="soccer-inline-link" href="/sign-in?return_to=%2Fsoccer">Sign in for Soccer access</a>`) {
		t.Error("signed-out Soccer page did not offer site sign-in for private actions")
	}
	if strings.Contains(body, "site sign-in is not available here") {
		t.Error("Soccer page called available site sign-in unavailable")
	}
}

func TestSoccerPageNamesTheMissingGrantInsteadOfAnUnavailableServer(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	doc := parsePlannerHTML(t, soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil).Body.String())
	lpsCard := plannerSingle(t, doc, "LPS connection card", plannerAttrIs("id", "soccer-lps-connection"))
	googleCard := plannerSingle(t, doc, "Google connection card", plannerAttrIs("id", "soccer-google-connection"))
	googleOption := plannerSingle(t, doc, "Google output option", func(node *html.Node) bool {
		return node.Data == "label" && strings.Contains(plannerText(node), "Google Calendar") && strings.Contains(soccerHTMLAttribute(node, "class"), "soccer-output-option")
	})
	linkedSource := plannerSingle(t, doc, "linked-player source", func(node *html.Node) bool { return plannerHasAttr(node, "data-soccer-linked-source") })
	for name, node := range map[string]*html.Node{"LPS card": lpsCard, "Google card": googleCard, "Google output option": googleOption, "linked-player source": linkedSource} {
		text := plannerText(node)
		if strings.Contains(text, "Not enabled on this server") || strings.Contains(text, "not enabled in this runtime") {
			t.Errorf("%s calls a configured capability unavailable to a signed-out visitor: %q", name, text)
		}
		if !strings.Contains(text, "Soccer access") {
			t.Errorf("%s does not name the missing Soccer access: %q", name, text)
		}
	}

	unconfigured := newTestApp(t)
	unconfigured.Config.SessionKey = nil
	enableTestSiteIdentity(unconfigured, map[string][]string{testSiteEmail: {"soccer"}})
	mux, _ := buildMux(unconfigured, unconfigured.Logger, false)
	doc = parsePlannerHTML(t, soccerGrantRequest(mux, http.MethodGet, "/soccer", nil).Body.String())
	for _, id := range []string{"soccer-lps-connection", "soccer-google-connection"} {
		card := plannerText(plannerSingle(t, doc, id, plannerAttrIs("id", id)))
		if !strings.Contains(card, "Not enabled on this server") || strings.Contains(card, "Soccer access") {
			t.Errorf("%s on a server without that capability = %q, want Not enabled on this server", id, card)
		}
	}
}
