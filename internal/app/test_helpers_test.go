package app

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	internalsession "portfolio/internal/session"
	"portfolio/internal/siteidentity"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/types"
)

// The invited principal a completed site sign-in leaves in a test browser.
const (
	testSiteIssuer  = "https://issuer.example.com/pool"
	testSiteSubject = "granted-subject"
	testSiteEmail   = "owner@example.com"
)

// enableTestSiteIdentity configures complete site identity on the test app
// with the given reviewed invitation map of emails to page grants.
func enableTestSiteIdentity(app *App, invitations map[string][]string) {
	app.Config.SiteSessionKey = bytes.Repeat([]byte("s"), 32)
	app.Config.SiteCognitoDomain = "https://auth.example.com"
	app.Config.SiteCognitoIssuer = testSiteIssuer
	app.Config.SiteCognitoClientID = "site-client"
	app.Config.SiteCognitoRedirectURI = "https://app.example.com/auth/callback"
	app.Config.SiteCognitoLogoutURI = "https://app.example.com/sign-in"
	app.Config.SiteInvitations = invitations
}

// testSiteSessionCookie returns the site session cookie a completed Cognito
// sign-in sets for the given subject and invited email.
func testSiteSessionCookie(t *testing.T, app *App, subject, email string) *http.Cookie {
	t.Helper()
	value, err := internalsession.EncryptJSONValue(app.Config.SiteSessionKey, map[string]any{
		"principal":  siteidentity.Principal{Issuer: testSiteIssuer, Subject: subject, Email: email},
		"expires_at": time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}
	return &http.Cookie{Name: config.SiteSessionCookieName, Value: value}
}

// signedInSiteCookie enables site identity on the test app, invites the test
// principal with the given page grants in the current configuration, and
// returns the site session cookie a completed Cognito sign-in would set.
func signedInSiteCookie(t *testing.T, app *App, grants ...string) *http.Cookie {
	t.Helper()
	enableTestSiteIdentity(app, map[string][]string{testSiteEmail: append([]string{}, grants...)})
	return testSiteSessionCookie(t, app, testSiteSubject, testSiteEmail)
}

// ownedBySiteVisitor binds imported LPS access to the signed-in test
// principal, as an import through that site session does.
func ownedBySiteVisitor(session *types.SessionData) *types.SessionData {
	session.OwnerIssuer = testSiteIssuer
	session.OwnerSubject = testSiteSubject
	return session
}

// asGrantedSoccerOwner gives a request that reaches a Soccer or Google
// handler directly the site identity the route assembly attaches for the
// signed-in test principal holding the soccer grant.
func asGrantedSoccerOwner(req *http.Request) *http.Request {
	principal := &siteidentity.Principal{Issuer: testSiteIssuer, Subject: testSiteSubject, Email: testSiteEmail}
	ctx := siteidentity.WithRequestIdentity(req.Context(), principal, []siteidentity.Grant{siteidentity.GrantSoccer}, req.URL.Path)
	return req.WithContext(ctx)
}

// grantedSoccerRoutes serves the real route assembly to a browser holding a
// current site session with the soccer grant.
func grantedSoccerRoutes(t *testing.T, app *App) http.Handler {
	t.Helper()
	cookie := signedInSiteCookie(t, app, "soccer")
	mux, _ := buildMux(app, app.Logger, false)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookie)
		mux.ServeHTTP(w, r)
	})
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	rootLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := &App{
		Config: config.Config{
			SessionKey:    []byte("0123456789abcdef0123456789abcdef"),
			LPSAPIBaseURL: config.DefaultLPSAPIBaseURL,
		},
		LPSClient:    &http.Client{Timeout: 5 * time.Second},
		LoginLimiter: internalsession.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys),
		Logger:       rootLogger.With(slog.String("component", "app")),
	}
	app.GoogleHandler = internalgoogle.NewHandler(
		&app.Config,
		app.LPSClient,
		rootLogger.With(slog.String("component", "google")),
		nil,
	)
	app.GoogleHandler.Soccer = newTestSoccerHandler(app)
	withoutLiveGoogle(app)
	t.Cleanup(func() {
		app.LoginLimiter.Close()
	})
	return app
}

// unreachableGoogle is a closed loopback address. Tests point Google at it
// until they attach their own fake, so none can reach live Google.
const unreachableGoogle = "http://127.0.0.1:1"

