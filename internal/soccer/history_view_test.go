package soccer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/lps"
	"portfolio/internal/soccerarchive"
)

// craigFCSeason77Games are Craig FC's (4101) LPS season-77 games as the
// archive reads them back: every kind of result, one game whose sides no
// Team ID places, and one future game.
const craigFCSeason77Games = `[
{"UGameID":7001,"Season":77,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2026-01-05T19:00:00Z","result":"3 - 1"},
{"UGameID":7002,"Season":77,"UTeam1":5002,"UTeam2":4101,"SchedGameDateTime":"2026-01-12T19:00:00Z","result":"1 - 2"},
{"UGameID":7003,"Season":77,"UTeam1":5003,"UTeam2":4101,"SchedGameDateTime":"2026-01-19T19:00:00Z","result":"4 - 0"},
{"UGameID":7004,"Season":77,"UTeam1":4101,"UTeam2":5004,"SchedGameDateTime":"2026-01-26T19:00:00Z","result":"2 - 2"},
{"UGameID":7005,"Season":77,"UTeam1":4101,"UTeam2":5005,"SchedGameDateTime":"2026-02-02T19:00:00Z","result":"canceled"},
{"UGameID":7006,"Season":77,"UTeam1":5006,"UTeam2":4101,"SchedGameDateTime":"2026-02-09T19:00:00Z","result":""},
{"UGameID":7007,"Season":77,"UTeam1":4101,"UTeam2":5007,"SchedGameDateTime":"2026-02-16T19:00:00Z","result":"Final"},
{"UGameID":7008,"Season":77,"UTeam1":4101,"UTeam2":5008,"SchedGameDateTime":"2026-02-23T19:00:00Z","result":"Forfeit"},
{"UGameID":7009,"Season":77,"SchedGameDateTime":"2026-03-02T19:00:00Z","result":"8 - 0","home_team":{"team_name":"Craig FC"},"visitor_team":{"team_name":"Rivals"}},
{"UGameID":7010,"Season":77,"UTeam1":4101,"UTeam2":5001,"SchedGameDateTime":"2099-03-09T19:00:00Z","result":""}]`

func TestTeamHistoryGamesPlaceEachSideAndScoreFromTheTeam(t *testing.T) {
	var games []lps.TeamScheduleGame
	if err := json.Unmarshal([]byte(craigFCSeason77Games), &games); err != nil {
		t.Fatalf("decode games: %v", err)
	}
	season := soccerarchive.TeamSeason{Team: lps.TeamSummary{UTeamID: 4101, TeamName: "Craig FC", Season: 77}, Games: games}
	response := buildHistoryResponse(&season, 1001, 4101, 77, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))

	shown := teamHistoryGames(response.Games, 4101)

	got := make([]string, 0, len(shown))
	for _, game := range shown {
		got = append(got, fmt.Sprintf("%d %q %q %s", game.ID, game.Opponent, game.Score, game.Result))
	}
	want := []string{
		`7009 "Craig FC vs Rivals" "8 - 0" Not counted`,
		`7008 "vs Team 5008" "Forfeit" Not counted`,
		`7007 "vs Team 5007" "Final" Not counted`,
		`7006 "at Team 5006" "No score" Not counted`,
		`7005 "vs Team 5005" "canceled" Not counted`,
		`7004 "vs Team 5004" "2–2" Draw`,
		`7003 "at Team 5003" "0–4" Loss`,
		`7002 "at Team 5002" "2–1" Win`,
		`7001 "vs Team 5001" "3–1" Win`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("games =\n%s\nwant\n%s", got, want)
	}
	oldest := shown[len(shown)-1]
	if oldest.Kickoff != "Mon 01/05/26 07:00 PM MST" || oldest.KickoffISO != "2026-01-05T19:00:00-07:00" {
		t.Errorf("game 7001 kickoff = %q (%q), want Mon 01/05/26 07:00 PM MST", oldest.Kickoff, oldest.KickoffISO)
	}
}

