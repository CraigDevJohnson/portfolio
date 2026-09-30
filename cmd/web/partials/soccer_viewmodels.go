package partials

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
