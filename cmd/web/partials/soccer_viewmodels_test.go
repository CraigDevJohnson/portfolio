package partials

import (
	stdhtml "html"
	"strings"
	"testing"

	"portfolio/types"
)

func TestSoccerLoginStateCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		props         SoccerLoginStateProps
		wantDiscovery bool
		wantGoogle    bool
	}{
		{name: "manual only"},
		{name: "login enabled", props: SoccerLoginStateProps{LoginAvailable: true}, wantDiscovery: true},
		{name: "authenticated only", props: SoccerLoginStateProps{Authenticated: true}, wantDiscovery: true},
		{name: "google available", props: SoccerLoginStateProps{GoogleAvailable: true}, wantGoogle: true},
		{name: "google connected", props: SoccerLoginStateProps{GoogleConnected: true}, wantGoogle: true},
		{name: "calendars only", props: SoccerLoginStateProps{GoogleCalendars: []types.GoogleCalendarOption{{ID: "calendar-1", Summary: "Matchdays"}}}, wantGoogle: true},
		{name: "selected calendar only", props: SoccerLoginStateProps{SelectedGoogleCalendarID: "calendar-1"}, wantGoogle: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.props.SupportsDiscovery(); got != test.wantDiscovery {
				t.Errorf("SupportsDiscovery() = %v, want %v", got, test.wantDiscovery)
			}
			if got := test.props.SupportsGoogle(); got != test.wantGoogle {
				t.Errorf("SupportsGoogle() = %v, want %v", got, test.wantGoogle)
			}
		})
	}

	// Team history needs a server whose history store is wired for this
	// granted visitor, and an import with at least one linked player.
	players := []types.LPSPlayer{{UPlayerID: 1001}}
	for _, history := range []struct {
		name  string
		props SoccerLoginStateProps
		want  bool
	}{
		{name: "history for an import with players", props: SoccerLoginStateProps{TeamHistoryAvailable: true, Authenticated: true, Players: players}, want: true},
		{name: "history without an import", props: SoccerLoginStateProps{TeamHistoryAvailable: true, Players: players}},
		{name: "history for an import without players", props: SoccerLoginStateProps{TeamHistoryAvailable: true, Authenticated: true}},
		{name: "an import without history", props: SoccerLoginStateProps{Authenticated: true, Players: players}},
	} {
		t.Run(history.name, func(t *testing.T) {
			if got := history.props.ShowsTeamHistory(); got != history.want {
				t.Errorf("ShowsTeamHistory() = %v, want %v", got, history.want)
			}
		})
	}
}

func TestSoccerSecurityNoticeVariantsShareCanonicalCopy(t *testing.T) {
	const canonicalMeaning = "Player Discovery uses the JWT (JSON web token) from your signed-in Let's Play Soccer account, which gives full access to your account (only used to query players/teams/schedules/results)."

	for _, compact := range []bool{false, true} {
		t.Run(map[bool]string{false: "full", true: "compact"}[compact], func(t *testing.T) {
			rendered := renderComponent(t, SoccerSecurityNotice(SoccerSecurityNoticeProps{Compact: compact}))
			if got := strings.Count(rendered, `data-soccer-security-notice`); got != 1 {
				t.Fatalf("security notice marker count = %d, want 1: %s", got, rendered)
			}
			plain := stdhtml.UnescapeString(rendered)
			plain = strings.Join(strings.Fields(stripHTMLTags(plain)), " ")
			if !strings.Contains(plain, canonicalMeaning) {
				t.Fatalf("security notice does not preserve canonical meaning\nwant: %q\n got: %q", canonicalMeaning, plain)
			}
		})
	}
}

func stripHTMLTags(value string) string {
	var output strings.Builder
	inTag := false
	for _, char := range value {
		switch char {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				output.WriteRune(char)
			}
		}
	}
	return output.String()
}
