package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

type resultSyncCalendar struct {
	mu        sync.Mutex
	events    map[string]internalgoogle.Event
	patches   map[string]int
	attempts  map[string]int
	conflicts map[string]bool
	listCalls map[string]int
	inserts   int
}

const deletedGoogleEventStatus = "cancelled" //nolint:misspell // Match Google Calendar's deleted-event wire status.

func (fake *resultSyncCalendar) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/calendar/v3/users/me/calendarList" {
		_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar"}]}`))
		return
	}
	if r.URL.Path == "/calendar/v3/calendars/team/events" && r.Method == http.MethodGet {
		gameID := strings.TrimPrefix(r.URL.Query().Get("privateExtendedProperty"), "game_id=")
		fake.listCalls[gameID]++
		matches := make([]internalgoogle.Event, 0)
		for eventID := range fake.events {
			event := fake.events[eventID]
			if event.ExtendedProperties.Private["game_id"] == gameID {
				matches = append(matches, event)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": matches})
		return
	}
	if r.URL.Path == "/calendar/v3/calendars/team/events" && r.Method == http.MethodPost {
		fake.inserts++
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	eventID := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/team/events/")
	event, exists := fake.events[eventID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(event)
	case http.MethodPatch:
		fake.attempts[eventID]++
		if r.Header.Get("If-Match") != event.ETag {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil || len(patch) != 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(patch["description"], &event.Description); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if fake.conflicts[eventID] {
			original := fake.events[eventID]
			original.Description += "\nPersonal concurrent edit"
			original.ETag = `"changed"`
			fake.events[eventID] = original
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		fake.patches[eventID]++
		event.ETag = `"v2"`
		fake.events[eventID] = event
		_ = json.NewEncoder(w).Encode(event)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func siteResultEvent(id, description, status string) internalgoogle.Event {
	event := internalgoogle.Event{ID: id, ETag: `"v1"`, Status: status, Description: description}
	event.ExtendedProperties.Private = map[string]string{"game_id": id, "portfolio_app": "soccer"}
	return event
}

func TestGoogleResultSyncPatchesOnlyOwnedResultTextWithoutCreatingEvents(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	application.Config.GoogleClientID = "google-client"
	application.Config.GoogleClientSecret = "google-secret"
	application.Config.GoogleConnectionTableName = "connections"
	store := &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	application.GoogleHandler.SetStore(store)
	token, err := application.GoogleHandler.EncryptToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatal(err)
	}
	store.records["connection-1"] = internalgoogle.ConnectionRecord{
		ConnectionID: "connection-1", OwnerIssuer: fixture.issuer, OwnerSubject: "stable-subject",
		AccountSubject: "calendar-account", AccountEmail: "calendar@example.com", TokenCiphertext: token,
		CalendarID: "team", CalendarSummary: "Team Calendar",
	}
	owned := internalgoogle.Event{
		ID: "8101", ETag: `"v1"`, Status: "confirmed", Summary: "My edited title", Location: "My own location",
		Description: "Personal heading: travel early\nHome is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: \nPersonal note: bring snacks",
		Start:       internalgoogle.EventDateTime{DateTime: "2026-09-20T19:00:00", TimeZone: "America/Denver"},
		End:         internalgoogle.EventDateTime{DateTime: "2026-09-20T19:45:00", TimeZone: "America/Denver"},
		Reminders:   &internalgoogle.EventReminders{UseDefault: false, Overrides: []internalgoogle.EventReminder{{Method: "popup", Minutes: 5}}},
	}
	owned.ExtendedProperties.Private = map[string]string{"game_id": "8101", "portfolio_app": "soccer"}
	ics := internalgoogle.Event{ID: "8102", ETag: `"ics"`, Status: "confirmed", Description: "Personal ICS import"}
	ics.ExtendedProperties.Private = map[string]string{"game_id": "8102"}
	deleted := siteResultEvent("8104", "Home is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: ", deletedGoogleEventStatus)
	ambiguousA := siteResultEvent("a-8105", "Home is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: ", "confirmed")
	ambiguousA.ExtendedProperties.Private["game_id"] = "8105"
	ambiguousB := siteResultEvent("b-8105", ambiguousA.Description, "confirmed")
	ambiguousB.ExtendedProperties.Private["game_id"] = "8105"
	concurrent := siteResultEvent("8106", "Home is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: ", "confirmed")
	malformed := siteResultEvent("8107", "Personal notes without the app result line", "confirmed")
	annotated := siteResultEvent("8109", "Home is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: Win (1-0) -- disputed call", "confirmed")
	googleFake := &resultSyncCalendar{
		events:  map[string]internalgoogle.Event{"8101": owned, "8102": ics, "8104": deleted, "a-8105": ambiguousA, "b-8105": ambiguousB, "8106": concurrent, "8107": malformed, "8109": annotated},
		patches: map[string]int{}, attempts: map[string]int{}, conflicts: map[string]bool{"8106": true}, listCalls: map[string]int{},
	}
	googleServer := httptest.NewServer(googleFake)
	t.Cleanup(googleServer.Close)
	application.GoogleHandler.CalendarAPIBaseURL = googleServer.URL + "/calendar/v3"
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/4101" {
			http.NotFound(w, r)
			return
		}
		games := make([]string, 0, 9)
		for _, game := range []struct {
			id     int
			result string
		}{{8101, "2-1"}, {8102, "1-0"}, {8103, "3-0"}, {8104, "1-1"}, {8105, "4-2"}, {8106, "2-0"}, {8107, "5-1"}, {8108, "6-0"}, {8109, "2-1"}} {
			games = append(games, fmt.Sprintf(`{"UGameID":%d,"UTeam1":4101,"UTeam2":4201,"Season":77,"SchedGameDateTime":%q,"result":%q,"home_team":{"UTeamID":4101,"team_name":"Home"},"visitor_team":{"UTeamID":4201,"team_name":"Away"}}`, game.id, past, game.result))
		}
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":4101,"team_name":"Home","Season":77},"games":[%s]}`, strings.Join(games, ","))
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	siteCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	googleCookie := &http.Cookie{Name: internalgoogle.ConnectionCookieName(fixture.issuer, "stable-subject"), Value: "connection-1"}

	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteCookie, googleCookie)
	fetch := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}}, siteCookie, googleCookie)
	if page.Code != http.StatusOK || fetch.Code != http.StatusOK || googleFake.inserts != 0 || len(googleFake.patches) != 0 {
		t.Fatal("viewing or fetching schedules wrote calendar events")
	}
	form := url.Values{"team_codes": {"4101"}, "selected": {"8101"}}
	if response := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", form, googleCookie); response.Code != http.StatusUnauthorized {
		t.Fatal("Google connection cookie authorized Sync without site sign-in")
	}
	application.Config.SiteInvitations["owner@example.com"] = nil
	if response := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", form, siteCookie, googleCookie); response.Code != http.StatusForbidden {
		t.Fatal("revoked Soccer grant authorized result Sync")
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer", "management"}
	sync := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"4101"}, "selected": {"8101", "8102", "8103", "8104", "8105", "8106", "8107", "8109"}}, siteCookie, googleCookie)
	if sync.Code != http.StatusOK || !strings.Contains(sync.Body.String(), "1 game result(s) updated") || !strings.Contains(sync.Body.String(), "Skipped 7") {
		t.Fatalf("Sync did not report one update and seven unsafe matches: %d %q", sync.Code, sync.Body.String())
	}
	googleFake.mu.Lock()
	updated := googleFake.events["8101"]
	icsAfter := googleFake.events["8102"]
	patchCount := googleFake.patches["8101"]
	insertCount := googleFake.inserts
	conflictAttempts := googleFake.attempts["8106"]
	concurrentAfter := googleFake.events["8106"]
	deletedAfter := googleFake.events["8104"]
	ambiguousAfter := googleFake.events["a-8105"]
	malformedAfter := googleFake.events["8107"]
	annotatedAfter := googleFake.events["8109"]
	deletedPatches := googleFake.patches["8104"]
	annotatedPatches := googleFake.patches["8109"]
	unselectedCalls := googleFake.listCalls["8108"]
	googleFake.mu.Unlock()
	if patchCount != 1 || insertCount != 0 || updated.Summary != owned.Summary || updated.Start != owned.Start || updated.End != owned.End || updated.Location != owned.Location || updated.Reminders.Overrides[0].Minutes != 5 || icsAfter.Description != ics.Description {
		t.Fatal("Sync replaced personal event edits, touched ICS, or inserted history")
	}
	if updated.Description != "Personal heading: travel early\nHome is playing Away\nDivision: Open\nFacility: Boise\nField: Field 1\nResult: Win (2-1)\nPersonal note: bring snacks" {
		t.Fatalf("Sync changed text outside the app-owned result line: %q", updated.Description)
	}
	if conflictAttempts != 1 || !strings.Contains(concurrentAfter.Description, "Personal concurrent edit") || deletedAfter.Status != deletedGoogleEventStatus || deletedPatches != 0 || ambiguousAfter.Description != ambiguousA.Description || malformedAfter.Description != malformed.Description || annotatedAfter.Description != annotated.Description || annotatedPatches != 0 || unselectedCalls != 0 {
		t.Fatal("Sync restored deletion, guessed an ambiguous or malformed match, overwrote concurrency, or touched an unselected game")
	}
	repeated := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"4101"}, "selected": {"8101"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	patchCount = googleFake.patches["8101"]
	googleFake.mu.Unlock()
	if repeated.Code != http.StatusOK || patchCount != 1 || !strings.Contains(repeated.Body.String(), "0 game result(s) updated") {
		t.Fatal("repeat Sync made an unnecessary second edit")
	}
}
