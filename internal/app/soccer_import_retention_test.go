package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"portfolio/internal/config"
)

// importedAccessShown is the LPS connection status a page shows only while
// the visitor can use imported linked-player access.
const importedAccessShown = "Imported in this browser"

// holdsCookie reports whether the browser would send the named cookie to a
// request for path.
func (b *siteBrowser) holdsCookie(name, path string) bool {
	target, _ := url.Parse("https://app.example.com" + path)
	for _, cookie := range b.jar.Cookies(target) {
		if cookie.Name == name {
			return true
		}
	}
	return false
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
