package app

import (
	"net/http"
	"strings"
	"testing"
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
