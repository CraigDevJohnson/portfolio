package google

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"portfolio/internal/config"
	"portfolio/types"
)

func TestEventPayloadTreatsMislabelledZuluTimestampsAsMountainTime(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := eventPayload(req, &types.Game{
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
		t.Fatal("EventPayload returned false")
	}
	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected start datetime: %q", event.Start.DateTime)
	}
	if event.Start.TimeZone != config.MountainTimeZoneID {
		t.Fatalf("unexpected start timezone: %q", event.Start.TimeZone)
	}
	if event.End.DateTime != "2026-03-29T18:50:00" {
		t.Fatalf("unexpected end datetime: %q", event.End.DateTime)
	}
	if event.End.TimeZone != config.MountainTimeZoneID {
		t.Fatalf("unexpected end timezone: %q", event.End.TimeZone)
	}
}

func TestEventPayloadUsesCanonicalFormatter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	req.Host = "example.com"
	event, ok := eventPayload(req, &types.Game{
		ID:               "3037322",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "FC CHAIN MAIL",
		DivisionName:     "Coed Over 30 B Sun",
		Facility: &types.Facility{
			Name:    "Boise",
			Address: "11448 W. President Drive",
			City:    "Boise",
			State:   "ID",
			ZIP:     "83713",
		},
		Field:   "Field 2",
		Result:  "7 - 3",
		StartAt: "2026-03-08T12:30:00-06:00",
	})
	if !ok {
		t.Fatal("EventPayload returned false")
	}
	if event.ID != "3037322" {
		t.Fatalf("unexpected event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs FC CHAIN MAIL - Field 2" {
		t.Fatalf("unexpected event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: Win (7-3)" {
		t.Fatalf("unexpected event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-08T12:30:00" {
		t.Fatalf("unexpected start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-08T13:15:00" {
		t.Fatalf("unexpected end datetime: %q", event.End.DateTime)
	}
	if event.Status != "confirmed" {
		t.Fatalf("unexpected event status: %q", event.Status)
	}
	if got := event.ExtendedProperties.Private["game_id"]; got != "3037322" {
		t.Fatalf("unexpected private game id: %q", got)
	}
	if _, exists := event.ExtendedProperties.Private["portfolio_game_id"]; exists {
		t.Fatalf("legacy portfolio_game_id should not be set: %#v", event.ExtendedProperties.Private)
	}
}

func TestEventPayloadMirrorsCanonicalFormatterForCancelledGame(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	event, ok := eventPayload(req, &types.Game{
		ID:               "3042954",
		PlayerTeamName:   "STRUGGLE BUS",
		OpponentTeamName: "MANEFESTO",
		DivisionName:     "Coed Over 30 B Sun",
		Facility: &types.Facility{
			Name:    "Boise",
			Address: "11448 W. President Drive",
			City:    "Boise",
			State:   "ID",
			ZIP:     "83713",
		},
		Field:   "Field 1",
		Result:  "canceled",
		StartAt: "2026-03-29T17:20:00-06:00",
	})
	if !ok {
		t.Fatal("EventPayload returned false")
	}
	if event.ID != "3042954" {
		t.Fatalf("unexpected event id: %q", event.ID)
	}
	if event.Summary != "STRUGGLE BUS vs MANEFESTO - Field 1" {
		t.Fatalf("unexpected event summary: %q", event.Summary)
	}
	if event.Description != "STRUGGLE BUS is playing MANEFESTO\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 1\nResult: Canceled" {
		t.Fatalf("unexpected event description: %q", event.Description)
	}
	if event.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected event location: %q", event.Location)
	}
	if event.Start.DateTime != "2026-03-29T17:20:00" {
		t.Fatalf("unexpected start datetime: %q", event.Start.DateTime)
	}
	if event.End.DateTime != "2026-03-29T18:05:00" {
		t.Fatalf("unexpected end datetime: %q", event.End.DateTime)
	}
	if event.Status != "canceled" {
		t.Fatalf("unexpected event status: %q", event.Status)
	}
	if got := event.ExtendedProperties.Private["game_id"]; got != "3042954" {
		t.Fatalf("unexpected private game id: %q", got)
	}
}

func TestEventMatchesGameIDUsesOnlyCanonicalGameIDFields(t *testing.T) {
	tests := []struct {
		name      string
		event     Event
		configure func(*Event)
		gameID    string
		expected  bool
	}{
		{
			name:     "matches raw event id",
			event:    Event{ID: "7001"},
			gameID:   "7001",
			expected: true,
		},
		{
			name:  "matches private game id when event id differs",
			event: Event{ID: "legacy-7002"},
			configure: func(e *Event) {
				e.ExtendedProperties.Private = map[string]string{"game_id": "7002"}
			},
			gameID:   "7002",
			expected: true,
		},
		{
			name:   "does not match legacy event without canonical id",
			event:  Event{ID: "legacy-7005"},
			gameID: "7005",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.event
			if tt.configure != nil {
				tt.configure(&event)
			}
			got := eventMatchesGameID(&event, tt.gameID)
			if got != tt.expected {
				t.Fatalf("eventMatchesGameID() = %t, want %t", got, tt.expected)
			}
		})
	}
}
