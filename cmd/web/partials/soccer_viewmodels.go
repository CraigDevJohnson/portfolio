package partials

import (
	"fmt"
	"slices"
	"strings"

	"portfolio/internal/schedule"
	"portfolio/types"
)

// SoccerHistoryNoticeField names the import form field that says which
// history notice the visitor saw. Only the import form that shows the
// indefinite-collection notice sends SoccerHistoryNoticeIndefinite, and only
// that import collects linked-player history.
const (
	SoccerHistoryNoticeField      = "history_notice"
	SoccerHistoryNoticeIndefinite = "indefinite"
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

// soccerTeamPalette holds one display color per selected team in a rendered
// schedule, so every row paints a team the same way. A team keeps the color
// LPS names for it. Colorless teams take Team ID fallbacks in Team ID order:
// each starts at the palette entry its ID selects and moves forward past
// entries that another selected team's LPS color or an earlier fallback
// already took, so a fallback never repeats another selected team's color
// while the palette has an unused entry. The assignment depends only on the
// selected team set, not on the order it was entered.
type soccerTeamPalette map[int]string

// newSoccerTeamPalette assigns colors to the selected teams the given games
// show: sides the resolver marked selected, and the team whose schedule
// listed each game.
func newSoccerTeamPalette(gameLists ...[]types.Game) soccerTeamPalette {
	palette := make(soccerTeamPalette)
	var colorless []int
	add := func(team types.TeamAppearance) {
		if team.ID <= 0 {
			return
		}
		if color := approvedSoccerTeamColor(team.Color); color != "" {
			palette[team.ID] = color
			return
		}
		if !slices.Contains(colorless, team.ID) {
			colorless = append(colorless, team.ID)
		}
	}
	for _, games := range gameLists {
		for i := range games {
			game := &games[i]
			if game.HomeTeam.Selected {
				add(game.HomeTeam)
			}
			if game.AwayTeam.Selected {
				add(game.AwayTeam)
			}
			add(game.ScheduleTeam)
		}
	}

	slices.Sort(colorless)
	taken := make(map[string]bool, len(palette)+len(colorless))
	for _, color := range palette {
		taken[color] = true
	}
	for _, teamID := range colorless {
		if palette[teamID] != "" {
			continue
		}
		color := teamIDFallbackColor(teamID, 0)
		for step := range len(fallbackSoccerColors) {
			if candidate := teamIDFallbackColor(teamID, step); !taken[candidate] {
				color = candidate
				break
			}
		}
		palette[teamID] = color
		taken[color] = true
	}
	return palette
}

// teamIDFallbackColor is the fallback step entries after the one a Team ID
// selects.
func teamIDFallbackColor(teamID, step int) string {
	return fallbackSoccerColors[(teamID+step)%len(fallbackSoccerColors)]
}

// soccerMatchRow paints a row from the sides the LPS resolver identified. A
// shared match shows both selected teams' colors with a neutral home-away
// score. Any other row shows one team's color, and a result is read from that
// team's side, or neutrally when LPS listed the game for a selected team that
// matches neither side.
func soccerMatchRow(game *types.Game, palette soccerTeamPalette) soccerMatchRowView {
	result := strings.TrimSpace(game.Result)
	home, away := game.HomeTeam, game.AwayTeam
	view := soccerMatchRowView{HasResult: result != ""}

	var outcome schedule.GameOutcome
	neutral := false
	switch {
	case home.Selected && away.Selected:
		view.Shared = true
		view.HomeColor, view.AwayColor = palette.color(home), palette.color(away)
		neutral = true
	case home.Selected:
		view.HomeColor = palette.color(home)
		outcome = schedule.ParseGameResultForSide(result, true)
	case away.Selected:
		view.HomeColor = palette.color(away)
		outcome = schedule.ParseGameResultForSide(result, false)
	case game.ScheduleTeam.ID > 0 || game.ScheduleTeam.Color != "":
		view.HomeColor = palette.color(game.ScheduleTeam)
		neutral = true
	default:
		// Games without side identity keep the player team's perspective.
		view.HomeColor = palette.color(home)
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

// color paints a team in its own approved color, then in the color the
// palette assigned its Team ID, then in the fallback its Team ID selects; a
// team without an ID is gray.
func (palette soccerTeamPalette) color(team types.TeamAppearance) string {
	if color := approvedSoccerTeamColor(team.Color); color != "" {
		return color
	}
	if color := palette[team.ID]; color != "" {
		return color
	}
	if team.ID > 0 {
		return teamIDFallbackColor(team.ID, 0)
	}
	return "gray"
}

// approvedSoccerTeamColor returns color when the stylesheet paints it.
func approvedSoccerTeamColor(color string) string {
	switch color {
	case "red", "blue", "navy", "green", "yellow", "gold", "orange", "purple", "pink", "teal", "maroon", "black", "white", "gray":
		return color
	}
	return ""
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

// Google Calendar consent paths: connecting asks Google to suggest the site
// sign-in account, and the choose path opens Google's account chooser.
const (
	soccerGoogleConnectPath       = "/soccer/google/connect"
	soccerGoogleChooseAccountPath = "/soccer/google/connect?account=choose"
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
