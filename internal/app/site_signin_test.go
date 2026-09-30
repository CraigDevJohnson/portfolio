package app

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
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
	"portfolio/internal/portal"
	"portfolio/internal/session"
	"portfolio/internal/siteidentity"
)

func newSiteSignInTestApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Config{
		LPSAPIBaseURL:          config.DefaultLPSAPIBaseURL,
		SiteSessionKey:         bytes.Repeat([]byte("a"), 32),
		SiteCognitoDomain:      "https://auth.example.com",
		SiteCognitoIssuer:      "https://issuer.example.com/pool",
		SiteCognitoClientID:    "site-client",
		SiteCognitoRedirectURI: "https://app.example.com/auth/callback",
		SiteCognitoLogoutURI:   "https://app.example.com/sign-in",
		SiteInvitations: map[string][]string{
			"owner@example.com": {"soccer", "management"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(&cfg, logger)
	t.Cleanup(application.LoginLimiter.Close)
	return application
}

func TestSiteSignInEntryKeepsPublicPageAndReturnDestination(t *testing.T) {
	application := newSiteSignInTestApp(t)
	mux, _ := buildMux(application, application.Logger, false)

	page := httptest.NewRecorder()
	mux.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("public About status = %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), `href="/sign-in?return_to=%2Fabout"`) {
		t.Fatal("shared navigation did not offer sign-in with the current page as return destination")
	}

	landing := httptest.NewRecorder()
	mux.ServeHTTP(landing, httptest.NewRequest(http.MethodGet, "https://app.example.com/sign-in?return_to=%2Fabout", nil))
	if landing.Code != http.StatusOK || !strings.Contains(landing.Body.String(), `action="/sign-in"`) || !strings.Contains(landing.Body.String(), `value="/about"`) {
		t.Fatalf("sign-in landing did not preserve a safe return destination: status %d", landing.Code)
	}
}

type fakeSiteCognito struct {
	issuer        string
	domain        string
	email         string
	emailVerified bool
	subject       string
	expiry        time.Time
}

func newFakeSiteCognito(t *testing.T) *fakeSiteCognito {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &fakeSiteCognito{email: "owner@example.com", emailVerified: true, subject: "stable-subject", expiry: time.Now().Add(time.Hour)}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pool/.well-known/jwks.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": "test", "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB",
			}}})
		case "/oauth2/token":
			if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("code") != "test-code" || r.Form.Get("code_verifier") == "" || r.Form.Get("client_id") != "site-client" || r.Form.Get("client_secret") != "" {
				t.Errorf("invalid Cognito token request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			claims := jwt.MapClaims{
				"iss": fixture.issuer, "aud": "site-client", "sub": fixture.subject,
				"exp": fixture.expiry.Unix(), "token_use": "id", "email": fixture.email,
				"email_verified": fixture.emailVerified,
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test"
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
	t.Cleanup(server.Close)
	oldClient := http.DefaultClient
	http.DefaultClient = server.Client()
	t.Cleanup(func() { http.DefaultClient = oldClient })
	fixture.domain = server.URL
	fixture.issuer = server.URL + "/pool"
	return fixture
}

func (f *fakeSiteCognito) app(t *testing.T) *App {
	t.Helper()
	application := newSiteSignInTestApp(t)
	application.Config.SiteCognitoDomain = f.domain
	application.Config.SiteCognitoIssuer = f.issuer
	application.SiteHandler.OIDC = portalOIDCForSiteTest(&application.Config)
	return application
}

func portalOIDCForSiteTest(cfg *config.Config) *portal.OIDCClient {
	return portal.NewOIDCClient(cfg.SiteCognitoDomain, cfg.SiteCognitoIssuer, cfg.SiteCognitoClientID, cfg.SiteCognitoRedirectURI, cfg.SiteCognitoLogoutURI)
}

func beginSiteSignIn(t *testing.T, handler http.Handler, returnTo string) (*http.Cookie, string) {
	t.Helper()
	form := url.Values{"return_to": {returnTo}}
	request := httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-in", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign-in start status = %d; body = %q", response.Code, response.Body.String())
	}
	target, err := url.Parse(response.Header().Get("Location"))
	if err != nil || target.Path != "/oauth2/authorize" || target.Query().Get("identity_provider") != "Google" || target.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("sign-in did not use Cognito Google and PKCE: %s", target)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.SiteOAuthStateCookieName {
			return cookie, target.Query().Get("state")
		}
	}
	t.Fatal("sign-in start did not set OAuth state")
	return nil, ""
}

