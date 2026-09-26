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

// soccerMatchRow paints a row from the sides the LPS resolver identified. A
// shared match shows both selected teams' colors with a neutral home-away
// score. Any other row shows one team's color, and a result is read from that
// team's side, or neutrally when LPS listed the game for a selected team that
// matches neither side.
func soccerMatchRow(game *types.Game) soccerMatchRowView {
	result := strings.TrimSpace(game.Result)
	home, away := game.HomeTeam, game.AwayTeam
	view := soccerMatchRowView{HasResult: result != ""}

	var outcome schedule.GameOutcome
	neutral := false
	switch {
	case home.Selected && away.Selected:
		view.Shared = true
		view.HomeColor, view.AwayColor = soccerTeamColor(home), soccerTeamColor(away)
		neutral = true
	case home.Selected:
		view.HomeColor = soccerTeamColor(home)
		outcome = schedule.ParseGameResultForSide(result, true)
	case away.Selected:
		view.HomeColor = soccerTeamColor(away)
		outcome = schedule.ParseGameResultForSide(result, false)
	case game.ScheduleTeam.ID > 0 || game.ScheduleTeam.Color != "":
		view.HomeColor = soccerTeamColor(game.ScheduleTeam)
		neutral = true
	default:
		// Games without side identity keep the player team's perspective.
		view.HomeColor = soccerTeamColor(home)
		outcome = schedule.ParseGameResult(result, game.PlayerTeamName, strings.TrimSpace(game.Home))
	}
	if !view.Shared {
		view.AwayColor = view.HomeColor
	}

	if neutral {
		outcome = schedule.ParseGameResultForSide(result, true)
	}
	view.ResultText = schedule.FormatResultLine(outcome)
	view.ResultClass = resultBadgeClass(outcome.Outcome)
	if neutral && outcome.Parsed && (outcome.Outcome == schedule.OutcomeWin || outcome.Outcome == schedule.OutcomeLoss || outcome.Outcome == schedule.OutcomeDraw) {
		view.ResultText = fmt.Sprintf("Home %d – Away %d", outcome.HomeScore, outcome.AwayScore)
		view.ResultClass = resultBadgeClass("")
	}
	return view
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

// Google Calendar consent paths: the suggested path asks Google to suggest the
// site sign-in account, and the choose path opens Google's account chooser.
const (
	soccerGoogleSuggestedConnectPath = "/soccer/google/connect?account=suggested"
	soccerGoogleChooseConnectPath    = "/soccer/google/connect"
)

// GoogleOffersSiteAccount reports whether the connected Google account is not
// the site sign-in account, so switching to the site account is offered.
func (props *SoccerLoginStateProps) GoogleOffersSiteAccount() bool {
	return props.GoogleConnected && props.GoogleSuggestedEmail != "" && props.GoogleSuggestedEmail != props.GoogleAccountEmail
}

// SupportsGoogle reports whether any Google Calendar capability or state is present.
func (props *SoccerLoginStateProps) SupportsGoogle() bool {
	return props.GoogleConnected || props.GoogleAvailable || len(props.GoogleCalendars) > 0 || props.SelectedGoogleCalendarID != ""
}
