package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"portfolio/cmd/web/pages"
	"portfolio/types"
)

func TestPortalFullPagesUseOperatorWorkspaceAndOneTypedTrail(t *testing.T) {
	dashboard := renderPortalTestPage(t, pages.PortalDashboard(pages.DashboardProps{
		Username: "operator@example.test",
		Preview:  true,
		Instances: []types.InstanceSummary{{
			ID:           "i-0f1e2d3c4b5a69788",
			Name:         "Portfolio web",
			State:        "running",
			InstanceType: "t3.small",
			AZ:           "us-east-1a",
		}},
	}))
	for _, marker := range []string{
		`data-shell="operator"`,
		`data-layout="operator-workspace"`,
		`class="signal-trail signal-trail-operator`,
		`>Back to portfolio</a>`,
		`All instance fields and controls are available in this table; scroll horizontally when needed.`,
	} {
		if !strings.Contains(dashboard, marker) {
			t.Errorf("PortalDashboard does not contain %q", marker)
		}
	}
	if strings.Contains(dashboard, `>Scroll horizontally to review every instance field and control.</p>`) {
		t.Error("PortalDashboard uses unconditional scroll guidance when the table may already fit")
	}
	if got := strings.Count(dashboard, "<h1"); got != 1 {
		t.Errorf("PortalDashboard h1 count = %d, want 1", got)
	}
	if got := strings.Count(dashboard, "data-signal-trail"); got != 1 {
		t.Errorf("PortalDashboard trail count = %d, want 1", got)
	}

	errorPage := renderPortalTestPage(t, pages.PortalError(pages.ErrorPageProps{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "The management connection is unavailable.",
	}))
	for _, marker := range []string{
		`data-shell="operator"`,
		`data-layout="operator-interruption"`,
		`class="signal-trail signal-trail-interruption`,
		`>Back to portfolio</a>`,
	} {
		if !strings.Contains(errorPage, marker) {
			t.Errorf("PortalError does not contain %q", marker)
		}
	}
	if got := strings.Count(errorPage, "<h1"); got != 1 {
		t.Errorf("PortalError h1 count = %d, want 1", got)
	}
	if got := strings.Count(errorPage, "data-signal-trail"); got != 1 {
		t.Errorf("PortalError trail count = %d, want 1", got)
	}
	if strings.Contains(dashboard, `aria-label="Footer navigation"`) || strings.Contains(errorPage, `aria-label="Footer navigation"`) {
		t.Fatal("Portal operator pages unexpectedly render the public footer navigation")
	}
}

func TestPortalPreviewRoutesExposeOnlyApprovedLocalFixtures(t *testing.T) {
	application := newTestApp(t)
	safeMux, _ := buildMux(application, application.Logger, true)
	normalMux, _ := buildMux(application, application.Logger, false)
	const anchor = "i-0f1e2d3c4b5a69788"

	tests := []struct {
		name     string
		method   string
		path     string
		status   int
		contains string
	}{
		{"dashboard normal", http.MethodGet, "/mgmt", http.StatusOK, "Portfolio web"},
		{"dashboard empty", http.MethodGet, "/mgmt?fixture=empty", http.StatusOK, "No instances found"},
		{"dashboard retrieval error", http.MethodGet, "/mgmt?fixture=retrieval-error", http.StatusOK, "Unable to load instances."},
		{"dashboard unknown fixture", http.MethodGet, "/mgmt?fixture=surprise", http.StatusNotFound, ""},
		{"dashboard inapplicable fixture", http.MethodGet, "/mgmt?fixture=error", http.StatusNotFound, ""},
		{"portal error page", http.MethodGet, "/__preview/portal/error", http.StatusServiceUnavailable, "Something interrupted the connection"},
		{"metrics loaded", http.MethodGet, "/mgmt/instances/" + anchor + "/metrics", http.StatusOK, "CPU %"},
		{"metrics empty", http.MethodGet, "/mgmt/instances/" + anchor + "/metrics?fixture=empty", http.StatusOK, "No data available"},
		{"metrics error", http.MethodGet, "/mgmt/instances/" + anchor + "/metrics?fixture=error", http.StatusInternalServerError, "Unable to load metrics."},
		{"metrics inapplicable fixture", http.MethodGet, "/mgmt/instances/" + anchor + "/metrics?fixture=retrieval-error", http.StatusNotFound, ""},
		{"logs loaded", http.MethodGet, "/mgmt/instances/" + anchor + "/logs", http.StatusOK, "Health check passed"},
		{"logs empty", http.MethodGet, "/mgmt/instances/" + anchor + "/logs?fixture=empty", http.StatusOK, "No recent log events"},
		{"logs error", http.MethodGet, "/mgmt/instances/" + anchor + "/logs?fixture=error", http.StatusInternalServerError, "Unable to load logs."},
		{"logs inapplicable fixture", http.MethodGet, "/mgmt/instances/" + anchor + "/logs?fixture=retrieval-error", http.StatusNotFound, ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			safeMux.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("%s status = %d, want %d: %s", test.path, response.Code, test.status, response.Body.String())
			}
			if test.contains != "" && !strings.Contains(response.Body.String(), test.contains) {
				t.Errorf("%s body does not contain %q: %s", test.path, test.contains, response.Body.String())
			}
			if strings.Contains(test.name, " error") && strings.Contains(test.path, "/instances/") {
				if got := response.Header().Get("X-Portal-Fragment-Error"); got != "true" {
					t.Errorf("%s X-Portal-Fragment-Error = %q, want true", test.path, got)
				}
			}
		})
	}

	for _, path := range []string{
		"/__preview/portal/error",
		"/__preview/soccer/manual",
	} {
		response := httptest.NewRecorder()
		normalMux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("non-preview GET %s status = %d, want 404", path, response.Code)
		}
	}
}

func renderPortalTestPage(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render Portal page: %v", err)
	}
	return output.String()
}
