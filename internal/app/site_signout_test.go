package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"portfolio/internal/config"
)

const (
	siteOrigin    = "https://app.example.com"
	anotherOrigin = "https://evil.example"
)

// browserForm is a top-level form POST a browser sends when a page on origin
// submits it to this site. Browsers send and apply this site's cookies for
// such a POST even when another site's page submitted it.
func browserForm(origin, path string, form url.Values) *http.Request {
	request := httptest.NewRequest(http.MethodPost, siteOrigin+path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", origin)
	if origin == siteOrigin {
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	} else {
		request.Header.Set("Sec-Fetch-Site", "cross-site")
	}
	return request
}

// newSignedInOwnerBrowser returns a browser signed in as the owner that holds
// the owner's imported LPS access, Google connection and pending Google
// consent, together with the Soccer URL those cookies are scoped to.
func newSignedInOwnerBrowser(t *testing.T, world *soccerGrantWorld) (*siteBrowser, *url.URL) {
	t.Helper()
	browser := newSiteBrowser(t, world.mux)
	soccerURL, _ := url.Parse(siteOrigin + "/soccer")
	owned := world.ownerPrivateState(t)
	for _, cookie := range owned {
		cookie.Path = config.SoccerCookiePath
	}
	browser.jar.SetCookies(soccerURL, owned)
	browser.jar.SetCookies(browser.origin, []*http.Cookie{testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail)})

	before := browser.get("/soccer").Body.String()
	if !strings.Contains(before, importedAccessShown) || !strings.Contains(before, "Calendar ready") {
		t.Fatal("signed-in owner did not start with imported access and a Google connection")
	}
	return browser, soccerURL
}

// One site sign-out ends more than the site session: the Soccer features that
// depend on it drop the imported LPS access and any pending Google consent
// held in this browser, while the owner-bound Google connection stays for the
// owner's next sign-in.
func TestSiteSignOutClearsImportedAccessAndPendingConsentButKeepsGoogleConnection(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	browser, soccerURL := newSignedInOwnerBrowser(t, world)

	signOut := browser.do(browserForm(siteOrigin, "/sign-out", nil))
	if signOut.Code != http.StatusSeeOther {
		t.Fatalf("sign-out status = %d", signOut.Code)
	}
	cleared := map[string]bool{}
	for _, cookie := range signOut.Result().Cookies() {
		if cookie.Path == config.SoccerCookiePath && cookie.Value == "" && cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
		if cookie.Name == ownerGoogleConnectionName {
			t.Errorf("sign-out changed the owner's Google connection cookie: %#v", cookie)
		}
	}
	for _, name := range []string{config.LPSSessionCookieName, config.LPSImportGuardCookieName, config.GoogleOAuthStateCookieName} {
		if !cleared[name] {
			t.Errorf("sign-out did not clear %s", name)
		}
	}
	held := map[string]bool{}
	for _, cookie := range browser.jar.Cookies(soccerURL) {
		held[cookie.Name] = true
	}
	if held[config.LPSSessionCookieName] || held[config.LPSImportGuardCookieName] || held[config.GoogleOAuthStateCookieName] || !held[ownerGoogleConnectionName] {
		t.Fatalf("browser Soccer cookies after sign-out = %v, want only the Google connection", held)
	}

	// The owner signs in again in the same browser.
	browser.jar.SetCookies(browser.origin, []*http.Cookie{testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail)})
	after := browser.get("/soccer").Body.String()
	if strings.Contains(after, importedAccessShown) {
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

// A page on another site cannot sign the owner out, because sign-out would
// also end the imported LPS access and pending Google consent that depend on
// the site session.
func TestAnotherSiteCannotSignTheOwnerOut(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	browser, _ := newSignedInOwnerBrowser(t, world)

	refused := browser.do(browserForm(anotherOrigin, "/sign-out", nil))
	if refused.Code != http.StatusForbidden {
		t.Errorf("cross-site sign-out status = %d, want 403", refused.Code)
	}
	for _, cookie := range refused.Result().Cookies() {
		t.Errorf("cross-site sign-out changed cookie %s (Max-Age %d)", cookie.Name, cookie.MaxAge)
	}
	page := browser.get("/soccer").Body.String()
	for _, kept := range []string{testSiteEmail, importedAccessShown, "Calendar ready"} {
		if !strings.Contains(page, kept) {
			t.Errorf("cross-site sign-out ended the owner's %q", kept)
		}
	}
}

// A page on another site cannot start site sign-in, which would replace any
// sign-in this browser has pending.
func TestAnotherSiteCannotStartSiteSignIn(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	browser := newSiteBrowser(t, mux)
	form := url.Values{"return_to": {"/about"}}

	refused := browser.do(browserForm(anotherOrigin, "/sign-in", form))
	if refused.Code != http.StatusForbidden || refused.Header().Get("Location") != "" {
		t.Errorf("cross-site sign-in start: %d %q, want 403 without a redirect", refused.Code, refused.Header().Get("Location"))
	}
	if browser.hasCookie(config.SiteOAuthStateCookieName) {
		t.Error("cross-site sign-in start left pending sign-in state")
	}

	started := browser.do(browserForm(siteOrigin, "/sign-in", form))
	if target, err := url.Parse(started.Header().Get("Location")); started.Code != http.StatusSeeOther || err != nil || target.Path != "/oauth2/authorize" {
		t.Fatalf("same-origin sign-in start: %d %q", started.Code, started.Header().Get("Location"))
	}
	if !browser.hasCookie(config.SiteOAuthStateCookieName) {
		t.Fatal("same-origin sign-in start left no pending sign-in state")
	}
}
