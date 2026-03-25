// LPS error classification and user-facing schedule error messages.
package main

import (
	"errors"
	"fmt"
	"net/http"
)

type lpsErrorKind string

const (
	lpsErrorMalformedToken lpsErrorKind = "malformed_token"
	lpsErrorUnauthorized   lpsErrorKind = "unauthorized"
	lpsErrorForbidden      lpsErrorKind = "forbidden"
	lpsErrorInvalidPlayer  lpsErrorKind = "invalid_player"
	lpsErrorInvalidTeam    lpsErrorKind = "invalid_team"
	lpsErrorUpstream       lpsErrorKind = "upstream"
)

type lpsFetchError struct {
	Kind       lpsErrorKind
	PlayerID   int
	StatusCode int
	Err        error
}

func (err *lpsFetchError) Error() string {
	if err == nil {
		return ""
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return "schedule fetch failed"
}

func (err *lpsFetchError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func newLPSFetchError(kind lpsErrorKind, playerID, statusCode int, format string, args ...any) error {
	return &lpsFetchError{
		Kind:       kind,
		PlayerID:   playerID,
		StatusCode: statusCode,
		Err:        fmt.Errorf(format, args...),
	}
}

type scheduleErrorDetails struct {
	clearSession    bool
	downloadMessage string
	downloadStatus  int
	feedbackHint    string
	feedbackMessage string
}

func scheduleFetchFeedback(err error) (string, string, bool) {
	if err == nil {
		return "", "", false
	}
	var fetchErr *lpsFetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := scheduleErrorDetail(fetchErr); ok {
			return detail.feedbackMessage, detail.feedbackHint, detail.clearSession
		}
	}
	if errors.Is(err, errSessionExpired) {
		return "Your imported Let's Play Soccer token expired.", "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.", true
	}
	return "Could not load schedules from Let's Play Soccer right now.", "Try again in a moment, or use team IDs manually.", false
}

func scheduleDownloadError(err error) (int, string) {
	var fetchErr *lpsFetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := scheduleErrorDetail(fetchErr); ok {
			return detail.downloadStatus, detail.downloadMessage
		}
	}
	return http.StatusBadGateway, "could not refresh the authenticated schedule"
}

func scheduleErrorDetail(fetchErr *lpsFetchError) (scheduleErrorDetails, bool) {
	if fetchErr == nil {
		return scheduleErrorDetails{}, false
	}

	switch fetchErr.Kind {
	case lpsErrorMalformedToken:
		return scheduleErrorDetails{
			clearSession:    true,
			downloadMessage: "the imported Let's Play Soccer token is malformed; import the full bearer JWT again",
			downloadStatus:  http.StatusUnauthorized,
			feedbackHint:    "Copy the full bearer JWT from letsplaysoccer.com and import it again.",
			feedbackMessage: "The imported Let's Play Soccer token is not a valid JWT.",
		}, true
	case lpsErrorUnauthorized:
		return scheduleErrorDetails{
			clearSession:    true,
			downloadMessage: "your imported Let's Play Soccer token was rejected; import a fresh bearer JWT from letsplaysoccer.com",
			downloadStatus:  http.StatusUnauthorized,
			feedbackHint:    "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.",
			feedbackMessage: "Your imported Let's Play Soccer token was rejected.",
		}, true
	case lpsErrorForbidden:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d; clear the imported players and import again", fetchErr.PlayerID),
			downloadStatus:  http.StatusForbidden,
			feedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			feedbackMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d.", fetchErr.PlayerID),
		}, true
	case lpsErrorInvalidPlayer:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("discovered player %d was not accepted by Let's Play Soccer; clear the imported players and import again", fetchErr.PlayerID),
			downloadStatus:  http.StatusBadRequest,
			feedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			feedbackMessage: fmt.Sprintf("Discovered player %d was not accepted by Let's Play Soccer.", fetchErr.PlayerID),
		}, true
	case lpsErrorInvalidTeam:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: fmt.Sprintf("team ID %d was not accepted by Let's Play Soccer; enter a valid numeric team ID and try again", fetchErr.PlayerID),
			downloadStatus:  http.StatusBadRequest,
			feedbackHint:    "Enter valid numeric team IDs from the Let's Play Soccer Team Schedules page and try again.",
			feedbackMessage: fmt.Sprintf("Team ID %d was not accepted by Let's Play Soccer.", fetchErr.PlayerID),
		}, true
	case lpsErrorUpstream:
		return scheduleErrorDetails{
			clearSession:    false,
			downloadMessage: "could not refresh the authenticated schedule because Let's Play Soccer is unavailable",
			downloadStatus:  http.StatusBadGateway,
			feedbackHint:    "Their API may be unavailable. Try again in a moment, or use team IDs manually.",
			feedbackMessage: "Could not load schedules from Let's Play Soccer right now.",
		}, true
	default:
		return scheduleErrorDetails{}, false
	}
}
