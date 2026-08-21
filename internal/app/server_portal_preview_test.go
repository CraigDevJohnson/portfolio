package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildMuxPortalPreviewUsesProductionURLsWithoutAuth(t *testing.T) {
	app := newTestApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux, _ := buildMux(app, logger, true)

	request := httptest.NewRequest(http.MethodGet, "/mgmt", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /mgmt status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Local preview") || !strings.Contains(body, "Portfolio web") {
		t.Fatalf("GET /mgmt did not render preview content: %s", body)
	}
}

func TestBuildMuxPortalPreviewRoutesAreHarmless(t *testing.T) {
	app := newTestApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux, _ := buildMux(app, logger, true)

	tests := []struct {
		method   string
		path     string
		status   int
		contains string
	}{
		{
			method:   http.MethodPost,
			path:     "/mgmt/instances/i-0f1e2d3c4b5a69788/restart",
			status:   http.StatusOK,
			contains: "no AWS restart action was sent",
		},
		{
			method:   http.MethodGet,
			path:     "/mgmt/instances/i-0f1e2d3c4b5a69788/metrics",
			status:   http.StatusOK,
			contains: "CPU %",
		},
		{
			method:   http.MethodGet,
			path:     "/mgmt/instances/i-0f1e2d3c4b5a69788/logs",
			status:   http.StatusOK,
			contains: "Health check passed",
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if body := response.Body.String(); !strings.Contains(body, test.contains) {
				t.Fatalf("response does not contain %q: %s", test.contains, body)
			}
		})
	}
}

func TestBuildMuxDoesNotRegisterPortalWhenDisabled(t *testing.T) {
	app := newTestApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux, _ := buildMux(app, logger, false)

	request := httptest.NewRequest(http.MethodGet, "/mgmt", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /mgmt status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestBuildMuxPortalPreviewAuthURLsReturnToDashboard(t *testing.T) {
	app := newTestApp(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux, _ := buildMux(app, logger, true)

	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("GET /login status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/mgmt" {
		t.Fatalf("Location = %q, want /mgmt", location)
	}
}

func TestBuildMuxPortalPreviewDoesNotAdvertiseUnavailableGoogleStore(t *testing.T) {
	app := newTestApp(t)
	app.Config.GoogleClientID = "configured-client"
	app.Config.GoogleClientSecret = "configured-secret"
	app.Config.GoogleConnectionTableName = "configured-table"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux, _ := buildMux(app, logger, true)

	request := httptest.NewRequest(http.MethodGet, "/soccer", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	body := response.Body.String()
	if strings.Contains(body, `href="/soccer/google/connect"`) {
		t.Fatal("portal preview advertised Google connect while its persistent connection store was unavailable")
	}
	if !strings.Contains(body, "Not enabled on this server") {
		t.Fatalf("portal preview did not explain the unavailable Google runtime: %s", body)
	}
}
