package portal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestPortalPreviewFixtureParserIsClosed(t *testing.T) {
	tests := []struct {
		raw  string
		want PortalPreviewFixture
		ok   bool
	}{
		{"", PortalPreviewFixtureNormal, true},
		{"empty", PortalPreviewFixtureEmpty, true},
		{"retrieval-error", PortalPreviewFixtureRetrievalError, true},
		{"error", PortalPreviewFixtureError, true},
		{"normal", PortalPreviewFixtureNormal, false},
		{"production", PortalPreviewFixtureNormal, false},
		{"EMPTY", PortalPreviewFixtureNormal, false},
		{" empty ", PortalPreviewFixtureNormal, false},
	}

	for _, test := range tests {
		got, ok := parsePortalPreviewFixture(test.raw)
		if ok != test.ok || got != test.want {
			t.Errorf("parsePortalPreviewFixture(%q) = (%q, %t), want (%q, %t)", test.raw, got, ok, test.want, test.ok)
		}
	}
}

func TestPreviewInstancesCoverEveryLifecycleExactlyOnce(t *testing.T) {
	instances := previewInstances()
	if got := len(instances); got != 6 {
		t.Fatalf("previewInstances length = %d, want exactly 6", got)
	}

	wantStates := map[string]int{
		"pending":       1,
		"running":       1,
		"stopping":      1,
		"stopped":       1,
		"shutting-down": 1,
		"terminated":    1,
	}
	gotStates := make(map[string]int)
	seenIDs := make(map[string]bool)
	for _, instance := range instances {
		gotStates[instance.State]++
		if !validInstanceID(instance.ID) {
			t.Errorf("preview instance ID %q is invalid", instance.ID)
		}
		if seenIDs[instance.ID] {
			t.Errorf("preview instance ID %q is duplicated", instance.ID)
		}
		seenIDs[instance.ID] = true
	}
	for state, want := range wantStates {
		if got := gotStates[state]; got != want {
			t.Errorf("preview state %q count = %d, want %d", state, got, want)
		}
	}
	if len(gotStates) != len(wantStates) {
		t.Errorf("preview states = %v, want only the six lifecycle states", gotStates)
	}

	const anchorID = "i-0f1e2d3c4b5a69788"
	foundAnchor := false
	for _, instance := range instances {
		if instance.ID == anchorID {
			foundAnchor = true
			if instance.State != "running" || instance.Name != "Portfolio web" {
				t.Errorf("preview anchor = %#v, want the stable running Portfolio web instance", instance)
			}
		}
	}
	if !foundAnchor {
		t.Fatalf("preview instances do not include stable anchor %s", anchorID)
	}
}

func TestPreviewDashboardFixturesAreExplicit(t *testing.T) {
	handler := newPreviewTestHandler()
	tests := []struct {
		fixture  string
		status   int
		contains []string
		excludes []string
	}{
		{
			fixture:  "",
			status:   http.StatusOK,
			contains: []string{"Portfolio web", "Pending", "Running", "Stopping", "Stopped", "Shutting down", "Terminated"},
		},
		{
			fixture:  "empty",
			status:   http.StatusOK,
			contains: []string{"No instances found", `class="ui-feedback ui-feedback-info`},
			excludes: []string{"i-0f1e2d3c4b5a69788"},
		},
		{
			fixture:  "retrieval-error",
			status:   http.StatusOK,
			contains: []string{"Unable to load instances.", `role="alert"`, `class="ui-feedback ui-feedback-error`},
			excludes: []string{"i-0f1e2d3c4b5a69788"},
		},
		{fixture: "error", status: http.StatusNotFound},
		{fixture: "unknown", status: http.StatusNotFound},
	}

	for _, test := range tests {
		name := test.fixture
		if name == "" {
			name = "normal"
		}
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.DashboardHandler(response, httptest.NewRequest(http.MethodGet, "/mgmt?fixture="+test.fixture, nil))
			if response.Code != test.status {
				t.Fatalf("fixture %q status = %d, want %d: %s", test.fixture, response.Code, test.status, response.Body.String())
			}
			body := response.Body.String()
			for _, marker := range test.contains {
				if !strings.Contains(body, marker) {
					t.Errorf("fixture %q body does not contain %q: %s", test.fixture, marker, body)
				}
			}
			for _, marker := range test.excludes {
				if strings.Contains(body, marker) {
					t.Errorf("fixture %q body unexpectedly contains %q", test.fixture, marker)
				}
			}
		})
	}
}

