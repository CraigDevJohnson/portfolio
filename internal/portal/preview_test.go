package portal

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newPreviewTestHandler() *PreviewHandler {
	handler := NewPreviewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler.now = func() time.Time {
		return time.Date(2026, time.August, 10, 18, 30, 0, 0, time.UTC)
	}
	return handler
}

func TestPreviewDashboardUsesMockData(t *testing.T) {
	handler := newPreviewTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/mgmt", nil)
	response := httptest.NewRecorder()

	handler.DashboardHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, expected := range []string{
		"Local preview",
		previewUsername,
		"Portfolio web",
		"Soccer sync worker",
		"Development sandbox",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("dashboard does not contain %q", expected)
		}
	}
}

func TestPreviewInstanceActionIsHarmless(t *testing.T) {
	handler := newPreviewTestHandler()
	request := httptest.NewRequest(http.MethodPost, "/mgmt/instances/i-0f1e2d3c4b5a69788/restart", nil)
	request.SetPathValue("id", "i-0f1e2d3c4b5a69788")
	response := httptest.NewRecorder()

	handler.InstanceActionHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); !strings.Contains(body, "no AWS restart action was sent") {
		t.Fatalf("unexpected action response: %s", body)
	}
}

func TestPreviewFragmentsUseMockData(t *testing.T) {
	handler := newPreviewTestHandler()

	tests := []struct {
		name     string
		path     string
		handler  http.HandlerFunc
		expected string
	}{
		{
			name:     "metrics",
			path:     "/mgmt/instances/i-0f1e2d3c4b5a69788/metrics",
			handler:  handler.MetricsHandler,
			expected: "21.18",
		},
		{
			name:     "logs",
			path:     "/mgmt/instances/i-0f1e2d3c4b5a69788/logs",
			handler:  handler.LogsHandler,
			expected: "Deployment completed successfully",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.SetPathValue("id", "i-0f1e2d3c4b5a69788")
			response := httptest.NewRecorder()

			test.handler(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if body := response.Body.String(); !strings.Contains(body, test.expected) {
				t.Fatalf("response does not contain %q: %s", test.expected, body)
			}
		})
	}
}

func TestPreviewHandlersRejectInvalidInstanceID(t *testing.T) {
	handler := newPreviewTestHandler()
	handlers := []http.HandlerFunc{
		handler.InstanceActionHandler,
		handler.MetricsHandler,
		handler.LogsHandler,
	}

	for _, target := range handlers {
		request := httptest.NewRequest(http.MethodGet, "/mgmt/instances/not-an-instance/metrics", nil)
		request.SetPathValue("id", "not-an-instance")
		response := httptest.NewRecorder()

		target(response, request)

		if response.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", response.Code, http.StatusBadRequest)
		}
		if got := response.Header().Get("X-Portal-Fragment-Error"); got != "true" {
			t.Errorf("X-Portal-Fragment-Error = %q, want true", got)
		}
	}
}

func TestPreviewAuthURLsRedirectToDashboard(t *testing.T) {
	handler := newPreviewTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	handler.RedirectToDashboardHandler(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/mgmt" {
		t.Fatalf("Location = %q, want /mgmt", location)
	}
}