// withoutLiveGoogle points every Google endpoint of the test app at a closed
// loopback address.
func withoutLiveGoogle(app *App) {
	app.GoogleHandler.OAuthAuthURL = unreachableGoogle + "/oauth/authorize"
	app.GoogleHandler.OAuthTokenURL = unreachableGoogle + "/oauth/token"
	app.GoogleHandler.OAuthUserInfoURL = unreachableGoogle + "/userinfo"
	app.GoogleHandler.CalendarAPIBaseURL = unreachableGoogle + "/calendar/v3"
}

// newTestSoccerHandler returns a handler wired to the test app dependencies.
func newTestSoccerHandler(app *App) *internalsoccer.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil)).With(slog.String("component", "soccer"))
	return internalsoccer.NewHandler(
		&app.Config,
		app.LPSClient,
		app.LoginLimiter,
		app.GoogleHandler,
		internalsoccer.NoopSoccerStore{},
		logger,
	)
}

// encryptTestSession creates an encrypted session cookie payload for tests.
func encryptTestSession(t *testing.T, app *App, session *types.SessionData) string {
	t.Helper()

	encrypted, err := internalsession.EncryptJSONValue(app.Config.SessionKey, session)
	if err != nil {
		t.Fatalf("EncryptJSONValue returned error: %v", err)
	}

	return encrypted
}

// decryptTestSession decodes an encrypted test session cookie payload.
func decryptTestSession(t *testing.T, app *App, value string) types.SessionData {
	t.Helper()

	var session types.SessionData
	err := internalsession.DecryptJSONValue(app.Config.SessionKey, value, &session)
	if err != nil {
		t.Fatalf("DecryptJSONValue returned error: %v", err)
	}

	return session
}

// addSessionCookie attaches an encrypted soccer session cookie and its import
// guard to the request. A session without an owner is bound to the signed-in
// test principal, as an import through that site session binds it; tests of
// legacy ownerless state encrypt the session themselves.
func addSessionCookie(t *testing.T, app *App, req *http.Request, session *types.SessionData) {
	t.Helper()
	owned := *session
	if owned.OwnerIssuer == "" && owned.OwnerSubject == "" {
		ownedBySiteVisitor(&owned)
	}
	for _, cookie := range importedAccessCookies(t, app, &owned) {
		req.AddCookie(cookie)
	}
}

// testImportGuard is the guard a test import shares with its guard cookie.
const testImportGuard = "test-import-guard"

// importedAccessCookies returns the cookies a browser holds after an import:
// the encrypted session and the import guard cookie it must match.
func importedAccessCookies(t *testing.T, app *App, session *types.SessionData) []*http.Cookie {
	t.Helper()
	guarded := *session
	if guarded.ImportGuard == "" {
		guarded.ImportGuard = testImportGuard
	}
	return []*http.Cookie{
		{Name: config.LPSSessionCookieName, Value: encryptTestSession(t, app, &guarded)},
		{Name: config.LPSImportGuardCookieName, Value: guarded.ImportGuard},
	}
}

// findImportGuardCookie returns the import guard cookie from an HTTP response.
func findImportGuardCookie(resp *http.Response) *http.Cookie {
	for _, cookie := range resp.Cookies() {
		if cookie.Name == config.LPSImportGuardCookieName {
			return cookie
		}
	}
	return nil
}

// findSessionCookie returns the soccer session cookie from an HTTP response.
func findSessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == config.LPSSessionCookieName {
			return cookie
		}
	}
	return nil
}

// assertClearedSessionCookie verifies that the session cookie was explicitly cleared.
func assertClearedSessionCookie(t *testing.T, resp *http.Response) {
	t.Helper()
	sessionCookie := findSessionCookie(t, resp)
	if sessionCookie == nil {
		t.Fatal("expected cleared session cookie to be set")
	}
	if sessionCookie.Value != "" {
		t.Fatalf("expected cleared session cookie value to be empty, got %q", sessionCookie.Value)
	}
	if sessionCookie.MaxAge >= 0 {
		t.Fatalf("expected cleared session cookie max-age to be negative, got %d", sessionCookie.MaxAge)
	}
	if !sessionCookie.Expires.Equal(time.Unix(0, 0)) {
		t.Fatalf("expected cleared session cookie expiry to be Unix epoch, got %v", sessionCookie.Expires)
	}
}
