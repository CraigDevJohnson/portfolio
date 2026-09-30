package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"portfolio/internal/config"
)

// One site sign-out ends more than the site session: the Soccer features that
// depend on it drop the imported LPS access and any pending Google consent
// held in this browser, while the owner-bound Google connection stays for the
// owner's next sign-in.
func TestSiteSignOutClearsImportedAccessAndPendingConsentButKeepsGoogleConnection(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	browser := newSiteBrowser(t, world.mux)
	soccerURL, _ := url.Parse("https://app.example.com/soccer")
	owned := world.ownerPrivateState(t)
	for _, cookie := range owned {
		cookie.Path = config.SoccerCookiePath
	}
	browser.jar.SetCookies(soccerURL, owned)
	browser.jar.SetCookies(browser.origin, []*http.Cookie{testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail)})

	before := browser.get("/soccer").Body.String()
	if !strings.Contains(before, "Imported for this session") || !strings.Contains(before, "Calendar ready") {
		t.Fatal("signed-in owner did not start with imported access and a Google connection")
	}

	signOut := browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil))
	if signOut.Code != http.StatusSeeOther {
		t.Fatalf("sign-out status = %d", signOut.Code)
	}
	cleared := map[string]bool{}
	for _, cookie := range signOut.Result().Cookies() {
		if cookie.Path == config.SoccerCookiePath && cookie.Value == "" && cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
		if cookie.Name == config.GoogleConnectionCookieName {
			t.Errorf("sign-out changed the owner's Google connection cookie: %#v", cookie)
		}
	}
	for _, name := range []string{config.LPSSessionCookieName, config.GoogleOAuthStateCookieName} {
		if !cleared[name] {
			t.Errorf("sign-out did not clear %s", name)
		}
	}
	held := map[string]bool{}
	for _, cookie := range browser.jar.Cookies(soccerURL) {
		held[cookie.Name] = true
	}
	if held[config.LPSSessionCookieName] || held[config.GoogleOAuthStateCookieName] || !held[config.GoogleConnectionCookieName] {
		t.Fatalf("browser Soccer cookies after sign-out = %v, want only the Google connection", held)
	}

	// The owner signs in again in the same browser.
	browser.jar.SetCookies(browser.origin, []*http.Cookie{testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail)})
	after := browser.get("/soccer").Body.String()
	if strings.Contains(after, "Imported for this session") {
		t.Error("imported LPS access survived site sign-out")
	}
	if !strings.Contains(after, "Calendar ready") {
		t.Error("site sign-out discarded the owner's Google connection")
	}
	consent := browser.get("/soccer?code=auth-code&state=" + grantWorldPendingState)
	if consent.Code != http.StatusSeeOther || consent.Header().Get("Location") != "/soccer?google=failed" || world.googleTokenCalls.Load() != 0 {
		t.Fatalf("Google consent pending before sign-out completed afterwards: %d %q, token exchanges %d", consent.Code, consent.Header().Get("Location"), world.googleTokenCalls.Load())
	}
}