func TestTeamSeasonNoticesNameEachCollectionState(t *testing.T) {
	fetchedAt := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	failedAt := fetchedAt.Add(24 * time.Hour)
	notFetched := historyCoverage{Status: soccerarchive.CoverageNotFetched}
	fetched := func(returned int) historyCoverage {
		return historyCoverage{Status: soccerarchive.CoverageFetched, FetchedAt: &fetchedAt, ReturnedGameCount: returned}
	}
	played := []historyGame{{Classification: "win"}}
	ready := &historyRefresh{Status: soccerarchive.RefreshReady, LastAttemptAt: &fetchedAt, NextDueAt: &failedAt}
	failed := func(kind lps.ErrorKind, status int) *historyRefresh {
		return &historyRefresh{Status: soccerarchive.RefreshRetryable, LastAttemptAt: &failedAt, LastErrorKind: kind, LastErrorStatusCode: status}
	}
	const pending = "No complete LPS response for this season has been saved yet."
	failure := func(reason string) partials.FeedbackProps {
		return partials.FeedbackProps{
			Kind: partials.FeedbackWarning, Title: "Latest refresh failed",
			Message: "The refresh at Tue 09/29/26 06:00 AM MDT failed: " + reason + ". Showing the copy collected Mon 09/28/26 06:00 AM MDT.",
		}
	}
	for _, c := range []struct {
		name    string
		history historyResponse
		want    []partials.FeedbackProps
	}{
		{
			name:    "awaiting the first collection",
			history: historyResponse{Coverage: notFetched, Refresh: &historyRefresh{Status: soccerarchive.RefreshReady, NextDueAt: &failedAt}},
			want:    []partials.FeedbackProps{{Kind: partials.FeedbackInfo, Title: "Not collected yet", Message: pending + " The next refresh is due Tue 09/29/26 06:00 AM MDT."}},
		},
		{
			name:    "a season LPS returned empty",
			history: historyResponse{Coverage: fetched(0), Refresh: ready},
			want:    []partials.FeedbackProps{{Kind: partials.FeedbackInfo, Title: "No games this season", Message: "LPS returned no games for this season."}},
		},
		{
			name:    "a season with only future games",
			history: historyResponse{Coverage: fetched(2), Refresh: ready},
			want:    []partials.FeedbackProps{{Kind: partials.FeedbackInfo, Title: "No completed games yet", Message: "No game LPS returned for this season has kicked off yet."}},
		},
		{
			name:    "a collected season with no news",
			history: historyResponse{Coverage: fetched(10), Refresh: ready, Games: played},
		},
		{
			name:    "an upstream failure",
			history: historyResponse{Coverage: fetched(10), Refresh: failed(lps.ErrorUpstream, 503), Games: played},
			want:    []partials.FeedbackProps{failure("LPS answered HTTP 503")},
		},
		{
			name:    "a response that could not be saved",
			history: historyResponse{Coverage: fetched(10), Refresh: failed(soccerarchive.RefreshStoreErrorKind, 0), Games: played},
			want:    []partials.FeedbackProps{failure("the response LPS sent could not be saved")},
		},
		{
			name:    "a schedule LPS refused",
			history: historyResponse{Coverage: fetched(10), Refresh: failed(lps.ErrorTeamRefused, 403), Games: played},
			want:    []partials.FeedbackProps{failure("LPS refused to share this team's schedule")},
		},
		{
			name:    "an unreachable LPS",
			history: historyResponse{Coverage: fetched(10), Refresh: failed(lps.ErrorUpstream, 0), Games: played},
			want:    []partials.FeedbackProps{failure("LPS could not be reached")},
		},
		{
			name:    "a first collection that failed",
			history: historyResponse{Coverage: notFetched, Refresh: failed(lps.ErrorUpstream, 503)},
			want: []partials.FeedbackProps{
				{Kind: partials.FeedbackInfo, Title: "Not collected yet", Message: pending},
				{Kind: partials.FeedbackWarning, Title: "Latest refresh failed", Message: "The refresh at Tue 09/29/26 06:00 AM MDT failed: LPS answered HTTP 503."},
			},
		},
		{
			name: "a Team ID LPS rejects",
			history: historyResponse{Coverage: notFetched, Refresh: &historyRefresh{
				Status: soccerarchive.RefreshInvalid, LastAttemptAt: &failedAt, LastErrorKind: lps.ErrorInvalidTeam, LastErrorStatusCode: 404,
			}},
			want: []partials.FeedbackProps{
				{Kind: partials.FeedbackInfo, Title: "Not collected yet", Message: pending},
				{Kind: partials.FeedbackWarning, Title: "LPS no longer accepts this team", Message: "Collected history is kept, but this team is no longer refreshed."},
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := teamSeasonNotices(&c.history); !reflect.DeepEqual(got, c.want) {
				t.Errorf("notices = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestTeamHistoryListNoticesExplainAShortOrEmptyList(t *testing.T) {
	unconfirmed := partials.FeedbackProps{
		Kind: partials.FeedbackInfo, Title: "Current teams not confirmed",
		Message: "LPS could not confirm Craig Johnson's current teams just now, so only seasons recorded by earlier imports are listed.",
	}
	none := partials.FeedbackProps{
		Kind: partials.FeedbackInfo, Title: "No team seasons yet",
		Message: "No team seasons are recorded for Craig Johnson. Seasons come from LPS imports that link this player; Team IDs entered by hand never grant history.",
	}
	for _, c := range []struct {
		verified bool
		seasons  int
		want     []partials.FeedbackProps
	}{
		{verified: true, seasons: 3},
		{verified: false, seasons: 2, want: []partials.FeedbackProps{unconfirmed}},
		{verified: true, seasons: 0, want: []partials.FeedbackProps{none}},
		{verified: false, seasons: 0, want: []partials.FeedbackProps{unconfirmed, none}},
	} {
		if got := teamHistoryListNotices("Craig Johnson", c.verified, c.seasons); !reflect.DeepEqual(got, c.want) {
			t.Errorf("list notices (verified %t, %d seasons) = %+v, want %+v", c.verified, c.seasons, got, c.want)
		}
	}
}
