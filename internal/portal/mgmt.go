package portal

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
)

// DashboardHandler renders the authenticated EC2 instance dashboard.
func (h *Handler) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	username, _ := UsernameFromContext(r.Context())
	if h.EC2 == nil {
		h.renderDashboard(w, r, pages.DashboardProps{Username: username, RetrievalError: "EC2 management is unavailable."})
		return
	}
	output, err := h.EC2.DescribeInstances(r.Context(), &ec2.DescribeInstancesInput{})
	if err != nil {
		h.Logger.Error("portal instance retrieval failed", slog.String("region", h.Config.PortalAWSRegion), slog.String("aws_error_code", awsErrorCode(err)), slog.Any("error", err))
		h.renderDashboard(w, r, pages.DashboardProps{Username: username, RetrievalError: "Unable to retrieve instances right now."})
		return
	}
	instances := make([]InstanceSummary, 0)
	for i := range output.Reservations {
		for j := range output.Reservations[i].Instances {
			instances = append(instances, summarizeInstance(&output.Reservations[i].Instances[j]))
		}
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	h.renderDashboard(w, r, pages.DashboardProps{Username: username, Instances: instances})
}

func summarizeInstance(instance *ec2types.Instance) InstanceSummary {
	name := "—"
	for _, tag := range instance.Tags {
		if aws.ToString(tag.Key) == "Name" && aws.ToString(tag.Value) != "" {
			name = aws.ToString(tag.Value)
			break
		}
	}
	state := ""
	if instance.State != nil {
		state = string(instance.State.Name)
	}
	return InstanceSummary{
		ID:           aws.ToString(instance.InstanceId),
		Name:         name,
		State:        state,
		InstanceType: string(instance.InstanceType),
		AZ:           availabilityZone(instance),
	}
}

// InstanceActionHandler starts, stops, or restarts one validated instance.
func (h *Handler) InstanceActionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validInstanceID(id) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, "Invalid instance ID.")
		return
	}
	action := pathAction(r.URL.Path)
	if action != "start" && action != "stop" && action != "restart" {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, "Invalid instance action.")
		return
	}
	if h.EC2 == nil {
		h.renderActionResultStatus(w, r, http.StatusServiceUnavailable, "EC2 management is unavailable.")
		return
	}
	inputID := []string{id}
	var err error
	switch action {
	case "start":
		_, err = h.EC2.StartInstances(r.Context(), &ec2.StartInstancesInput{InstanceIds: inputID})
	case "stop":
		_, err = h.EC2.StopInstances(r.Context(), &ec2.StopInstancesInput{InstanceIds: inputID})
	case "restart":
		if _, err = h.EC2.StopInstances(r.Context(), &ec2.StopInstancesInput{InstanceIds: inputID}); err == nil {
			_, err = h.EC2.StartInstances(r.Context(), &ec2.StartInstancesInput{InstanceIds: inputID})
		}
	}
	username, _ := UsernameFromContext(r.Context())
	if err != nil {
		h.Logger.Error("portal instance action failed", slog.String("operator_username", username), slog.String("instance_id", id), slog.String("action", action), slog.String("outcome", "failure"), slog.Any("error", err))
		h.renderActionResultStatus(w, r, http.StatusInternalServerError, "The instance action failed.")
		return
	}
	h.Logger.Info("portal instance action completed", slog.String("operator_username", username), slog.String("instance_id", id), slog.String("action", action), slog.String("outcome", "success"))
	h.renderActionResult(w, r, true, fmt.Sprintf("Instance %s requested successfully.", action))
}

func pathAction(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 4 {
		return parts[3]
	}
	return ""
}

func validInstanceID(id string) bool { return instanceIDRegex.MatchString(id) }

func availabilityZone(instance *ec2types.Instance) string {
	if instance.Placement == nil {
		return ""
	}
	return aws.ToString(instance.Placement.AvailabilityZone)
}

func awsErrorCode(err error) string {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode()
	}
	return ""
}

func (h *Handler) renderActionResultStatus(w http.ResponseWriter, r *http.Request, status int, message string) {
	h.setHTMLContentType(w)
	w.Header().Set("X-Portal-Fragment-Error", "true")
	w.WriteHeader(status)
	if err := partials.ActionResult(partials.ActionResultProps{Success: false, Message: message}).Render(r.Context(), w); err != nil {
		h.Logger.Error("portal action result render failed", slog.Any("error", err))
	}
}
