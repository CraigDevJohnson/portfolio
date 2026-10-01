package pages

import (
	"fmt"
	"net/http"
	"strconv"
)

type ErrorPageProps struct {
	StatusCode int
	Message    string
}

func (props ErrorPageProps) PageTitle() string {
	return fmt.Sprintf("%d - Management portal", props.StatusCode)
}

func (props ErrorPageProps) StatusLabel() string {
	return "Status " + props.StatusText()
}

func (props ErrorPageProps) StatusText() string {
	return strconv.Itoa(props.StatusCode)
}

func (props ErrorPageProps) Heading() string {
	if props.StatusCode == http.StatusForbidden {
		return "Management access required"
	}
	return "Something interrupted the connection"
}

func (props ErrorPageProps) FeedbackTitle() string {
	if props.StatusCode == http.StatusForbidden {
		return "Access denied"
	}
	return "Connection status"
}
