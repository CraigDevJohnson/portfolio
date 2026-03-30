package soccer

import "errors"

var (
	ErrSessionExpired        = errors.New("session expired")
	ErrPlayerSessionRequired = errors.New("an imported session is required for discovered players")
	ErrInvalidTeamSelection  = errors.New("one or more team IDs were invalid")
	ErrScheduleSelection     = errors.New("at least one team ID or discovered player is required")
)
