package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
)

// newPreviewLinkedBrowser serves the loopback preview route assembly with
// every configured external URL, LPS and Google alike, pointing at a fake
// that fails the test, so the preview account journeys must stay in process.
func newPreviewLinkedBrowser(t *testing.T, preview bool) *siteBrowser {
	t.Helper()
	application := newTestApp(t)
	offLimits := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the preview account journey reached a configured external service at %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(offLimits.Close)
	application.Config.LPSAPIBaseURL = offLimits.URL
	application.GoogleHandler.OAuthAuthURL = offLimits.URL + "/oauth/authorize"
	application.GoogleHandler.OAuthTokenURL = offLimits.URL + "/oauth/token"
	application.GoogleHandler.OAuthUserInfoURL = offLimits.URL + "/userinfo"
	application.GoogleHandler.CalendarAPIBaseURL = offLimits.URL + "/calendar/v3"
	mux, _ := buildMux(application, slog.New(slog.NewTextHandler(io.Discard, nil)), preview)
	return newSiteBrowser(t, mux)
}

// googleOutputOption returns the Soccer page's Google Calendar output choice.
func googleOutputOption(t *testing.T, doc *html.Node) *html.Node {
	t.Helper()
	return plannerSingle(t, doc, "Google output option", func(node *html.Node) bool {
		return soccerHTMLAttribute(node, "name") == "calendar_output" && soccerHTMLAttribute(node, "value") == "google"
	})
}

// The preview's Google account is the granted preview account on a server
// that offers Google Calendar: its Google output is a real choice, so a
// browser proof can switch to Google mode without editing the page.
func TestPreviewGoogleAccountIsOfferedGoogleOutput(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)

	entry := browser.get("/__preview/account/soccer-google")
	if entry.Code != http.StatusSeeOther || entry.Header().Get("Location") != "/soccer" {
		t.Fatalf("preview Google entry: status %d, Location %q", entry.Code, entry.Header().Get("Location"))
	}
	body := browser.get("/soccer").Body.String()
	if !strings.Contains(body, "invited.visitor@example.com") || strings.Contains(body, "Private Soccer access") {
		t.Fatal("the preview Google page is not the granted preview account")
	}
	if plannerHasAttr(googleOutputOption(t, parsePlannerHTML(t, body)), "disabled") {
		t.Fatal("the preview Google account is not offered Google output")
	}

	// A Team ID lookup reviews the scored past games in Google mode beside
	// the connect prompt a disconnected account sees.
	results := parsePlannerHTML(t, browser.postForm("/soccer/fetch", url.Values{"team_codes": {"479691, 479147"}}).Body.String())
	if past := plannerRowIDs(plannerGameRows(results, "past-results")); !slices.Equal(past, previewPastResultsNewestFirst) {
		t.Fatalf("preview Google past rows = %v, want %v", past, previewPastResultsNewestFirst)
	}
	assertPastResultControlsGoogleOnly(t, results)
	connect := plannerSingle(t, results, "Google connect prompt", plannerAttrIs("href", "/soccer/google/connect"))
	if gate := plannerOutputGate(connect); gate == nil || soccerHTMLAttribute(gate, "data-soccer-output-only") != "google" {
		t.Error("the Google connect prompt is not limited to Google mode")
	}
	if text := plannerText(results); strings.Contains(text, "unavailable in this environment") {
		t.Errorf("the preview Google lookup says Google is unavailable: %q", text)
	}
}

