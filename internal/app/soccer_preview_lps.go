package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"portfolio/internal/lps"
	"portfolio/internal/schedule"
)

// Preview team IDs served by the fake Let's Play Soccer API.
const (
	previewLPSPondMintTeamID = 479691
	previewLPSCampfireTeamID = 479147
	previewLPSRosehipTeamID  = 479800
)

// previewLPSBaseURL is the LPS API base URL the loopback preview uses, so a
// browser proof drives the real public Team ID routes against canned data.
func previewLPSBaseURL(listenAddress string) string {
	return "http://" + listenAddress + "/__preview/lps"
}

// soccerPreviewLPS stands in for the Let's Play Soccer team schedule API in
// the loopback preview. Pond Mint United publishes one more upcoming game
// after its first schedule request, so a proof can refetch the same team set
// and find a newly discovered game. Relaunch the preview to start over.
// Scored past games from both teams, one from over a year ago, and a
// postponed game without a score let a proof review past results in Google
// mode. Like every recorded live payload, Pond Mint United and Campfire
// Rovers, the page's example Team IDs, name no color, and both IDs select the
// same fallback. Rosehip Athletic, their opponent, names its color as LPS
// might, in mixed case with padding, and serves its own schedule, so adding it
// to the example teams shows a recognized LPS color beside their fallbacks.
type soccerPreviewLPS struct {
	mu       sync.Mutex
	requests map[int]int
	now      func() time.Time
}

func newSoccerPreviewLPS() *soccerPreviewLPS {
	return &soccerPreviewLPS{requests: make(map[int]int), now: time.Now}
}

func (fake *soccerPreviewLPS) teamScheduleHandler(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || (teamID != previewLPSPondMintTeamID && teamID != previewLPSCampfireTeamID && teamID != previewLPSRosehipTeamID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"team not found"}`))
		return
	}

	fake.mu.Lock()
	fake.requests[teamID]++
	published := fake.requests[teamID] > 1
	fake.mu.Unlock()

	pondMint := lps.TeamSummary{UTeamID: previewLPSPondMintTeamID, TeamName: "Pond Mint United"}
	campfire := lps.TeamSummary{UTeamID: previewLPSCampfireTeamID, TeamName: "Campfire Rovers"}
	rosehip := lps.TeamSummary{UTeamID: previewLPSRosehipTeamID, TeamName: "Rosehip Athletic", Color: "  kelly GREEN "}
	wanderers := lps.TeamSummary{UTeamID: 479801, TeamName: "Candle Oat Wanderers"}
	mulberry := lps.TeamSummary{UTeamID: 479802, TeamName: "Night Mulberry FC"}

	shared := fake.game(7001, 9, "Field 1", &pondMint, &campfire, "")
	rosehipHome := fake.game(7002, 16, "Field 2", &rosehip, &pondMint, "")
	rosehipAway := fake.game(6990, -400, "Field 2", &campfire, &rosehip, "1 - 3")
	response := lps.TeamScheduleResponse{}
	switch teamID {
	case previewLPSPondMintTeamID:
		response.Team = pondMint
		response.Games = []lps.TeamScheduleGame{
			fake.game(7000, -5, "Field 1", &pondMint, &mulberry, "4 - 2"),
			shared,
			rosehipHome,
		}
		if published {
			response.Games = append(response.Games, fake.game(7004, 23, "Field 4", &pondMint, &wanderers, ""))
		}
	case previewLPSCampfireTeamID:
		response.Team = campfire
		response.Games = []lps.TeamScheduleGame{
			rosehipAway,
			fake.game(6995, -12, "Field 3", &campfire, &wanderers, "postponed"),
			fake.game(6998, -19, "Field 4", &mulberry, &campfire, "2 - 2"),
			fake.game(7003, 2, "Field 3", &campfire, &wanderers, ""),
			shared,
		}
	case previewLPSRosehipTeamID:
		response.Team = rosehip
		response.Games = []lps.TeamScheduleGame{rosehipAway, rosehipHome}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// game returns an LPS game dayOffset days from today at 7:15 PM Mountain time.
// LPS labels local wall-clock times with a Z suffix, which the fake repeats.
func (fake *soccerPreviewLPS) game(id, dayOffset int, field string, home, visitor *lps.TeamSummary, result string) lps.TeamScheduleGame {
	location := schedule.MountainTimeLocation()
	today := fake.now().In(location)
	start := time.Date(today.Year(), today.Month(), today.Day()+dayOffset, 19, 15, 0, 0, location)
	return lps.TeamScheduleGame{
		UGameID:           id,
		FieldName:         field,
		SchedGameDateTime: start.Format("2006-01-02T15:04:05.000") + "Z",
		FacilityName:      "Treasure Valley Fieldhouse",
		Result:            result,
		UTeam1:            home.UTeamID,
		UTeam2:            visitor.UTeamID,
		HomeTeam:          *home,
		VisitorTeam:       *visitor,
	}
}
