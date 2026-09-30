package app

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
	"portfolio/types"
)

// importedAccessShown is the LPS connection status a page shows only while
// the visitor can use imported linked-player access.
const importedAccessShown = "Imported in this browser"

// cookieValue returns the named cookie the browser would send to a request
// for path, or "" when it holds none.
func (b *siteBrowser) cookieValue(name, path string) string {
	target, _ := url.Parse("https://app.example.com" + path)
	for _, cookie := range b.jar.Cookies(target) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

// holdsCookie reports whether the browser would send the named cookie to a
// request for path.
func (b *siteBrowser) holdsCookie(name, path string) bool {
	return b.cookieValue(name, path) != ""
}

// restart closes and reopens the browser: cookies set without an expiry are
// discarded, and persistent cookies survive until they expire.
func (b *siteBrowser) restart() {
	b.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		b.t.Fatal(err)
	}
	for _, cookie := range b.lastSet {
		if cookie.MaxAge > 0 || (cookie.MaxAge == 0 && !cookie.Expires.IsZero()) {
			jar.SetCookies(b.origin, []*http.Cookie{cookie})
		}
	}
	b.jar = jar
}

// expireSiteSession drops the site session as the browser does when its
// Max-Age ends: an automatic timeout, not an explicit sign-out.
func (b *siteBrowser) expireSiteSession() {
	expired := &http.Cookie{Name: config.SiteSessionCookieName, Path: config.SiteCookiePath, MaxAge: -1}
	b.jar.SetCookies(b.origin, []*http.Cookie{expired})
	b.lastSet[expired.Name+"\x00"+expired.Path] = expired
}

// newRetainedImportBrowser signs the invited owner in to a browser and
// imports the fake LPS account's linked players through the real routes.
func newRetainedImportBrowser(t *testing.T) (*soccerGrantWorld, *siteBrowser) {
	t.Helper()
	cognito := newFakeSiteCognito(t)
	application := cognito.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	world := newSoccerGrantWorldFor(t, application)
	browser := newSiteBrowser(t, world.mux)
	if landing := browser.signIn("/soccer"); landing.Code != http.StatusSeeOther {
		t.Fatalf("owner sign-in status = %d", landing.Code)
	}
	if imported := browser.postForm("/soccer/import", url.Values{"jwt": {world.jwt}}); !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("owner import did not list linked players: status %d", imported.Code)
	}
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
		t.Fatal("owner page did not show the imported access")
	}
	return world, browser
}

func TestExplicitSiteSignOutClearsImportedLPSAccess(t *testing.T) {
	world, browser := newRetainedImportBrowser(t)

	signOut := browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil))
	if signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	if browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Error("site sign-out left the imported LPS access in the browser")
	}

	// Unlike a site-session timeout, signing in again after an explicit
	// sign-out must not restore the import.
	browser.signIn("/soccer")
	if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) || !strings.Contains(page.Body.String(), "Import access") {
		t.Error("the owner's next sign-in restored imported access that sign-out should have cleared")
	}
	calls := world.lpsCredentialCalls.Load()
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), "Import a bearer JWT to discover teams.") {
		t.Errorf("linked-player discovery after sign-out: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() != calls {
		t.Error("linked-player discovery after sign-out used the cleared LPS credential")
	}
}

func TestBrowserRestartKeepsImportedAccessForTheSameOwner(t *testing.T) {
	world, browser := newRetainedImportBrowser(t)

	browser.restart()
	if !browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Fatal("a browser restart within the JWT and 12-hour bounds lost the imported LPS access")
	}
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
		t.Error("the owner's page after a browser restart did not show the imported access")
	}
	calls := world.lpsCredentialCalls.Load()
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), "Craig FC") {
		t.Errorf("linked-player discovery after a browser restart: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() == calls {
		t.Error("linked-player discovery after a browser restart did not use the retained LPS credential")
	}
}

func TestSiteSessionTimeoutWithholdsImportUntilTheSameOwnerSignsIn(t *testing.T) {
	world, browser := newRetainedImportBrowser(t)

	browser.expireSiteSession()
	browser.restart()
	retained := browser.cookieValue(config.LPSSessionCookieName, "/soccer")
	calls := world.lpsCredentialCalls.Load()
	if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) || strings.Contains(page.Body.String(), `hx-post="/soccer/logout"`) {
		t.Error("the Soccer page showed imported access after the site session timed out")
	}
	if refused := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); refused.Code != http.StatusUnauthorized {
		t.Errorf("linked-player discovery after timeout status = %d, want 401", refused.Code)
	}
	if lookup := browser.postForm("/soccer/fetch", url.Values{"team_codes": {"4101"}}); lookup.Code != http.StatusOK || !strings.Contains(lookup.Body.String(), "Craig FC") {
		t.Errorf("public Team ID lookup after timeout: status %d", lookup.Code)
	}
	if world.lpsCredentialCalls.Load() != calls {
		t.Error("requests after the site session timed out used the imported LPS credential")
	}
	if retained == "" || browser.cookieValue(config.LPSSessionCookieName, "/soccer") != retained {
		t.Fatal("the site-session timeout discarded or replaced a still-valid import")
	}

	browser.signIn("/soccer")
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
		t.Error("signing in again as the same owner did not restore the still-valid import")
	}
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), "Craig FC") {
		t.Errorf("linked-player discovery after signing in again: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() == calls {
		t.Error("linked-player discovery after signing in again did not use the restored LPS credential")
	}
}

func TestTeamIDLookupThatDiscardsAnExpiredImportSavesTheLookup(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	browser := newSiteBrowser(t, world.mux)
	expired := ownedBySiteVisitor(&types.SessionData{
		JWT:       testutil.TestJWT(t, time.Now().Add(-time.Minute)),
		Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}},
		ExpiresAt: time.Now().Add(-time.Minute),
	})
	soccerPage, _ := url.Parse("https://app.example.com/soccer")
	browser.jar.SetCookies(soccerPage, []*http.Cookie{{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, world.app, expired), Path: config.SoccerCookiePath}})

	if lookup := browser.postForm("/soccer/fetch", url.Values{"team_codes": {"4101"}}); lookup.Code != http.StatusOK || !strings.Contains(lookup.Body.String(), `value="7001"`) {
		t.Fatalf("anonymous Team ID lookup: status %d, body %q", lookup.Code, lookup.Body.String())
	}
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), `value="7001"`) {
		t.Error("the Soccer page did not restore a Team ID lookup made while an expired import was discarded")
	}
	if world.lpsCredentialCalls.Load() != 0 {
		t.Error("the anonymous lookup used the expired LPS credential")
	}
}
