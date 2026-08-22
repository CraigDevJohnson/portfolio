package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandlerReturnsRevisionWithoutDependencyProbe(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	healthHandler("0123456789abcdef").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Body.String(); got != "{\"revision\":\"0123456789abcdef\",\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
}
