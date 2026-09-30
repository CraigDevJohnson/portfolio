package app

import (
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

const (
	devSiteOrigin  = "https://dev.craigdevjohnson.com"
	prodSiteOrigin = "https://craigdevjohnson.com"
	// The invitation JSON the service module renders for each environment's
	// reviewed map (infra/lambda/modules/service tests assert the same text).
	reviewedSiteInvitationsJSON = `{"craigdevjohnson@gmail.com":["management","soccer"]}`
	devSiteSessionKeyHex        = "6464646464646464646464646464646464646464646464646464646464646464"
	prodSiteSessionKeyHex       = "7070707070707070707070707070707070707070707070707070707070707070"
)

// fakeSitePool stands in for one environment's Cognito user pool: its own
// managed-login origin, issuer and public app client.
type fakeSitePool struct {
	server   *httptest.Server
	issuer   string
	clientID string
}

// fakeSiteFederation mints tokens for the development and production pools.
// Both sign with one key, so only each environment's configured issuer and
// client decide whether it accepts a token.
type fakeSiteFederation struct {
	t     *testing.T
	key   *rsa.PrivateKey
	pools map[string]*fakeSitePool
}

func newFakeSiteFederation(t *testing.T) *fakeSiteFederation {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	federation := &fakeSiteFederation{t: t, key: key, pools: map[string]*fakeSitePool{}}
	for environment, pool := range map[string]struct{ id, client string }{
		"dev":  {id: "us-west-2_DevSite", client: "devsiteclient"},
		"prod": {id: "us-west-2_ProdSite", client: "prodsiteclient"},
	} {
		server := httptest.NewTLSServer(federation.poolHandler(environment, pool.id))
		t.Cleanup(server.Close)
		federation.pools[environment] = &fakeSitePool{server: server, issuer: server.URL + "/" + pool.id, clientID: pool.client}
	}
	// httptest TLS servers share one certificate, so either client trusts both pools.
	previousClient := http.DefaultClient
	http.DefaultClient = federation.pools["prod"].server.Client()
	t.Cleanup(func() { http.DefaultClient = previousClient })
	return federation
}

// poolHandler serves one pool's JWKS and token endpoint. An authorization code
// names the pool that issued the identity and the identity's kind, for example
// "dev-invited" or "prod-unverified", so a test can hand one environment
// another environment's token.
func (f *fakeSiteFederation) poolHandler(environment, poolID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + poolID + "/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": "site-test", "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(f.key.N.Bytes()), "e": "AQAB",
			}}})
		case "/oauth2/token":
			if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("code_verifier") == "" || r.Form.Get("client_secret") != "" ||
				r.Form.Get("client_id") != f.pools[environment].clientID {
				f.t.Errorf("%s pool received an invalid token exchange for client %q", environment, r.Form.Get("client_id"))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			issuing, kind, _ := strings.Cut(r.Form.Get("code"), "-")
			pool, ok := f.pools[issuing]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			email, verified := "craigdevjohnson@gmail.com", true
			switch kind {
			case "uninvited":
				email = "uninvited@example.com"
			case "unverified":
				verified = false
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
				"iss": pool.issuer, "aud": pool.clientID, "sub": issuing + "-stable-subject",
				"exp": time.Now().Add(time.Hour).Unix(), "token_use": "id",
				"email": email, "email_verified": verified,
			})
			token.Header["kid"] = "site-test"
			raw, err := token.SignedString(f.key)
			if err != nil {
				f.t.Error(err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": raw})
		default:
			http.NotFound(w, r)
		}
	})
}

// productionLikeSite is one environment assembled from the SITE_* Lambda
// environment the service module renders from that environment's site root.
type productionLikeSite struct {
	app    *App
	mux    http.Handler
	pool   *fakeSitePool
	origin string
}

