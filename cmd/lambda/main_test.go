package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/httpx"
	"portfolio/internal/lps"
	"portfolio/internal/session"
	"portfolio/internal/siteauth"
	"portfolio/internal/siteidentity"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/soccerarchive"
	"portfolio/internal/soccerarchive/archivetest"
)

// failingIndexTable is an archive table whose due-teams index is unavailable.
type failingIndexTable struct{ *archivetest.Table }

func (failingIndexTable) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, errors.New("due-teams index unavailable")
}

// scheduledLogs captures the structured JSON log records of one test.
func scheduledLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return func() []map[string]any {
		lines := bytes.Split(bytes.TrimSpace(buffer.Bytes()), []byte("\n"))
		records := make([]map[string]any, 0, len(lines))
		for _, line := range lines {
			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				t.Fatalf("log line %q is not JSON: %v", line, err)
			}
			records = append(records, record)
		}
		buffer.Reset()
		return records
	}
}

// schedulerEvent is the input the daily schedule sends the worker.
var schedulerEvent = json.RawMessage(`{"source":"portfolio.soccer-history.daily"}`)

func TestScheduledInvocationRefreshesDueTeamsAndReportsPartialWork(t *testing.T) {
	logs := scheduledLogs(t)
	limits := soccerarchive.Limits{MaxEnrolledTeams: 2, MaxRequestsPerRun: 2, MinRequestInterval: time.Millisecond}
	table := archivetest.NewTable()
	store, err := soccerarchive.NewDynamoStoreWithAPI(table, "portfolio-lambda-dev-soccer-history", limits)
	if err != nil {
		t.Fatal(err)
	}
	// Both teams were entered by Team ID yesterday, so both are due.
	for _, teamID := range []int{101, 202} {
		if err := store.SaveTeamSnapshot(t.Context(), &soccerarchive.Snapshot{
			TeamID: teamID, Team: lps.TeamSummary{UTeamID: teamID, Season: 169}, FetchedAt: time.Now().Add(-25 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/101":
			_, _ = fmt.Fprint(w, `{"team":{"UTeamID":101,"Season":169},"games":[{"UGameID":9101,"UTeam1":101,"UTeam2":303,"Season":169,"result":"2-1"}]}`)
		case "/teams/202":
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		default:
			t.Errorf("unexpected LPS request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	worker, err := soccerarchive.NewDailyWorker(store, server.URL, server.Client(), limits, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := newDailyLambdaHandler(worker)

	partial, err := handler(t.Context(), schedulerEvent)
	if err != nil || partial.Complete || partial.Requests != 2 || len(partial.Results) != 2 ||
		partial.Results[0] != (soccerarchive.RefreshResult{TeamID: 101, Outcome: soccerarchive.RefreshSucceeded}) ||
		partial.Results[1].TeamID != 202 || partial.Results[1].Outcome != soccerarchive.RefreshRetryableFailure {
		t.Fatalf("scheduled partial run = %#v, err %v", partial, err)
	}
	history, err := store.ReadTeamSeason(t.Context(), 101, 169)
	if err != nil || len(history.Games) != 1 || history.Games[0].Result != "2-1" {
		t.Fatalf("refreshed history = %#v, err %v", history, err)
	}
	// The incomplete-run alarm matches this message in the worker's log.
	if records := logs(); len(records) != 1 || records[0]["msg"] != "soccer_history_daily_incomplete" || records[0]["level"] != "WARN" || records[0]["results"] == nil {
		t.Fatalf("partial run logs = %v", records)
	}

	// A repeated delivery finds the refreshed team not due and the rate-limited
	// team still backing off, so it makes no requests and reports completion.
	repeated, err := handler(t.Context(), schedulerEvent)
	if err != nil || !repeated.Complete || repeated.Requests != 0 || len(repeated.Results) != 0 {
		t.Fatalf("repeated delivery = %#v, err %v", repeated, err)
	}
	if records := logs(); len(records) != 1 || records[0]["msg"] != "soccer_history_daily_completed" {
		t.Fatalf("repeated delivery logs = %v", records)
	}
}

func TestScheduledInvocationFailsWhenDueTeamsCannotBeSelected(t *testing.T) {
	logs := scheduledLogs(t)
	limits := soccerarchive.Limits{MaxEnrolledTeams: 1, MaxRequestsPerRun: 1, MinRequestInterval: time.Millisecond}
	store, err := soccerarchive.NewDynamoStoreWithAPI(failingIndexTable{archivetest.NewTable()}, "portfolio-lambda-dev-soccer-history", limits)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := soccerarchive.NewDailyWorker(store, "http://127.0.0.1:9", nil, limits, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := newDailyLambdaHandler(worker)(t.Context(), schedulerEvent); err == nil {
		t.Fatal("a run that could not select due teams reported success, so no failure destination or error alarm would see it")
	}
	if records := logs(); len(records) != 1 || records[0]["msg"] != "soccer_history_daily_failed" || records[0]["level"] != "ERROR" {
		t.Fatalf("failed run logs = %v", records)
	}
}

func TestScheduledWorkerIsNotBuiltWithoutEveryReviewedLimit(t *testing.T) {
	configured := map[string]string{
		"SOCCER_ARCHIVE_TABLE_NAME":      "portfolio-lambda-dev-soccer-history",
		"SOCCER_HISTORY_MAX_TEAMS":       "4",
		"SOCCER_HISTORY_PLAYER_RESERVED": "2",
		"SOCCER_HISTORY_MAX_REQUESTS":    "8",
		"SOCCER_HISTORY_MAX_RETRIES":     "1",
		"SOCCER_HISTORY_MIN_INTERVAL_MS": "250",
	}
	for unset := range configured {
		t.Run(unset, func(t *testing.T) {
			// Any AWS client built by mistake points at a closed loopback port.
			for name, value := range map[string]string{
				"AWS_ENDPOINT_URL": "http://127.0.0.1:9", "AWS_REGION": "us-west-2",
				"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_EC2_METADATA_DISABLED": "true",
				"LPS_API_BASE_URL": "http://127.0.0.1:9",
			} {
				t.Setenv(name, value)
			}
			for name, value := range configured {
				t.Setenv(name, value)
			}
			t.Setenv(unset, "")

			if worker, err := initializeDailyLambda(t.Context()); err == nil || worker != nil {
				t.Fatalf("scheduled worker built without %s: %v", unset, err)
			}
		})
	}
}

type testConnectionStore struct{}

func (testConnectionStore) Delete(context.Context, string) error { return nil }
func (testConnectionStore) Get(context.Context, string) (*internalgoogle.ConnectionRecord, error) {
	return nil, nil
}

func (testConnectionStore) Put(context.Context, *internalgoogle.ConnectionRecord) error { return nil }

func (testConnectionStore) PutIfUnchanged(context.Context, *internalgoogle.ConnectionRecord, time.Time) error {
	return nil
}

type recordingProxyV2 struct {
	calls  int
	events []events.APIGatewayV2HTTPRequest
}

func (p *recordingProxyV2) ProxyWithContext(_ context.Context, event events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) { //nolint:gocritic // The upstream adapter interface passes this event by value.
	p.calls++
	p.events = append(p.events, event)
	return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusNoContent}, nil
}

func gatewayEvent(method, path string) events.APIGatewayV2HTTPRequest {
	return events.APIGatewayV2HTTPRequest{
		RawPath: path,
		Headers: map[string]string{"host": "dev.craigdevjohnson.com"},
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			DomainName: "dev.craigdevjohnson.com",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:   method,
				Path:     path,
				Protocol: "HTTP/1.1",
				SourceIP: "203.0.113.10",
			},
		},
	}
}

func responseCookie(t *testing.T, response events.APIGatewayV2HTTPResponse, name string) *http.Cookie {
	t.Helper()
	parsed := (&http.Response{Header: http.Header{"Set-Cookie": response.Cookies}}).Cookies()
	for _, cookie := range parsed {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response cookies %q do not include %q", response.Cookies, name)
	return nil
}

func responseHeader(response events.APIGatewayV2HTTPResponse, name string) string {
	for key, value := range response.Headers {
		if http.CanonicalHeaderKey(key) == http.CanonicalHeaderKey(name) {
			return value
		}
	}
	return ""
}

// Production breaks caught: trusting request headers instead of the typed gateway
// context yields HTTP OAuth callbacks and cookies without Secure; accepting an empty gateway domain
// lets requests reach handlers without a trustworthy origin.
func TestAPIGatewayOriginSecuresProductionCookiesAndRedirects(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		SessionKey:                bytes.Repeat([]byte{0x42}, 32),
		LPSAPIBaseURL:             config.DefaultLPSAPIBaseURL,
		GoogleClientID:            "google-client-id",
		GoogleClientSecret:        "google-client-secret",
		GoogleConnectionTableName: "google-connections",
	}
	googleHandler := internalgoogle.NewHandler(&cfg, &http.Client{}, logger, nil)
	googleHandler.SetStore(testConnectionStore{})
	limiter := session.NewLoginRateLimiter(5, time.Minute, 10)
	t.Cleanup(limiter.Close)
	soccerHandler := internalsoccer.NewHandler(
		&cfg,
		&http.Client{},
		limiter,
		googleHandler,
		internalsoccer.NoopSoccerStore{},
		logger,
	)

	siteHandler := siteauth.NewHandler(&config.Config{}, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /soccer/google/connect", googleHandler.ConnectHandler)
	owner := siteidentity.Principal{Issuer: "https://issuer.example.com/pool", Subject: "owner-subject"}
	mux.HandleFunc("GET /test/google-connection", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(siteidentity.WithRequestIdentity(r.Context(), &owner, []siteidentity.Grant{siteidentity.GrantSoccer}, r.URL.Path))
		internalgoogle.SetConnectionCookie(w, r, "connection-id")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /test/soccer-logout", soccerHandler.LogoutHandler)
	mux.HandleFunc("POST /test/site-logout", siteHandler.LogoutHandler)
	mux.HandleFunc("GET /test/origin", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, httpx.RequestBaseURL(r))
	})
	adapter := httpadapter.NewV2(withAPIGatewayOrigin(mux))

	tests := []struct {
		name   string
		event  events.APIGatewayV2HTTPRequest
		assert func(*testing.T, events.APIGatewayV2HTTPResponse)
	}{
		{
			name:  "Google OAuth uses HTTPS callback and secure state cookie",
			event: gatewayEvent(http.MethodGet, "/soccer/google/connect"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				if response.StatusCode != http.StatusSeeOther {
					t.Fatalf("status = %d, want %d; body = %q", response.StatusCode, http.StatusSeeOther, response.Body)
				}
				location, err := url.Parse(responseHeader(response, "Location"))
				if err != nil {
					t.Fatalf("parse Location: %v", err)
				}
				if got := location.Query().Get("redirect_uri"); got != "https://dev.craigdevjohnson.com/soccer" {
					t.Fatalf("redirect_uri = %q, want gateway HTTPS origin", got)
				}
				cookie := responseCookie(t, response, config.GoogleOAuthStateCookieName)
				if !cookie.Secure || !cookie.HttpOnly || cookie.Path != config.SoccerCookiePath || cookie.SameSite != http.SameSiteLaxMode {
					t.Fatalf("google_oauth_state attributes = Secure:%t HttpOnly:%t Path:%q SameSite:%d", cookie.Secure, cookie.HttpOnly, cookie.Path, cookie.SameSite)
				}
			},
		},
		{
			name:  "Google connection cookie remains secure",
			event: gatewayEvent(http.MethodGet, "/test/google-connection"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				if cookie := responseCookie(t, response, internalgoogle.ConnectionCookieName(owner.Issuer, owner.Subject)); !cookie.Secure {
					t.Fatal("Google connection cookie is not Secure")
				}
			},
		},
		{
			name:  "Soccer logout cookie remains secure",
			event: gatewayEvent(http.MethodPost, "/test/soccer-logout"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				cookie := responseCookie(t, response, config.LPSSessionCookieName)
				if !cookie.Secure || cookie.MaxAge >= 0 {
					t.Fatalf("expired lps_session attributes = Secure:%t MaxAge:%d", cookie.Secure, cookie.MaxAge)
				}
			},
		},
		{
			name:  "Shared site logout cookie remains secure and lax",
			event: gatewayEvent(http.MethodPost, "/test/site-logout"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				cookie := responseCookie(t, response, config.SiteSessionCookieName)
				if !cookie.Secure || cookie.MaxAge >= 0 || cookie.Path != config.SiteCookiePath || cookie.SameSite != http.SameSiteLaxMode {
					t.Fatalf("expired site_session attributes = Secure:%t MaxAge:%d Path:%q SameSite:%d", cookie.Secure, cookie.MaxAge, cookie.Path, cookie.SameSite)
				}
			},
		},
		{
			name:  "Probe sees the typed gateway origin",
			event: gatewayEvent(http.MethodGet, "/test/origin"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				if response.StatusCode != http.StatusOK || response.Body != "https://dev.craigdevjohnson.com" {
					t.Fatalf("probe response = status %d body %q", response.StatusCode, response.Body)
				}
			},
		},
		{
			name: "Typed gateway domain overrides the host header",
			event: func() events.APIGatewayV2HTTPRequest {
				event := gatewayEvent(http.MethodGet, "/test/origin")
				event.Headers["host"] = "attacker.example"
				return event
			}(),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				if response.StatusCode != http.StatusOK || response.Body != "https://dev.craigdevjohnson.com" {
					t.Fatalf("probe response = status %d body %q", response.StatusCode, response.Body)
				}
			},
		},
		{
			name: "Missing gateway domain fails closed",
			event: func() events.APIGatewayV2HTTPRequest {
				event := gatewayEvent(http.MethodGet, "/test/origin")
				event.RequestContext.DomainName = ""
				return event
			}(),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				if response.StatusCode != http.StatusInternalServerError || response.Body != "gateway request context missing\n" {
					t.Fatalf("missing-domain response = status %d body %q", response.StatusCode, response.Body)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := adapter.ProxyWithContext(t.Context(), test.event)
			if err != nil {
				t.Fatalf("ProxyWithContext: %v", err)
			}
			test.assert(t, response)
		})
	}

	for _, path := range []string{"/login", "/auth/callback", "/mgmt"} {
		t.Run("portal route absent "+path, func(t *testing.T) {
			response, err := adapter.ProxyWithContext(t.Context(), gatewayEvent(http.MethodGet, path))
			if err != nil {
				t.Fatalf("ProxyWithContext: %v", err)
			}
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNotFound)
			}
		})
	}
}

