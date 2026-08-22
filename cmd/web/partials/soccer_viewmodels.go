package partials

// SoccerSecurityNoticeProps controls the layout density of the shared security
// notice without changing its meaning.
type SoccerSecurityNoticeProps struct {
	Compact bool
}

// SupportsDiscovery reports whether the player-discovery workflow is present.
func (props *SoccerLoginStateProps) SupportsDiscovery() bool {
	return props.Authenticated || props.LoginAvailable
}

// SupportsGoogle reports whether any Google Calendar capability or state is present.
func (props *SoccerLoginStateProps) SupportsGoogle() bool {
	return props.GoogleConnected || props.GoogleAvailable || len(props.GoogleCalendars) > 0 || props.SelectedGoogleCalendarID != ""
}
