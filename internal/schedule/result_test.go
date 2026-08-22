package schedule

import "testing"

func TestParseGameResultAndFormatResultLine(t *testing.T) {
	tests := []struct {
		name           string
		result         string
		playerTeamName string
		homeTeamName   string
		wantParsed     bool
		wantOutcome    string
		wantFormatted  string
	}{
		{
			name:          "empty result",
			result:        "",
			wantParsed:    false,
			wantOutcome:   "",
			wantFormatted: "",
		},
		{
			name:           "player home win",
			result:         "7 - 3",
			playerTeamName: "Team A",
			homeTeamName:   "Team A",
			wantParsed:     true,
			wantOutcome:    "Win",
			wantFormatted:  "Win (7-3)",
		},
		{
			name:           "player away loss",
			result:         "7 - 3",
			playerTeamName: "Team B",
			homeTeamName:   "Team A",
			wantParsed:     true,
			wantOutcome:    "Loss",
			wantFormatted:  "Loss (3-7)",
		},
		{
			name:           "draw",
			result:         "3 - 3",
			playerTeamName: "Team A",
			homeTeamName:   "Team A",
			wantParsed:     true,
			wantOutcome:    "Draw",
			wantFormatted:  "Draw (3-3)",
		},
		{
			name:          "canceled case insensitive",
			result:        "CaNcElEd",
			wantParsed:    true,
			wantOutcome:   "Canceled",
			wantFormatted: "Canceled",
		},
		{
			name:          "final case insensitive",
			result:        "fInAl",
			wantParsed:    true,
			wantOutcome:   "Final",
			wantFormatted: "Final",
		},
		{
			name:          "unknown format",
			result:        "postponed",
			wantParsed:    false,
			wantOutcome:   "postponed",
			wantFormatted: "postponed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outcome := ParseGameResult(tc.result, tc.playerTeamName, tc.homeTeamName)
			if outcome.Parsed != tc.wantParsed {
				t.Fatalf("Parsed = %v, want %v", outcome.Parsed, tc.wantParsed)
			}
			if outcome.Outcome != tc.wantOutcome {
				t.Fatalf("Outcome = %q, want %q", outcome.Outcome, tc.wantOutcome)
			}
			if got := FormatResultLine(outcome); got != tc.wantFormatted {
				t.Fatalf("FormatResultLine() = %q, want %q", got, tc.wantFormatted)
			}
		})
	}
}
