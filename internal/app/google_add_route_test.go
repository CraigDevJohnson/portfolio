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

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/testutil"
)

type fakeWritableCalendars struct {
	mu           sync.Mutex
	writableTeam bool
	eventDenied  bool
	events       map[string]map[string]internalgoogle.Event
	inserts      map[string]int
	updates      map[string]int
}

func newFakeWritableCalendars() *fakeWritableCalendars {
	return &fakeWritableCalendars{
		writableTeam: true,
		events:       map[string]map[string]internalgoogle.Event{"primary": {}, "team": {}},
		inserts:      map[string]int{},
		updates:      map[string]int{},
	}
}

func (fake *fakeWritableCalendars) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/calendar/v3/users/me/calendarList" {
		if r.URL.Query().Get("minAccessRole") != "writer" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if fake.writableTeam {
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true},{"id":"team","summary":"Team Calendar"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"items":[{"id":"primary","summary":"Primary Calendar","primary":true}]}`))
		}
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "events" {
		http.NotFound(w, r)
		return
	}
	calendarID := parts[0]
	if calendarID == "team" && (!fake.writableTeam || fake.eventDenied) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if fake.events[calendarID] == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 3 && r.Method == http.MethodGet {
		event, ok := fake.events[calendarID][parts[2]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(event)
		return
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		gameID := strings.TrimPrefix(r.URL.Query().Get("privateExtendedProperty"), "game_id=")
		matches := []internalgoogle.Event{}
		for eventID := range fake.events[calendarID] {
			event := fake.events[calendarID][eventID]
			if event.ExtendedProperties.Private["game_id"] == gameID {
				matches = append(matches, event)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": matches})
		return
	}
	var event internalgoogle.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	key := calendarID + "/" + event.ID
	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		fake.inserts[key]++
		if _, exists := fake.events[calendarID][event.ID]; exists {
			w.WriteHeader(http.StatusConflict)
			return
		}
		fake.events[calendarID][event.ID] = event
		w.WriteHeader(http.StatusCreated)
	case len(parts) == 3 && r.Method == http.MethodPut:
		fake.updates[key]++
		fake.events[calendarID][event.ID] = event
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestGoogleAddUsesChosenWritableCalendarAndOnlySelectedUpcomingGames(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	application.Config.SessionKey = []byte("0123456789abcdef0123456789abcdef")
	application.Config.GoogleClientID = "google-client"
	application.Config.GoogleClientSecret = "google-secret"
	application.Config.GoogleConnectionTableName = "connections"
	store := &appTestGoogleConnectionStore{records: map[string]internalgoogle.ConnectionRecord{}}
	application.GoogleHandler.SetStore(store)
	encryptedToken, err := application.GoogleHandler.EncryptToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatal(err)
	}
	store.records["connection-1"] = internalgoogle.ConnectionRecord{
		ConnectionID: "connection-1", OwnerIssuer: fixture.issuer, OwnerSubject: "stable-subject",
		AccountSubject: "calendar-account", AccountEmail: "calendar@example.com", TokenCiphertext: encryptedToken,
	}
	googleFake := newFakeWritableCalendars()
	googleServer := httptest.NewServer(googleFake)
	t.Cleanup(googleServer.Close)
	application.GoogleHandler.CalendarAPIBaseURL = googleServer.URL + "/calendar/v3"
	futureOne := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testutil.MislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	past := testutil.MislabelledLPSZuluTime(time.Now().Add(-24 * time.Hour))
	lpsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/4101" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"team":{"UTeamID":4101,"team_name":"Home","Season":77},"games":[{"UGameID":7001,"UTeam1":4101,"UTeam2":4201,"Season":77,"SchedGameDateTime":%q,"home_team":{"UTeamID":4101,"team_name":"Home"},"visitor_team":{"UTeamID":4201,"team_name":"Away"}},{"UGameID":7002,"UTeam1":4101,"UTeam2":4202,"Season":77,"SchedGameDateTime":%q,"home_team":{"UTeamID":4101,"team_name":"Home"},"visitor_team":{"UTeamID":4202,"team_name":"Rivals"}},{"UGameID":7003,"UTeam1":4101,"UTeam2":4203,"Season":77,"SchedGameDateTime":%q,"result":"2-1","home_team":{"UTeamID":4101,"team_name":"Home"},"visitor_team":{"UTeamID":4203,"team_name":"Old"}}]}`, futureOne, futureTwo, past)
	}))
	t.Cleanup(lpsServer.Close)
	application.Config.LPSAPIBaseURL = lpsServer.URL
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer")
	siteCookie := siteCookie(t, completeSiteSignIn(t, mux, stateCookie, state))
	googleCookie := &http.Cookie{Name: config.GoogleConnectionCookieName, Value: "connection-1"}

	page := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteCookie, googleCookie)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Primary Calendar") || store.records["connection-1"].CalendarID != "primary" {
		t.Fatal("primary calendar was not selected on the connected page")
	}
	fetched := soccerGrantRequest(mux, http.MethodPost, "/soccer/fetch", url.Values{"team_codes": {"4101"}}, siteCookie, googleCookie)
	if fetched.Code != http.StatusOK {
		t.Fatalf("schedule fetch failed: %d", fetched.Code)
	}
	googleFake.mu.Lock()
	writesBeforeAdd := len(googleFake.events["primary"]) + len(googleFake.events["team"])
	googleFake.mu.Unlock()
	if writesBeforeAdd != 0 {
		t.Fatal("viewing or fetching schedules wrote Google events")
	}

	add := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001", "7003"}}, siteCookie, googleCookie)
	if add.Code != http.StatusOK || !strings.Contains(add.Body.String(), "Added 1 selected game") {
		t.Fatalf("explicit Add did not report only the selected upcoming game: %d %q", add.Code, add.Body.String())
	}
	googleFake.mu.Lock()
	primaryEvent := googleFake.events["primary"]["7001"]
	primaryCount := len(googleFake.events["primary"])
	primaryInserts := googleFake.inserts["primary/7001"]
	googleFake.mu.Unlock()
	if primaryCount != 1 || primaryInserts != 1 || primaryEvent.ID != "7001" || primaryEvent.ExtendedProperties.Private["game_id"] != "7001" || primaryEvent.ExtendedProperties.Private["portfolio_app"] != "soccer" || primaryEvent.Source == nil || primaryEvent.Source.URL != "https://app.example.com/soccer" {
		t.Fatalf("Add lacked stable site-owned identity or included past game: event %#v, count %d", primaryEvent, primaryCount)
	}
	repeated := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, siteCookie, googleCookie)
	if repeated.Code != http.StatusOK {
		t.Fatalf("repeat Add failed: %d", repeated.Code)
	}
	googleFake.mu.Lock()
	primaryCount = len(googleFake.events["primary"])
	primaryInserts = googleFake.inserts["primary/7001"]
	googleFake.mu.Unlock()
	if primaryCount != 1 || primaryInserts != 1 {
		t.Fatal("repeat Add created a duplicate event")
	}

	selected := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"team"}}, siteCookie, googleCookie)
	if selected.Code != http.StatusOK || store.records["connection-1"].CalendarID != "team" {
		t.Fatal("alternate writable calendar was not selected")
	}
	add = soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7002"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	teamEventID := googleFake.events["team"]["7002"].ID
	primaryEventID := googleFake.events["primary"]["7001"].ID
	googleFake.eventDenied = true
	googleFake.mu.Unlock()
	if add.Code != http.StatusOK || teamEventID != "7002" || primaryEventID != "7001" {
		t.Fatal("destination change moved old event or missed new calendar")
	}
	unwritable := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	teamCount := len(googleFake.events["team"])
	googleFake.eventDenied = false
	googleFake.writableTeam = false
	googleFake.mu.Unlock()
	if unwritable.Code != http.StatusOK || !strings.Contains(unwritable.Body.String(), "Choose a writable calendar") || teamCount != 1 || store.records["connection-1"].CalendarID != "team" {
		t.Fatal("calendar that rejects event access did not pause writes and retain the choice")
	}

	lostPage := soccerGrantRequest(mux, http.MethodGet, "/soccer", nil, siteCookie, googleCookie)
	if lostPage.Code != http.StatusOK || store.records["connection-1"].CalendarID != "team" || !strings.Contains(lostPage.Body.String(), "Choose a writable calendar") {
		t.Fatal("lost calendar silently fell back to primary or lacked a recovery prompt")
	}
	paused := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7002"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	primaryCount = len(googleFake.events["primary"])
	primarySecondInserts := googleFake.inserts["primary/7002"]
	teamSecondInserts := googleFake.inserts["team/7002"]
	googleFake.writableTeam = true
	googleFake.mu.Unlock()
	if paused.Code != http.StatusOK || !strings.Contains(paused.Body.String(), "Choose a writable calendar") || primaryCount != 1 || primarySecondInserts != 0 || teamSecondInserts != 1 {
		t.Fatal("lost calendar write did not pause safely")
	}
	syncPaused := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/sync-results", url.Values{"team_codes": {"4101"}, "selected": {"7003"}}, siteCookie, googleCookie)
	if syncPaused.Code != http.StatusOK || !strings.Contains(syncPaused.Body.String(), "Choose a writable calendar") {
		t.Fatal("lost destination still permitted result-sync writes")
	}
	recoveredWithoutChoice := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7001"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	teamCount = len(googleFake.events["team"])
	googleFake.writableTeam = false
	googleFake.mu.Unlock()
	if recoveredWithoutChoice.Code != http.StatusOK || !strings.Contains(recoveredWithoutChoice.Body.String(), "Choose a writable calendar") || teamCount != 1 {
		t.Fatal("previously lost destination resumed without an explicit new choice")
	}
	invalid := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"missing"}}, siteCookie, googleCookie)
	if invalid.Code != http.StatusOK || store.records["connection-1"].CalendarID != "team" {
		t.Fatal("invalid selection silently switched destination")
	}
	reselected := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/calendar", url.Values{"calendar_id": {"primary"}}, siteCookie, googleCookie)
	if reselected.Code != http.StatusOK || store.records["connection-1"].CalendarID != "primary" {
		t.Fatal("valid new selection was not saved")
	}
	resumed := soccerGrantRequest(mux, http.MethodPost, "/soccer/google/add", url.Values{"team_codes": {"4101"}, "selected": {"7002"}}, siteCookie, googleCookie)
	googleFake.mu.Lock()
	primarySecondID := googleFake.events["primary"]["7002"].ID
	teamSecondID := googleFake.events["team"]["7002"].ID
	googleFake.mu.Unlock()
	if resumed.Code != http.StatusOK || primarySecondID != "7002" || teamSecondID != "7002" {
		t.Fatal("explicit new choice did not resume writes while retaining the old calendar event")
	}
}
