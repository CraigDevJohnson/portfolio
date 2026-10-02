package partials

import (
	"slices"
	"strings"
	"testing"

	"portfolio/types"
)

func TestSoccerTeamHistoryRecordText(t *testing.T) {
	for _, c := range []struct {
		record                           SoccerTeamHistoryRecord
		line, spoken, scored, notCounted string
	}{
		{
			record: SoccerTeamHistoryRecord{Wins: 2, Losses: 1, Draws: 1, ScoredGames: 4, Unclassified: 5},
			line:   "2 W · 1 L · 1 D", spoken: "2 wins, 1 loss, 1 draw", scored: "from 4 scored games", notCounted: "5 completed games not counted.",
		},
		{
			record: SoccerTeamHistoryRecord{Wins: 1, ScoredGames: 1, Unclassified: 1},
			line:   "1 W · 0 L · 0 D", spoken: "1 win, 0 losses, 0 draws", scored: "from 1 scored game", notCounted: "1 completed game not counted.",
		},
		{
			record: SoccerTeamHistoryRecord{},
			line:   "0 W · 0 L · 0 D", spoken: "0 wins, 0 losses, 0 draws", scored: "from 0 scored games", notCounted: "",
		},
	} {
		if got := soccerTeamHistoryRecordLine(c.record); got != c.line {
			t.Errorf("record line for %+v = %q, want %q", c.record, got, c.line)
		}
		if got := soccerTeamHistoryRecordSpoken(c.record); got != c.spoken {
			t.Errorf("spoken record for %+v = %q, want %q", c.record, got, c.spoken)
		}
		if got := soccerTeamHistoryScoredLine(c.record); got != c.scored {
			t.Errorf("scored line for %+v = %q, want %q", c.record, got, c.scored)
		}
		if got := soccerTeamHistoryNotCounted(c.record); got != c.notCounted {
			t.Errorf("not-counted line for %+v = %q, want %q", c.record, got, c.notCounted)
		}
	}
}

func TestSoccerTeamHistorySeasonMeta(t *testing.T) {
	if got, want := soccerTeamHistorySeasonMeta("Open A", 77, false), "Open A · LPS season 77 · Former"; got != want {
		t.Errorf("former season meta = %q, want %q", got, want)
	}
	if got, want := soccerTeamHistorySeasonMeta("", 80, true), "LPS season 80 · Current"; got != want {
		t.Errorf("current season meta without a division = %q, want %q", got, want)
	}
}

// soccerTeamHistoryFixture is Craig's view of a former season whose team
// name needs escaping, beside a current season he can switch to.
func soccerTeamHistoryFixture() SoccerTeamHistoryViewProps {
	return SoccerTeamHistoryViewProps{
		Players: []SoccerTeamHistoryPlayer{{ID: 1001, Name: "Craig Johnson", Selected: true}, {ID: 1002, Name: "Taylor Johnson"}},
		Seasons: []SoccerTeamHistorySeason{
			{PlayerID: 1001, TeamID: 4101, SeasonID: 80, TeamName: "Craig FC", Division: "Open A", Current: true},
			{PlayerID: 1001, TeamID: 4102, SeasonID: 78, TeamName: "Rovers & <Friends>", Selected: true},
		},
		Season: &SoccerTeamHistorySeasonDetail{
			TeamID: 4102, SeasonID: 78, TeamName: "Rovers & <Friends>",
			Record: SoccerTeamHistoryRecord{
				Label: "Calculated from numeric game scores; not official standings",
				Wins:  2, Losses: 1, Draws: 1, ScoredGames: 4, Unclassified: 5,
			},
			Collected: "Collected Mon 09/28/26 06:00 AM MDT · LPS returned 10 games for this season",
			Notices:   []FeedbackProps{{Kind: FeedbackWarning, Title: "Latest refresh failed", Message: "The refresh failed."}},
			Games: []SoccerTeamHistoryGame{
				{ID: 7002, Kickoff: "Mon 01/12/26 07:00 PM MST", KickoffISO: "2026-01-12T19:00:00-07:00", Opponent: "at Team 5002", Score: "2–1", Result: "Win"},
				{ID: 7001, Kickoff: "Mon 01/05/26 07:00 PM MST", KickoffISO: "2026-01-05T19:00:00-07:00", Opponent: "vs Team 5001", Score: "canceled", Result: "Not counted"},
			},
		},
	}
}