func TestPreviewLinkedAccountDrivesTheRealLinkedPlayerRoutes(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)

	entry := browser.get("/__preview/account/soccer-linked")
	if entry.Code != http.StatusSeeOther || entry.Header().Get("Location") != "/soccer" {
		t.Fatalf("preview linked entry: status %d, Location %q", entry.Code, entry.Header().Get("Location"))
	}
	page := browser.get("/soccer")
	if body := page.Body.String(); !strings.Contains(body, "invited.visitor@example.com") || strings.Contains(body, "Private Soccer access") {
		t.Fatal("the preview linked page is not the granted preview account")
	}

	jwt := testutil.TestJWT(t, time.Now().Add(time.Hour))
	if imported := browser.postForm("/soccer/import", url.Values{"jwt": {jwt}}); imported.Code != http.StatusOK || !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("preview import: status %d, body %q", imported.Code, imported.Body.String())
	}
	players := parsePlannerHTML(t, browser.get("/soccer").Body.String())
	if got := linkedOptionLabels(players, "player_ids"); !slices.Equal(got, []string{"Craig Johnson Primary player", "Taylor Alexandra Johnson-Summit"}) {
		t.Fatalf("preview linked players = %q", got)
	}

	taylor := parsePlannerHTML(t, browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1669081"}}).Body.String())
	if got := linkedOptionLabels(taylor, "team_ids"); !slices.Equal(got, []string{"Campfire Rovers Season 170"}) {
		t.Fatalf("preview teams for Taylor = %q", got)
	}
	both := parsePlannerHTML(t, browser.postForm("/soccer/discover-teams", linkedFormValues(t, players, "soccer-player-select-form")).Body.String())
	if got := linkedOptionLabels(both, "team_ids"); !slices.Equal(got, []string{"Pond Mint United Season 169", "Campfire Rovers Season 170"}) {
		t.Fatalf("preview teams for both players = %q", got)
	}

	results := parsePlannerHTML(t, browser.postForm("/soccer/fetch", linkedFormValues(t, both, "soccer-team-select-form")).Body.String())
	if got := plannerRowIDs(plannerGameRows(results, "upcoming-games")); !slices.Equal(got, []string{"7003", "7001", "7002"}) {
		t.Fatalf("preview linked rows = %v, want [7003 7001 7002]", got)
	}
	if past := plannerRowIDs(plannerGameRows(results, "past-results")); !slices.Equal(past, previewPastResultsNewestFirst) {
		t.Fatalf("preview linked past rows = %v, want the scored past games newest first %v for Google mode", past, previewPastResultsNewestFirst)
	}
	assertPastResultControlsGoogleOnly(t, results)
	download := linkedFormValues(t, results, "upcoming-games-form")
	download["selected"] = []string{"7003", "7002"}
	ics := browser.postForm("/soccer/download", download)
	if events := icsEvents(testutil.UnfoldICS(ics.Body.String())); ics.Code != http.StatusOK || len(events) != 2 || events["7003"] == "" || events["7002"] == "" {
		t.Fatalf("preview linked .ics: status %d, body %q", ics.Code, ics.Body.String())
	}

	// The granted preview account's Team ID lookup of the same teams reviews
	// the same scored past games in Google mode.
	teamIDs := parsePlannerHTML(t, browser.postForm("/soccer/fetch", url.Values{"team_codes": {"479691, 479147"}}).Body.String())
	if past := plannerRowIDs(plannerGameRows(teamIDs, "past-results")); !slices.Equal(past, previewPastResultsNewestFirst) {
		t.Fatalf("preview Team ID past rows = %v, want the scored past games newest first %v for Google mode", past, previewPastResultsNewestFirst)
	}
	assertPastResultControlsGoogleOnly(t, teamIDs)
}

// The preview's Google account has no Google connection, so every Google
// Calendar route it can reach, including the connect prompt its page offers,
// answers in the preview and never reaches Google.
func TestPreviewGoogleAccountNeverReachesGoogle(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)
	browser.get("/__preview/account/soccer-google")

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "https://app.example.com/soccer/google/connect", nil),
		httptest.NewRequest(http.MethodGet, "https://app.example.com/soccer/google/connect?account=choose", nil),
		browserForm(siteOrigin, "/soccer/google/add", url.Values{"team_codes": {"479691"}, "selected": {"7003"}}),
		browserForm(siteOrigin, "/soccer/google/sync-results", url.Values{"team_codes": {"479691"}, "selected": {"7000"}}),
		browserForm(siteOrigin, "/soccer/google/calendar", url.Values{"calendar_id": {"primary"}}),
		browserForm(siteOrigin, "/soccer/google/disconnect", nil),
	} {
		target := request.Method + " " + request.URL.RequestURI()
		answer := browser.do(request)
		if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), "Preview only") {
			t.Errorf("%s: status %d, body %q; want the preview-only answer", target, answer.Code, answer.Body.String())
		}
		if location := answer.Header().Get("Location"); location != "" {
			t.Errorf("%s sent the browser to %q", target, location)
		}
		if cache := answer.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: Cache-Control %q, want no-store", target, cache)
		}
	}
}

func TestPreviewLinkedAccountIsOnlyForItsOwnBrowserInThePreview(t *testing.T) {
	discover := url.Values{"player_ids": {"1669080"}}
	marked := &http.Cookie{Name: "preview_soccer_account", Value: "linked", Path: config.SoccerCookiePath}

	t.Run("outside the preview", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, false)
		if entry := browser.get("/__preview/account/soccer-linked"); entry.Code != http.StatusNotFound {
			t.Errorf("preview linked entry outside the preview: status %d, want 404", entry.Code)
		}
		soccer, _ := url.Parse("https://app.example.com/soccer")
		browser.jar.SetCookies(soccer, []*http.Cookie{marked})
		if refused := browser.postForm("/soccer/discover-teams", discover); refused.Code != http.StatusUnauthorized {
			t.Errorf("marked request outside the preview: status %d, want 401", refused.Code)
		}
	})
	t.Run("another preview browser", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, true)
		if refused := browser.postForm("/soccer/discover-teams", discover); refused.Code != http.StatusUnauthorized {
			t.Errorf("unmarked preview request: status %d, want 401", refused.Code)
		}
		if refused := browser.postForm("/soccer/import", url.Values{"jwt": {testutil.TestJWT(t, time.Now().Add(time.Hour))}}); refused.Code != http.StatusUnauthorized {
			t.Errorf("unmarked preview import: status %d, want 401", refused.Code)
		}
	})
}

