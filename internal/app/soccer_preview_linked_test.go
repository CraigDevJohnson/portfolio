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

	"portfolio/internal/config"
	"portfolio/internal/testutil"
)

// newPreviewLinkedBrowser serves the loopback preview route assembly with
// every configured external URL pointing at a fake that fails the test, so
// the linked-player preview journey must stay in process.
func newPreviewLinkedBrowser(t *testing.T, preview bool) *siteBrowser {
	t.Helper()
	application := newTestApp(t)
	offLimits := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the preview linked journey reached the configured LPS at %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(offLimits.Close)
	application.Config.LPSAPIBaseURL = offLimits.URL
	mux, _ := buildMux(application, slog.New(slog.NewTextHandler(io.Discard, nil)), preview)
	return newSiteBrowser(t, mux)
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
	download := linkedFormValues(t, results, "upcoming-games-form")
	download["selected"] = []string{"7003", "7002"}
	ics := browser.postForm("/soccer/download", download)
	if events := icsEvents(testutil.UnfoldICS(ics.Body.String())); ics.Code != http.StatusOK || len(events) != 2 || events["7003"] == "" || events["7002"] == "" {
		t.Fatalf("preview linked .ics: status %d, body %q", ics.Code, ics.Body.String())
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