// soccerOpeningTag returns the opening tag of the element whose attributes
// include marker.
func soccerOpeningTag(t *testing.T, html, marker string) string {
	t.Helper()
	at := strings.Index(html, marker)
	if at < 0 {
		t.Fatalf("rendered view lacks %s", marker)
	}
	start := strings.LastIndex(html[:at], "<")
	end := strings.Index(html[at:], ">")
	if start < 0 || end < 0 {
		t.Fatalf("no opening tag around %s", marker)
	}
	return html[start : at+end+1]
}

func TestSoccerTeamHistoryViewRendersSwitchersDetailAndGames(t *testing.T) {
	html := renderComponent(t, SoccerTeamHistoryView(soccerTeamHistoryFixture()))

	switchAttributes := []string{
		`type="button"`,
		`hx-target="#soccer-team-history-panel"`,
		`hx-swap="innerHTML"`,
		`hx-sync="#soccer-team-history-panel:replace"`,
		`hx-indicator="#soccer-team-history-loading"`,
	}
	craig := soccerOpeningTag(t, html, `id="soccer-team-history-player-1001"`)
	taylor := soccerOpeningTag(t, html, `id="soccer-team-history-player-1002"`)
	current := soccerOpeningTag(t, html, `id="soccer-team-history-season-4101-80"`)
	former := soccerOpeningTag(t, html, `id="soccer-team-history-season-4102-78"`)
	for name, tag := range map[string]string{"Craig's button": craig, "Taylor's button": taylor, "season 80": current, "season 78": former} {
		for _, attribute := range switchAttributes {
			if !strings.Contains(tag, attribute) {
				t.Errorf("%s %s lacks %s", name, tag, attribute)
			}
		}
	}
	for tag, want := range map[string][]string{
		craig:   {`aria-pressed="true"`, `hx-get="/soccer/history/view?player_id=1001"`},
		taylor:  {`aria-pressed="false"`, `hx-get="/soccer/history/view?player_id=1002"`},
		current: {`data-soccer-team-history-season="4101/80"`, `data-current="true"`, `hx-get="/soccer/history/view?player_id=1001&amp;team_id=4101&amp;season_id=80"`},
		former:  {`data-soccer-team-history-season="4102/78"`, `data-current="false"`, `aria-current="true"`, `hx-get="/soccer/history/view?player_id=1001&amp;team_id=4102&amp;season_id=78"`},
	} {
		for _, attribute := range want {
			if !strings.Contains(tag, attribute) {
				t.Errorf("%s lacks %s", tag, attribute)
			}
		}
	}
	if got := strings.Count(html, "aria-current="); got != 1 {
		t.Errorf("aria-current appears %d times, want only on the open season", got)
	}
	if !strings.Contains(html, `<ol class="soccer-team-history-seasons" aria-label="Team seasons, newest first">`) {
		t.Error("the seasons are not an ordered list labeled newest first")
	}

	detail := soccerOpeningTag(t, html, `data-soccer-team-history-detail="4102/78"`)
	if !strings.HasPrefix(detail, "<article") {
		t.Errorf("season detail is %s, want an article", detail)
	}
	if !strings.Contains(html, `id="soccer-team-history-detail-title">Rovers &amp; &lt;Friends&gt;</h3>`) || strings.Contains(html, "<Friends>") {
		t.Error("the team name is not escaped in the season heading")
	}
	for _, want := range []string{
		`<span aria-hidden="true">2 W · 1 L · 1 D</span>`,
		`<span class="sr-only">2 wins, 1 loss, 1 draw</span>`,
		"from 4 scored games",
		"Calculated from numeric game scores; not official standings. 5 completed games not counted.",
		"Collected Mon 09/28/26 06:00 AM MDT · LPS returned 10 games for this season",
		"Latest refresh failed",
		"LPS season 78 · Former",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("season detail lacks %q", want)
		}
	}
	if notice := soccerOpeningTag(t, html, "soccer-team-history-notice"); !strings.Contains(notice, "ui-feedback") {
		t.Errorf("season notice %s is not a shared feedback", notice)
	}

	games := strings.Index(html, `<ol class="soccer-team-history-games" aria-label="Completed games, newest first">`)
	newer := strings.Index(html, `data-soccer-team-history-game="7002"`)
	older := strings.Index(html, `data-soccer-team-history-game="7001"`)
	if games < 0 || newer < games || older < newer {
		t.Fatalf("games list at %d, 7002 at %d, 7001 at %d; want 7002 then 7001 inside the labeled list", games, newer, older)
	}
	for _, want := range []string{
		`<time datetime="2026-01-12T19:00:00-07:00">Mon 01/12/26 07:00 PM MST</time>`,
		">at Team 5002<", ">2–1<", ">Win<", ">canceled<", ">Not counted<",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("games lack %q", want)
		}
	}
}

