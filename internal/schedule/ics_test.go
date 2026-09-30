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
	if formatted.Description != "STRUGGLE BUS is playing FC CHAIN MAIL\nDivision: Coed Over 30 B Sun\nFacility: Boise\nField: Field 2\nResult: Win (7-3)" {
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
		"DESCRIPTION:STRUGGLE BUS is playing MANEFESTO\\nDivision: Coed Over 30 B Sun\\nFacility: Boise\\nField: Field 1\\nResult: Canceled",
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

// ReplaceCanonicalResult may change only the result slot of the block Add
// wrote, and only when that block, and nothing else in the description,
// claims the result.
func TestReplaceCanonicalResult(t *testing.T) {
	// North FC beat Rivals 2-1 at home.
	played := types.Game{
		ID: "9101", PlayerTeamName: "North FC", OpponentTeamName: "Rivals", Home: "North FC", Away: "Rivals",
		DivisionName: "Coed B", Facility: &types.Facility{Name: "Boise"}, Field: "Field 1", Result: "2 - 1", StartAt: "2026-09-20T10:00:00-06:00",
	}
	// The blocks Add wrote before the game was played, from North FC's
	// schedule and from Rivals'.
	upcoming := played
	upcoming.Result = ""
	fromRivals := upcoming
	fromRivals.PlayerTeamName, fromRivals.OpponentTeamName = "Rivals", "North FC"
	block := canonicalDescription(t, &upcoming)
	rivalsBlock := canonicalDescription(t, &fromRivals)
	if block != "North FC is playing Rivals\nDivision: Coed B\nFacility: Boise\nField: Field 1\nResult: " ||
		rivalsBlock != "Rivals is playing North FC\nDivision: Coed B\nFacility: Boise\nField: Field 1\nResult: " {
		t.Fatalf("Add wrote blocks %q and %q", block, rivalsBlock)
	}
	html := func(text string) string { return strings.ReplaceAll(text, "\n", "<br>") }

	for _, tc := range []struct {
		name, description string
		// want is the description with the result written; "" means the
		// description is not the site's to change.
		want string
	}{
		{name: "blank result slot", description: block, want: block + "Win (2-1)"},
		{
			name:        "notes around the block",
			description: "Bring oranges\n\n" + block + "\nCarpool: Sam drives  ",
			want:        "Bring oranges\n\n" + block + "Win (2-1)\nCarpool: Sam drives  ",
		},
		{name: "a result in the site's format is corrected", description: block + "Win (5-0)", want: block + "Win (2-1)"},
		{name: "a canceled slot", description: block + "Canceled", want: block + "Win (2-1)"},
		{name: "already current", description: "Notes\n" + block + "Win (2-1)", want: "Notes\n" + block + "Win (2-1)"},
		{name: "a shared game worded for the team the heading names", description: rivalsBlock, want: rivalsBlock + "Loss (1-2)"},
		{
			name:        "CRLF line breaks",
			description: strings.ReplaceAll("Bring oranges\n"+block+"\nCarpool", "\n", "\r\n"),
			want:        strings.ReplaceAll("Bring oranges\n"+block+"Win (2-1)\nCarpool", "\n", "\r\n"),
		},
		{
			name:        "an HTML description saved by Google Calendar's editor",
			description: "<b>Bring oranges</b><br>" + html(block) + "<br><i>Carpool: Sam drives</i>",
			want:        "<b>Bring oranges</b><br>" + html(block+"Win (2-1)") + "<br><i>Carpool: Sam drives</i>",
		},
		{
			name:        "every HTML line break spelling",
			description: "Notes<BR />North FC is playing Rivals<br/>Division: Coed B<br />Facility: Boise<br>\nField: Field 1<br>Result: <br>After",
			want:        "Notes<BR />North FC is playing Rivals<br/>Division: Coed B<br />Facility: Boise<br>\nField: Field 1<br>Result: Win (2-1)<br>After",
		},
		{name: "an HTML description already current", description: html("Notes\n" + block + "Win (2-1)"), want: html("Notes\n" + block + "Win (2-1)")},
		{name: "a result in the visitor's words", description: block + "we won on penalties!"},
		{name: "two blocks", description: block + "\n\n" + rivalsBlock},
		{name: "a result line after the block", description: block + "\nResult: 3-0 in the replay"},
		{name: "a result line before the block", description: "Result: Win (9-0)\n" + block},
		{name: "a heading that names neither team", description: strings.Replace(block, "North FC is playing", "Someone Else is playing", 1)},
		{name: "no block", description: "Bring oranges"},
		{name: "an empty description", description: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReplaceCanonicalResult(tc.description, &played)
			if tc.want == "" {
				if ok {
					t.Fatalf("ReplaceCanonicalResult(%q) claimed the description and wrote %q", tc.description, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("ReplaceCanonicalResult(%q) = %q, %t; want %q", tc.description, got, ok, tc.want)
			}
		})
	}
}

func canonicalDescription(t *testing.T, game *types.Game) string {
	t.Helper()
	formatted, ok := CanonicalGameEvent(game)
	if !ok {
		t.Fatalf("CanonicalGameEvent(%+v) returned false", game)
	}
	return formatted.Description
}
