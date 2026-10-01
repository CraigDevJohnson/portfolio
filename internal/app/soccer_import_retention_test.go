package app

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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

// sendForm posts a form with the cookies the browser holds now and returns a
// function that delivers the response later, as a response still in flight
// while the browser does something else.
func (b *siteBrowser) sendForm(path string, form url.Values) (deliver func() *httptest.ResponseRecorder) {
	b.t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://app.example.com"+path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range b.jar.Cookies(request.URL) {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	b.handler.ServeHTTP(response, request)
	return func() *httptest.ResponseRecorder {
		b.receive(request.URL, response)
		return response
	}
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
	world, browser, _ := newRetainedImportBrowserWithCognito(t)
	return world, browser
}

// newRetainedImportBrowserWithCognito is newRetainedImportBrowser that also
// returns the fake Cognito, whose identity decides the next sign-in.
func newRetainedImportBrowserWithCognito(t *testing.T) (*soccerGrantWorld, *siteBrowser, *fakeSiteCognito) {
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
	return world, browser, cognito
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
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), endedImportNotice) {
		t.Errorf("linked-player discovery after sign-out: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() != calls {
		t.Error("linked-player discovery after sign-out used the cleared LPS credential")
	}
}

func TestSoccerResponseInFlightWhenImportIsClearedCannotRestoreIt(t *testing.T) {
	for _, clearing := range []struct {
		name string
		// clear removes the import; signedOut reports whether it also ended
		// the site session, so the owner must sign in again.
		clear     func(browser *siteBrowser) *httptest.ResponseRecorder
		signedOut bool
	}{
		{name: "site sign-out", signedOut: true, clear: func(browser *siteBrowser) *httptest.ResponseRecorder {
			return browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil))
		}},
		{name: "Clear import", clear: func(browser *siteBrowser) *httptest.ResponseRecorder {
			return browser.postForm("/soccer/logout", url.Values{})
		}},
	} {
		t.Run(clearing.name, func(t *testing.T) {
			world, browser := newRetainedImportBrowser(t)

			inFlight := browser.sendForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}})
			if cleared := clearing.clear(browser); cleared.Code >= http.StatusBadRequest {
				t.Fatalf("%s status = %d", clearing.name, cleared.Code)
			}
			if stale := inFlight(); !strings.Contains(stale.Body.String(), "Craig FC") || findSessionCookie(t, stale.Result()) == nil {
				t.Fatalf("the discovery response in flight did not rewrite the import cookie: status %d", stale.Code)
			}

			browser.restart()
			if clearing.signedOut {
				browser.signIn("/soccer")
			}
			if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) {
				t.Error("a response in flight restored imported access that was cleared")
			}
			calls := world.lpsCredentialCalls.Load()
			if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), endedImportNotice) {
				t.Errorf("linked-player discovery after the import was cleared: status %d", discovered.Code)
			}
			if world.lpsCredentialCalls.Load() != calls {
				t.Error("linked-player discovery used the cleared LPS credential")
			}
		})
	}
}

// inFlightImportWait bounds each wait on an import held in flight, so a test
// whose import never reaches LPS or never finishes fails instead of hanging.
const inFlightImportWait = 10 * time.Second

// holdLPSAccountLookup holds the fake LPS account lookup an import makes until
// release is called or the lookup's own request ends; started is closed once
// the lookup has reached LPS.
func (world *soccerGrantWorld) holdLPSAccountLookup(t *testing.T) (started <-chan struct{}, release func()) {
	t.Helper()
	reached, released := make(chan struct{}), make(chan struct{})
	var reachedOnce, releaseOnce sync.Once
	world.app.LPSClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/users/check" {
			reachedOnce.Do(func() { close(reached) })
			select {
			case <-released:
			case <-request.Context().Done():
				return nil, request.Context().Err()
			}
		}
		return http.DefaultTransport.RoundTrip(request)
	})
	release = func() { releaseOnce.Do(func() { close(released) }) }
	t.Cleanup(release)
	return reached, release
}

