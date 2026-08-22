package portal

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlogstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"

	"portfolio/cmd/web/partials"
)

// MetricsHandler returns CPU utilization for the last hour as an HTMX fragment.
func (h *Handler) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validInstanceID(id) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, "Invalid instance ID.")
		return
	}
	if h.CloudWatch == nil {
		h.renderActionResultStatus(w, r, http.StatusServiceUnavailable, "Metrics are unavailable.")
		return
	}
	end := time.Now().UTC()
	start := end.Add(-time.Hour)
	output, err := h.CloudWatch.GetMetricStatistics(r.Context(), &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cloudwatchtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String(id)}},
		StartTime:  aws.Time(start),
		EndTime:    aws.Time(end),
		Period:     aws.Int32(300),
		Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
	})
	if err != nil {
		h.Logger.Error("portal metrics retrieval failed", slog.String("instance_id", id), slog.Any("error", err))
		h.renderActionResultStatus(w, r, http.StatusInternalServerError, "Unable to load metrics.")
		return
	}
	points := make([]MetricPoint, 0, len(output.Datapoints))
	for _, point := range output.Datapoints {
		if point.Timestamp == nil || point.Average == nil {
			continue
		}
		points = append(points, MetricPoint{Timestamp: point.Timestamp.UTC(), CPUPercent: roundTwo(*point.Average)})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
	h.renderFragment(w, r, partials.MetricsTable(points))
}

// LogsHandler returns up to 100 recent log events as an HTMX fragment.
func (h *Handler) LogsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validInstanceID(id) {
		h.renderActionResultStatus(w, r, http.StatusBadRequest, "Invalid instance ID.")
		return
	}
	if h.Logs == nil {
		h.renderActionResultStatus(w, r, http.StatusServiceUnavailable, "Logs are unavailable.")
		return
	}
	end := time.Now()
	start := end.Add(-30 * time.Minute)
	group := "/ec2/" + id
	output, err := h.Logs.FilterLogEvents(r.Context(), &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(group),
		StartTime:    aws.Int64(start.UnixMilli()),
		EndTime:      aws.Int64(end.UnixMilli()),
		Limit:        aws.Int32(100),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceNotFoundException" {
			h.renderActionResult(w, r, false, "Log group not found.")
			return
		}
		h.Logger.Error("portal logs retrieval failed", slog.String("instance_id", id), slog.String("log_group", group), slog.Any("error", err))
		h.renderActionResultStatus(w, r, http.StatusInternalServerError, "Unable to load logs.")
		return
	}
	events := make([]LogEvent, 0, len(output.Events))
	for _, event := range output.Events {
		events = append(events, logEvent(event))
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })
	if len(events) > 100 {
		events = events[:100]
	}
	h.renderFragment(w, r, partials.LogsList(events))
}

func roundTwo(value float64) float64 { return float64(int64(value*100+0.5)) / 100 }

func logEvent(event cwlogstypes.FilteredLogEvent) LogEvent {
	when := time.UnixMilli(aws.ToInt64(event.Timestamp)).UTC()
	return LogEvent{Timestamp: when, Message: aws.ToString(event.Message)}
}
