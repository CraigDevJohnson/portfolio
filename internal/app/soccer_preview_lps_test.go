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

	"portfolio/internal/config"
)

// newPreviewPlannerRoutes serves the loopback preview route assembly and
// points the soccer handler at its in-process fake LPS, as Run does.
func newPreviewPlannerRoutes(t *testing.T) http.Handler {
	t.Helper()
	app := newTestApp(t)
	mux, _ := buildMux(app, slog.New(slog.NewTextHandler(io.Discard, nil)), true)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	baseURL, err := config.NormalizeLPSAPIBaseURL(previewLPSBaseURL(strings.TrimPrefix(server.URL, "http://")))
	if err != nil {
		t.Fatalf("preview LPS base URL is not a valid LPS API base URL: %v", err)
	}
	app.Config.LPSAPIBaseURL = baseURL
	return mux
}

// previewPastResultsNewestFirst are the preview's scored past games, newest
// first, which the granted preview account reviews in Google mode: Pond Mint
// United's 7000 from five days ago, and Campfire Rovers' 6998 from 19 days ago
// and 6990 from over a year ago. Campfire Rovers' postponed 6995 has no score
// and stays out.
var previewPastResultsNewestFirst = []string{"7000", "6998", "6990"}

func TestPreviewLPSServesPublicTeamLookupWithNewlyPublishedGame(t *testing.T) {
	routes := newPreviewPlannerRoutes(t)

	first := servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"479691, 479147"}})
	if first.Code != http.StatusOK {
		t.Fatalf("POST /soccer/fetch status = %d", first.Code)
	}
	firstDoc := parsePlannerHTML(t, first.Body.String())
	rows := plannerGameRows(firstDoc, "upcoming-games")
	if got, want := plannerRowIDs(rows), []string{"7003", "7001", "7002"}; !slices.Equal(got, want) {
		t.Fatalf("preview rows = %v, want the shared game once and soonest first %v", got, want)
	}
	for _, row := range rows {
		if !row.Checked {
			t.Errorf("preview game %s did not begin selected", row.ID)
		}
	}
	if past := plannerGameRows(firstDoc, "past-results"); len(past) != 0 {
		t.Errorf("public preview lookup rendered past rows %v", plannerRowIDs(past))
	}

	refetch := parsePlannerHTML(t, servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"479147 479691"}}).Body.String())
	if got, want := plannerRowIDs(plannerGameRows(refetch, "upcoming-games")), []string{"7003", "7001", "7002", "7004"}; !slices.Equal(got, want) {
		t.Fatalf("preview refetch rows = %v, want the newly published game last %v", got, want)
	}

	download := servePublicPlanner(t, routes, http.MethodPost, "/soccer/download", url.Values{
		"team_codes": {"479147,479691"},
		"selected":   {"7003", "7004"},
	})
	if download.Code != http.StatusOK {
		t.Fatalf("POST /soccer/download status = %d body %q", download.Code, download.Body.String())
	}
	if events := icsEvents(download.Body.String()); len(events) != 2 || events["7003"] == "" || events["7004"] == "" {
		t.Fatalf("preview ICS events = %v, want only 7003 and 7004", events)
	}

	unknown := parsePlannerHTML(t, servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"999"}}).Body.String())
	if text := plannerText(unknown); !strings.Contains(text, "Team ID 999 was not accepted by Let's Play Soccer.") {
		t.Errorf("unknown preview team state = %q", text)
	}
}

// The team-color browser proof drives the public Team ID lookup against the
// preview fake LPS. Like every recorded live payload, the fake names no color
// for the page's example teams, whose Team IDs both select purple, so their
// shared game must still show one half per team. Rosehip Athletic names its
// color as LPS might, in mixed case with padding, so adding it shows an LPS
// color beside the example teams' fallbacks.
func TestPreviewLPSPaintsTheExampleTeamsApartAndAnLPSColorBesideThem(t *testing.T) {
	routes := newPreviewPlannerRoutes(t)

	type rowColors struct {
		game, home, away string
		shared           bool
	}
	// In Team ID order, 479147 keeps the purple its ID selects and 479691
	// (also 3 mod 8) takes the next unused fallback, orange. Rosehip
	// Athletic's "  kelly GREEN " normalizes to green.
	for _, fetch := range []struct {
		codes string
		want  []rowColors
	}{
		{codes: "479691, 479147", want: []rowColors{
			{game: "7001", home: "orange", away: "purple", shared: true},
			{game: "7002", home: "orange", away: "orange"},
			{game: "7003", home: "purple", away: "purple"},
		}},
		{codes: "479147 479691", want: []rowColors{
			{game: "7001", home: "orange", away: "purple", shared: true},
			{game: "7002", home: "orange", away: "orange"},
			{game: "7003", home: "purple", away: "purple"},
		}},
		{codes: "479691, 479147, 479800", want: []rowColors{
			{game: "7001", home: "orange", away: "purple", shared: true},
			{game: "7002", home: "green", away: "orange", shared: true},
			{game: "7003", home: "purple", away: "purple"},
		}},
	} {
		resp := servePublicPlanner(t, routes, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {fetch.codes}})
		if resp.Code != http.StatusOK {
			t.Fatalf("POST /soccer/fetch %q status = %d", fetch.codes, resp.Code)
		}
		if body := resp.Body.String(); strings.Contains(body, "kelly") || strings.Contains(body, "GREEN") {
			t.Fatalf("fetch %q: the raw LPS color text reached the rendered schedule", fetch.codes)
		}
		rows := soccerMatchRows(parsePlannerHTML(t, resp.Body.String()))
		for _, want := range fetch.want {
			row := onlySoccerRow(t, rows, want.game)
			if home, away := htmlAttr(row, "data-home-color"), htmlAttr(row, "data-away-color"); home != want.home || away != want.away {
				t.Errorf("fetch %q game %s colors = %q/%q, want %q/%q", fetch.codes, want.game, home, away, want.home, want.away)
			}
			if shared := htmlAttr(row, "data-shared-match") != ""; shared != want.shared {
				t.Errorf("fetch %q game %s shared = %t, want %t", fetch.codes, want.game, shared, want.shared)
			}
		}
		if text := htmlText(onlySoccerRow(t, rows, "7001")); !strings.Contains(text, "Pond Mint United vs Campfire Rovers") {
			t.Errorf("fetch %q: shared game text = %q, want both team names", fetch.codes, text)
		}
	}
}

func TestPreviewLPSIsNotRegisteredOutsidePreview(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, slog.New(slog.NewTextHandler(io.Discard, nil)), false)

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/__preview/lps/teams/479691", nil))

	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /__preview/lps/teams/479691 status = %d outside preview, want 404", resp.Code)
	}
}
