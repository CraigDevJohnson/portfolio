package portal

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"portfolio/internal/config"
)

type portalEC2Mock struct {
	describeOutput *ec2.DescribeInstancesOutput
	stopErr        error
	startCalls     int
	stopCalls      int
}

func (m *portalEC2Mock) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.describeOutput, nil
}

func (m *portalEC2Mock) StartInstances(context.Context, *ec2.StartInstancesInput, ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	m.startCalls++
	return &ec2.StartInstancesOutput{}, nil
}

func (m *portalEC2Mock) StopInstances(context.Context, *ec2.StopInstancesInput, ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	m.stopCalls++
	return &ec2.StopInstancesOutput{}, m.stopErr
}

func newPortalTestHandler(ec2Client EC2ClientIface) *Handler {
	return NewHandler(&config.Config{PortalSessionKey: make([]byte, 32), PortalCognitoDomain: "https://issuer.example", PortalCognitoClientID: "client", PortalAWSRegion: "us-east-1"}, nil, ec2Client, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestInstanceActionRejectsInvalidIDBeforeAWSCall(t *testing.T) {
	mock := &portalEC2Mock{}
	h := newPortalTestHandler(mock)
	r := httptest.NewRequest(http.MethodPost, "/mgmt/instances/not-an-id/restart", nil)
	r.SetPathValue("id", "not-an-id")
	rr := httptest.NewRecorder()
	h.InstanceActionHandler(rr, r)
	if rr.Code != http.StatusBadRequest || mock.stopCalls != 0 || mock.startCalls != 0 {
		t.Fatalf("status=%d stop=%d start=%d", rr.Code, mock.stopCalls, mock.startCalls)
	}
	if got := rr.Header().Get("X-Portal-Fragment-Error"); got != "true" {
		t.Fatalf("X-Portal-Fragment-Error = %q, want true", got)
	}
}

func TestRestartDoesNotStartAfterStopFailure(t *testing.T) {
	mock := &portalEC2Mock{stopErr: context.Canceled}
	h := newPortalTestHandler(mock)
	r := httptest.NewRequest(http.MethodPost, "/mgmt/instances/i-0123456789abcdef0/restart", nil)
	r.SetPathValue("id", "i-0123456789abcdef0")
	rr := httptest.NewRecorder()
	h.InstanceActionHandler(rr, r)
	if rr.Code != http.StatusInternalServerError || mock.stopCalls != 1 || mock.startCalls != 0 {
		t.Fatalf("status=%d stop=%d start=%d", rr.Code, mock.stopCalls, mock.startCalls)
	}
}

func TestDashboardSortsInstancesAndUsesFallbackName(t *testing.T) {
	mock := &portalEC2Mock{describeOutput: &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{
		{InstanceId: aws.String("i-fffffffffffffffff"), InstanceType: ec2types.InstanceTypeT3Micro, State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning}},
		{InstanceId: aws.String("i-00000000"), InstanceType: ec2types.InstanceTypeT3Micro, State: &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped}, Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("first")}}},
	}}}}}
	h := newPortalTestHandler(mock)
	rr := httptest.NewRecorder()
	h.DashboardHandler(rr, httptest.NewRequest(http.MethodGet, "/mgmt", nil))
	body := rr.Body.String()
	if !strings.Contains(body, "No") && !strings.Contains(body, "first") {
		t.Fatalf("unexpected dashboard response: %s", body)
	}
	if strings.Index(body, "i-00000000") > strings.Index(body, "i-fffffffffffffffff") {
		t.Fatal("instances were not sorted by ID")
	}
	if !strings.Contains(body, "—") {
		t.Fatal("missing Name tag did not use fallback")
	}
}
