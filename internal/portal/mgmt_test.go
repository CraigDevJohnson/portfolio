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
	"golang.org/x/net/html"

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

func TestDashboardInstanceControlsRequireExactManagementTag(t *testing.T) {
	nameTag := ec2types.Tag{Key: aws.String("Name"), Value: aws.String("Inventory instance")}
	tests := []struct {
		name    string
		tags    []ec2types.Tag
		allowed bool
	}{
		{name: "untagged"},
		{name: "name only", tags: []ec2types.Tag{nameTag}},
		{name: "opt in after name", tags: []ec2types.Tag{nameTag, {Key: aws.String("PortfolioManagement"), Value: aws.String("dev")}}, allowed: true},
		{name: "opt in before name", tags: []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("dev")}, nameTag}, allowed: true},
		{name: "different environment", tags: []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("prod")}}},
		{name: "different key case", tags: []ec2types.Tag{{Key: aws.String("portfoliomanagement"), Value: aws.String("dev")}}},
		{name: "different value case", tags: []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("Dev")}}},
		{name: "key whitespace", tags: []ec2types.Tag{{Key: aws.String("PortfolioManagement "), Value: aws.String("dev")}}},
		{name: "value whitespace", tags: []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("dev ")}}},
		{name: "nil tag fields", tags: []ec2types.Tag{{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const instanceID = "i-0123456789abcdef0"
			mock := &portalEC2Mock{describeOutput: &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
				InstanceId: aws.String(instanceID),
				State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
				Tags:       test.tags,
			}}}}}}
			response := httptest.NewRecorder()
			newPortalTestHandler(mock).DashboardHandler(response, httptest.NewRequest(http.MethodGet, "/mgmt", nil))
			body := response.Body.String()
			if response.Code != http.StatusOK || !strings.Contains(body, instanceID) {
				t.Fatalf("inventory instance missing: status=%d body=%s", response.Code, body)
			}
			if got := strings.Contains(body, "Read only — instance actions are unavailable."); got == test.allowed {
				t.Errorf("read-only explanation present = %t, want %t", got, !test.allowed)
			}
			assertDashboardControls(t, body, map[string]bool{"start": false, "stop": test.allowed, "restart": test.allowed})
		})
	}
}

func assertDashboardControls(t *testing.T, body string, enabled map[string]bool) {
	t.Helper()
	document, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse dashboard: %v", err)
	}
	actions, details := 0, 0
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "button" {
			attributes := make(map[string]string, len(node.Attr))
			for _, attribute := range node.Attr {
				attributes[attribute.Key] = attribute.Val
			}
			_, disabled := attributes["disabled"]
			if action := pathAction(attributes["hx-post"]); action != "" {
				actions++
				if disabled != !enabled[action] || (attributes["aria-disabled"] == "true") != !enabled[action] {
					t.Errorf("action %q disabled=%t aria-disabled=%q, want disabled=%t", action, disabled, attributes["aria-disabled"], !enabled[action])
				}
			}
			if kind := pathAction(attributes["hx-get"]); kind == "metrics" || kind == "logs" {
				details++
				if disabled {
					t.Errorf("read-only %s control is disabled", kind)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	if actions != 3 || details != 2 {
		t.Errorf("rendered %d action controls and %d detail controls, want 3 and 2", actions, details)
	}
}
