package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
)

// The Google accounts the fake Google lets the visitor choose during consent.
// The site account matches the fake Cognito sign-in email; the alternate is
// another Google account the visitor may prefer for Calendar.
const (
	journeySiteAccount      = "owner@example.com"
	journeyAlternateAccount = "family.calendar@example.net"
)

// googleAccountJourney is the real route assembly behind fake Cognito sign-in
// and a fake Google that grants Calendar access to whichever account the
// visitor chose on Google's consent page, identified by the authorization
// code Google returns. Any other Google request, such as a grant revocation,
// fails the test.
type googleAccountJourney struct {
	cognito *fakeSiteCognito
	app     *App
	mux     http.Handler
	store   *appTestGoogleConnectionStore
	browser *siteBrowser
}

func newGoogleAccountJourney(t *testing.T) *googleAccountJourney {
	t.Helper()
	journey := &googleAccountJourney{cognito: newFakeSiteCognito(t)}
	journey.app = journey.cognito.app(t)
	journey.app.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	journey.app.Config.GoogleClientID = "google-client"
	journey.app.Config.GoogleClientSecret = "google-secret"
	journey.app.Config.GoogleConnectionTableName = "connections"
	journey.store = &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	journey.app.GoogleHandler.SetStore(journey.store)

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			// The authorization code names the account chosen at Google.
			if err := r.ParseForm(); err != nil {
				t.Errorf("token request form: %v", err)
			}
			account := r.PostForm.Get("code")
			_, _ = fmt.Fprintf(w, `{"access_token":"access:%s","refresh_token":"refresh:%s","token_type":"Bearer","expires_in":3600}`, account, account)
		case "/userinfo":
			account := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer access:")
			_, _ = fmt.Fprintf(w, `{"sub":"google-%s","email":%q,"email_verified":true}`, account, account)
		case "/calendar/v3/users/me/calendarList":
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true,"accessRole":"owner"}]}`))
		default:
			t.Errorf("unexpected Google request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(google.Close)
	journey.app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	journey.app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	journey.app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	journey.app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	journey.mux, _ = buildMux(journey.app, journey.app.Logger, false)
	journey.browser = newSiteBrowser(t, journey.mux)
	return journey
}

// on returns the journey continued in another browser against the same site
// and the same fake Google.
func (journey *googleAccountJourney) on(browser *siteBrowser) *googleAccountJourney {
	other := *journey
	other.browser = browser
	return &other
}

// startConsent follows a Google Calendar connect control to Google's consent
// page and returns that page's address.
func (journey *googleAccountJourney) startConsent(t *testing.T, href string) *url.URL {
	t.Helper()
	start := journey.browser.get(href)
	consent, err := url.Parse(start.Header().Get("Location"))
	if start.Code != http.StatusSeeOther || err != nil || !strings.HasPrefix(consent.String(), journey.app.GoogleHandler.OAuthAuthURL+"?") {
		t.Fatalf("%s did not start Google consent: %d %q", href, start.Code, start.Header().Get("Location"))
	}
	return consent
}

// consentAs completes Google consent as the chosen account and returns the
// app's response to Google's callback.
func (journey *googleAccountJourney) consentAs(t *testing.T, consent *url.URL, account string) *httptest.ResponseRecorder {
	t.Helper()
	return journey.browser.get("/soccer?code=" + url.QueryEscape(account) + "&state=" + url.QueryEscape(consent.Query().Get("state")))
}

func TestGrantedVisitorConnectsGoogleWithTheSuggestedSiteAccountOrAnother(t *testing.T) {
	journey := newGoogleAccountJourney(t)
	journey.browser.signIn("/soccer")

	page := journey.browser.get("/soccer").Body.String()
	for _, want := range []string{"Not connected", "Google will suggest", journeySiteAccount, `href="/soccer/google/connect"`, `href="/soccer/google/connect?account=choose"`} {
		if !strings.Contains(page, want) {
			t.Errorf("disconnected Google card lacks %q", want)
		}
	}

	suggested := journey.startConsent(t, "/soccer/google/connect")
	if got := suggested.Query().Get("login_hint"); got != journeySiteAccount {
		t.Errorf("suggested consent login_hint = %q, want the site account %q", got, journeySiteAccount)
	}
	for _, scope := range []string{"openid", "email", "https://www.googleapis.com/auth/calendar.events"} {
		if !strings.Contains(suggested.Query().Get("scope"), scope) {
			t.Errorf("Google consent scope %q lacks %q", suggested.Query().Get("scope"), scope)
		}
	}

	another := journey.startConsent(t, "/soccer/google/connect?account=choose")
	if got := another.Query().Get("login_hint"); got != "" {
		t.Errorf("another-account consent suggested %q instead of letting the visitor choose", got)
	}
	if prompt := another.Query().Get("prompt"); !strings.Contains(prompt, "select_account") {
		t.Errorf("another-account consent prompt = %q, want Google's account chooser", prompt)
	}
}

// holdsGoogleConnectionCookie reports whether the browser would send the
// site owner's Google connection cookie to the Soccer page.
func (journey *googleAccountJourney) holdsGoogleConnectionCookie() bool {
	soccer, _ := url.Parse("https://app.example.com/soccer")
	owner := internalgoogle.ConnectionCookieName(journey.cognito.issuer, journey.cognito.subject)
	for _, cookie := range journey.browser.jar.Cookies(soccer) {
		if cookie.Name == owner {
			return true
		}
	}
	return false
}

// connectedAccount returns the Google account the Soccer page reports as
// connected, or "" when it reports no connection.
func (journey *googleAccountJourney) connectedAccount(t *testing.T) string {
	t.Helper()
	page := journey.browser.get("/soccer")
	if page.Code != http.StatusOK {
		t.Fatalf("Soccer page status = %d", page.Code)
	}
	body := page.Body.String()
	const marker = `<strong data-google-account>`
	start := strings.Index(body, marker)
	if start < 0 {
		if strings.Contains(body, "Calendar ready") {
			t.Fatal("Soccer page reported a ready calendar without naming its Google account")
		}
		return ""
	}
	account, _, closed := strings.Cut(body[start+len(marker):], "</strong>")
	if !closed {
		t.Fatal("Soccer page left the connected Google account unclosed")
	}
	return account
}

func TestChosenGoogleAccountStaysWithItsSiteOwnerUntilChanged(t *testing.T) {
	journey := newGoogleAccountJourney(t)
	journey.browser.signIn("/soccer")

	// The visitor declines the suggestion and consents as another account.
	callback := journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect?account=choose"), journeyAlternateAccount)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?google=connected" {
		t.Fatalf("Google callback = %d %q", callback.Code, callback.Header().Get("Location"))
	}
	if got := journey.connectedAccount(t); got != journeyAlternateAccount {
		t.Fatalf("connected account shown = %q, want the account that consented %q", got, journeyAlternateAccount)
	}
	if page := journey.browser.get("/soccer").Body.String(); !strings.Contains(page, "Switch to site account") || !strings.Contains(page, journeySiteAccount) {
		t.Error("page did not offer the site account as an alternative to the connected account")
	}

	// Site sign-out hides the connection; the next sign-in as the same owner
	// finds the chosen account still connected.
	if signOut := journey.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	if page := journey.browser.get("/soccer").Body.String(); strings.Contains(page, journeyAlternateAccount) || strings.Contains(page, "Calendar ready") {
		t.Error("signed-out Soccer page showed the owner's Google connection")
	}
	journey.browser.signIn("/soccer")
	if got := journey.connectedAccount(t); got != journeyAlternateAccount {
		t.Fatalf("after signing in again the connected account = %q, want %q", got, journeyAlternateAccount)
	}

	// Changing to the suggested site account replaces the connection.
	journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect"), journeySiteAccount)
	if got := journey.connectedAccount(t); got != journeySiteAccount {
		t.Fatalf("after changing account the connected account = %q, want %q", got, journeySiteAccount)
	}
	if len(journey.store.records) != 1 {
		t.Errorf("changing the Google account left %d stored connections, want 1", len(journey.store.records))
	}
	if page := journey.browser.get("/soccer").Body.String(); strings.Contains(page, "Switch to site account") {
		t.Error("page offered to switch to the site account that is already connected")
	}
}

func TestDisconnectRemovesTheOwnersGoogleConnectionWhileSignOutKeepsIt(t *testing.T) {
	journey := newGoogleAccountJourney(t)
	journey.browser.signIn("/soccer")
	journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect"), journeySiteAccount)

	if signOut := journey.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	if len(journey.store.records) != 1 || !journey.holdsGoogleConnectionCookie() {
		t.Fatalf("site sign-out removed the Google connection: %d stored", len(journey.store.records))
	}

	journey.browser.signIn("/soccer")
	disconnect := journey.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/soccer/google/disconnect", nil))
	if disconnect.Code != http.StatusOK || !strings.Contains(disconnect.Body.String(), "Not connected") {
		t.Fatalf("disconnect = %d %q", disconnect.Code, disconnect.Body.String())
	}
	if len(journey.store.records) != 0 {
		t.Errorf("disconnect left %d stored Google connections", len(journey.store.records))
	}
	if journey.holdsGoogleConnectionCookie() {
		t.Error("disconnect left the Google connection cookie in the browser")
	}
	if got := journey.connectedAccount(t); got != "" {
		t.Errorf("after disconnect the page still reports %q connected", got)
	}
}

// A second invited site owner who shares the family's Google account.
const (
	journeySecondOwnerEmail   = "second.owner@example.com"
	journeySecondOwnerSubject = "second-owner-subject"
)

// signInAs signs the journey's browser in as another invited site owner.
func (journey *googleAccountJourney) signInAs(t *testing.T, email, subject string) {
	t.Helper()
	journey.app.Config.SiteInvitations[email] = []string{"soccer"}
	previousEmail, previousSubject := journey.cognito.email, journey.cognito.subject
	journey.cognito.email, journey.cognito.subject = email, subject
	defer func() { journey.cognito.email, journey.cognito.subject = previousEmail, previousSubject }()
	journey.browser.signIn("/soccer")
}

func TestDisconnectLeavesOtherConnectionsToTheSameGoogleAccountConnected(t *testing.T) {
	for _, other := range []struct {
		name   string
		signIn func(t *testing.T, journey *googleAccountJourney)
	}{
		{name: "same owner on another device", signIn: func(_ *testing.T, journey *googleAccountJourney) { journey.browser.signIn("/soccer") }},
		{name: "another site owner", signIn: func(t *testing.T, journey *googleAccountJourney) {
			journey.signInAs(t, journeySecondOwnerEmail, journeySecondOwnerSubject)
		}},
	} {
		t.Run(other.name, func(t *testing.T) {
			laptop := newGoogleAccountJourney(t)
			laptop.browser.signIn("/soccer")
			phone := laptop.on(newSiteBrowser(t, laptop.mux))
			other.signIn(t, phone)
			// Both connect the shared family Google account.
			for _, device := range []*googleAccountJourney{laptop, phone} {
				device.consentAs(t, device.startConsent(t, "/soccer/google/connect?account=choose"), journeyAlternateAccount)
				if got := device.connectedAccount(t); got != journeyAlternateAccount {
					t.Fatalf("connected account = %q, want %q", got, journeyAlternateAccount)
				}
			}

			disconnect := laptop.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/soccer/google/disconnect", nil))
			if disconnect.Code != http.StatusOK || !strings.Contains(disconnect.Body.String(), "Not connected") {
				t.Fatalf("disconnect = %d %q", disconnect.Code, disconnect.Body.String())
			}
			if got := laptop.connectedAccount(t); got != "" {
				t.Errorf("after disconnect the laptop still reports %q connected", got)
			}
			if got := phone.connectedAccount(t); got != journeyAlternateAccount {
				t.Errorf("disconnecting the laptop left the other connection showing %q, want %q still connected", got, journeyAlternateAccount)
			}
		})
	}
}

// signOut ends the browser's site session.
func (journey *googleAccountJourney) signOut(t *testing.T) {
	t.Helper()
	if signOut := journey.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
}

func TestSiteOwnersSharingABrowserEachKeepTheirGoogleConnection(t *testing.T) {
	journey := newGoogleAccountJourney(t)
	journey.browser.signIn("/soccer")
	journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect?account=choose"), journeyAlternateAccount)
	journey.signOut(t)

	// A second owner signs in on the same browser and connects their own
	// Google account.
	journey.signInAs(t, journeySecondOwnerEmail, journeySecondOwnerSubject)
	if got := journey.connectedAccount(t); got != "" {
		t.Fatalf("second site owner saw the first owner's Google account %q", got)
	}
	journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect"), journeySecondOwnerEmail)
	if got := journey.connectedAccount(t); got != journeySecondOwnerEmail {
		t.Fatalf("second site owner's connected account = %q, want %q", got, journeySecondOwnerEmail)
	}
	journey.signOut(t)

	journey.browser.signIn("/soccer")
	if got := journey.connectedAccount(t); got != journeyAlternateAccount {
		t.Errorf("first site owner returned to connected account %q, want %q", got, journeyAlternateAccount)
	}
	journey.signOut(t)
	journey.signInAs(t, journeySecondOwnerEmail, journeySecondOwnerSubject)
	if got := journey.connectedAccount(t); got != journeySecondOwnerEmail {
		t.Errorf("second site owner returned to connected account %q, want %q", got, journeySecondOwnerEmail)
	}
	if len(journey.store.records) != 2 {
		t.Errorf("the two owners' connections left %d stored rows, want 2", len(journey.store.records))
	}
}

func TestAFailedGoogleConnectionReadKeepsTheOwnersConnection(t *testing.T) {
	world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
	cookies := []*http.Cookie{
		testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail),
		{Name: ownerGoogleConnectionName, Value: grantWorldConnectionID},
	}
	world.store.getErr = errors.New("connection table unavailable")
	for _, request := range []struct {
		method, path string
		form         url.Values
	}{
		{method: http.MethodGet, path: "/soccer"},
		{method: http.MethodPost, path: "/soccer/fetch", form: url.Values{"team_codes": {"4101"}}},
	} {
		response := soccerGrantRequest(world.mux, request.method, request.path, request.form, cookies...)
		if response.Code != http.StatusOK {
			t.Errorf("%s %s status = %d during a failed connection read", request.method, request.path, response.Code)
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == ownerGoogleConnectionName && cookie.MaxAge < 0 {
				t.Errorf("%s %s cleared the owner's Google connection cookie after a failed read", request.method, request.path)
			}
		}
	}

	world.store.getErr = nil
	if page := soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil, cookies...); !strings.Contains(page.Body.String(), "Calendar ready") {
		t.Error("the owner's Google connection did not return once the connection table recovered")
	}
}

func TestOwnerConnectionWithoutAVerifiedGoogleAccountNeedsReconnection(t *testing.T) {
	for _, action := range []struct {
		name, method, path string
	}{
		{name: "disconnect", method: http.MethodPost, path: "/soccer/google/disconnect"},
		{name: "reconnect", method: http.MethodGet, path: "/soccer/google/connect"},
	} {
		t.Run(action.name, func(t *testing.T) {
			world := newSoccerGrantWorld(t, map[string][]string{testSiteEmail: {"soccer"}})
			// A connection saved before consent recorded the Google account.
			unverified := world.store.records[grantWorldConnectionID]
			unverified.AccountSubject, unverified.AccountEmail = "", ""
			world.store.records[grantWorldConnectionID] = unverified
			cookies := []*http.Cookie{
				testSiteSessionCookie(t, world.app, testSiteSubject, testSiteEmail),
				{Name: config.GoogleConnectionCookieName, Value: grantWorldConnectionID},
			}

			page := soccerGrantRequest(world.mux, http.MethodGet, "/soccer", nil, cookies...)
			if body := page.Body.String(); strings.Contains(body, "Calendar ready") || !strings.Contains(body, "Connect Google Calendar") {
				t.Error("page presented a connection whose Google account was never verified")
			}
			if calls := world.googleCalls.Load(); calls != 0 {
				t.Errorf("page used the unverified connection's token %d time(s)", calls)
			}

			soccerGrantRequest(world.mux, action.method, action.path, nil, cookies...)
			if _, kept := world.store.records[grantWorldConnectionID]; kept {
				t.Errorf("%s left the owner's unverified connection and its token stored", action.name)
			}
		})
	}
}

func TestGooglePreviewFixturesShowTheSuggestedAndConnectedAccountsInertly(t *testing.T) {
	application := newTestApp(t)
	preview, _ := buildMux(application, application.Logger, true)
	for _, fixture := range []struct {
		name          string
		present, gone []string
	}{
		{
			name: "google-disconnected",
			present: []string{
				"Not connected", "Google will suggest <strong data-google-suggested-account>site@example.com</strong>",
				`<button type="button" class="btn btn-primary" disabled aria-disabled="true">Connect Google Calendar</button>`,
				`<button type="button" class="btn soccer-secondary-btn" disabled aria-disabled="true">Use another Google account</button>`,
			},
			gone: []string{"data-google-account>", "Calendar ready"},
		},
		{
			name: "google-connected",
			present: []string{
				"Calendar ready", "Connected Google account: <strong data-google-account>calendar@example.com</strong>",
				"Your site sign-in account, <strong>site@example.com</strong>, is a different Google account.",
				`<button type="button" class="btn soccer-secondary-btn" disabled aria-disabled="true">Switch to site account</button>`,
				`<button type="button" class="btn soccer-secondary-btn" disabled aria-disabled="true">Change Google account</button>`,
			},
			gone: []string{"data-google-suggested-account"},
		},
	} {
		body := soccerGrantRequest(preview, http.MethodGet, "/__preview/soccer/"+fixture.name, nil).Body.String()
		for _, marker := range fixture.present {
			if !strings.Contains(body, marker) {
				t.Errorf("%s preview lacks %q", fixture.name, marker)
			}
		}
		for _, marker := range append(fixture.gone, `href="/soccer/google/connect`) {
			if strings.Contains(body, marker) {
				t.Errorf("%s preview shows %q", fixture.name, marker)
			}
		}
	}
}
