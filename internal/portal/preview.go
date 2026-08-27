package portal

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/types"
)

const previewUsername = "local.preview@portfolio.test"

// PreviewHandler renders a local-only portal using representative mock data.
// It intentionally has no Cognito, EC2, CloudWatch, or CloudWatch Logs clients.
type PreviewHandler struct {
	Logger *slog.Logger
	now    func() time.Time
}

// NewPreviewHandler constructs a handler for the loopback-only portal preview.
func NewPreviewHandler(logger *slog.Logger) *PreviewHandler {
	if logger == nil {
		logger = slog.Default().With(slog.String("component", "portal_preview"))
	}
	return &PreviewHandler{Logger: logger, now: time.Now}
}

// DashboardHandler renders the management portal with representative instance
// states so its layout can be reviewed without AWS credentials.
func (h *PreviewHandler) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	fixture, ok := parsePortalPreviewFixture(r.URL.Query().Get("fixture"))
	if !ok || fixture == PortalPreviewFixtureError {
		http.NotFound(w, r)
		return
	}

	props := pages.DashboardProps{
		Username: previewUsername,
		Preview:  true,
	}
	switch fixture {
	case PortalPreviewFixtureNormal:
		props.Instances = previewInstances()
	case PortalPreviewFixtureEmpty:
		props.Instances = nil
	case PortalPreviewFixtureRetrievalError:
		props.RetrievalError = "The local fixture could not retrieve the instance inventory."
	default:
		http.NotFound(w, r)
		return
	}

	h.renderComponent(w, r, pages.PortalDashboard(pages.DashboardProps{
		Username:       props.Username,
		Instances:      props.Instances,
		RetrievalError: props.RetrievalError,
		Preview:        props.Preview,
	}))
}

// ErrorPageHandler renders the full operator interruption state without any
// authentication or AWS dependency. Its route is registered only by the
// loopback-safe local preview mux branch.
func (h *PreviewHandler) ErrorPageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", htmlContentType)
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := pages.PortalError(pages.ErrorPageProps{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "The local preview is showing the management connection interruption state.",
	}).Render(r.Context(), w); err != nil {
		h.Logger.Error("portal preview error page render failed", slog.Any("error", err))
	}
}

// RedirectToDashboardHandler keeps the production auth URLs harmless and
// navigable while preview mode is active.
func (h *PreviewHandler) RedirectToDashboardHandler(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/mgmt", http.StatusSeeOther)
}

// InstanceActionHandler renders feedback without sending any AWS request.
func (h *PreviewHandler) InstanceActionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validInstanceID(id) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, false, "Invalid instance ID.")
		return
	}
	action := pathAction(r.URL.Path)
	if action != "start" && action != "stop" && action != "restart" {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, false, "Invalid instance action.")
		return
	}
	h.renderActionResultStatus(
		w,
		r,
		http.StatusOK,
		true,
		fmt.Sprintf("Preview only — no AWS %s action was sent for %s.", action, id),
	)
}

// MetricsHandler renders sample CPU utilization without querying CloudWatch.
func (h *PreviewHandler) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	if !validInstanceID(r.PathValue("id")) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, false, "Invalid instance ID.")
		return
	}
	fixture, ok := parsePortalPreviewFixture(r.URL.Query().Get("fixture"))
	if !ok || fixture == PortalPreviewFixtureRetrievalError {
		http.NotFound(w, r)
		return
	}
	switch fixture {
	case PortalPreviewFixtureNormal:
		h.renderComponent(w, r, partials.MetricsTable(previewMetricPoints(h.currentTime())))
	case PortalPreviewFixtureEmpty:
		h.renderComponent(w, r, partials.MetricsTable(nil))
	case PortalPreviewFixtureError:
		h.renderActionResultStatus(w, r, http.StatusInternalServerError, false, "Unable to load metrics.")
	default:
		http.NotFound(w, r)
	}
}