func loadProductionLikeSite(t *testing.T, pool *fakeSitePool, origin, sessionKeyHex, lpsURL string) *productionLikeSite {
	t.Helper()
	for name, value := range map[string]string{
		"SITE_SESSION_KEY":             sessionKeyHex,
		"SITE_COGNITO_DOMAIN":          pool.server.URL,
		"SITE_COGNITO_ISSUER":          pool.issuer,
		"SITE_COGNITO_CLIENT_ID":       pool.clientID,
		"SITE_COGNITO_REDIRECT_URI":    origin + "/auth/callback",
		"SITE_COGNITO_LOGOUT_URI":      origin + "/sign-in",
		"SITE_INVITATIONS_JSON":        reviewedSiteInvitationsJSON,
		"SITE_ALLOW_LOCAL_CALLBACK":    "false",
		"LPS_API_BASE_URL":             lpsURL,
		"MGMT_SESSION_KEY":             "",
		"CLIENT_ID_KEY":                "",
		"CLIENT_SECRET_KEY":            "",
		"GOOGLE_CONNECTION_TABLE_NAME": "",
		"SOCCER_SESSION_TABLE_NAME":    "",
	} {
		t.Setenv(name, value)
	}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg := config.Load()
	slog.SetDefault(previousLogger)
	if !cfg.SiteEnabled() {
		t.Fatalf("the %s SITE_* environment did not enable site sign-in", origin)
	}
	application := New(&cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(application.LoginLimiter.Close)
	if application.PortalHandler != nil {
		t.Fatal("site sign-in unexpectedly depends on management AWS clients")
	}
	mux, _ := buildMux(application, application.Logger, false)
	return &productionLikeSite{app: application, mux: mux, pool: pool, origin: origin}
}

// startSignIn begins sign-in and checks it uses this environment's own
// Google-federated client and callback.
func (s *productionLikeSite) startSignIn(t *testing.T) (*http.Cookie, string) {
	t.Helper()
	form := url.Values{"return_to": {"/soccer"}}
	request := httptest.NewRequest(http.MethodPost, s.origin+"/sign-in", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil || response.Code != http.StatusSeeOther ||
		"https://"+target.Host != s.pool.server.URL || target.Path != "/oauth2/authorize" ||
		target.Query().Get("client_id") != s.pool.clientID ||
		target.Query().Get("redirect_uri") != s.origin+"/auth/callback" ||
		target.Query().Get("identity_provider") != "Google" {
		t.Fatalf("%s sign-in did not use its own Google-federated pool, client and callback: %d %s", s.origin, response.Code, target)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.SiteOAuthStateCookieName {
			return cookie, target.Query().Get("state")
		}
	}
	t.Fatal("sign-in did not set a pending OAuth state")
	return nil, ""
}

func (s *productionLikeSite) callback(t *testing.T, code string) *httptest.ResponseRecorder {
	t.Helper()
	stateCookie, state := s.startSignIn(t)
	request := httptest.NewRequest(http.MethodGet, s.origin+"/auth/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil)
	request.AddCookie(stateCookie)
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	return response
}

// restricted reports the status a restricted action gated by grant returns
// behind this environment's shared site identity.
func (s *productionLikeSite) restricted(t *testing.T, cookie *http.Cookie, grant siteidentity.Grant) int {
	t.Helper()
	action := s.app.SiteHandler.WithIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	request := httptest.NewRequest(http.MethodPost, s.origin+"/restricted", nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	action.ServeHTTP(response, request)
	return response.Code
}

func assertNoSiteSession(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.SiteSessionCookieName && cookie.Value != "" {
			t.Fatal("identity received a site session")
		}
	}
}

func publicTeamLPS(t *testing.T) *httptest.Server {
	t.Helper()
	gameDate := testutil.MislabelledLPSZuluTime(time.Now().Add(24 * time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/teams/479691" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"games":[{"UGameID":7001,"SchedGameDateTime":%q,"home_team":{"team_name":"PUBLIC FC"},"visitor_team":{"team_name":"VISITORS"},"Season":169}]}`, gameDate)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProductionLikeSiteIdentitySeparatesEnvironmentsAndKeepsPublicRoutes(t *testing.T) {
	federation := newFakeSiteFederation(t)
	lps := publicTeamLPS(t)
	dev := loadProductionLikeSite(t, federation.pools["dev"], devSiteOrigin, devSiteSessionKeyHex, lps.URL)
	prod := loadProductionLikeSite(t, federation.pools["prod"], prodSiteOrigin, prodSiteSessionKeyHex, lps.URL)

	signedIn := map[string]*http.Cookie{}
	for name, site := range map[string]*productionLikeSite{"dev": dev, "prod": prod} {
		response := site.callback(t, name+"-invited")
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/soccer" {
			t.Fatalf("%s invited sign-in failed: %d %s", name, response.Code, response.Body.String())
		}
		signedIn[name] = siteCookie(t, response)
		if got := site.restricted(t, signedIn[name], siteidentity.GrantManagement); got != http.StatusNoContent {
			t.Fatalf("%s invited identity lacks its current management grant: %d", name, got)
		}
	}

	t.Run("only an invited verified identity signs in to production", func(t *testing.T) {
		for _, code := range []string{"prod-uninvited", "prod-unverified"} {
			response := prod.callback(t, code)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s status = %d, want 401", code, response.Code)
			}
			assertNoSiteSession(t, response)
		}
	})

	t.Run("one environment's token cannot sign in to the other", func(t *testing.T) {
		for _, attempt := range []struct {
			site *productionLikeSite
			code string
		}{{prod, "dev-invited"}, {dev, "prod-invited"}} {
			response := attempt.site.callback(t, attempt.code)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("%s at %s status = %d, want 401", attempt.code, attempt.site.origin, response.Code)
			}
			assertNoSiteSession(t, response)
		}
	})

	t.Run("one environment's session cannot authorize the other's restricted action", func(t *testing.T) {
		if got := prod.restricted(t, signedIn["dev"], siteidentity.GrantManagement); got != http.StatusUnauthorized {
			t.Fatalf("development session in production = %d, want 401", got)
		}
		if got := dev.restricted(t, signedIn["prod"], siteidentity.GrantManagement); got != http.StatusUnauthorized {
			t.Fatalf("production session in development = %d, want 401", got)
		}
		// The issuer boundary holds even if an operator reused one session key.
		sharedKey := loadProductionLikeSite(t, federation.pools["prod"], prodSiteOrigin, devSiteSessionKeyHex, lps.URL)
		if got := sharedKey.restricted(t, signedIn["dev"], siteidentity.GrantManagement); got != http.StatusUnauthorized {
			t.Fatalf("development session in production with a shared key = %d, want 401", got)
		}
	})

	t.Run("production checks its own current grant map on every request", func(t *testing.T) {
		const owner = "craigdevjohnson@gmail.com"
		prod.app.Config.SiteInvitations[owner] = []string{"soccer"}
		if got := prod.restricted(t, signedIn["prod"], siteidentity.GrantManagement); got != http.StatusForbidden {
			t.Fatalf("revoked production management grant = %d, want 403", got)
		}
		if got := prod.restricted(t, signedIn["prod"], siteidentity.GrantSoccer); got != http.StatusNoContent {
			t.Fatalf("remaining production soccer grant = %d, want 204", got)
		}
		if got := dev.restricted(t, signedIn["dev"], siteidentity.GrantManagement); got != http.StatusNoContent {
			t.Fatalf("production grant change reached development: %d", got)
		}
		delete(prod.app.Config.SiteInvitations, owner)
		if got := prod.restricted(t, signedIn["prod"], siteidentity.GrantSoccer); got != http.StatusUnauthorized {
			t.Fatalf("revoked production invitation = %d, want 401", got)
		}
	})

	t.Run("public portfolio and anonymous Team ID and ICS stay available in production", func(t *testing.T) {
		for _, path := range []string{"/", "/about", "/experience", "/skills", "/projects", "/education", "/contact", "/soccer"} {
			response := httptest.NewRecorder()
			prod.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, prodSiteOrigin+path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("anonymous %s status = %d", path, response.Code)
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
			req := httptest.NewRequest(http.MethodPost, prodSiteOrigin+request.path, strings.NewReader(request.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			prod.mux.ServeHTTP(response, req)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), request.want) {
				t.Fatalf("anonymous %s status = %d, body = %q", request.path, response.Code, response.Body.String())
			}
		}
	})
}

// Production also maps www.craigdevjohnson.com to the same API, but Cognito
// returns only to the apex callback and site cookies are host-only.
const prodSiteWWWOrigin = "https://www.craigdevjohnson.com"

func TestProductionSiteSignInMovesTheWWWAliasToTheCallbackHost(t *testing.T) {
	federation := newFakeSiteFederation(t)
	prod := loadProductionLikeSite(t, federation.pools["prod"], prodSiteOrigin, prodSiteSessionKeyHex, publicTeamLPS(t).URL)

	for _, request := range []struct{ method, path string }{
		{method: http.MethodGet, path: "/soccer?team_codes=479691"},
		{method: http.MethodGet, path: "/sign-in?return_to=%2Fsoccer"},
		{method: http.MethodPost, path: "/sign-in"},
		{method: http.MethodGet, path: "/auth/callback?code=prod-invited&state=pending"},
		{method: http.MethodPost, path: "/sign-out"},
		// A path that looks like another host still stays on the apex.
		{method: http.MethodGet, path: "//evil.example/steal"},
	} {
		body := strings.NewReader(url.Values{"return_to": {"/soccer"}}.Encode())
		req := httptest.NewRequest(request.method, prodSiteWWWOrigin+request.path, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		prod.mux.ServeHTTP(response, req)
		// 308 keeps the method and form, so a www sign-in resumes on the apex.
		if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != prodSiteOrigin+request.path {
			t.Fatalf("www %s %s = %d %q, want 308 to the apex", request.method, request.path, response.Code, response.Header().Get("Location"))
		}
		if cookies := response.Result().Cookies(); len(cookies) != 0 {
			t.Fatalf("www %s %s set host-only cookies the apex callback cannot read: %v", request.method, request.path, cookies)
		}
	}

	// Following the redirect completes the journey on the one callback host.
	response := prod.callback(t, "prod-invited")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/soccer" {
		t.Fatalf("apex sign-in after the www redirect failed: %d %s", response.Code, response.Body.String())
	}
	if got := prod.restricted(t, siteCookie(t, response), siteidentity.GrantSoccer); got != http.StatusNoContent {
		t.Fatalf("apex session after the www redirect = %d, want 204", got)
	}
}

func TestWWWAliasKeepsServingWhileSiteSignInIsOff(t *testing.T) {
	application := newTestApp(t)
	mux, _ := buildMux(application, application.Logger, false)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, prodSiteWWWOrigin+"/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("www home without site sign-in = %d, want 200", response.Code)
	}
}
