package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"

	"portfolio/internal/portal"
	"portfolio/internal/session"
)

const (
	taggedInstanceID   = "i-0123456789abcdef0"
	untaggedInstanceID = "i-11111111111111111"
	// formerPortalCookie is the retired management-only session cookie name.
	formerPortalCookie = "mgmt_session"
)

// fakeManagementAWS stands in for EC2, CloudWatch and CloudWatch Logs. By
// default it enforces the portal runtime role's current permission limits:
// read-only instance inventory and CPU metrics, with no EC2 start/stop and no
// /ec2/i-* log reads (D22 in infra/lambda/modules/service/iam.tf). An
// unrestricted role, such as an operator's own local credentials, also permits
// the commands and log reads.
type fakeManagementAWS struct {
	unrestricted    bool
	describes       int
	metricInstances []string
	starts          []string
	stops           []string
	logGroups       []string
}

func (f *fakeManagementAWS) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	f.describes++
	return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
		InstanceId: aws.String(taggedInstanceID),
		State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		Tags:       []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("dev")}},
	}, {
		InstanceId: aws.String(untaggedInstanceID),
		State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
	}}}}}, nil
}

func (f *fakeManagementAWS) StartInstances(_ context.Context, input *ec2.StartInstancesInput, _ ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	f.starts = append(f.starts, input.InstanceIds...)
	if !f.unrestricted {
		return nil, &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "not authorized to perform ec2:StartInstances"}
	}
	return &ec2.StartInstancesOutput{}, nil
}

func (f *fakeManagementAWS) StopInstances(_ context.Context, input *ec2.StopInstancesInput, _ ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	f.stops = append(f.stops, input.InstanceIds...)
	if !f.unrestricted {
		return nil, &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "not authorized to perform ec2:StopInstances"}
	}
	return &ec2.StopInstancesOutput{}, nil
}

func (f *fakeManagementAWS) GetMetricStatistics(_ context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	for _, dimension := range input.Dimensions {
		f.metricInstances = append(f.metricInstances, aws.ToString(dimension.Value))
	}
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

func (f *fakeManagementAWS) FilterLogEvents(_ context.Context, input *cloudwatchlogs.FilterLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	f.logGroups = append(f.logGroups, aws.ToString(input.LogGroupName))
	if !f.unrestricted {
		return nil, &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "not authorized to perform logs:FilterLogEvents"}
	}
	return &cloudwatchlogs.FilterLogEventsOutput{}, nil
}

func (f *fakeManagementAWS) calls() int {
	return f.describes + len(f.metricInstances) + len(f.starts) + len(f.stops) + len(f.logGroups)
}

// managementPortal serves the real route assembly with fake Cognito and AWS.
type managementPortal struct {
	cognito *fakeSiteCognito
	app     *App
	mux     http.Handler
	aws     *fakeManagementAWS
}

func newManagementPortal(t *testing.T, cognito *fakeSiteCognito, invitations map[string][]string) *managementPortal {
	t.Helper()
	application := cognito.app(t)
	application.Config.SiteInvitations = invitations
	fakeAWS := &fakeManagementAWS{}
	application.PortalHandler = portal.NewHandler(&application.Config, fakeAWS, fakeAWS, fakeAWS, application.Logger)
	mux, _ := buildMux(application, application.Logger, false)
	return &managementPortal{cognito: cognito, app: application, mux: mux, aws: fakeAWS}
}

func (p *managementPortal) signIn(t *testing.T, email, returnTo string) *http.Cookie {
	t.Helper()
	p.cognito.email = email
	stateCookie, state := beginSiteSignIn(t, p.mux, returnTo)
	callback := completeSiteSignIn(t, p.mux, stateCookie, state)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != returnTo {
		t.Fatalf("site sign-in for %s did not return to %s: %d %q", email, returnTo, callback.Code, callback.Header().Get("Location"))
	}
	return siteCookie(t, callback)
}

