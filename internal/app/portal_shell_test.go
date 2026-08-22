package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	webpages "portfolio/cmd/web/pages"
)

func TestPortalErrorUsesOperatorShell(t *testing.T) {
	var output bytes.Buffer
	if err := webpages.PortalError(webpages.ErrorPageProps{
		StatusCode: 503,
		Message:    "The management connection is unavailable.",
	}).Render(context.Background(), &output); err != nil {
		t.Fatalf("render PortalError: %v", err)
	}

	html := output.String()
	for _, marker := range []string{`data-shell="operator"`, `>Back to portfolio</a>`} {
		if !strings.Contains(html, marker) {
			t.Errorf("PortalError output does not contain %q", marker)
		}
	}
	if strings.Contains(html, `aria-label="Footer navigation"`) {
		t.Error("PortalError output unexpectedly contains public footer navigation")
	}
}