func completeSiteSignIn(t *testing.T, handler http.Handler, stateCookie *http.Cookie, state string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "https://app.example.com/auth/callback?code=test-code&state="+url.QueryEscape(state), nil)
	request.AddCookie(stateCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func siteCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.SiteSessionCookieName {
			return cookie
		}
	}
	t.Fatal("site session cookie missing")
	return nil
}

func TestInvitedSiteJourneyUsesCurrentGrantsAndSignsOut(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	if application.PortalHandler != nil {
		t.Fatal("site identity test unexpectedly constructed management clients")
	}
	mux, _ := buildMux(application, application.Logger, false)
	stateCookie, state := beginSiteSignIn(t, mux, "/soccer?output=ics")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/soccer?output=ics" {
		t.Fatalf("callback did not return to the starting local page: %d %s", callback.Code, callback.Header().Get("Location"))
	}
	accountCookie := siteCookie(t, callback)
	if accountCookie.Value == "" || !accountCookie.HttpOnly || !accountCookie.Secure {
		t.Fatal("site session cookie is missing or unprotected")
	}

	pageRequest := httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil)
	pageRequest.AddCookie(accountCookie)
	page := httptest.NewRecorder()
	mux.ServeHTTP(page, pageRequest)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "owner@example.com") || !strings.Contains(page.Body.String(), `action="/sign-out"`) {
		t.Fatalf("shared navigation did not show the active account: %d", page.Code)
	}

	grantProbe := application.SiteHandler.WithIdentity(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal, ok := siteidentity.PrincipalFromContext(r.Context()); !ok || principal.Issuer != fixture.issuer || principal.Subject != "stable-subject" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !siteidentity.HasGrant(r.Context(), siteidentity.GrantSoccer) || !siteidentity.HasGrant(r.Context(), siteidentity.GrantManagement) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	probeRequest := httptest.NewRequest(http.MethodGet, "https://app.example.com/private", nil)
	probeRequest.AddCookie(accountCookie)
	probe := httptest.NewRecorder()
	grantProbe.ServeHTTP(probe, probeRequest)
	if probe.Code != http.StatusNoContent {
		t.Fatalf("verified site identity and initial grants unavailable: %d", probe.Code)
	}

	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer"}
	probe = httptest.NewRecorder()
	grantProbe.ServeHTTP(probe, probeRequest)
	if probe.Code != http.StatusForbidden {
		t.Fatalf("grant change did not apply on the next request: %d", probe.Code)
	}
	delete(application.Config.SiteInvitations, "owner@example.com")
	probe = httptest.NewRecorder()
	grantProbe.ServeHTTP(probe, probeRequest)
	if probe.Code != http.StatusUnauthorized {
		t.Fatalf("revoked invitation retained site identity: %d", probe.Code)
	}
	if cleared := siteCookie(t, probe); cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatal("revoked invitation left a reusable browser session")
	}
	application.Config.SiteInvitations["owner@example.com"] = []string{"soccer", "management"}

	signOutRequest := httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)
	signOutRequest.AddCookie(accountCookie)
	signOut := httptest.NewRecorder()
	mux.ServeHTTP(signOut, signOutRequest)
	target, err := url.Parse(signOut.Header().Get("Location"))
	if err != nil || target.Host != strings.TrimPrefix(fixture.domain, "https://") || target.Path != "/logout" || target.Query().Get("logout_uri") != application.Config.SiteCognitoLogoutURI {
		t.Fatalf("sign-out did not end managed login: %d %s", signOut.Code, target)
	}
	if cleared := siteCookie(t, signOut); cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatal("sign-out did not clear site session")
	}
	probe = httptest.NewRecorder()
	grantProbe.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "https://app.example.com/private", nil))
	if probe.Code != http.StatusUnauthorized {
		t.Fatal("signed-out browser reached a restricted action")
	}
	landing := httptest.NewRecorder()
	mux.ServeHTTP(landing, httptest.NewRequest(http.MethodGet, application.Config.SiteCognitoLogoutURI, nil))
	if landing.Code != http.StatusOK || !strings.Contains(landing.Body.String(), "Sign in with Google") {
		t.Fatal("managed logout landing restarted authentication or hid sign-in")
	}
}