// Production break caught: a nil Lambda event would panic when dereferenced or
// reach the proxy as an empty request.
func TestLambdaHandlerRejectsNilEvent(t *testing.T) {
	proxy := &recordingProxyV2{}
	handler := newLambdaHandler(proxy)

	response, err := handler(t.Context(), nil)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if response.StatusCode != http.StatusBadRequest || response.Body != "invalid request" {
		t.Fatalf("nil-event response = status %d body %q", response.StatusCode, response.Body)
	}
	if proxy.calls != 0 {
		t.Fatalf("proxy calls = %d, want 0", proxy.calls)
	}
}

// Production break caught: rebuilding the adapter during each invocation loses
// the warm Lambda proxy and repeats application and AWS initialization.
func TestLambdaHandlerReusesWarmProxy(t *testing.T) {
	proxy := &recordingProxyV2{}
	handler := newLambdaHandler(proxy)
	first := gatewayEvent(http.MethodGet, "/first")
	second := gatewayEvent(http.MethodGet, "/second")

	for _, event := range []events.APIGatewayV2HTTPRequest{first, second} {
		response, err := handler(t.Context(), &event)
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
		}
	}

	if proxy.calls != 2 {
		t.Fatalf("proxy calls = %d, want 2", proxy.calls)
	}
	if len(proxy.events) != 2 || proxy.events[0].RawPath != "/first" || proxy.events[1].RawPath != "/second" {
		t.Fatalf("proxied events = %#v", proxy.events)
	}
}