// An import still running when the owner signs out finishes afterwards and
// writes its own import cookies and guard after the sign-out response has
// cleared them. The site session that authorized it was issued before the
// sign-out, so the owner's next sign-in must not make it usable.
func TestImportInFlightAtSiteSignOutCannotRestoreImportedAccess(t *testing.T) {
	world, browser := newRetainedImportBrowser(t)
	lookupStarted, releaseLookup := world.holdLPSAccountLookup(t)

	// Buffered, so the import can finish and exit even after the test failed.
	sent := make(chan func() *httptest.ResponseRecorder, 1)
	go func() { sent <- browser.sendForm("/soccer/import", url.Values{"jwt": {world.jwt}}) }()
	select {
	case <-lookupStarted:
	case deliver := <-sent:
		t.Fatalf("the import finished without reaching the LPS account lookup: status %d", deliver().Code)
	case <-time.After(inFlightImportWait):
		t.Fatal("the import never reached the LPS account lookup")
	}
	if signOut := browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	releaseLookup()
	var inFlight func() *httptest.ResponseRecorder
	select {
	case inFlight = <-sent:
	case <-time.After(inFlightImportWait):
		t.Fatal("the import in flight never finished after the LPS account lookup was released")
	}
	if late := inFlight(); !strings.Contains(late.Body.String(), `name="player_ids"`) || findSessionCookie(t, late.Result()) == nil || findImportGuardCookie(late.Result()) == nil {
		t.Fatalf("the import in flight did not write its import cookies: status %d", late.Code)
	}

	browser.restart()
	browser.signIn("/soccer")
	if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) || !strings.Contains(page.Body.String(), "Import access") {
		t.Error("an import in flight at sign-out was usable after the owner signed in again")
	}
	calls := world.lpsCredentialCalls.Load()
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), endedImportNotice) {
		t.Errorf("linked-player discovery after sign-out: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() != calls {
		t.Error("linked-player discovery used the LPS credential of an import in flight at sign-out")
	}
	if browser.holdsCookie(config.LPSSessionCookieName, "/soccer") || browser.holdsCookie(config.LPSImportGuardCookieName, "/soccer") {
		t.Error("the browser kept the import that was in flight at sign-out")
	}
}

// The browser's record of an explicit sign-out ends only imports authorized
// before it: an import made after the owner signs in again is retained like
// any other, and a later site-session timeout only withholds it.
func TestImportAfterSigningInAgainIsRetainedThroughTheNextTimeout(t *testing.T) {
	world, browser := newRetainedImportBrowser(t)
	if signOut := browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}

	browser.signIn("/soccer")
	if imported := browser.postForm("/soccer/import", url.Values{"jwt": {world.jwt}}); !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("import after signing in again did not list linked players: status %d", imported.Code)
	}
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
		t.Fatal("an import made after signing in again was not usable")
	}

	browser.expireSiteSession()
	browser.restart()
	browser.signIn("/soccer")
	if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
		t.Error("signing in again after a timeout did not restore an import made after the earlier sign-out")
	}
	calls := world.lpsCredentialCalls.Load()
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), "Craig FC") {
		t.Errorf("linked-player discovery after the timeout: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() == calls {
		t.Error("linked-player discovery after the timeout did not use the restored LPS credential")
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

func TestExpiredImportIsClearedWhetherOrNotItsOwnerIsSignedIn(t *testing.T) {
	for _, expiry := range []struct {
		name       string
		jwtExpiry  time.Time
		importEnds time.Time
	}{
		{name: "JWT expired", jwtExpiry: time.Now().Add(-time.Minute), importEnds: time.Now().Add(-time.Minute)},
		{name: "12-hour limit reached before a longer JWT", jwtExpiry: time.Now().Add(36 * time.Hour), importEnds: time.Now().Add(-time.Minute)},
	} {
		for _, visitor := range []struct {
			name     string
			signedIn bool
		}{{name: "owner signed in", signedIn: true}, {name: "owner timed out"}} {
			t.Run(expiry.name+"/"+visitor.name, func(t *testing.T) {
				world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
				imported := ownedBySiteVisitor(&types.SessionData{
					JWT:       testutil.TestJWT(t, expiry.jwtExpiry),
					Players:   []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true}},
					StartedAt: expiry.importEnds.Add(-12 * time.Hour),
					ExpiresAt: expiry.importEnds,
				})
				cookies := []*http.Cookie{{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, world.app, imported)}}
				if visitor.signedIn {
					cookies = append(cookies, testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail))
				}
				page := soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil, cookies...)
				if strings.Contains(page.Body.String(), importedAccessShown) {
					t.Error("the Soccer page presented an expired import as active")
				}
				assertClearedSessionCookie(t, page.Result())
				if world.lpsCredentialCalls.Load() != 0 {
					t.Error("the Soccer page used an expired LPS credential")
				}
			})
		}
	}
}

