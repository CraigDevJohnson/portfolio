package app

import (
	"context"
	"net/http"
	"slices"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/schedule"
	"portfolio/internal/soccerarchive"
)

// previewHistoryUnconfirmedPlayer is the history account's player whose
// current team lookup the fake LPS cannot answer.
const previewHistoryUnconfirmedPlayer = "1669083"

// historyRoutes serves the Team history journey's fake LPS: the linked
// journey's players and teams, plus Robin, whom LPS lists on no team, and
// Jordan, whose current team lookup LPS cannot answer.
func (fake *soccerPreviewLPS) historyRoutes() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /teams/{id}", fake.teamScheduleHandler)
	routes.HandleFunc("GET /users/check", previewAccountCheck(`{"first_name":"Craig","last_name":"Johnson","players":[`+
		`{"UPlayerID":1669080,"FirstName":"Craig","LastName":"Johnson","is_main_player":true},`+
		`{"UPlayerID":1669081,"FirstName":"Taylor Alexandra","LastName":"Johnson-Summit"},`+
		`{"UPlayerID":1669082,"FirstName":"Robin","LastName":"Johnson"},`+
		`{"UPlayerID":1669083,"FirstName":"Jordan","LastName":"Johnson"}],`+
		`"user_players":[{"player_id":1669080,"deleted":false},{"player_id":1669081,"deleted":false},`+
		`{"player_id":1669082,"deleted":false},{"player_id":1669083,"deleted":false}]}`))
	routes.HandleFunc("GET /players/{id}/my_teams", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == previewHistoryUnconfirmedPlayer {
			if previewBearer(w, r) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"message":"service unavailable"}`))
			}
			return
		}
		previewPlayerTeams(w, r)
	})
	return routes
}

// previewHistoryStore is the history account's in-memory durable archive:
// fixed team seasons, refresh states, and the preview account's stored
// membership proof, built once and only read. It implements neither
// soccerarchive.MembershipStore nor soccerarchive.PlayerRemovalStore, so the
// account's import collects nothing and offers no removal, and it discards a
// Team ID lookup's snapshot.
type previewHistoryStore struct {
	memberships map[int][]soccerarchive.PlayerMembership
	seasons     map[[2]int]soccerarchive.TeamSeason
	refresh     map[int]soccerarchive.RefreshState
}

// newPreviewHistoryStore seeds the fixture as of now, so it reaches every
// Team history state: Craig's current season with a scored record, his
// former seasons LPS returned empty, failed to refresh, and no longer
// accepts; Taylor's season with no game played yet; Robin, with no proven
// season; and Jordan, whose only season awaits its first collection.
func newPreviewHistoryStore(now time.Time) *previewHistoryStore {
	fake := newSoccerPreviewLPS()
	fake.now = func() time.Time { return now }
	location := schedule.MountainTimeLocation()
	today := now.In(location)
	at := func(dayOffset, hour int) time.Time {
		return time.Date(today.Year(), today.Month(), today.Day()+dayOffset, hour, 0, 0, 0, location)
	}

	pondMint := lps.TeamSummary{UTeamID: previewLPSPondMintTeamID, TeamName: "Pond Mint United", Season: 169}
	campfire := lps.TeamSummary{UTeamID: previewLPSCampfireTeamID, TeamName: "Campfire Rovers", Season: 170}
	wanderers := lps.TeamSummary{UTeamID: 479801, TeamName: "Candle Oat Wanderers", DivisionName: "Coed Open B", Season: 168}
	mulberry := lps.TeamSummary{UTeamID: 479802, TeamName: "Night Mulberry FC", DivisionName: "Coed Open B", Season: 167}
	rosehip := lps.TeamSummary{UTeamID: previewLPSRosehipTeamID, TeamName: "Rosehip Athletic", DivisionName: "Coed Rec", Season: 166}
	lantern := lps.TeamSummary{UTeamID: 479803, TeamName: "Lantern Hill FC", Season: 165}
	fetched := func(dayOffset, returned int) soccerarchive.Coverage {
		return soccerarchive.Coverage{Status: soccerarchive.CoverageFetched, FetchedAt: at(dayOffset, 6), ReturnedGameCount: returned}
	}
	ready := func(teamID int) soccerarchive.RefreshState {
		return soccerarchive.RefreshState{TeamID: teamID, Status: soccerarchive.RefreshReady, LastAttemptAt: at(-1, 6), NextDueAt: at(1, 5)}
	}
	failedAt := now.Add(-2 * time.Hour).Truncate(time.Minute)

	return &previewHistoryStore{
		memberships: map[int][]soccerarchive.PlayerMembership{
			1669080: {{PlayerID: 1669080, Team: wanderers}, {PlayerID: 1669080, Team: mulberry}, {PlayerID: 1669080, Team: rosehip}},
			1669083: {{PlayerID: 1669083, Team: lantern}},
		},
		seasons: map[[2]int]soccerarchive.TeamSeason{
			{pondMint.UTeamID, 169}: {Team: pondMint, Coverage: fetched(-1, 6), Games: []lps.TeamScheduleGame{
				fake.game(8101, -5, "Field 1", &pondMint, &mulberry, "4 - 2"),
				fake.game(8102, -12, "Field 2", &campfire, &pondMint, "2 - 2"),
				fake.game(8103, -19, "Field 1", &pondMint, &wanderers, "0 - 1"),
				fake.game(8104, -26, "Field 3", &pondMint, &rosehip, "postponed"),
				fake.game(8105, 9, "Field 1", &pondMint, &mulberry, ""),
				fake.game(8106, 16, "Field 2", &wanderers, &pondMint, ""),
			}},
			{wanderers.UTeamID, 168}: {Team: wanderers, Coverage: fetched(-300, 0)},
			{mulberry.UTeamID, 167}: {Team: mulberry, Coverage: fetched(-400, 2), Games: []lps.TeamScheduleGame{
				fake.game(8121, -420, "Field 4", &mulberry, &pondMint, "3 - 1"),
				fake.game(8122, -427, "Field 4", &mulberry, &campfire, "1 - 2"),
			}},
			{rosehip.UTeamID, 166}: {Team: rosehip, Coverage: fetched(-450, 1), Games: []lps.TeamScheduleGame{
				fake.game(8131, -460, "Field 2", &rosehip, &wanderers, "1 - 1"),
			}},
			{campfire.UTeamID, 170}: {Team: campfire, Coverage: fetched(-1, 2), Games: []lps.TeamScheduleGame{
				fake.game(8141, 2, "Field 3", &campfire, &wanderers, ""),
				fake.game(8142, 9, "Field 4", &mulberry, &campfire, ""),
			}},
		},
		refresh: map[int]soccerarchive.RefreshState{
			pondMint.UTeamID:  ready(pondMint.UTeamID),
			wanderers.UTeamID: ready(wanderers.UTeamID),
			mulberry.UTeamID: {
				TeamID: mulberry.UTeamID, Status: soccerarchive.RefreshRetryable, LastAttemptAt: failedAt, NextDueAt: failedAt.Add(15 * time.Minute),
				LastErrorKind: lps.ErrorUpstream, LastErrorStatusCode: http.StatusServiceUnavailable,
			},
			rosehip.UTeamID: {
				TeamID: rosehip.UTeamID, Status: soccerarchive.RefreshInvalid, LastAttemptAt: at(-30, 6),
				LastErrorKind: lps.ErrorInvalidTeam, LastErrorStatusCode: http.StatusNotFound,
			},
			campfire.UTeamID: ready(campfire.UTeamID),
			lantern.UTeamID:  ready(lantern.UTeamID),
		},
	}
}

// SaveTeamSnapshot discards a Team ID lookup's snapshot: the fixture stays
// as seeded.
func (*previewHistoryStore) SaveTeamSnapshot(context.Context, *soccerarchive.Snapshot) error {
	return nil
}

// ListPlayerMemberships returns the stored proof the preview account holds.
func (s *previewHistoryStore) ListPlayerMemberships(_ context.Context, ownerIssuer, ownerSubject string, playerID int) ([]soccerarchive.PlayerMembership, error) {
	owner := previewAccountPrincipal()
	if ownerIssuer != owner.Issuer || ownerSubject != owner.Subject {
		return nil, nil
	}
	return slices.Clone(s.memberships[playerID]), nil
}

func (s *previewHistoryStore) HasPlayerMembership(ctx context.Context, ownerIssuer, ownerSubject string, playerID, teamID, seasonID int) (bool, error) {
	memberships, err := s.ListPlayerMemberships(ctx, ownerIssuer, ownerSubject, playerID)
	return slices.ContainsFunc(memberships, func(membership soccerarchive.PlayerMembership) bool {
		return membership.Team.UTeamID == teamID && membership.Team.Season == seasonID
	}), err
}

func (s *previewHistoryStore) ReadTeamSeason(_ context.Context, teamID, seasonID int) (soccerarchive.TeamSeason, error) {
	season, ok := s.seasons[[2]int{teamID, seasonID}]
	if !ok {
		return soccerarchive.TeamSeason{}, soccerarchive.ErrNoArchive
	}
	season.Games = slices.Clone(season.Games)
	return season, nil
}

func (s *previewHistoryStore) ReadRefreshState(_ context.Context, teamID int) (soccerarchive.RefreshState, error) {
	state, ok := s.refresh[teamID]
	if !ok {
		return soccerarchive.RefreshState{}, soccerarchive.ErrNotEnrolled
	}
	return state, nil
}
