package schedule

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"portfolio/types"
)

func unfoldICS(ics string) string {
	return strings.ReplaceAll(ics, "\r\n ", "")
}

func TestCanonicalGameEventUsesEnrichedScheduleFields(t *testing.T) {
	formatted, ok := CanonicalGameEvent(&types.Game{
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
		t.Fatal("CanonicalGameEvent returned false")
	}

	if formatted.ID != "3037322" {
		t.Fatalf("unexpected canonical game id: %q", formatted.ID)
	}
	if formatted.Summary != "STRUGGLE BUS vs FC CHAIN MAIL - Field 2" {
		t.Fatalf("unexpected canonical summary: %q", formatted.Summary)
	}
	if formatted.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: 7 - 3" {
		t.Fatalf("unexpected canonical description: %q", formatted.Description)
	}
	if formatted.Location != "11448 W. President Drive, Boise, ID, 83713" {
		t.Fatalf("unexpected canonical location: %q", formatted.Location)
	}
	if formatted.Start.Format(time.RFC3339) != "2026-03-08T12:30:00-06:00" {
		t.Fatalf("unexpected canonical start: %s", formatted.Start.Format(time.RFC3339))
	}
	if formatted.End.Format(time.RFC3339) != "2026-03-08T13:15:00-06:00" {
		t.Fatalf("unexpected canonical end: %s", formatted.End.Format(time.RFC3339))
	}
	if formatted.Status != "confirmed" {
		t.Fatalf("unexpected canonical status: %q", formatted.Status)
	}
}

func TestBuildICSFoldsLongLines(t *testing.T) {
	ics := BuildICS([]types.Game{
		{
			ID:       strings.Repeat("abc123", 8),
			Home:     strings.Repeat("Home Team ", 6),
			Away:     strings.Repeat("Away Team ", 6),
			StartAt:  "2026-01-11T14:55:00-07:00",
			EndAt:    "2026-01-11T16:25:00-07:00",
			Location: strings.Repeat("Championship Field Complex ", 4),
			Season:   strings.Repeat("Spring ", 8),
		},
	})

	if !strings.Contains(ics, "\r\n ") {
		t.Fatalf("expected folded ICS output, got %q", ics)
	}

	for _, line := range strings.Split(ics, "\r\n") {
		if line == "" {
			continue
		}
		if len([]byte(line)) > 75 {
			t.Fatalf("ics line exceeds 75 octets: %d bytes in %q", len([]byte(line)), line)
		}
	}
}

func TestBuildICSFoldsUTF8Lines(t *testing.T) {
	ics := BuildICS([]types.Game{
		{
			ID:       "utf8-game",
			Home:     strings.Repeat("⚽", 20),
			Away:     strings.Repeat("ゴール", 10),
			StartAt:  "2026-01-11T14:55:00-07:00",
			EndAt:    "2026-01-11T16:25:00-07:00",
			Location: strings.Repeat("Équipe ", 12),
		},
	})

	if !utf8.ValidString(ics) {
		t.Fatalf("ics output is not valid UTF-8: %q", ics)
	}

	for _, line := range strings.Split(ics, "\r\n") {
		if line == "" {
			continue
		}
		if len([]byte(line)) > 75 {
			t.Fatalf("ics utf8 line exceeds 75 octets: %d bytes in %q", len([]byte(line)), line)
		}
	}
}

func TestBuildICSUsesMountainTimezoneForMislabelledZuluTimestamps(t *testing.T) {
	ics := BuildICS([]types.Game{{
		ID:      "mountain-game",
		Home:    "Team A",
		Away:    "Team B",
		StartAt: "2026-03-29T17:20:00.000Z",
		EndAt:   "2026-03-29T18:50:00.000Z",
	}})

	if !strings.Contains(ics, "X-WR-TIMEZONE:America/Denver") {
		t.Fatalf("expected calendar timezone in ICS output, got %q", ics)
	}
	if !strings.Contains(ics, "DTSTART;TZID=America/Denver:20260329T172000") {
		t.Fatalf("expected mountain DTSTART in ICS output, got %q", ics)
	}
	if !strings.Contains(ics, "DTEND;TZID=America/Denver:20260329T185000") {
		t.Fatalf("expected mountain DTEND in ICS output, got %q", ics)
	}
}

func TestBuildICSMirrorsCanonicalFormatterForCancelledGame(t *testing.T) {
	ics := unfoldICS(BuildICS([]types.Game{{
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
	}}))

	expectedLines := []string{
		"UID:3042954",
		"DTSTART;TZID=America/Denver:20260329T172000",
		"DTEND;TZID=America/Denver:20260329T180500",
		"SUMMARY:STRUGGLE BUS vs MANEFESTO - Field 1",
		"DESCRIPTION:STRUGGLE BUS is playing MANEFESTO\\nDivision: Coed Over 30 B Sun\\nFacility: Boise\\nField: Field 1\\nResult: canceled",
		"LOCATION:11448 W. President Drive\\, Boise\\, ID\\, 83713",
		"STATUS:CANCELED",
	}

	for _, expectedLine := range expectedLines {
		if !strings.Contains(ics, expectedLine) {
			t.Fatalf("expected ICS to contain %q, got %q", expectedLine, ics)
		}
	}
}

func TestBuildICSSkipsGamesWithUnparseableStartTime(t *testing.T) {
	ics := BuildICS([]types.Game{
		{
			ID:      "good-game",
			Home:    "Team A",
			Away:    "Team B",
			StartAt: "2026-01-11T14:55:00-07:00",
			EndAt:   "2026-01-11T16:25:00-07:00",
		},
		{
			ID:   "bad-game",
			Home: "Team C",
			Away: "Team D",
		},
	})

	if !strings.Contains(ics, "good-game") {
		t.Fatal("expected good-game in ICS output")
	}
	if strings.Contains(ics, "bad-game") {
		t.Fatal("expected bad-game to be skipped in ICS output")
	}
}
