package app

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/session"
	"portfolio/internal/siteidentity"
)

// expiredSiteSessionCookie is the site session cookie a browser still holds
// after the session it carries has expired. The Soccer grant matrix, in
// TestSoccerGrantDecidesEveryPrivateRouteLikeThePage, uses it for Soccer.
func expiredSiteSessionCookie(t *testing.T, key []byte, principal siteidentity.Principal) *http.Cookie {
	t.Helper()
	value, err := session.EncryptJSONValue(key, map[string]any{
		"principal":  principal,
		"expires_at": time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: config.SiteSessionCookieName, Value: value}
}

func TestExpiredSiteSessionStopsManagementPortalUntilSignInAgain(t *testing.T) {
	cognito := newFakeSiteCognito(t)
	portalApp := newManagementPortal(t, cognito, map[string][]string{"owner@example.com": {"management"}})
	expired := expiredSiteSessionCookie(t, portalApp.app.Config.SiteSessionKey, siteidentity.Principal{
		Issuer: cognito.issuer, Subject: cognito.subject, Email: "owner@example.com",
	})

	for _, route := range managementRoutes {
		response := portalApp.request(route.method, route.path, false, expired)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in?return_to=%2Fmgmt" {
			t.Errorf("expired site session on %s %s: %d %q, want site sign-in", route.method, route.path, response.Code, response.Header().Get("Location"))
		}
	}
	if calls := portalApp.aws.calls(); calls != 0 {
		t.Fatalf("expired site session reached AWS %d times", calls)
	}

	owner := portalApp.signIn(t, "owner@example.com", "/mgmt")
	dashboard := portalApp.request(http.MethodGet, "/mgmt", false, owner)
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), "Signed in as owner@example.com") {
		t.Fatalf("signing in again did not restore the portal: %d", dashboard.Code)
	}
}
