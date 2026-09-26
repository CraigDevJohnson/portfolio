package app

import (
	"context"
	"errors"
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

	"portfolio/internal/config"
	"portfolio/internal/portal"
	"portfolio/internal/session"
)

type managementEC2Fake struct {
	describes int
	starts    int
	stops     int
	denied    int
}

func (f *managementEC2Fake) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	f.describes++
	return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
		InstanceId: aws.String("i-0123456789abcdef0"),
		State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		Tags:       []ec2types.Tag{{Key: aws.String("PortfolioManagement"), Value: aws.String("dev")}},
	}, {
		InstanceId: aws.String("i-11111111111111111"),
		State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
	}}}}}, nil
}

func (f *managementEC2Fake) StartInstances(_ context.Context, input *ec2.StartInstancesInput, _ ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	if len(input.InstanceIds) != 1 || input.InstanceIds[0] != "i-0123456789abcdef0" {
		f.denied++
		return nil, errors.New("AccessDenied")
	}
	f.starts++
	return &ec2.StartInstancesOutput{}, nil
}

func (f *managementEC2Fake) StopInstances(_ context.Context, input *ec2.StopInstancesInput, _ ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	if len(input.InstanceIds) != 1 || input.InstanceIds[0] != "i-0123456789abcdef0" {
		f.denied++
		return nil, errors.New("AccessDenied")
	}
	f.stops++
	return &ec2.StopInstancesOutput{}, nil
}

type managementMetricsFake struct{ calls int }

func (f *managementMetricsFake) GetMetricStatistics(context.Context, *cloudwatch.GetMetricStatisticsInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	f.calls++
	return &cloudwatch.GetMetricStatisticsOutput{}, nil
}

type managementLogsFake struct{ calls int }

func (f *managementLogsFake) FilterLogEvents(context.Context, *cloudwatchlogs.FilterLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	f.calls++
	return &cloudwatchlogs.FilterLogEventsOutput{}, nil
}

func TestManagementRoutesUseCurrentSiteGrant(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	ec2Fake := &managementEC2Fake{}
	metricsFake := &managementMetricsFake{}
	logsFake := &managementLogsFake{}
	application.PortalHandler = portal.NewHandler(&application.Config, ec2Fake, metricsFake, logsFake, application.Logger)
	mux, _ := buildMux(application, application.Logger, false)

	stateCookie, state := beginSiteSignIn(t, mux, "/mgmt")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/mgmt" {
		t.Fatalf("site sign-in did not return to management: %d %q", callback.Code, callback.Header().Get("Location"))
	}
	siteSession := siteCookie(t, callback)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/mgmt"},
		{http.MethodPost, "/mgmt/instances/i-0123456789abcdef0/start"},
		{http.MethodPost, "/mgmt/instances/i-0123456789abcdef0/stop"},
		{http.MethodPost, "/mgmt/instances/i-0123456789abcdef0/restart"},
		{http.MethodGet, "/mgmt/instances/i-0123456789abcdef0/metrics"},
		{http.MethodGet, "/mgmt/instances/i-0123456789abcdef0/logs"},
	}
	formerSession, err := session.EncryptJSONValue([]byte("0123456789abcdef0123456789abcdef"), map[string]any{
		"username":   "owner@example.com",
		"expires_at": time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "https://app.example.com"+path, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	for _, route := range routes {
		t.Run("signed out "+route.path, func(t *testing.T) {
			response := request(route.method, route.path)
			if response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/sign-in?return_to=") {
				t.Fatalf("signed-out request reached management: %d %q", response.Code, response.Header().Get("Location"))
			}
		})
		t.Run("former session only "+route.path, func(t *testing.T) {
			response := request(route.method, route.path, &http.Cookie{Name: config.PortalSessionCookieName, Value: formerSession})
			if response.Code != http.StatusSeeOther {
				t.Fatalf("former management session authorized request: %d", response.Code)
			}
		})
	}
	if ec2Fake.describes != 0 || ec2Fake.starts != 0 || ec2Fake.stops != 0 || metricsFake.calls != 0 || logsFake.calls != 0 {
		t.Fatal("denied request reached an AWS client")
	}
	htmxRequest := httptest.NewRequest(http.MethodGet, "https://app.example.com/mgmt/instances/i-0123456789abcdef0/metrics", nil)
	htmxRequest.Header.Set("HX-Request", "true")
	htmxResponse := httptest.NewRecorder()
	mux.ServeHTTP(htmxResponse, htmxRequest)
	if htmxResponse.Code != http.StatusNoContent || htmxResponse.Header().Get("HX-Redirect") != "/sign-in?return_to=%2Fmgmt" {
		t.Fatalf("signed-out HTMX request did not navigate to site sign-in: %d %q", htmxResponse.Code, htmxResponse.Header().Get("HX-Redirect"))
	}

	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	for _, route := range routes {
		response := request(route.method, route.path, siteSession, &http.Cookie{Name: config.PortalSessionCookieName, Value: formerSession})
		if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "management access") {
			t.Fatalf("missing management grant was not explained for %s: %d %q", route.path, response.Code, response.Body.String())
		}
	}
	if ec2Fake.describes != 0 || ec2Fake.starts != 0 || ec2Fake.stops != 0 || metricsFake.calls != 0 || logsFake.calls != 0 {
		t.Fatal("revoked grant reached an AWS client")
	}
	htmxRequest.AddCookie(siteSession)
	htmxResponse = httptest.NewRecorder()
	mux.ServeHTTP(htmxResponse, htmxRequest)
	if htmxResponse.Code != http.StatusNoContent || htmxResponse.Header().Get("HX-Redirect") != "/mgmt" {
		t.Fatalf("missing grant HTMX denial did not open the access message: %d %q", htmxResponse.Code, htmxResponse.Header().Get("HX-Redirect"))
	}

	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer", "management"}
	for _, route := range routes {
		response := request(route.method, route.path, siteSession)
		if response.Code != http.StatusOK {
			t.Fatalf("granted request %s failed: %d %q", route.path, response.Code, response.Body.String())
		}
		if route.path == "/mgmt" {
			body := response.Body.String()
			if !strings.Contains(body, "owner@example.com") || !strings.Contains(body, `action="/sign-out"`) {
				t.Fatal("portal account and sign-out differ from shared site account")
			}
			if !strings.Contains(body, "i-11111111111111111") || !strings.Contains(body, "Read only — instance actions are unavailable.") {
				t.Fatal("portal no longer shows untagged inventory as read-only")
			}
		}
	}
	if response := request(http.MethodPost, "/mgmt/instances/i-11111111111111111/stop", siteSession); response.Code != http.StatusInternalServerError || ec2Fake.denied != 1 {
		t.Fatalf("untagged instance escaped the fake AWS permission limit: status=%d denied=%d", response.Code, ec2Fake.denied)
	}
	if ec2Fake.describes != 1 || ec2Fake.starts != 2 || ec2Fake.stops != 2 || metricsFake.calls != 1 || logsFake.calls != 1 {
		t.Fatalf("granted operations: EC2=%+v metrics=%d logs=%d", ec2Fake, metricsFake.calls, logsFake.calls)
	}

	signOut := request(http.MethodPost, "/sign-out", siteSession)
	if cleared := siteCookie(t, signOut); cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatal("site sign-out retained portal access")
	}
	if response := request(http.MethodGet, "/mgmt"); response.Code != http.StatusSeeOther {
		t.Fatal("signed-out browser still reached portal")
	}
}