// LogsHandler renders representative events without querying CloudWatch Logs.
func (h *PreviewHandler) LogsHandler(w http.ResponseWriter, r *http.Request) {
	if !validInstanceID(r.PathValue("id")) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, false, "Invalid instance ID.")
		return
	}
	fixture, ok := parsePortalPreviewFixture(r.URL.Query().Get("fixture"))
	if !ok || fixture == PortalPreviewFixtureRetrievalError {
		http.NotFound(w, r)
		return
	}
	switch fixture {
	case PortalPreviewFixtureNormal:
		h.renderComponent(w, r, partials.LogsList(previewLogEvents(h.currentTime())))
	case PortalPreviewFixtureEmpty:
		h.renderComponent(w, r, partials.LogsList(nil))
	case PortalPreviewFixtureError:
		h.renderActionResultStatus(w, r, http.StatusInternalServerError, false, "Unable to load logs.")
	default:
		http.NotFound(w, r)
	}
}

func (h *PreviewHandler) currentTime() time.Time {
	if h.now == nil {
		return time.Now()
	}
	return h.now()
}

func (h *PreviewHandler) renderActionResultStatus(w http.ResponseWriter, r *http.Request, status int, success bool, message string) {
	w.Header().Set("Content-Type", htmlContentType)
	if status >= http.StatusBadRequest {
		w.Header().Set("X-Portal-Fragment-Error", "true")
	}
	w.WriteHeader(status)
	if err := partials.ActionResult(partials.ActionResultProps{Success: success, Message: message}).Render(r.Context(), w); err != nil {
		h.Logger.Error("portal preview action result render failed", slog.Any("error", err))
	}
}

func (h *PreviewHandler) renderComponent(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", htmlContentType)
	if err := component.Render(r.Context(), w); err != nil {
		h.Logger.Error("portal preview component render failed", slog.String("path", r.URL.Path), slog.Any("error", err))
	}
}

func previewInstances() []types.InstanceSummary {
	return []types.InstanceSummary{
		{
			ID:           "i-0f1e2d3c4b5a69788",
			Name:         "Portfolio web",
			State:        "running",
			InstanceType: "t3.small",
			AZ:           "us-east-1a",
		},
		{
			ID:           "i-0123abcd4567ef890",
			Name:         "Soccer sync worker",
			State:        "stopped",
			InstanceType: "t3.micro",
			AZ:           "us-east-1b",
		},
		{
			ID:           "i-0abc1234def567890",
			Name:         "Development sandbox",
			State:        "pending",
			InstanceType: "t3.medium",
			AZ:           "us-east-1c",
		},
		{
			ID:           "i-0d4e5f6a7b8c90123",
			Name:         "Deployment canary",
			State:        "stopping",
			InstanceType: "t3.micro",
			AZ:           "us-east-1a",
		},
		{
			ID:           "i-0aa11bb22cc33dd44",
			Name:         "Legacy report runner",
			State:        "shutting-down",
			InstanceType: "t3.small",
			AZ:           "us-east-1b",
		},
		{
			ID:           "i-0deadbeef00c0ffee",
			Name:         "Retired build host",
			State:        "terminated",
			InstanceType: "t3.medium",
			AZ:           "us-east-1c",
		},
	}
}

func previewMetricPoints(now time.Time) []types.MetricPoint {
	now = now.UTC().Truncate(time.Minute)
	return []types.MetricPoint{
		{Timestamp: now.Add(-45 * time.Minute), CPUPercent: 8.42},
		{Timestamp: now.Add(-30 * time.Minute), CPUPercent: 13.75},
		{Timestamp: now.Add(-15 * time.Minute), CPUPercent: 21.18},
		{Timestamp: now, CPUPercent: 11.06},
	}
}

func previewLogEvents(now time.Time) []types.LogEvent {
	now = now.UTC().Truncate(time.Second)
	return []types.LogEvent{
		{Timestamp: now.Add(-2 * time.Minute), Message: "Health check passed on /"},
		{Timestamp: now.Add(-8 * time.Minute), Message: "Deployment completed successfully"},
		{Timestamp: now.Add(-14 * time.Minute), Message: "Application process started"},
	}
}