func TestAnotherSiteOwnerInTheSameBrowserCannotRecoverTheImport(t *testing.T) {
	world, browser, cognito := newRetainedImportBrowserWithCognito(t)
	world.app.Config.SiteInvitations[otherSiteEmail] = []string{"soccer"}

	browser.expireSiteSession()
	cognito.subject, cognito.email = otherSiteSubject, otherSiteEmail
	browser.signIn("/soccer")
	calls := world.lpsCredentialCalls.Load()
	if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) {
		t.Error("another site owner saw the previous owner's imported access")
	}
	if discovered := browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1001"}}); !strings.Contains(discovered.Body.String(), endedImportNotice) {
		t.Errorf("another owner's linked-player discovery: status %d, body %q", discovered.Code, discovered.Body.String())
	}
	if world.lpsCredentialCalls.Load() != calls {
		t.Error("another site owner used the previous owner's LPS credential")
	}
	if browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Error("the browser kept the previous owner's import after another owner signed in")
	}

	// The discarded import stays gone when its owner returns.
	browser.expireSiteSession()
	cognito.subject, cognito.email = "stable-subject", "owner@example.com"
	browser.signIn("/soccer")
	if page := browser.get("/soccer"); strings.Contains(page.Body.String(), importedAccessShown) {
		t.Error("the original owner recovered an import another owner's visit discarded")
	}
}

func TestTeamIDLookupThatLPSRefusesKeepsTheRetainedImport(t *testing.T) {
	for _, visitor := range []struct {
		name     string
		timedOut bool
	}{{name: "owner timed out", timedOut: true}, {name: "owner signed in"}} {
		for _, lookup := range []struct {
			path string
			form url.Values
		}{
			{path: "/soccer/fetch", form: url.Values{"team_codes": {refusedFacilityTeamID}}},
			{path: "/soccer/download", form: url.Values{"team_codes": {refusedFacilityTeamID}, "selected": {refusedFacilityGameID}}},
		} {
			t.Run(visitor.name+lookup.path, func(t *testing.T) {
				world, browser := newRetainedImportBrowser(t)
				if visitor.timedOut {
					browser.expireSiteSession()
				}
				calls := world.lpsCredentialCalls.Load()

				// The lookup never sends the imported token, so LPS refusing one of
				// its facilities says nothing about the import.
				refused := browser.postForm(lookup.path, lookup.form)
				if strings.Contains(refused.Body.String(), "token was rejected") {
					t.Error("a refusal of a request without the token was reported as a rejected import")
				}
				if cookie := findSessionCookie(t, refused.Result()); cookie != nil {
					t.Errorf("a Team ID lookup LPS refused replaced the retained import: %#v", cookie)
				}
				if world.lpsCredentialCalls.Load() != calls {
					t.Error("the Team ID lookup sent the imported LPS credential")
				}

				if visitor.timedOut {
					browser.signIn("/soccer")
				}
				if page := browser.get("/soccer"); !strings.Contains(page.Body.String(), importedAccessShown) {
					t.Error("the owner lost a still-valid import to a Team ID lookup LPS refused")
				}
			})
		}
	}
}
