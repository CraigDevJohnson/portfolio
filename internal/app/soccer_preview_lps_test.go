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
// first: Pond Mint United's 7000 from five days ago, and Campfire Rovers' 6998
// from 19 days ago and 6990 from over a year ago. Campfire Rovers' postponed
// 6995 has no score and stays out.
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
	if past := plannerRowIDs(plannerGameRows(firstDoc, "past-results")); !slices.Equal(past, previewPastResultsNewestFirst) {
		t.Errorf("manual preview lookup past rows = %v, want the scored past games newest first %v for Google mode", past, previewPastResultsNewestFirst)
	}
	assertPastResultControlsGoogleOnly(t, firstDoc)

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

func TestPreviewLPSIsNotRegisteredOutsidePreview(t *testing.T) {
	app := newTestApp(t)
	mux, _ := buildMux(app, slog.New(slog.NewTextHandler(io.Discard, nil)), false)

	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/__preview/lps/teams/479691", nil))

	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /__preview/lps/teams/479691 status = %d outside preview, want 404", resp.Code)
	}
}
