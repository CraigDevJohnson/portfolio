package partials

import (
	"fmt"
	"strconv"
	"strings"

	"portfolio/internal/schedule"
)

// SoccerTeamHistoryViewPath is the private Team history fragment route. Each
// player and season button asks it for the panel's next content.
const SoccerTeamHistoryViewPath = "/soccer/history/view"

// SoccerTeamHistoryViewProps is the Team history panel: the linked players,
// then either a refusal or one player's proven team seasons with the open
// season's record and completed games.
type SoccerTeamHistoryViewProps struct {
	Players []SoccerTeamHistoryPlayer
	// Notices explain the season list: LPS could not confirm the player's
	// current teams, or no team season is proven for the player.
	Notices []FeedbackProps
	Seasons []SoccerTeamHistorySeason
	// Season is the open team season; nil when none is proven.
	Season *SoccerTeamHistorySeasonDetail
	// Refusal replaces the notices, seasons and season when the request was
	// refused.
	Refusal *FeedbackProps
}

// SoccerTeamHistoryPlayer is one linked player's button.
type SoccerTeamHistoryPlayer struct {
	ID       int
	Name     string
	Selected bool
}

// SoccerTeamHistorySeason is one proven team season in the season list.
type SoccerTeamHistorySeason struct {
	PlayerID int
	TeamID   int
	SeasonID int
	TeamName string
	Division string
	Current  bool
	Selected bool
}

// SoccerTeamHistorySeasonDetail is the open team season.
type SoccerTeamHistorySeasonDetail struct {
	TeamID   int
	SeasonID int
	TeamName string
	Division string
	Current  bool
	Record   SoccerTeamHistoryRecord
	// Collected is the status line of a fetched season; empty otherwise.
	Collected string
	Notices   []FeedbackProps
	Games     []SoccerTeamHistoryGame
}

// SoccerTeamHistoryRecord is a team-relative record calculated from scored
// games. Unclassified completed games are listed but not counted.
type SoccerTeamHistoryRecord struct {
	Label        string
	Wins         int
	Losses       int
	Draws        int
	ScoredGames  int
	Unclassified int
}

// SoccerTeamHistoryGame is one completed game as the team played it.
type SoccerTeamHistoryGame struct {
	ID int
	// Kickoff is the display time in Mountain Time; KickoffISO is the same
	// time in RFC 3339 for the time element.
	Kickoff    string
	KickoffISO string
	Opponent   string
	Score      string
	// Result is Win, Loss, Draw, or Not counted.
	Result string
}

func soccerTeamHistoryRecordLine(record SoccerTeamHistoryRecord) string {
	return fmt.Sprintf("%d W · %d L · %d D", record.Wins, record.Losses, record.Draws)
}

func soccerTeamHistoryRecordSpoken(record SoccerTeamHistoryRecord) string {
	return strings.Join([]string{
		soccerTeamHistoryCount(record.Wins, "win", "wins"),
		soccerTeamHistoryCount(record.Losses, "loss", "losses"),
		soccerTeamHistoryCount(record.Draws, "draw", "draws"),
	}, ", ")
}

func soccerTeamHistoryScoredLine(record SoccerTeamHistoryRecord) string {
	return "from " + soccerTeamHistoryCount(record.ScoredGames, "scored game", "scored games")
}

// soccerTeamHistoryNotCounted names the completed games the record leaves
// out, or nothing when every completed game is counted.
func soccerTeamHistoryNotCounted(record SoccerTeamHistoryRecord) string {
	if record.Unclassified == 0 {
		return ""
	}
	return soccerTeamHistoryCount(record.Unclassified, "completed game", "completed games") + " not counted."
}

// soccerTeamHistoryRecordNote is the record's label with the games it leaves
// out.
func soccerTeamHistoryRecordNote(record SoccerTeamHistoryRecord) string {
	note := record.Label + "."
	if notCounted := soccerTeamHistoryNotCounted(record); notCounted != "" {
		note += " " + notCounted
	}
	return note
}

func soccerTeamHistoryCount(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

func soccerTeamHistorySeasonMeta(division string, seasonID int, current bool) string {
	parts := make([]string, 0, 3)
	if division = strings.TrimSpace(division); division != "" {
		parts = append(parts, division)
	}
	parts = append(parts, "LPS season "+strconv.Itoa(seasonID))
	if current {
		parts = append(parts, "Current")
	} else {
		parts = append(parts, "Former")
	}
	return strings.Join(parts, " · ")
}

func soccerTeamHistoryPlayerURL(playerID int) string {
	return fmt.Sprintf("%s?player_id=%d", SoccerTeamHistoryViewPath, playerID)
}

func soccerTeamHistorySeasonURL(season SoccerTeamHistorySeason) string {
	return fmt.Sprintf("%s?player_id=%d&team_id=%d&season_id=%d", SoccerTeamHistoryViewPath, season.PlayerID, season.TeamID, season.SeasonID)
}

func soccerTeamHistoryPlayerID(player SoccerTeamHistoryPlayer) string {
	return fmt.Sprintf("soccer-team-history-player-%d", player.ID)
}

func soccerTeamHistorySeasonID(season SoccerTeamHistorySeason) string {
	return fmt.Sprintf("soccer-team-history-season-%d-%d", season.TeamID, season.SeasonID)
}

func soccerTeamHistoryBool(value bool) string {
	return strconv.FormatBool(value)
}

func soccerTeamHistorySeasonKey(teamID, seasonID int) string {
	return fmt.Sprintf("%d/%d", teamID, seasonID)
}

// soccerTeamHistoryResultClass paints a counted result like the schedule's
// past results and every other game as neutral.
func soccerTeamHistoryResultClass(result string) string {
	switch result {
	case "Win":
		return resultBadgeClass(schedule.OutcomeWin)
	case "Loss":
		return resultBadgeClass(schedule.OutcomeLoss)
	case "Draw":
		return resultBadgeClass(schedule.OutcomeDraw)
	default:
		return resultBadgeClass("")
	}
}

func soccerTeamHistoryNotice(notice *FeedbackProps) FeedbackProps {
	styled := *notice
	styled.ExtraClass = mergeClasses("soccer-team-history-notice", notice.ExtraClass)
	return styled
}

func soccerTeamHistoryRefusal(refusal *FeedbackProps) FeedbackProps {
	styled := *refusal
	styled.ExtraClass = mergeClasses("soccer-team-history-refusal", refusal.ExtraClass)
	return styled
}
