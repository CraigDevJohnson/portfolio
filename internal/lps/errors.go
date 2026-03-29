package lps

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorKind classifies expected LPS fetch failures for user feedback and HTTP responses.
type ErrorKind string

const (
	ErrorMalformedToken ErrorKind = "malformed_token"
	ErrorUnauthorized   ErrorKind = "unauthorized"
	ErrorForbidden      ErrorKind = "forbidden"
	ErrorInvalidPlayer  ErrorKind = "invalid_player"
	ErrorInvalidTeam    ErrorKind = "invalid_team"
	ErrorUpstream       ErrorKind = "upstream"
)

// FetchError wraps an LPS fetch failure with a stable classification and HTTP status.
type FetchError struct {
	Kind       ErrorKind
	ResourceID int
	StatusCode int
	Err        error
}

// Error returns the wrapped error message.
func (err *FetchError) Error() string {
	if err == nil {
		return ""
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return "schedule fetch failed"
}

// Unwrap exposes the wrapped cause.
func (err *FetchError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// NewFetchError constructs a classified LPS fetch error.
func NewFetchError(kind ErrorKind, resourceID, statusCode int, format string, args ...any) error {
	return &FetchError{
		Kind:       kind,
		ResourceID: resourceID,
		StatusCode: statusCode,
		Err:        fmt.Errorf(format, args...),
	}
}

// ScheduleErrorDetails describes the user-facing response for a classified schedule failure.
type ScheduleErrorDetails struct {
	ClearSession    bool
	DownloadMessage string
	DownloadStatus  int
	FeedbackHint    string
	FeedbackMessage string
}

// ScheduleFetchFeedback returns the fragment feedback text for an LPS schedule fetch failure.
func ScheduleFetchFeedback(err error) (string, string, bool) {
	if err == nil {
		return "", "", false
	}

	var fetchErr *FetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := ScheduleErrorDetail(fetchErr); ok {
			return detail.FeedbackMessage, detail.FeedbackHint, detail.ClearSession
		}
	}

	return "Could not load schedules from Let's Play Soccer right now.", "Try again in a moment, or use team IDs manually.", false
}

// ScheduleDownloadError returns the download HTTP response for an LPS schedule fetch failure.
func ScheduleDownloadError(err error) (int, string) {
	var fetchErr *FetchError
	if errors.As(err, &fetchErr) {
		if detail, ok := ScheduleErrorDetail(fetchErr); ok {
			return detail.DownloadStatus, detail.DownloadMessage
		}
	}

	return http.StatusBadGateway, "could not refresh the authenticated schedule"
}

// ScheduleErrorDetail maps a classified LPS error to user-facing feedback.
func ScheduleErrorDetail(fetchErr *FetchError) (ScheduleErrorDetails, bool) {
	if fetchErr == nil {
		return ScheduleErrorDetails{}, false
	}

	switch fetchErr.Kind {
	case ErrorMalformedToken:
		return ScheduleErrorDetails{
			ClearSession:    true,
			DownloadMessage: "the imported Let's Play Soccer token is malformed; import the full bearer JWT again",
			DownloadStatus:  http.StatusUnauthorized,
			FeedbackHint:    "Copy the full bearer JWT from letsplaysoccer.com and import it again.",
			FeedbackMessage: "The imported Let's Play Soccer token is not a valid JWT.",
		}, true
	case ErrorUnauthorized:
		return ScheduleErrorDetails{
			ClearSession:    true,
			DownloadMessage: "your imported Let's Play Soccer token was rejected; import a fresh bearer JWT from letsplaysoccer.com",
			DownloadStatus:  http.StatusUnauthorized,
			FeedbackHint:    "Copy a fresh bearer JWT from letsplaysoccer.com and import it again.",
			FeedbackMessage: "Your imported Let's Play Soccer token was rejected.",
		}, true
	case ErrorForbidden:
		return ScheduleErrorDetails{
			ClearSession:    false,
			DownloadMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d; clear the imported players and import again", fetchErr.ResourceID),
			DownloadStatus:  http.StatusForbidden,
			FeedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			FeedbackMessage: fmt.Sprintf("Let's Play Soccer denied access to discovered player %d.", fetchErr.ResourceID),
		}, true
	case ErrorInvalidPlayer:
		return ScheduleErrorDetails{
			ClearSession:    false,
			DownloadMessage: fmt.Sprintf("discovered player %d was not accepted by Let's Play Soccer; clear the imported players and import again", fetchErr.ResourceID),
			DownloadStatus:  http.StatusBadRequest,
			FeedbackHint:    "Clear the imported players and import again to refresh the discovered player list.",
			FeedbackMessage: fmt.Sprintf("Discovered player %d was not accepted by Let's Play Soccer.", fetchErr.ResourceID),
		}, true
	case ErrorInvalidTeam:
		return ScheduleErrorDetails{
			ClearSession:    false,
			DownloadMessage: fmt.Sprintf("team ID %d was not accepted by Let's Play Soccer; enter a valid numeric team ID and try again", fetchErr.ResourceID),
			DownloadStatus:  http.StatusBadRequest,
			FeedbackHint:    "Enter valid numeric team IDs from the Let's Play Soccer Team Schedules page and try again.",
			FeedbackMessage: fmt.Sprintf("Team ID %d was not accepted by Let's Play Soccer.", fetchErr.ResourceID),
		}, true
	case ErrorUpstream:
		return ScheduleErrorDetails{
			ClearSession:    false,
			DownloadMessage: "could not refresh the authenticated schedule because Let's Play Soccer is unavailable",
			DownloadStatus:  http.StatusBadGateway,
			FeedbackHint:    "Their API may be unavailable. Try again in a moment, or use team IDs manually.",
			FeedbackMessage: "Could not load schedules from Let's Play Soccer right now.",
		}, true
	default:
		return ScheduleErrorDetails{}, false
	}
}