// Google output is offered only to a preview browser that opened the Google
// account: never outside the preview, never to another preview browser, and
// not to the linked account, which keeps Google Calendar off.
func TestPreviewGoogleAccountIsOnlyForItsOwnBrowserInThePreview(t *testing.T) {
	soccer, _ := url.Parse("https://app.example.com/soccer")
	marked := &http.Cookie{Name: "preview_soccer_account", Value: "google", Path: config.SoccerCookiePath}
	googleOffered := func(t *testing.T, browser *siteBrowser) bool {
		t.Helper()
		return !plannerHasAttr(googleOutputOption(t, parsePlannerHTML(t, browser.get("/soccer").Body.String())), "disabled")
	}

	t.Run("outside the preview", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, false)
		if entry := browser.get("/__preview/account/soccer-google"); entry.Code != http.StatusNotFound {
			t.Errorf("preview Google entry outside the preview: status %d, want 404", entry.Code)
		}
		browser.jar.SetCookies(soccer, []*http.Cookie{marked})
		if googleOffered(t, browser) {
			t.Error("a marked browser outside the preview is offered Google output")
		}
		if refused := browser.get("/soccer/google/connect"); refused.Code != http.StatusUnauthorized {
			t.Errorf("marked Google connect outside the preview: status %d, want 401", refused.Code)
		}
	})
	t.Run("another preview browser", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, true)
		if googleOffered(t, browser) {
			t.Error("an unmarked preview browser is offered Google output")
		}
		if refused := browser.get("/soccer/google/connect"); refused.Code != http.StatusUnauthorized {
			t.Errorf("unmarked preview Google connect: status %d, want 401", refused.Code)
		}
	})
	t.Run("the linked account", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, true)
		browser.get("/__preview/account/soccer-linked")
		if googleOffered(t, browser) {
			t.Error("the linked preview account is offered Google output")
		}
	})
}

func TestPreviewLinkedLPSCanWithdrawAnImportedToken(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)
	browser.get("/__preview/account/soccer-linked")
	// The preview LPS accepts a token signed "revoked" for the import, then
	// refuses its team lookups, as when LPS withdraws access after an import.
	parts := strings.Split(testutil.TestJWT(t, time.Now().Add(time.Hour)), ".")
	revoked := parts[0] + "." + parts[1] + ".revoked"
	if imported := browser.postForm("/soccer/import", url.Values{"jwt": {revoked}}); !strings.Contains(imported.Body.String(), `name="player_ids"`) {
		t.Fatalf("preview import of the withdrawn token: status %d, body %q", imported.Code, imported.Body.String())
	}

	lost := parsePlannerHTML(t, browser.postForm("/soccer/discover-teams", url.Values{"player_ids": {"1669081"}}).Body.String())

	_, notice := linkedAccessNotice(t, lost)
	if !strings.Contains(notice, "Your imported Let's Play Soccer token was rejected.") {
		t.Errorf("preview withdrawn-token notice = %q", notice)
	}
	if browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Error("the preview kept the withdrawn import")
	}
}

// The linked preview shares the production Soccer route table, so its import
// refuses a page on another origin just as production does, and only the
// Soccer page itself can replace the preview account's imported access.
func TestPreviewLinkedImportRefusesOtherOrigins(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)
	browser.get("/__preview/account/soccer-linked")
	form := url.Values{"jwt": {testutil.TestJWT(t, time.Now().Add(time.Hour))}}

	if refused := browser.do(browserForm(anotherOrigin, "/soccer/import", form)); refused.Code != http.StatusForbidden {
		t.Errorf("cross-site preview import: status %d, want 403", refused.Code)
	}
	if browser.holdsCookie(config.LPSSessionCookieName, "/soccer") {
		t.Fatal("a cross-site preview import stored imported access")
	}
	if accepted := browser.do(browserForm(siteOrigin, "/soccer/import", form)); accepted.Code != http.StatusOK || !strings.Contains(accepted.Body.String(), `name="player_ids"`) {
		t.Fatalf("same-origin preview import: status %d, body %q", accepted.Code, accepted.Body.String())
	}
}