func (p *managementPortal) request(method, path string, htmx bool, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://app.example.com"+path, nil)
	if htmx {
		request.Header.Set("HX-Request", "true")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	p.mux.ServeHTTP(response, request)
	return response
}

var managementRoutes = []struct{ method, path string }{
	{http.MethodGet, "/mgmt"},
	{http.MethodPost, "/mgmt/instances/" + taggedInstanceID + "/start"},
	{http.MethodPost, "/mgmt/instances/" + taggedInstanceID + "/stop"},
	{http.MethodPost, "/mgmt/instances/" + taggedInstanceID + "/restart"},
	{http.MethodGet, "/mgmt/instances/" + taggedInstanceID + "/metrics"},
	{http.MethodGet, "/mgmt/instances/" + taggedInstanceID + "/logs"},
}

// formerManagementSession is a cookie the retired management-only portal
// issued: an encrypted username and expiry under its own session key.
func formerManagementSession(t *testing.T, key []byte, username string) *http.Cookie {
	t.Helper()
	value, err := session.EncryptJSONValue(key, map[string]any{"username": username, "expires_at": time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: formerPortalCookie, Value: value}
}

func TestManagementPortalDeniesVisitorsWithoutCurrentGrant(t *testing.T) {
	portalApp := newManagementPortal(t, newFakeSiteCognito(t), map[string][]string{
		"owner@example.com":  {"soccer", "management"},
		"friend@example.com": {"soccer"},
	})
	formerSession := formerManagementSession(t, []byte("0123456789abcdef0123456789abcdef"), "owner@example.com")
	ungranted := portalApp.signIn(t, "friend@example.com", "/about")

	for _, route := range managementRoutes {
		t.Run("signed out "+route.method+" "+route.path, func(t *testing.T) {
			response := portalApp.request(route.method, route.path, false)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in?return_to=%2Fmgmt" {
				t.Fatalf("signed-out request was not sent to site sign-in: %d %q", response.Code, response.Header().Get("Location"))
			}
			htmx := portalApp.request(route.method, route.path, true)
			if htmx.Code != http.StatusNoContent || htmx.Header().Get("HX-Redirect") != "/sign-in?return_to=%2Fmgmt" || htmx.Body.Len() != 0 {
				t.Fatalf("signed-out HTMX request did not navigate to site sign-in: %d %q", htmx.Code, htmx.Header().Get("HX-Redirect"))
			}
		})
		t.Run("former management session "+route.method+" "+route.path, func(t *testing.T) {
			response := portalApp.request(route.method, route.path, false, formerSession)
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in?return_to=%2Fmgmt" {
				t.Fatalf("former management session was treated as signed in: %d %q", response.Code, response.Header().Get("Location"))
			}
		})
		t.Run("signed in without grant "+route.method+" "+route.path, func(t *testing.T) {
			response := portalApp.request(route.method, route.path, false, ungranted, formerSession)
			body := response.Body.String()
			if response.Code != http.StatusForbidden || !strings.Contains(body, "Management access required") || !strings.Contains(body, "Your account does not have management access.") {
				t.Fatalf("missing management grant was not denied clearly: %d", response.Code)
			}
			if !strings.Contains(body, `<span class="site-account-email" title="friend@example.com">friend@example.com</span>`) {
				t.Fatal("denial page did not keep the signed-in account in shared navigation")
			}
			htmx := portalApp.request(route.method, route.path, true, ungranted)
			if htmx.Code != http.StatusNoContent || htmx.Header().Get("HX-Redirect") != "/mgmt" || htmx.Body.Len() != 0 {
				t.Fatalf("missing-grant HTMX request did not navigate to the access message: %d %q", htmx.Code, htmx.Header().Get("HX-Redirect"))
			}
		})
	}
	if calls := portalApp.aws.calls(); calls != 0 {
		t.Fatalf("denied management requests reached AWS %d times", calls)
	}
}

func TestManagementGrantAdmitsPortalWithinCurrentAWSLimits(t *testing.T) {
	portalApp := newManagementPortal(t, newFakeSiteCognito(t), map[string][]string{"owner@example.com": {"management"}})
	owner := portalApp.signIn(t, "owner@example.com", "/mgmt")

	dashboard := portalApp.request(http.MethodGet, "/mgmt", false, owner)
	body := dashboard.Body.String()
	if dashboard.Code != http.StatusOK || !strings.Contains(body, "Signed in as owner@example.com") || !strings.Contains(body, "2 total") {
		t.Fatalf("granted dashboard did not list the inventory for the site account: %d", dashboard.Code)
	}
	if !strings.Contains(body, `data-portal-instance="`+untaggedInstanceID+`"`) || !strings.Contains(body, "Read only — instance actions are unavailable.") {
		t.Fatal("dashboard no longer shows an untagged instance as read-only")
	}
	metrics := portalApp.request(http.MethodGet, "/mgmt/instances/"+taggedInstanceID+"/metrics", true, owner)
	if metrics.Code != http.StatusOK || metrics.Header().Get("X-Portal-Fragment-Error") != "" {
		t.Fatalf("granted metrics read failed: %d %q", metrics.Code, metrics.Body.String())
	}

	// The grant opens the portal; the runtime role still denies EC2 commands and instance log reads.
	for _, action := range []string{"start", "stop", "restart"} {
		response := portalApp.request(http.MethodPost, "/mgmt/instances/"+taggedInstanceID+"/"+action, true, owner)
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "The instance action failed.") {
			t.Fatalf("%s beyond the AWS permission limit was not reported as failed: %d %q", action, response.Code, response.Body.String())
		}
	}
	logs := portalApp.request(http.MethodGet, "/mgmt/instances/"+taggedInstanceID+"/logs", true, owner)
	if logs.Code != http.StatusInternalServerError || !strings.Contains(logs.Body.String(), "Unable to load logs.") {
		t.Fatalf("instance log read beyond the AWS permission limit was not reported as failed: %d %q", logs.Code, logs.Body.String())
	}

	got := portalApp.aws
	if got.describes != 1 || strings.Join(got.metricInstances, ",") != taggedInstanceID {
		t.Fatalf("granted reads = describes %d, metrics %v", got.describes, got.metricInstances)
	}
	// start, stop, and the stop half of restart reach AWS once each; a denied stop never starts.
	if strings.Join(got.starts, ",") != taggedInstanceID || strings.Join(got.stops, ",") != taggedInstanceID+","+taggedInstanceID {
		t.Fatalf("EC2 commands = starts %v, stops %v", got.starts, got.stops)
	}
	if strings.Join(got.logGroups, ",") != "/ec2/"+taggedInstanceID {
		t.Fatalf("instance log reads = %v", got.logGroups)
	}
}

