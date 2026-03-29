package google

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"portfolio/internal/config"
	"portfolio/types"
)

func TestAddHandlerAddsUpdatesCancelsAndSkipsByCanonicalGameID(t *testing.T) {
	store := &fakeConnectionStore{records: map[string]ConnectionRecord{}}
	h := newTestHandler(t, store)
	bridge := h.Soccer.(*stubSoccerBridge)

	previousLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = previousLocal }()

	tokenCiphertext, err := h.EncryptToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("EncryptToken returned error: %v", err)
	}
	store.records["connection-1"] = ConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	futureOne := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	futureThree := testMislabelledLPSZuluTime(time.Now().Add(72 * time.Hour))
	futureFour := testMislabelledLPSZuluTime(time.Now().Add(96 * time.Hour))
	futureFive := testMislabelledLPSZuluTime(time.Now().Add(120 * time.Hour))
	futureSix := testMislabelledLPSZuluTime(time.Now().Add(144 * time.Hour))

	bridge.games = []types.Game{
		{ID: "7001", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "GALACTICOS FC", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 3", Result: "", StartAt: futureOne},
		{ID: "7002", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "CLASSIC XI", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 1", Result: "1 - 0", StartAt: futureTwo},
		{ID: "7003", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "RED STARS", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 2", Result: "", StartAt: futureThree},
		{ID: "7004", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "NIGHT OWLS", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 4", Result: "canceled", StartAt: futureFour},
		{ID: "7005", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "OLD GUARD", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 5", Result: "", StartAt: futureFive},
		{ID: "7006", PlayerTeamName: "UNITED NATIONS", OpponentTeamName: "RESERVES", DivisionName: "Coed F Fri", FacilityName: "Boise", FacilityAddress: "123 Main St", FacilityCity: "Boise", FacilityState: "ID", FacilityZIP: "83702", Field: "Field 6", Result: "", StartAt: futureSix},
	}

	insertedIDs := make([]string, 0, 2)
	privateLookupIDs := make([]string, 0, 4)
	updatedEvents := map[string]Event{}

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/calendar/v3/calendars/primary/events" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("io.ReadAll returned error: %v", err)
			}
			var event Event
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			insertedIDs = append(insertedIDs, event.ID)
			if event.ID == "7001" {
				w.WriteHeader(http.StatusCreated)
				return
			}
			if event.ID == "7005" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			t.Fatalf("unexpected insert for event id %q", event.ID)

		case strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/") && r.Method == http.MethodGet:
			eventID := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/")
			switch eventID {
			case "7001", "7002", "7005":
				w.WriteHeader(http.StatusNotFound)
			case "7003":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"7003","status":"canceled","extendedProperties":{"private":{"game_id":"7003"}}}`))
			case "7004":
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"7004","status":"confirmed","extendedProperties":{"private":{"game_id":"7004"}}}`))
			default:
				t.Fatalf("unexpected get for event id %q", eventID)
			}

		case r.URL.Path == "/calendar/v3/calendars/primary/events" && r.Method == http.MethodGet:
			privateLookup := r.URL.Query().Get("privateExtendedProperty")
			privateLookupIDs = append(privateLookupIDs, strings.TrimPrefix(privateLookup, "game_id="))
			w.Header().Set("Content-Type", "application/json")
			switch privateLookup {
			case "game_id=7002":
				_, _ = w.Write([]byte(`{"items":[{"id":"legacy-7002","status":"confirmed","extendedProperties":{"private":{"game_id":"7002"}}}]}`))
			case "game_id=7005":
				_, _ = w.Write([]byte(`{"items":[{"id":"legacy-7005","status":"confirmed","extendedProperties":{"private":{"game_id":"legacy-7005"}}}]}`))
			default:
				_, _ = w.Write([]byte(`{"items":[]}`))
			}

		case strings.HasPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/") && r.Method == http.MethodPut:
			eventID := strings.TrimPrefix(r.URL.Path, "/calendar/v3/calendars/primary/events/")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("io.ReadAll returned error: %v", err)
			}
			var event Event
			if err := json.Unmarshal(body, &event); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			updatedEvents[eventID] = event
			w.WriteHeader(http.StatusOK)

		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer apiServer.Close()

	h.CalendarAPIBaseURL = apiServer.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/add", strings.NewReader(url.Values{
		"team_codes": {"479691"},
		"selected":   {"7001", "7002", "7003", "7004", "7005"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: config.GoogleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	h.AddHandler(resp, req)

	body := resp.Body.String()
	if !strings.Contains(body, "Added 1 selected game(s) to Google Calendar.") {
		t.Fatalf("expected add success message, got %q", body)
	}
	if !strings.Contains(body, "Updated/restored 3 matching game(s).") {
		t.Fatalf("expected update success message, got %q", body)
	}
	if !strings.Contains(body, "Skipped 1 game(s) that could not be matched to the same Google game ID.") {
		t.Fatalf("expected skip message, got %q", body)
	}
	if len(insertedIDs) != 2 {
		t.Fatalf("unexpected insert attempt count: got %d want 2", len(insertedIDs))
	}
	if len(updatedEvents) != 3 {
		t.Fatalf("unexpected update count: got %d want 3", len(updatedEvents))
	}
	if updated, ok := updatedEvents["legacy-7002"]; !ok {
		t.Fatalf("expected private game_id match to update legacy-7002: %#v", updatedEvents)
	} else {
		if updated.ID != "legacy-7002" {
			t.Fatalf("expected existing google event id to be preserved, got %q", updated.ID)
		}
		if updated.Status != "confirmed" {
			t.Fatalf("expected private-match update to stay confirmed, got %q", updated.Status)
		}
		if got := updated.ExtendedProperties.Private["game_id"]; got != "7002" {
			t.Fatalf("expected canonical private game id, got %q", got)
		}
		if updated.Summary != "UNITED NATIONS vs CLASSIC XI - Field 1" {
			t.Fatalf("unexpected updated summary: %q", updated.Summary)
		}
		if !strings.Contains(updated.Description, "UNITED NATIONS is playing CLASSIC XI") || !strings.Contains(updated.Description, "Field: Field 1") || !strings.Contains(updated.Description, "Result: 1 - 0") {
			t.Fatalf("unexpected updated description: %q", updated.Description)
		}
		if updated.Location != "123 Main St, Boise, ID, 83702" {
			t.Fatalf("unexpected updated location: %q", updated.Location)
		}
		if updated.Source == nil || updated.Source.Title != "Soccer Schedule" || updated.Source.URL != "http://example.com/soccer" {
			t.Fatalf("unexpected updated source: %#v", updated.Source)
		}
	}
	if updated, ok := updatedEvents["7003"]; !ok {
		t.Fatalf("expected canceled matching event to be restored: %#v", updatedEvents)
	} else if updated.Status != "confirmed" {
		t.Fatalf("expected restored status to be confirmed, got %q", updated.Status)
	}
	if updated, ok := updatedEvents["7004"]; !ok {
		t.Fatalf("expected confirmed matching event to be canceled: %#v", updatedEvents)
	} else if updated.Status != "canceled" {
		t.Fatalf("expected canceled status to be propagated, got %q", updated.Status)
	}
	if _, exists := updatedEvents["legacy-7005"]; exists {
		t.Fatalf("non-matching legacy event should not be mutated: %#v", updatedEvents["legacy-7005"])
	}
	if len(privateLookupIDs) == 0 {
		t.Fatal("expected at least one private game_id lookup")
	}
	if insertedIDs[0] != "7001" || insertedIDs[1] != "7005" {
		t.Fatalf("unexpected insert ids: %#v", insertedIDs)
	}
	for _, lookedUpGameID := range privateLookupIDs {
		if lookedUpGameID == "7006" {
			t.Fatalf("unselected game should not trigger private game_id lookup: %#v", privateLookupIDs)
		}
	}
	if _, exists := updatedEvents["7006"]; exists {
		t.Fatalf("unselected game should not be updated: %#v", updatedEvents["7006"])
	}
}
