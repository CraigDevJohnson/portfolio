package app

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// publicSoccerAccessSentence closes every private Soccer access notice. The
// page hero has already introduced the calendar (.ics) file.
const publicSoccerAccessSentence = "Team ID lookup and .ics file downloads are still available."

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
	if !strings.Contains(body, publicSoccerAccessSentence) || strings.Contains(body, "ICS download") {
		t.Error("Soccer access notice did not name the public .ics file download as the page introduced it")
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

func TestSoccerPageExplainsMissingAccessOnlyForPrivateActionsTheServerOffers(t *testing.T) {
	application := newTestApp(t)
	application.Config.SessionKey = nil
	enableTestSiteIdentity(application, map[string][]string{testSiteEmail: {"soccer"}})
	mux, _ := buildMux(application, application.Logger, false)
	body := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil).Body.String()
	for _, notice := range []string{"Private Soccer access", "Sign in for Soccer access"} {
		if strings.Contains(body, notice) {
			t.Errorf("Soccer page showed %q on a server without LPS import or Google Calendar", notice)
		}
	}
	if !strings.Contains(body, `href="/sign-in?return_to=%2Fsoccer"`) {
		t.Error("shared navigation stopped offering site sign-in")
	}
}

func TestSoccerAccessPreviewFixturesShowEachGrantStateLocally(t *testing.T) {
	application := newTestApp(t)
	preview, _ := buildMux(application, application.Logger, true)
	live, _ := buildMux(application, application.Logger, false)
	for _, fixture := range []struct {
		name          string
		present, gone []string
	}{
		{
			name:    "soccer-signed-out",
			present: []string{"Sign in with an invited account", `<a class="soccer-inline-link" href="/sign-in?return_to=%2Fsoccer">Sign in for Soccer access</a>`, "Needs Soccer access", "Team IDs", publicSoccerAccessSentence},
			gone:    []string{"Import access", "Connect Google Calendar", `action="/sign-out"`},
		},
		{
			name:    "soccer-ungranted",
			present: []string{"has not been granted", "invited.visitor@example.com", `action="/sign-out"`, "Needs Soccer access", "Team IDs", publicSoccerAccessSentence},
			gone:    []string{"Import access", "Connect Google Calendar", "Sign in for Soccer access"},
		},
		{
			name:    "soccer-granted",
			present: []string{"invited.visitor@example.com", "Import access", "Connect Google Calendar", "Team IDs"},
			gone:    []string{"Private Soccer access", "Needs Soccer access"},
		},
	} {
		page := soccerGrantRequest(preview, http.MethodGet, "/__preview/account/"+fixture.name, nil)
		if page.Code != http.StatusOK {
			t.Errorf("%s preview status = %d", fixture.name, page.Code)
			continue
		}
		body := page.Body.String()
		for _, marker := range fixture.present {
			if !strings.Contains(body, marker) {
				t.Errorf("%s preview lacks %q", fixture.name, marker)
			}
		}
		for _, marker := range fixture.gone {
			if strings.Contains(body, marker) {
				t.Errorf("%s preview shows %q", fixture.name, marker)
			}
		}
		for _, actionable := range []string{`hx-post="/soccer/`, `action="/soccer/`, `href="/soccer/google/`} {
			if strings.Contains(body, actionable) {
				t.Errorf("%s preview contains live Soccer action %q", fixture.name, actionable)
			}
		}
		if exposed := soccerGrantRequest(live, http.MethodGet, "/__preview/account/"+fixture.name, nil); exposed.Code != http.StatusNotFound {
			t.Errorf("%s preview outside local preview status = %d", fixture.name, exposed.Code)
		}
	}
}
