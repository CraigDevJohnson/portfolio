// Package portal provides the EC2 management portal handlers, OIDC authentication,
// and AWS client interfaces for the /login, /auth/callback, and /mgmt route families.
package portal

import (
	"log/slog"
	"net/http"

	"portfolio/internal/config"
)

const htmlContentType = "text/html; charset=utf-8"

// Handler owns all portal route handlers and their runtime dependencies.
type Handler struct {
	Config     *config.Config
	OIDC       *OIDCClient
	EC2        EC2ClientIface
	CloudWatch CloudWatchClientIface
	Logs       CloudWatchLogsClientIface
	Logger     *slog.Logger
}

// NewHandler constructs a portal Handler with its runtime dependencies.
func NewHandler(
	cfg *config.Config,
	oidc *OIDCClient,
	ec2 EC2ClientIface,
	cw CloudWatchClientIface,
	logs CloudWatchLogsClientIface,
	logger *slog.Logger,
) *Handler {
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "portal"))
	}
	return &Handler{
		Config:     cfg,
		OIDC:       oidc,
		EC2:        ec2,
		CloudWatch: cw,
		Logs:       logs,
		Logger:     logger,
	}
}

// setHTMLContentType sets the Content-Type header to text/html; charset=utf-8.
func (h *Handler) setHTMLContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", htmlContentType)
}