func TestSoccerTeamHistoryViewRendersARefusalWithoutSeasonsOrGames(t *testing.T) {
	props := soccerTeamHistoryFixture()
	props.Notices = []FeedbackProps{{Kind: FeedbackInfo, Title: "Current teams not confirmed"}}
	props.Refusal = &FeedbackProps{Kind: FeedbackError, Title: "Not available", Message: "Team-season membership is unverified."}

	html := renderComponent(t, SoccerTeamHistoryView(props))

	for _, want := range []string{`role="alert"`, "soccer-team-history-refusal", "Team-season membership is unverified.", `id="soccer-team-history-player-1001"`} {
		if !strings.Contains(html, want) {
			t.Errorf("refusal lacks %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"data-soccer-team-history-season", "data-soccer-team-history-detail", "data-soccer-team-history-game", "Current teams not confirmed"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("refusal shows %q", unwanted)
		}
	}
}

func TestSoccerTeamHistoryViewOmitsEmptyLists(t *testing.T) {
	html := renderComponent(t, SoccerTeamHistoryView(SoccerTeamHistoryViewProps{
		Players: []SoccerTeamHistoryPlayer{{ID: 1001, Name: "Craig Johnson", Selected: true}},
		Notices: []FeedbackProps{{Kind: FeedbackInfo, Title: "No team seasons yet", Message: "No team seasons are recorded for Craig Johnson."}},
	}))
	if !strings.Contains(html, "No team seasons yet") {
		t.Error("the list notice is missing")
	}
	for _, unwanted := range []string{"soccer-team-history-seasons", "soccer-team-history-detail", "soccer-team-history-games"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("a view without seasons renders %q", unwanted)
		}
	}

	noGames := soccerTeamHistoryFixture()
	noGames.Season.Games = nil
	if html := renderComponent(t, SoccerTeamHistoryView(noGames)); strings.Contains(html, "soccer-team-history-games") {
		t.Error("a season without completed games renders an empty games list")
	}
}

// The section names each player as the loaded view does, so a button keeps
// its label once a player is chosen.
func TestSoccerTeamHistorySectionPlayersMatchTheViewsNames(t *testing.T) {
	got := soccerTeamHistorySectionPlayers([]types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson"}, {UPlayerID: 1003}})
	want := []SoccerTeamHistoryPlayer{{ID: 1001, Name: "Craig Johnson"}, {ID: 1003, Name: "Player 1003"}}
	if !slices.Equal(got, want) {
		t.Errorf("section players = %+v, want %+v", got, want)
	}
}

// Focus stays on the pressed button through a swap, and the loading status
// and each notice announce themselves, so the panel itself is not a live
// region that would read every player, season, and game again on each
// switch.
func TestSoccerTeamHistorySectionPanelIsNotALiveRegion(t *testing.T) {
	html := renderComponent(t, SoccerTeamHistorySection(SoccerLoginStateProps{
		TeamHistoryAvailable: true, Authenticated: true, Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson"}},
	}, false))
	if panel := soccerOpeningTag(t, html, `id="soccer-team-history-panel"`); strings.Contains(panel, "aria-live") {
		t.Errorf("the panel is a live region: %s", panel)
	}
	if loading := soccerOpeningTag(t, html, `id="soccer-team-history-loading"`); !strings.Contains(loading, `role="status"`) {
		t.Errorf("the loading indicator is not a status: %s", loading)
	}
}
