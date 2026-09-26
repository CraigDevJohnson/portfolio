package app

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"portfolio/internal/config"
	"portfolio/internal/siteidentity"
	"portfolio/internal/testutil"
)

func TestProductionLikeSiteIdentitySeparatesEnvironmentsAndKeepsPublicRoutes(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := map[string]string{}
	federation := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dev/.well-known/jwks.json", "/prod/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": "site-test", "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB",
			}}})
		case "/oauth2/token":
			if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("code_verifier") == "" || r.Form.Get("client_secret") != "" {
				t.Error("invalid Cognito token exchange")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			client := r.Form.Get("client_id")
			code := r.Form.Get("code")
			if client != "dev-site-client" && client != "prod-site-client" {
				t.Errorf("unexpected app client %q", client)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			identity := strings.TrimSuffix(client, "-site-client")
			if strings.HasPrefix(code, "dev-") {
				identity = "dev"
			} else if strings.HasPrefix(code, "prod-") {
				identity = "prod"
			}
			email := "craigdevjohnson@gmail.com"
			if strings.HasSuffix(code, "-uninvited") {
				email = "uninvited@example.com"
			}
			claims := jwt.MapClaims{
				"iss": issuer[identity], "aud": identity + "-site-client", "sub": identity + "-stable-subject",
				"exp": time.Now().Add(time.Hour).Unix(), "token_use": "id", "email": email, "email_verified": true,
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "site-test"
			raw, signErr := token.SignedString(key)
			if signErr != nil {
				t.Error(signErr)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": raw})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(federation.Close)
	issuer["dev"] = federation.URL + "/dev"
	issuer["prod"] = federation.URL + "/prod"
	previousClient := http.DefaultClient
	http.DefaultClient = federation.Client()
	t.Cleanup(func() { http.DefaultClient = previousClient })

	newEnvironment := func(environment string) (*App, http.Handler) {
		t.Helper()
		origin := "https://dev.craigdevjohnson.com"
		keyByte := byte('d')
		if environment == "prod" {
			origin = "https://craigdevjohnson.com"
			keyByte = 'p'
		}
		cfg := config.Config{
			LPSAPIBaseURL:          config.DefaultLPSAPIBaseURL,
			SiteSessionKey:         bytes.Repeat([]byte{keyByte}, 32),
			SiteCognitoDomain:      federation.URL,
			SiteCognitoIssuer:      issuer[environment],
			SiteCognitoClientID:    environment + "-site-client",
			SiteCognitoRedirectURI: origin + "/auth/callback",
			SiteCognitoLogoutURI:   origin + "/sign-in",
			SiteInvitations: map[string][]string{
				"craigdevjohnson@gmail.com": {"soccer", "management"},
			},
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		application := New(&cfg, logger)
		t.Cleanup(application.LoginLimiter.Close)
		mux, _ := buildMux(application, logger, false)
		return application, mux
	}

	dev, devMux := newEnvironment("dev")
	prod, prodMux := newEnvironment("prod")
	if dev.PortalHandler != nil || prod.PortalHandler != nil {
		t.Fatal("site sign-in unexpectedly depends on management AWS clients")
	}
	start := func(handler http.Handler, origin, client, callback string) (*http.Cookie, string) {
		t.Helper()
		form := url.Values{"return_to": {"/soccer"}}
		req := httptest.NewRequest(http.MethodPost, origin+"/sign-in", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		target, parseErr := url.Parse(response.Header().Get("Location"))
		if parseErr != nil || response.Code != http.StatusSeeOther || target.Query().Get("client_id") != client || target.Query().Get("redirect_uri") != callback || target.Query().Get("identity_provider") != "Google" {
			t.Fatalf("sign-in did not use its own Google-federated client and callback: %d %s", response.Code, target)
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == config.SiteOAuthStateCookieName {
				return cookie, target.Query().Get("state")
			}
		}
		t.Fatal("sign-in did not set a pending OAuth state")
		return nil, ""
	}
	finish := func(handler http.Handler, origin, code string, stateCookie *http.Cookie, state string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, origin+"/auth/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil)
		req.AddCookie(stateCookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	devStateCookie, devState := start(devMux, "https://dev.craigdevjohnson.com", "dev-site-client", dev.Config.SiteCognitoRedirectURI)
	devCallback := finish(devMux, "https://dev.craigdevjohnson.com", "dev-code", devStateCookie, devState)
	if devCallback.Code != http.StatusSeeOther || devCallback.Header().Get("Location") != "/soccer" {
		t.Fatalf("development sign-in failed: %d %s", devCallback.Code, devCallback.Body.String())
	}
	devCookie := siteCookie(t, devCallback)
	prodStateCookie, prodState := start(prodMux, "https://craigdevjohnson.com", "prod-site-client", prod.Config.SiteCognitoRedirectURI)
	prodCallback := finish(prodMux, "https://craigdevjohnson.com", "prod-code", prodStateCookie, prodState)
	if prodCallback.Code != http.StatusSeeOther || prodCallback.Header().Get("Location") != "/soccer" {
		t.Fatalf("production sign-in failed: %d %s", prodCallback.Code, prodCallback.Body.String())
	}
	prodCookie := siteCookie(t, prodCallback)

	protected := func(app *App, cookie *http.Cookie, grant siteidentity.Grant) int {
		t.Helper()
		probe := app.SiteHandler.WithIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := siteidentity.PrincipalFromContext(r.Context()); !ok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !siteidentity.HasGrant(r.Context(), grant) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodPost, "https://craigdevjohnson.com/restricted", nil)
		req.AddCookie(cookie)
		response := httptest.NewRecorder()
		probe.ServeHTTP(response, req)
		return response.Code
	}
	if protected(dev, devCookie, siteidentity.GrantManagement) != http.StatusNoContent || protected(prod, prodCookie, siteidentity.GrantManagement) != http.StatusNoContent {
		t.Fatal("invited verified identities did not receive their current management grant")
	}
	if protected(prod, devCookie, siteidentity.GrantManagement) != http.StatusUnauthorized || protected(dev, prodCookie, siteidentity.GrantManagement) != http.StatusUnauthorized {
		t.Fatal("one environment's session authorized the other's restricted action")
	}
	prod.Config.SiteInvitations["craigdevjohnson@gmail.com"] = []string{"soccer"}
	if protected(prod, prodCookie, siteidentity.GrantManagement) != http.StatusForbidden || protected(prod, prodCookie, siteidentity.GrantSoccer) != http.StatusNoContent {
		t.Fatal("production did not check the current page grant on the next request")
	}

	for _, attempt := range []struct {
		name     string
		mux      http.Handler
		client   string
		returnTo string
		origin   string
		code     string
	}{
		{name: "development token in production", mux: prodMux, client: "prod-site-client", returnTo: prod.Config.SiteCognitoRedirectURI, origin: "https://craigdevjohnson.com", code: "dev-code"},
		{name: "production token in development", mux: devMux, client: "dev-site-client", returnTo: dev.Config.SiteCognitoRedirectURI, origin: "https://dev.craigdevjohnson.com", code: "prod-code"},
		{name: "uninvited production identity", mux: prodMux, client: "prod-site-client", returnTo: prod.Config.SiteCognitoRedirectURI, origin: "https://craigdevjohnson.com", code: "prod-uninvited"},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			stateCookie, state := start(attempt.mux, attempt.origin, attempt.client, attempt.returnTo)
			response := finish(attempt.mux, attempt.origin, attempt.code, stateCookie, state)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("foreign or uninvited token status = %d, want 401", response.Code)
			}
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == config.SiteSessionCookieName && cookie.Value != "" {
					t.Fatal("foreign or uninvited identity received a site session")
				}
			}
		})
	}

	gameDate := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	lps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/479691" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":7001,"SchedGameDateTime":%q,"home_team":{"team_name":"PUBLIC FC"},"visitor_team":{"team_name":"VISITORS"},"Season":169}]}`, gameDate)
	}))
	t.Cleanup(lps.Close)
	prod.Config.LPSAPIBaseURL = lps.URL
	for _, path := range []string{"/", "/soccer"} {
		response := httptest.NewRecorder()
		prodMux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://craigdevjohnson.com"+path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("anonymous public %s status = %d", path, response.Code)
		}
	}
	for _, request := range []struct {
		path string
		form url.Values
		want string
	}{
		{path: "/soccer/fetch", form: url.Values{"team_codes": {"479691"}}, want: "PUBLIC FC"},
		{path: "/soccer/download", form: url.Values{"team_codes": {"479691"}, "selected": {"7001"}}, want: "BEGIN:VCALENDAR"},
	} {
		req := httptest.NewRequest(http.MethodPost, "https://craigdevjohnson.com"+request.path, strings.NewReader(request.form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		prodMux.ServeHTTP(response, req)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), request.want) {
			t.Fatalf("anonymous %s status = %d, body = %q", request.path, response.Code, response.Body.String())
		}
	}
}
