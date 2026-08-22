package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/httpx"
	"portfolio/internal/portal"
	"portfolio/internal/session"
	internalsoccer "portfolio/internal/soccer"
)

type testConnectionStore struct{}

func (testConnectionStore) Delete(context.Context, string) error { return nil }
func (testConnectionStore) Get(context.Context, string) (*internalgoogle.ConnectionRecord, error) {
	return nil, nil
}

func (testConnectionStore) Put(context.Context, *internalgoogle.ConnectionRecord) error { return nil }

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
// context yields HTTP OAuth callbacks and cookies without Secure; enabling the
// empty portal config exposes management routes; accepting an empty gateway domain
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

	portalConfig := config.Config{}
	portalHandler := portal.NewHandler(&portalConfig, nil, nil, nil, nil, logger)
	if portalHandler.Config.PortalEnabled() {
		t.Fatal("empty portal config unexpectedly enabled the portal")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /soccer/google/connect", googleHandler.ConnectHandler)
	mux.HandleFunc("GET /test/google-connection", func(w http.ResponseWriter, r *http.Request) {
		internalgoogle.SetConnectionCookie(w, r, "connection-id")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /test/soccer-logout", soccerHandler.LogoutHandler)
	mux.HandleFunc("POST /test/portal-logout", portalHandler.LogoutHandler)
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
				if cookie := responseCookie(t, response, config.GoogleConnectionCookieName); !cookie.Secure {
					t.Fatal("google_connection cookie is not Secure")
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
			name:  "Portal logout cookie remains secure and strict",
			event: gatewayEvent(http.MethodPost, "/test/portal-logout"),
			assert: func(t *testing.T, response events.APIGatewayV2HTTPResponse) {
				cookie := responseCookie(t, response, config.PortalSessionCookieName)
				if !cookie.Secure || cookie.MaxAge >= 0 || cookie.Path != config.PortalCookiePath || cookie.SameSite != http.SameSiteStrictMode {
					t.Fatalf("expired mgmt_session attributes = Secure:%t MaxAge:%d Path:%q SameSite:%d", cookie.Secure, cookie.MaxAge, cookie.Path, cookie.SameSite)
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
