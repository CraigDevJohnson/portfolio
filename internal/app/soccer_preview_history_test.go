package app

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/testutil"
)

// The preview's history account is the granted preview account on a server
// whose history store is wired with a fixture that reaches every Team
// history state, without AWS or a live LPS.
func TestPreviewHistoryAccountReachesEveryTeamHistoryState(t *testing.T) {
	browser := newPreviewLinkedBrowser(t, true)

	entry := browser.get("/__preview/account/soccer-history")
	if entry.Code != http.StatusSeeOther || entry.Header().Get("Location") != "/soccer" {
		t.Fatalf("preview history entry: status %d, Location %q", entry.Code, entry.Header().Get("Location"))
	}
	if imported := browser.postForm("/soccer/import", url.Values{"jwt": {testutil.TestJWT(t, time.Now().Add(time.Hour))}}); imported.Code != http.StatusOK {
		t.Fatalf("preview import: status %d, body %q", imported.Code, imported.Body.String())
	}
	section := historySection(t, parsePlannerHTML(t, browser.get("/soccer").Body.String()))
	if plannerHasAttr(section, "hidden") {
		t.Fatal("the preview history account's page hides Team history")
	}
	buttons := plannerElements(section, hasHistoryAttr("data-soccer-team-history-player"))
	players := make([]string, 0, len(buttons))
	for _, button := range buttons {
		players = append(players, soccerHTMLAttribute(button, "data-soccer-team-history-player"))
	}
	if want := []string{"1669080", "1669081", "1669082", "1669083"}; !slices.Equal(players, want) {
		t.Fatalf("preview players = %q, want %q", players, want)
	}

	craig := readHistoryView(t, browser, historyViewPath(1669080))
	if got, want := historyViewSeasons(craig), "479691/169 current, 479801/168 former, 479802/167 former, 479800/166 former"; got != want {
		t.Errorf("Craig's seasons = %q, want %q", got, want)
	}
	if got := historyViewOpen(t, craig); got != "479691/169" {
		t.Errorf("Craig's open season = %q, want 479691/169", got)
	}
	record := plannerText(plannerSingle(t, craig, "record", hasHistoryAttr("data-soccer-team-history-record")))
	for _, want := range []string{"1 W · 1 L · 1 D", "from 3 scored games", "1 completed game not counted."} {
		if !strings.Contains(record, want) {
			t.Errorf("Craig's record %q lacks %q", record, want)
		}
	}
	if notices := historyViewNotices(craig); len(notices) != 0 {
		t.Errorf("Craig's current season shows notices %q", notices)
	}

	for _, state := range []struct {
		path    string
		notices []string
	}{
		{historyViewSeasonPath(1669080, 479801, 168), []string{"No games this season"}},
		{historyViewSeasonPath(1669080, 479802, 167), []string{"Latest refresh failed"}},
		{historyViewSeasonPath(1669080, 479800, 166), []string{"LPS no longer accepts this team"}},
		{historyViewPath(1669081), []string{"No completed games yet"}},
		{historyViewPath(1669082), []string{"No team seasons yet"}},
		{historyViewPath(1669083), []string{"Current teams not confirmed", "Not collected yet"}},
	} {
		if got := historyViewNotices(readHistoryView(t, browser, state.path)); !slices.Equal(got, state.notices) {
			t.Errorf("%s notices = %q, want %q", state.path, got, state.notices)
		}
	}
	jordan := readHistoryView(t, browser, historyViewPath(1669083))
	if got := historyViewOpen(t, jordan); got != "479803/165" || !strings.Contains(plannerText(jordan), "The next refresh is due") {
		t.Errorf("Jordan opens %q with text %q", got, plannerText(jordan))
	}
}

// The history account is only for a preview browser that opened it: never
// outside the preview and never for another preview browser. The linked
// account keeps no history.
func TestPreviewHistoryAccountIsOnlyForItsOwnBrowserInThePreview(t *testing.T) {
	marked := &http.Cookie{Name: "preview_soccer_account", Value: "history", Path: config.SoccerCookiePath}

	t.Run("outside the preview", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, false)
		if entry := browser.get("/__preview/account/soccer-history"); entry.Code != http.StatusNotFound {
			t.Errorf("preview history entry outside the preview: status %d, want 404", entry.Code)
		}
		soccer, _ := url.Parse("https://app.example.com/soccer")
		browser.jar.SetCookies(soccer, []*http.Cookie{marked})
		if refused := browser.get(historyViewPath(1669080)); refused.Code != http.StatusUnauthorized {
			t.Errorf("marked request outside the preview: status %d, want 401", refused.Code)
		}
	})
	t.Run("another preview browser", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, true)
		if refused := browser.get(historyViewPath(1669080)); refused.Code != http.StatusUnauthorized {
			t.Errorf("unmarked preview request: status %d, want 401", refused.Code)
		}
	})
	t.Run("the linked account", func(t *testing.T) {
		browser := newPreviewLinkedBrowser(t, true)
		browser.get("/__preview/account/soccer-linked")
		if imported := browser.postForm("/soccer/import", url.Values{"jwt": {testutil.TestJWT(t, time.Now().Add(time.Hour))}}); imported.Code != http.StatusOK {
			t.Fatalf("preview linked import: status %d", imported.Code)
		}
		requireHiddenHistorySection(t, historySection(t, parsePlannerHTML(t, browser.get("/soccer").Body.String())), "on the linked account")
		if refused := browser.get(historyViewPath(1669080)); refused.Code != http.StatusUnauthorized {
			t.Errorf("linked account's history request: status %d, want the ordinary 401", refused.Code)
		}
	})
}
