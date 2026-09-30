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
	"portfolio/internal/session"
	"portfolio/internal/siteidentity"
)

func TestUnconfiguredSiteSignInIsNotAdvertisedOnPublicPages(t *testing.T) {
	application := newTestApp(t)
	mux, _ := buildMux(application, application.Logger, false)

	page := httptest.NewRecorder()
	mux.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("public About status = %d", page.Code)
	}
	if strings.Contains(page.Body.String(), `href="/sign-in`) {
		t.Fatal("shared navigation advertised site sign-in although it is not configured")
	}
}

func TestSiteAccountResponsesAreNotStored(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	serve := func(request *http.Request, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		return response
	}

	anonymous := serve(httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil))
	if got := anonymous.Header().Get("Cache-Control"); anonymous.Code != http.StatusOK || got != "" {
		t.Fatalf("anonymous public page: status %d, Cache-Control %q", anonymous.Code, got)
	}

	landing := serve(httptest.NewRequest(http.MethodGet, "https://app.example.com/sign-in?return_to=%2Fabout", nil))
	start := serve(signInStartRequest("/about"))
	stateCookie, state := beginSiteSignIn(t, mux, "/about")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	accountCookie := siteCookie(t, callback)
	signedInPage := serve(httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil), accountCookie)
	signOut := serve(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil), accountCookie)

	for name, response := range map[string]*httptest.ResponseRecorder{
		"sign-in landing":   landing,
		"sign-in start":     start,
		"sign-in callback":  callback,
		"signed-in page":    signedInPage,
		"sign-out redirect": signOut,
	} {
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", name, got)
		}
	}
	if !strings.Contains(signedInPage.Body.String(), "owner@example.com") {
		t.Fatal("signed-in page fixture did not render the active account")
	}
}

func signInStartRequest(returnTo string) *http.Request {
	form := url.Values{"return_to": {returnTo}}
	request := httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-in", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

func TestSiteSignInDoesNotReturnToItsOwnCallback(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)

	stateCookie, state := beginSiteSignIn(t, mux, "/auth/callback?code=replayed&state=replayed")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/" {
		t.Fatalf("sign-in returned to its callback: %d %q", callback.Code, callback.Header().Get("Location"))
	}
	if accountCookie := siteCookie(t, callback); accountCookie.Value == "" || accountCookie.MaxAge <= 0 {
		t.Fatal("invited identity did not keep its new site session")
	}
}

// siteBrowser keeps the cookies a browser would retain between site requests.
type siteBrowser struct {
	t       *testing.T
	handler http.Handler
	jar     *cookiejar.Jar
	origin  *url.URL
}

func newSiteBrowser(t *testing.T, handler http.Handler) *siteBrowser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := url.Parse("https://app.example.com/")
	return &siteBrowser{t: t, handler: handler, jar: jar, origin: origin}
}

