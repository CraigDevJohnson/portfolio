package partials

import (
	"fmt"
	"strings"

	"portfolio/internal/schedule"
	"portfolio/types"
)

var fallbackSoccerColors = [...]string{"blue", "red", "green", "purple", "orange", "teal", "pink", "gold"}

type soccerMatchRowView struct {
	HomeColor   string
	AwayColor   string
	Shared      bool
	HasResult   bool
	ResultText  string
	ResultClass string
}

func soccerMatchRow(game *types.Game) soccerMatchRowView {
	homeColor := soccerTeamColor(game.HomeTeam)
	awayColor := soccerTeamColor(game.AwayTeam)
	shared := game.HomeTeam.Selected && game.AwayTeam.Selected
	if !shared {
		switch {
		case game.HomeTeam.Selected:
			awayColor = homeColor
		case game.AwayTeam.Selected:
			homeColor = awayColor
		}
	}

	outcome := schedule.ParseGameResult(strings.TrimSpace(game.Result), game.PlayerTeamName, strings.TrimSpace(game.Home))
	resultText := schedule.FormatResultLine(outcome)
	resultClass := resultBadgeClass(outcome.Outcome)
	if shared && outcome.Parsed && (outcome.Outcome == schedule.OutcomeWin || outcome.Outcome == schedule.OutcomeLoss || outcome.Outcome == schedule.OutcomeDraw) {
		resultText = fmt.Sprintf("Home %d – Away %d", outcome.HomeScore, outcome.AwayScore)
		resultClass = resultBadgeClass("")
	}
	return soccerMatchRowView{
		HomeColor:   homeColor,
		AwayColor:   awayColor,
		Shared:      shared,
		HasResult:   strings.TrimSpace(game.Result) != "",
		ResultText:  resultText,
		ResultClass: resultClass,
	}
}

func soccerTeamColor(team types.TeamAppearance) string {
	switch team.Color {
	case "red", "blue", "navy", "green", "yellow", "gold", "orange", "purple", "pink", "teal", "maroon", "black", "white", "gray":
		return team.Color
	}
	if team.ID > 0 {
		return fallbackSoccerColors[team.ID%len(fallbackSoccerColors)]
	}
	return "gray"
}

// SoccerSecurityNoticeProps controls the layout density of the shared security
// notice without changing its meaning.
type SoccerSecurityNoticeProps struct {
	Compact bool
}

type soccerEmptyState struct {
	Heading          string
	Message          string
	Hint             string
	NextStep         string
	ShowImportAction bool
}

func (props *SoccerTableFragmentProps) emptyState() soccerEmptyState {
	state := soccerEmptyState{ShowImportAction: props.ImportAvailable}
	switch {
	case props.FetchError:
		state.Heading = "Could not fetch games"
	case props.Message != "":
		state.Heading = "Schedule needs attention"
	default:
		state.Heading = "No upcoming games found"
	}
	if props.Message != "" {
		state.Message = props.Message
	} else {
		state.Message = "There are no upcoming games for the selected teams."
	}
	if props.Hint != "" {
		state.Hint = props.Hint
	} else {
		state.Hint = "Check your team IDs and try again later."
	}
	switch {
	case props.Discovery:
		state.NextStep = "Import fresh LPS access, then choose your players and find their teams again."
	case len(props.PlayerIDs) > 0:
		state.NextStep = "Confirm that your selected players and teams are still current, then fetch again."
	case props.RetryLater:
		state.NextStep = "Your Team IDs are still entered above; fetch their schedules again in a moment."
	default:
		state.NextStep = "Check your Team IDs, then fetch their schedules again."
	}
	return state
}

// SupportsDiscovery reports whether the player-discovery workflow is present.
func (props *SoccerLoginStateProps) SupportsDiscovery() bool {
	return props.Authenticated || props.LoginAvailable
}

// SupportsGoogle reports whether any Google Calendar capability or state is present.
func (props *SoccerLoginStateProps) SupportsGoogle() bool {
	return props.GoogleConnected || props.GoogleAvailable || len(props.GoogleCalendars) > 0 || props.SelectedGoogleCalendarID != ""
}
