package google

import "net/http"

type calendarListResponse struct {
	Items         []calendar `json:"items"`
	NextPageToken string     `json:"nextPageToken"`
}

type eventListResponse struct {
	Items         []Event `json:"items"`
	NextPageToken string  `json:"nextPageToken"`
}

type calendar struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
	Summary string `json:"summary"`
}

// EventDateTime describes a Google Calendar datetime field.
type EventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

// EventReminder configures a single Google Calendar reminder override.
type EventReminder struct {
	Method  string `json:"method"`
	Minutes int    `json:"minutes"`
}

// EventReminders configures Google Calendar reminder behavior.
type EventReminders struct {
	Overrides  []EventReminder `json:"overrides,omitempty"`
	UseDefault bool            `json:"useDefault"`
}

// Event represents a Google Calendar event payload.
type Event struct {
	Description        string        `json:"description,omitempty"`
	End                EventDateTime `json:"end"`
	ETag               string        `json:"etag,omitempty"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties"`
	ID        string          `json:"id,omitempty"`
	Location  string          `json:"location,omitempty"`
	Reminders *EventReminders `json:"reminders,omitempty"`
	Source    *EventSource    `json:"source,omitempty"`
	Start     EventDateTime   `json:"start"`
	Status    string          `json:"status,omitempty"`
	Summary   string          `json:"summary"`
}

// EventSource attributes an event to its originating site.
type EventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// APIError wraps an HTTP status code and message from Google APIs.
type APIError struct {
	StatusCode int
	Message    string
	// Reason is the first reason in Google's error body, such as
	// requiredAccessLevel or rateLimitExceeded.
	Reason string
}

func (err *APIError) Error() string {
	if err == nil {
		return ""
	}
	if err.Message != "" {
		return err.Message
	}
	return "google api request failed"
}

// credentialsRejected reports whether Google refused the connection itself:
// its token is no longer valid, or it lacks the Calendar access consent
// asked for. Only a new consent can recover it.
func (err *APIError) credentialsRejected() bool {
	return err.StatusCode == http.StatusUnauthorized ||
		(err.StatusCode == http.StatusForbidden && (err.Reason == "insufficientPermissions" || err.Reason == "authError"))
}

// usageLimited reports whether Google refused the request only because a
// usage limit was reached, so a later retry can succeed.
func (err *APIError) usageLimited() bool {
	switch err.Reason {
	case "rateLimitExceeded", "userRateLimitExceeded", "dailyLimitExceeded", "quotaExceeded":
		return err.StatusCode == http.StatusForbidden || err.StatusCode == http.StatusTooManyRequests
	default:
		return false
	}
}

// destinationRejected reports whether the calendar itself refused the
// request: it is gone, or it no longer accepts this account's writes.
func (err *APIError) destinationRejected() bool {
	if err.credentialsRejected() || err.usageLimited() {
		return false
	}
	return err.StatusCode == http.StatusForbidden || err.StatusCode == http.StatusNotFound || err.StatusCode == http.StatusGone
}

// eventRefused reports whether Google refused a request about one existing
// event for a reason that names neither the connection, a usage limit, nor
// the calendar's access level, such as an update to an event another
// organizer owns. The calendar may still accept this account's writes.
func (err *APIError) eventRefused() bool {
	return err.StatusCode == http.StatusForbidden && !err.credentialsRejected() && !err.usageLimited() && err.Reason != "requiredAccessLevel"
}

type calendarEventAction int

const (
	calendarEventSkipped calendarEventAction = iota
	calendarEventInserted
	calendarEventUpdated
)
