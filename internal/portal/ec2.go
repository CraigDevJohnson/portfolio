package portal

import (
	"context"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwlogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"portfolio/types"
)

// instanceIDRegex matches valid EC2 instance IDs (e.g. "i-0abc123def456").
// Compiled once at package init to avoid per-request overhead.
var instanceIDRegex = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)

// EC2ClientIface is the subset of the AWS EC2 client used by the portal.
type EC2ClientIface interface {
	DescribeInstances(ctx context.Context, input *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	StartInstances(ctx context.Context, input *ec2.StartInstancesInput, opts ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, input *ec2.StopInstancesInput, opts ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
}

// CloudWatchClientIface is the subset of the AWS CloudWatch client used by the portal.
type CloudWatchClientIface interface {
	GetMetricStatistics(ctx context.Context, input *cloudwatch.GetMetricStatisticsInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
}

// CloudWatchLogsClientIface is the subset of the AWS CloudWatch Logs client used by the portal.
type CloudWatchLogsClientIface interface {
	FilterLogEvents(ctx context.Context, input *cwlogs.FilterLogEventsInput, opts ...func(*cwlogs.Options)) (*cwlogs.FilterLogEventsOutput, error)
}

// InstanceSummary is the portal-level view model for an EC2 instance.
type InstanceSummary = types.InstanceSummary

// MetricPoint is the portal-level view model for a single CPU utilization data point.
type MetricPoint = types.MetricPoint

// LogEvent is the portal-level view model for a single CloudWatch log event.
type LogEvent = types.LogEvent