func TestPreviewMetricsAndLogsFixturesAreTypedAndFailClosed(t *testing.T) {
	handler := newPreviewTestHandler()
	const instanceID = "i-0f1e2d3c4b5a69788"
	tests := []struct {
		name       string
		kind       string
		fixture    string
		handler    http.HandlerFunc
		status     int
		contains   string
		errorState bool
	}{
		{"metrics loaded", "metrics", "", handler.MetricsHandler, http.StatusOK, "CPU %", false},
		{"metrics empty", "metrics", "empty", handler.MetricsHandler, http.StatusOK, "No data available", false},
		{"metrics error", "metrics", "error", handler.MetricsHandler, 0, "Unable to load metrics.", true},
		{"metrics inapplicable", "metrics", "retrieval-error", handler.MetricsHandler, http.StatusNotFound, "", false},
		{"metrics unknown", "metrics", "surprise", handler.MetricsHandler, http.StatusNotFound, "", false},
		{"logs loaded", "logs", "", handler.LogsHandler, http.StatusOK, "Health check passed", false},
		{"logs empty", "logs", "empty", handler.LogsHandler, http.StatusOK, "No recent log events", false},
		{"logs error", "logs", "error", handler.LogsHandler, 0, "Unable to load logs.", true},
		{"logs inapplicable", "logs", "retrieval-error", handler.LogsHandler, http.StatusNotFound, "", false},
		{"logs unknown", "logs", "surprise", handler.LogsHandler, http.StatusNotFound, "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/mgmt/instances/" + instanceID + "/" + test.kind + "?fixture=" + test.fixture
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.SetPathValue("id", instanceID)
			response := httptest.NewRecorder()
			test.handler(response, request)
			if test.errorState {
				if response.Code < http.StatusBadRequest {
					t.Fatalf("error fixture status = %d, want non-2xx/3xx", response.Code)
				}
				if got := response.Header().Get("X-Portal-Fragment-Error"); got != "true" {
					t.Errorf("error fixture X-Portal-Fragment-Error = %q, want true", got)
				}
			} else if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
			body := response.Body.String()
			if test.contains != "" && !strings.Contains(body, test.contains) {
				t.Errorf("body does not contain %q: %s", test.contains, body)
			}
			if test.fixture == "empty" || test.fixture == "error" {
				if !strings.Contains(body, "ui-feedback") {
					t.Errorf("typed %s %s response does not use shared feedback anatomy: %s", test.kind, test.fixture, body)
				}
			}
			if strings.Contains(body, "data-signal-trail") || strings.Contains(strings.ToLower(body), "<!doctype") || strings.Contains(strings.ToLower(body), "<html") {
				t.Errorf("%s fixture %q unexpectedly rendered a full-page shell: %s", test.kind, test.fixture, body)
			}
		})
	}
}

func TestProductionDashboardIgnoresPreviewFixtureQuery(t *testing.T) {
	mock := &portalEC2Mock{describeOutput: &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
		{
			InstanceId:   aws.String("i-0fedcba9876543210"),
			InstanceType: ec2types.InstanceTypeT3Micro,
			State:        &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
			Tags:         []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("Production fixture sentinel")}},
		},
	}}}}}
	handler := newPortalTestHandler(mock)
	response := httptest.NewRecorder()
	handler.DashboardHandler(response, httptest.NewRequest(http.MethodGet, "/mgmt?fixture=empty", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Production fixture sentinel") {
		t.Fatalf("production dashboard honored preview fixture query: status=%d body=%s", response.Code, response.Body.String())
	}
}