func TestSiteSignInDeniesUninvitedAndUnsafeReturn(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	for _, unsafe := range []string{"https://elsewhere.example/private", "//elsewhere.example/private", "/%2Felsewhere.example/private"} {
		t.Run(unsafe, func(t *testing.T) {
			stateCookie, state := beginSiteSignIn(t, mux, unsafe)
			callback := completeSiteSignIn(t, mux, stateCookie, state)
			if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/" {
				t.Fatalf("unsafe return was accepted: %d %q", callback.Code, callback.Header().Get("Location"))
			}
		})
	}
	fixture.email = "uninvited@example.com"
	stateCookie, state := beginSiteSignIn(t, mux, "/about")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	if callback.Code != http.StatusUnauthorized || !strings.Contains(callback.Body.String(), "Sign-in could not be completed.") {
		t.Fatalf("uninvited identity did not receive a clear denial: %d", callback.Code)
	}
	if sessionCookie := siteCookie(t, callback); sessionCookie.Value != "" || sessionCookie.MaxAge >= 0 {
		t.Fatal("uninvited identity received a site session")
	}
	fixture.email = "owner@example.com"
	fixture.emailVerified = false
	stateCookie, state = beginSiteSignIn(t, mux, "/about")
	callback = completeSiteSignIn(t, mux, stateCookie, state)
	if callback.Code != http.StatusUnauthorized || siteCookie(t, callback).MaxAge >= 0 {
		t.Fatal("unverified invited email received a site session")
	}
	publicPage := httptest.NewRecorder()
	mux.ServeHTTP(publicPage, httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil))
	if publicPage.Code != http.StatusOK {
		t.Fatal("public portfolio page unavailable after sign-in denial")
	}
}

func TestSiteSessionExpiryAndOtherEnvironmentAreDenied(t *testing.T) {
	fixture := newFakeSiteCognito(t)
	application := fixture.app(t)
	mux, _ := buildMux(application, application.Logger, false)
	claims := map[string]any{
		"principal":  map[string]string{"Issuer": fixture.issuer, "Subject": "stable-subject", "Email": "owner@example.com"},
		"expires_at": time.Now().Add(time.Hour),
	}
	valid, err := session.EncryptJSONValue(application.Config.SiteSessionKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	validRequest := httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil)
	validRequest.AddCookie(&http.Cookie{Name: config.SiteSessionCookieName, Value: valid})
	validResponse := httptest.NewRecorder()
	mux.ServeHTTP(validResponse, validRequest)
	if !strings.Contains(validResponse.Body.String(), "owner@example.com") {
		t.Fatal("valid session fixture did not establish a site account")
	}
	claims["expires_at"] = time.Now().Add(-time.Minute)
	expired, err := session.EncryptJSONValue(application.Config.SiteSessionKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil)
	request.AddCookie(&http.Cookie{Name: config.SiteSessionCookieName, Value: expired})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `href="/sign-in?return_to=%2Fabout"`) {
		t.Fatal("expired site session affected public page or retained account state")
	}

	stateCookie, state := beginSiteSignIn(t, mux, "/about")
	callback := completeSiteSignIn(t, mux, stateCookie, state)
	validCookie := siteCookie(t, callback)
	other := fixture.app(t)
	other.Config.SiteCognitoIssuer = fixture.domain + "/other-pool"
	other.SiteHandler.OIDC = portalOIDCForSiteTest(&other.Config)
	otherMux, _ := buildMux(other, other.Logger, false)
	otherRequest := httptest.NewRequest(http.MethodGet, "https://app.example.com/about", nil)
	otherRequest.AddCookie(validCookie)
	otherResponse := httptest.NewRecorder()
	otherMux.ServeHTTP(otherResponse, otherRequest)
	if otherResponse.Code != http.StatusOK || strings.Contains(otherResponse.Body.String(), "owner@example.com") {
		t.Fatal("one environment's site session authorized another environment")
	}
}

func TestSiteSignInHeadRendersLandingWithoutStartingSignIn(t *testing.T) {
	application := newSiteSignInTestApp(t)
	mux, _ := buildMux(application, application.Logger, false)

	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "https://app.example.com/sign-in?return_to=%2Fabout", nil))
	if response.Code != http.StatusOK || response.Header().Get("Location") != "" {
		t.Fatalf("HEAD sign-in started a redirect: %d %q", response.Code, response.Header().Get("Location"))
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == config.SiteOAuthStateCookieName {
			t.Fatal("HEAD sign-in replaced pending OAuth state")
		}
	}
}
