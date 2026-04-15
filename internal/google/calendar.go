package google

type calendarListResponse struct {
	Items []calendar `json:"items"`
}

type eventListResponse struct {
	Items []Event `json:"items"`
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

// Event represents a Google Calendar event payload.
type Event struct {
	Description        string        `json:"description,omitempty"`
	End                EventDateTime `json:"end"`
	ExtendedProperties struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties"`
	ID       string        `json:"id,omitempty"`
	Location string        `json:"location,omitempty"`
	Source   *EventSource  `json:"source,omitempty"`
	Start    EventDateTime `json:"start"`
	Status   string        `json:"status,omitempty"`
	Summary  string        `json:"summary"`
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

type calendarEventAction int

const (
	calendarEventSkipped calendarEventAction = iota
	calendarEventInserted
	calendarEventUpdated
)
