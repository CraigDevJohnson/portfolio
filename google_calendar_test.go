package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGoogleEventPayloadTreatsMislabelledZuluTimestampsAsMountainTime(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := googleEventPayload(req, &Game{
		ID:               "zulu-game",
		Home:             "Team A",
		Away:             "Team B",
		PlayerTeamName:   "Team A",
		OpponentTeamName: "Team B",
		Field:            "Field 1",
		StartAt:          "2026-03-29T17:20:00.000Z",
		EndAt:            "2026-03-29T18:50:00.000Z",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.Start.TimeZone != mountainTimeZoneID {
		t.Fatalf("unexpected google start timezone: %q", event.Start.TimeZone)
	}
	if event.End.DateTime != "2026-03-29T18:50:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.End.TimeZone != mountainTimeZoneID {
		t.Fatalf("unexpected google end timezone: %q", event.End.TimeZone)
	}
}


func TestGoogleEventPayloadUsesCanonicalFormatter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	req.Host = "example.com"
	event, ok := googleEventPayload(req, &Game{
		ID:               "3037322",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "FC CHAIN MAIL",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 2",
		Result:           "7 - 3",
		StartAt:          "2026-03-08T12:30:00-06:00",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.ID != "3037322" {
		t.Fatalf("unexpected google event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs FC CHAIN MAIL - Field 2" {
		t.Fatalf("unexpected google event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: 7 - 3" {
		t.Fatalf("unexpected google event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected google event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-08T12:30:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-08T13:15:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.Status != "confirmed" {
		t.Fatalf("unexpected google event status: %q", event.Status)
	}
	if got := event.ExtendedProperties.Private["game_id"]; got != "3037322" {
		t.Fatalf("unexpected google private game id: %q", got)
	}
	if _, exists := event.ExtendedProperties.Private["portfolio_game_id"]; exists {
		t.Fatalf("legacy portfolio_game_id should not be set: %#v", event.ExtendedProperties.Private)
	}
}


func TestGoogleEventPayloadMirrorsCanonicalFormatterForCancelledGame(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := googleEventPayload(req, &Game{
		ID:               "3042954",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "MANEFESTO",
		DivisionName:     "Coed Over 30 B Sun",
		FacilityName:     "Boise",
		FacilityAddress:  "11448 W. President Drive",
		FacilityCity:     "Boise",
		FacilityState:    "ID",
		FacilityZIP:      "83713",
		Field:            "Field 1",
		Result:           "canceled",
		StartAt:          "2026-03-29T17:20:00-06:00",
	})
	if !ok {
		t.Fatal("googleEventPayload returned false")
	}

	if event.ID != "3042954" {
		t.Fatalf("unexpected google event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs MANEFESTO - Field 1" {
		t.Fatalf("unexpected google event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing MANEFESTO\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 1\nResult: canceled" {
		t.Fatalf("unexpected google event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected google event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected google start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-29T18:05:00" {
		t.Fatalf("unexpected google end datetime: %q", event.End.DateTime)
	}
	if event.Status != "canceled" {
		t.Fatalf("unexpected google event status: %q", event.Status)
	}
	if got := event.ExtendedProperties.Private["game_id"]; got != "3042954" {
		t.Fatalf("unexpected google private game id: %q", got)
	}
}


func TestGoogleEventMatchesGameIDUsesOnlyCanonicalGameIDFields(t *testing.T) {
	tests := []struct {
		name      string
		event     googleEvent
		configure func(*googleEvent)
		gameID    string
		expected  bool
	}{
		{
			name:     "matches raw google event id",
			event:    googleEvent{ID: "7001"},
			gameID:   "7001",
			expected: true,
		},
		{
			name:  "matches private game id when event id differs",
			event: googleEvent{ID: "legacy-7002"},
			configure: func(event *googleEvent) {
				event.ExtendedProperties.Private = map[string]string{"game_id": "7002"}
			},
			gameID:   "7002",
			expected: true,
		},
		{
			name:   "does not match legacy event without canonical id",
			event:  googleEvent{ID: "legacy-7005"},
			gameID: "7005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.event
			if tt.configure != nil {
				tt.configure(&event)
			}
			got := googleEventMatchesGameID(&event, tt.gameID)
			if got != tt.expected {
				t.Fatalf("googleEventMatchesGameID() = %t, want %t", got, tt.expected)
			}
		})
	}
}


func TestSoccerGoogleAddHandlerAddsUpdatesCancelsAndSkipsByCanonicalGameID(t *testing.T) {
	store := &fakeGoogleConnectionStore{records: map[string]googleConnectionRecord{}}
	configureGoogleTestRuntime(t, store, "", "", "")
	previousConfig := configData
	previousLocal := time.Local
	time.Local = time.UTC
	defer func() {
		configData = previousConfig
		time.Local = previousLocal
	}()
	tokenCiphertext, err := encryptGoogleToken(&oauth2.Token{AccessToken: "access-token"})
	if err != nil {
		t.Fatalf("encryptGoogleToken returned error: %v", err)
	}
	store.records["connection-1"] = googleConnectionRecord{
		ConnectionID:    "connection-1",
		TokenCiphertext: tokenCiphertext,
		CalendarID:      "primary",
		CalendarSummary: "Primary Calendar",
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	insertedIDs := make([]string, 0, 2)
	privateLookupIDs := make([]string, 0, 4)
	updatedEvents := map[string]googleEvent{}
	futureOne := testMislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	futureTwo := testMislabelledLPSZuluTime(time.Now().Add(48 * time.Hour))
	futureThree := testMislabelledLPSZuluTime(time.Now().Add(72 * time.Hour))
	futureFour := testMislabelledLPSZuluTime(time.Now().Add(96 * time.Hour))
	futureFive := testMislabelledLPSZuluTime(time.Now().Add(120 * time.Hour))
	futureSix := testMislabelledLPSZuluTime(time.Now().Add(144 * time.Hour))
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/teams/479691":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"games": [
					{
						"UGameID": 7001,
						"SchedGameDateTime": %q,
						"field_name": "Field 3",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "GALACTICOS FC"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7002,
						"SchedGameDateTime": %q,
						"field_name": "Field 1",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "CLASSIC XI"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "1 - 0",
						"Season": 169
					},
					{
						"UGameID": 7003,
						"SchedGameDateTime": %q,
						"field_name": "Field 2",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "RED STARS"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7004,
						"SchedGameDateTime": %q,
						"field_name": "Field 4",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "NIGHT OWLS"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "canceled",
						"Season": 169
					},
					{
						"UGameID": 7005,
						"SchedGameDateTime": %q,
						"field_name": "Field 5",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "OLD GUARD"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					},
					{
						"UGameID": 7006,
						"SchedGameDateTime": %q,
						"field_name": "Field 6",
						"facilityName": "Boise",
						"FacilityID": 4,
						"Address": "123 Main St",
						"City": "Boise",
						"State": "ID",
						"ZIP": "83702",
						"home_team": {"team_name": "UNITED NATIONS"},
						"visitor_team": {"team_name": "RESERVES"},
						"division_name": "Coed F Fri",
						"DivisionName": "Coed F Fri",
						"result": "",
						"Season": 169
					}
				]
			}`, futureOne, futureTwo, futureThree, futureFour, futureFive, futureSix)))
		case r.URL.Path == "/facilities/4":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"facilityName":"Boise","Address":"123 Main St","City":"Boise","State":"ID","ZIP":"83702"}`))
		case r.URL.Path == "/calendar/v3/calendars/primary/events" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("io.ReadAll returned error: %v", err)
			}
			var event googleEvent
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
			var event googleEvent
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

	configData.LPSAPIBaseURL = apiServer.URL
	googleCalendarAPIBaseURL = apiServer.URL + "/calendar/v3"

	req := httptest.NewRequest(http.MethodPost, "/soccer/google/add", strings.NewReader(url.Values{
		"team_codes": {"479691"},
		"selected":   {"7001", "7002", "7003", "7004", "7005"},
	}.Encode()))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: googleConnectionCookieName, Value: "connection-1"})
	resp := httptest.NewRecorder()

	soccerGoogleAddHandler(resp, req)

	if !strings.Contains(resp.Body.String(), "Added 1 selected game(s) to Google Calendar.") {
		t.Fatalf("expected add success message, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Updated/restored 3 matching game(s).") {
		t.Fatalf("expected update success message, got %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Skipped 1 game(s) that could not be matched to the same Google game ID.") {
		t.Fatalf("expected skip message, got %q", resp.Body.String())
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

