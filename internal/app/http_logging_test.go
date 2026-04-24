package app

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithRequestLoggingLevel(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantLevel string
	}{
		{
			name:      "chrome devtools probe 404 logs as info",
			path:      "/.well-known/appspecific/com.chrome.devtools.json",
			wantLevel: `"level":"INFO"`,
		},
		{
			name:      "other 404 logs as warn",
			path:      "/missing",
			wantLevel: `"level":"WARN"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			handler := withRequestLogging(logger, http.NotFoundHandler())

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp := httptest.NewRecorder()

			handler.ServeHTTP(resp, req)

			if resp.Code != http.StatusNotFound {
				t.Fatalf("unexpected status code: got %d want %d", resp.Code, http.StatusNotFound)
			}
			if got := strings.TrimSpace(logs.String()); !strings.Contains(got, tt.wantLevel) {
				t.Fatalf("expected log output to contain %s, got %q", tt.wantLevel, got)
			}
		})
	}
}
