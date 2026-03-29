package soccer

import "errors"

var (
	// ErrSessionExpired indicates an encrypted soccer session or state value has expired.
	ErrSessionExpired = errors.New("session expired")
	// ErrPlayerSessionRequired indicates discovered-player requests require an imported JWT session.
	ErrPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	// ErrInvalidTeamSelection indicates one or more manual team IDs were invalid.
	ErrInvalidTeamSelection = errors.New("one or more team IDs were invalid")
	// ErrScheduleSelection indicates neither valid team IDs nor discovered players were selected.
	ErrScheduleSelection = errors.New("at least one team ID or discovered player is required")
)