func TestManagementGrantKeepsPortalCommandsWorkingWhereAWSPermitsThem(t *testing.T) {
	portalApp := newManagementPortal(t, newFakeSiteCognito(t), map[string][]string{"owner@example.com": {"management"}})
	portalApp.aws.unrestricted = true
	owner := portalApp.signIn(t, "owner@example.com", "/mgmt")

	for action, feedback := range map[string]string{
		"start":   "Instance start requested successfully.",
		"stop":    "Instance stop requested successfully.",
		"restart": "Instance restart requested successfully.",
	} {
		response := portalApp.request(http.MethodPost, "/mgmt/instances/"+taggedInstanceID+"/"+action, true, owner)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), feedback) {
			t.Fatalf("permitted %s failed: %d %q", action, response.Code, response.Body.String())
		}
	}
	logs := portalApp.request(http.MethodGet, "/mgmt/instances/"+taggedInstanceID+"/logs", true, owner)
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), "No recent log events") {
		t.Fatalf("permitted log read failed: %d %q", logs.Code, logs.Body.String())
	}
	if got := portalApp.aws; len(got.starts) != 2 || len(got.stops) != 2 || len(got.logGroups) != 1 {
		t.Fatalf("permitted commands = starts %v, stops %v, logs %v", got.starts, got.stops, got.logGroups)
	}
}

func TestRevokedManagementGrantDeniesNextRequestFromNewConfiguration(t *testing.T) {
	cognito := newFakeSiteCognito(t)
	granted := newManagementPortal(t, cognito, map[string][]string{"owner@example.com": {"soccer", "management"}})
	owner := granted.signIn(t, "owner@example.com", "/mgmt")
	if response := granted.request(http.MethodGet, "/mgmt", false, owner); response.Code != http.StatusOK {
		t.Fatalf("granted dashboard status = %d", response.Code)
	}

	// A reviewed configuration change keeps the invitation but drops management.
	revoked := newManagementPortal(t, cognito, map[string][]string{"owner@example.com": {"soccer"}})
	account := revoked.request(http.MethodGet, "/about", false, owner)
	if account.Code != http.StatusOK || !strings.Contains(account.Body.String(), `<span class="site-account-email" title="owner@example.com">owner@example.com</span>`) {
		t.Fatal("the existing site session did not remain valid under the new configuration")
	}
	for _, route := range managementRoutes {
		response := revoked.request(route.method, route.path, false, owner)
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "Management access required") {
			t.Fatalf("revoked grant still reached %s %s: %d", route.method, route.path, response.Code)
		}
	}
	if calls := revoked.aws.calls(); calls != 0 {
		t.Fatalf("revoked grant reached AWS %d times", calls)
	}
}

func TestSiteSignOutEndsManagementPortalAccess(t *testing.T) {
	portalApp := newManagementPortal(t, newFakeSiteCognito(t), map[string][]string{"owner@example.com": {"management"}})
	owner := portalApp.signIn(t, "owner@example.com", "/mgmt")

	dashboard := portalApp.request(http.MethodGet, "/mgmt", false, owner).Body.String()
	if !strings.Contains(dashboard, `<span class="site-account-email" title="owner@example.com">owner@example.com</span>`) || !strings.Contains(dashboard, "Signed in as owner@example.com") {
		t.Fatal("portal and shared navigation do not show the same site account")
	}
	if strings.Count(dashboard, `<form method="POST" action="/sign-out">`) < 2 || strings.Contains(dashboard, `action="/logout"`) {
		t.Fatal("portal sign-out is not the shared site sign-out")
	}

	signOut := portalApp.request(http.MethodPost, "/sign-out", false, owner)
	if cleared := siteCookie(t, signOut); signOut.Code != http.StatusSeeOther || cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("site sign-out did not end the site session: %d", signOut.Code)
	}
	// The browser no longer sends the expired cookie.
	if response := portalApp.request(http.MethodGet, "/mgmt", false); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in?return_to=%2Fmgmt" {
		t.Fatalf("signed-out browser still reached the portal: %d", response.Code)
	}
	if page := portalApp.request(http.MethodGet, "/about", false).Body.String(); strings.Contains(page, "owner@example.com") || !strings.Contains(page, `href="/sign-in?return_to=%2Fabout"`) {
		t.Fatal("shared navigation still shows the signed-out account")
	}
	if portalApp.aws.describes != 1 {
		t.Fatalf("signed-out request reached AWS: describes = %d", portalApp.aws.describes)
	}
}
