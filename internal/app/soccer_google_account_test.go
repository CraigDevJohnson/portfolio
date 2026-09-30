package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
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
// code Google returns.
type googleAccountJourney struct {
	cognito *fakeSiteCognito
	app     *App
	store   *appTestGoogleConnectionStore
	browser *siteBrowser

	mu      sync.Mutex
	revoked []string
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
		case "/revoke":
			if err := r.ParseForm(); err != nil {
				t.Errorf("revoke request form: %v", err)
			}
			journey.mu.Lock()
			journey.revoked = append(journey.revoked, r.PostForm.Get("token"))
			journey.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected Google request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(google.Close)
	journey.app.GoogleHandler.OAuthAuthURL = google.URL + "/oauth/authorize"
	journey.app.GoogleHandler.OAuthTokenURL = google.URL + "/oauth/token"
	journey.app.GoogleHandler.OAuthUserInfoURL = google.URL + "/userinfo"
	journey.app.GoogleHandler.OAuthRevokeURL = google.URL + "/revoke"
	journey.app.GoogleHandler.CalendarAPIBaseURL = google.URL + "/calendar/v3"

	mux, _ := buildMux(journey.app, journey.app.Logger, false)
	journey.browser = newSiteBrowser(t, mux)
	return journey
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

// holdsGoogleConnectionCookie reports whether the browser would send a Google
// connection cookie to the Soccer page.
func (journey *googleAccountJourney) holdsGoogleConnectionCookie() bool {
	soccer, _ := url.Parse("https://app.example.com/soccer")
	for _, cookie := range journey.browser.jar.Cookies(soccer) {
		if cookie.Name == config.GoogleConnectionCookieName {
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
	rest := body[start+len(marker):]
	return rest[:strings.Index(rest, "</strong>")]
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

func TestDisconnectRevokesTheOwnersGoogleAccessWhileSignOutKeepsIt(t *testing.T) {
	journey := newGoogleAccountJourney(t)
	journey.browser.signIn("/soccer")
	journey.consentAs(t, journey.startConsent(t, "/soccer/google/connect"), journeySiteAccount)

	if signOut := journey.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
		t.Fatalf("site sign-out status = %d", signOut.Code)
	}
	if len(journey.store.records) != 1 || len(journey.revoked) != 0 || !journey.holdsGoogleConnectionCookie() {
		t.Fatalf("site sign-out removed the Google connection: %d stored, %d revoked", len(journey.store.records), len(journey.revoked))
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
	if want := "refresh:" + journeySiteAccount; len(journey.revoked) != 1 || journey.revoked[0] != want {
		t.Errorf("Google received revocations %q, want only the connection's refresh token %q", journey.revoked, want)
	}
	if got := journey.connectedAccount(t); got != "" {
		t.Errorf("after disconnect the page still reports %q connected", got)
	}
}
