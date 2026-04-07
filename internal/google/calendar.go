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

type EventDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone,omitempty"`
}

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

type EventSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

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