func (b *siteBrowser) do(request *http.Request) *httptest.ResponseRecorder {
	b.t.Helper()
	for _, cookie := range b.jar.Cookies(request.URL) {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	b.handler.ServeHTTP(response, request)
	b.jar.SetCookies(request.URL, response.Result().Cookies())
	return response
}

func (b *siteBrowser) get(target string) *httptest.ResponseRecorder {
	b.t.Helper()
	return b.do(httptest.NewRequest(http.MethodGet, "https://app.example.com"+target, nil))
}

// signIn follows the site journey through the fake Cognito callback.
func (b *siteBrowser) signIn(returnTo string) *httptest.ResponseRecorder {
	b.t.Helper()
	start := b.do(signInStartRequest(returnTo))
	target, err := url.Parse(start.Header().Get("Location"))
	if start.Code != http.StatusSeeOther || err != nil || target.Query().Get("state") == "" {
		b.t.Fatalf("sign-in did not start: %d %q", start.Code, start.Header().Get("Location"))
	}
	return b.get("/auth/callback?code=test-code&state=" + url.QueryEscape(target.Query().Get("state")))
}

func (b *siteBrowser) setCookie(name, value string) {
	b.jar.SetCookies(b.origin, []*http.Cookie{{Name: name, Value: value, Path: "/"}})
}

func (b *siteBrowser) hasCookie(name string) bool {
	for _, cookie := range b.jar.Cookies(b.origin) {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

var publicSitePages = []string{"/", "/about", "/experience", "/skills", "/projects", "/education", "/contact", "/soccer"}

func assertPublicPagesAvailable(t *testing.T, browser *siteBrowser, signedIn bool) {
	t.Helper()
	for _, page := range publicSitePages {
		response := browser.get(page)
		if response.Code != http.StatusOK {
			t.Errorf("public page %s status = %d", page, response.Code)
			continue
		}
		if shown := strings.Contains(response.Body.String(), "owner@example.com"); shown != signedIn {
			t.Errorf("public page %s account shown = %t, want %t", page, shown, signedIn)
		}
	}
}

func TestPublicPagesRemainAvailableThroughoutSiteSignIn(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)

	t.Run("signed out", func(t *testing.T) {
		assertPublicPagesAvailable(t, newSiteBrowser(t, mux), false)
	})
	t.Run("sign-in pending", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		browser.do(signInStartRequest("/about"))
		if !browser.hasCookie(config.SiteOAuthStateCookieName) {
			t.Fatal("sign-in start did not leave pending state")
		}
		assertPublicPagesAvailable(t, browser, false)
	})
	t.Run("signed in", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		browser.signIn("/about")
		assertPublicPagesAvailable(t, browser, true)
	})
	t.Run("sign-in failure", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		fixture.email = "uninvited@example.com"
		t.Cleanup(func() { fixture.email = "owner@example.com" })
		if denied := browser.signIn("/about"); denied.Code != http.StatusUnauthorized {
			t.Fatalf("uninvited sign-in status = %d", denied.Code)
		}
		assertPublicPagesAvailable(t, browser, false)
	})
	t.Run("provider error", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		start := browser.do(signInStartRequest("/about"))
		target, _ := url.Parse(start.Header().Get("Location"))
		if denied := browser.get("/auth/callback?error=access_denied&state=" + url.QueryEscape(target.Query().Get("state"))); denied.Code != http.StatusUnauthorized {
			t.Fatalf("provider error status = %d", denied.Code)
		}
		assertPublicPagesAvailable(t, browser, false)
	})
	t.Run("expired session", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		expired, err := session.EncryptJSONValue(application.Config.SiteSessionKey, map[string]any{
			"principal":  map[string]string{"Issuer": fixture.issuer, "Subject": "stable-subject", "Email": "owner@example.com"},
			"expires_at": time.Now().Add(-time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		browser.setCookie(config.SiteSessionCookieName, expired)
		assertPublicPagesAvailable(t, browser, false)
		if browser.hasCookie(config.SiteSessionCookieName) {
			t.Fatal("expired site session was not cleared")
		}
	})
	t.Run("tampered session", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		browser.setCookie(config.SiteSessionCookieName, "not-an-encrypted-session")
		assertPublicPagesAvailable(t, browser, false)
		if browser.hasCookie(config.SiteSessionCookieName) {
			t.Fatal("tampered site session was not cleared")
		}
	})
	t.Run("signed out after session", func(t *testing.T) {
		browser := newSiteBrowser(t, mux)
		browser.signIn("/about")
		if signOut := browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
			t.Fatalf("sign-out status = %d", signOut.Code)
		}
		if browser.hasCookie(config.SiteSessionCookieName) {
			t.Fatal("sign-out left the site session in the browser")
		}
		assertPublicPagesAvailable(t, browser, false)
	})
	t.Run("sign-in unconfigured", func(t *testing.T) {
		unconfigured := newTestApp(t)
		unconfiguredMux, _ := buildMux(unconfigured, unconfigured.Logger, false)
		browser := newSiteBrowser(t, unconfiguredMux)
		browser.setCookie(config.SiteSessionCookieName, "left-from-another-deployment")
		assertPublicPagesAvailable(t, browser, false)
	})
}

func TestUninvitedIdentityGetsNoSessionOrPageGrant(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	browser := newSiteBrowser(t, mux)

	fixture.email = "uninvited@example.com"
	denied := browser.signIn("/about")
	if denied.Code != http.StatusUnauthorized || !strings.Contains(denied.Body.String(), "Sign-in could not be completed.") {
		t.Fatalf("uninvited identity was not denied: %d", denied.Code)
	}
	if browser.hasCookie(config.SiteSessionCookieName) {
		t.Fatal("browser kept a site session after an uninvited sign-in")
	}

	var sawPrincipal, sawGrant bool
	probe := application.SiteHandler.WithIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawPrincipal = siteidentity.PrincipalFromContext(r.Context())
		sawGrant = siteidentity.HasGrant(r.Context(), siteidentity.GrantSoccer) || siteidentity.HasGrant(r.Context(), siteidentity.GrantManagement)
	}))
	browser.handler = probe
	browser.get("/private")
	if sawPrincipal || sawGrant {
		t.Fatalf("uninvited browser principal = %t, grant = %t", sawPrincipal, sawGrant)
	}
}

func TestSignedInStaticAssetsRemainCacheable(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	browser := newSiteBrowser(t, mux)
	browser.signIn("/about")
	t.Chdir("../..")

	asset := browser.get("/static/images/favicon.svg")
	if asset.Code != http.StatusOK {
		t.Fatalf("static asset status = %d", asset.Code)
	}
	if got := asset.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("signed-in static asset Cache-Control = %q, want the file server default", got)
	}
}
