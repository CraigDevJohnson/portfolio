package portal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuthorizationURL(t *testing.T) {
	client := NewOIDCClient("https://example.auth.us-east-1.amazoncognito.com", "https://issuer.example/pool", "client", "https://app.example/callback", "")
	parsed, err := url.Parse(client.AuthorizationURL("state", "challenge"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"response_type": "code", "scope": "openid email profile", "client_id": "client",
		"redirect_uri": "https://app.example/callback", "state": "state",
		"code_challenge": "challenge", "code_challenge_method": "S256", "identity_provider": "Google",
	} {
		if query.Get(key) != want {
			t.Errorf("query %s = %q, want %q", key, query.Get(key), want)
		}
	}
}

type oidcFixture struct {
	client      *OIDCClient
	key         *rsa.PrivateKey
	issuer      string
	code        string
	verifier    string
	tokenStatus int
	tokenBody   string

	// The issuer's JWKS: the key set it publishes, the raw body and status it
	// answers with instead when jwksBody is set, and how many fetches it served.
	jwksMu      sync.Mutex
	published   []jwk
	jwksStatus  int
	jwksBody    string
	jwksFetches int
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	fixture := &oidcFixture{tokenStatus: http.StatusOK}
	fixture.key = fixture.publishKey(t, "test")
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pool/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		fixture.jwksMu.Lock()
		defer fixture.jwksMu.Unlock()
		fixture.jwksFetches++
		if fixture.jwksBody != "" {
			w.WriteHeader(fixture.jwksStatus)
			_, _ = w.Write([]byte(fixture.jwksBody))
			return
		}
		_ = json.NewEncoder(w).Encode(jwksResponse{Keys: fixture.published})
	}))
	t.Cleanup(issuer.Close)
	fixture.issuer = issuer.URL + "/pool"
	hostedUI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth2/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for field, expected := range map[string]string{"grant_type": "authorization_code", "client_id": "client", "redirect_uri": "https://app.example/callback", "code": fixture.code, "code_verifier": fixture.verifier} {
			if r.Form.Get(field) != expected {
				t.Errorf("token request %s differs from expected value", field)
			}
		}
		if r.Form.Get("client_secret") != "" {
			t.Error("public PKCE client sent a secret")
		}
		w.WriteHeader(fixture.tokenStatus)
		_, _ = w.Write([]byte(fixture.tokenBody))
	}))
	t.Cleanup(hostedUI.Close)
	fixture.client = NewOIDCClient(hostedUI.URL, fixture.issuer, "client", "https://app.example/callback", "https://app.example/login")
	return fixture
}

// publishKey adds a new RSA key under kid to the issuer's JWKS, alongside the
// keys already there, and returns its private half for signing.
func (f *oidcFixture) publishKey(t *testing.T, kid string) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.jwksMu.Lock()
	defer f.jwksMu.Unlock()
	f.published = append(f.published, jwk{Kid: kid, Kty: "RSA", Alg: "RS256", Use: "sig", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: "AQAB"})
	return key
}

// answerJWKS makes the issuer answer JWKS fetches with status and body instead
// of its key set; an empty body restores the key set.
func (f *oidcFixture) answerJWKS(status int, body string) {
	f.jwksMu.Lock()
	defer f.jwksMu.Unlock()
	f.jwksStatus, f.jwksBody = status, body
}

func (f *oidcFixture) fetches() int {
	f.jwksMu.Lock()
	defer f.jwksMu.Unlock()
	return f.jwksFetches
}

// validate signs the fixture's claims with key under kid and validates them.
func (f *oidcFixture) validate(t *testing.T, key *rsa.PrivateKey, kid string) error {
	t.Helper()
	_, err := f.client.ValidateIDToken(context.Background(), signTestToken(t, f.claims(), key, jwt.SigningMethodRS256, kid))
	return err
}

func (f *oidcFixture) claims() jwt.MapClaims {
	return jwt.MapClaims{"iss": f.issuer, "aud": "client", "sub": "google-subject", "exp": time.Now().Add(time.Hour).Unix(), "token_use": "id", "email": "craigdevjohnson@gmail.com", "email_verified": true}
}

func signTestToken(t *testing.T, claims jwt.MapClaims, key *rsa.PrivateKey, method jwt.SigningMethod, kid string) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestValidateIDTokenSignedClaims(t *testing.T) {
	fixture := newOIDCFixture(t)
	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(jwt.MapClaims)
		key    *rsa.PrivateKey
		method jwt.SigningMethod
		kid    string
		valid  bool
	}{
		{name: "valid", valid: true},
		{name: "missing expiry", change: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "string expiry", change: func(c jwt.MapClaims) { c["exp"] = "9999999999" }},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{name: "wrong issuer", change: func(c jwt.MapClaims) { c["iss"] = "https://wrong.example/pool" }},
		{name: "wrong audience", change: func(c jwt.MapClaims) { c["aud"] = "wrong-client" }},
		{name: "wrong token use", change: func(c jwt.MapClaims) { c["token_use"] = "access" }},
		{name: "missing token use", change: func(c jwt.MapClaims) { delete(c, "token_use") }},
		{name: "missing subject", change: func(c jwt.MapClaims) { delete(c, "sub") }},
		{name: "blank subject", change: func(c jwt.MapClaims) { c["sub"] = " " }},
		{name: "wrong key", key: wrongKey},
		{name: "wrong algorithm", method: jwt.SigningMethodRS512},
		{name: "unknown kid", kid: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := fixture.claims()
			if tc.change != nil {
				tc.change(claims)
			}
			key := tc.key
			if key == nil {
				key = fixture.key
			}
			method := tc.method
			if method == nil {
				method = jwt.SigningMethodRS256
			}
			kid := tc.kid
			if kid == "" {
				kid = "test"
			}
			raw := signTestToken(t, claims, key, method, kid)
			got, validateErr := fixture.client.ValidateIDToken(context.Background(), raw)
			if tc.valid {
				if validateErr != nil {
					t.Fatal(validateErr)
				}
				if got.Sub != "google-subject" || got.Email != "craigdevjohnson@gmail.com" {
					t.Fatal("missing validated identity")
				}
			} else if validateErr == nil {
				t.Fatal("invalid signed claims accepted")
			}
		})
	}
}

func TestExchangeCodeDoesNotExposeResponseBody(t *testing.T) {
	fixture := newOIDCFixture(t)
	fixture.tokenStatus = http.StatusBadRequest
	fixture.tokenBody = "sentinel-response-body"
	fixture.code, fixture.verifier = "sentinel-code", "sentinel-verifier"
	_, err := fixture.client.ExchangeCode(context.Background(), fixture.code, fixture.verifier)
	if err == nil || strings.Contains(err.Error(), fixture.tokenBody) || strings.Contains(err.Error(), fixture.code) {
		t.Fatalf("unsafe or missing token exchange error: %v", err)
	}
}

// Cognito publishes a new signing key before it signs with it. A process that
// cached the earlier key set must accept the new key at once, not after the
// hour-long cache lifetime.
func TestValidateIDTokenRefetchesJWKSForRotatedKey(t *testing.T) {
	fixture := newOIDCFixture(t)
	if err := fixture.validate(t, fixture.key, "test"); err != nil {
		t.Fatal(err)
	}
	rotated := fixture.publishKey(t, "rotated")
	if err := fixture.validate(t, rotated, "rotated"); err != nil {
		t.Fatalf("token signed by the rotated key: %v", err)
	}
	if err := fixture.validate(t, fixture.key, "test"); err != nil {
		t.Fatalf("token signed by the earlier key after rotation: %v", err)
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches = %d, want the first fill and one refetch for the rotated key", got)
	}
}

// fakeClock stands in for the JWKS cache's clock so tests can cross its
// refetch interval without waiting.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (f *oidcFixture) useFakeClock() *fakeClock {
	clock := &fakeClock{now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	f.client.jwksCache.now = clock.Now
	return clock
}

// Tokens naming keys the issuer never published must not make every request
// fetch the JWKS.
func TestValidateIDTokenRefetchesJWKSForUnknownKidAtMostOncePerInterval(t *testing.T) {
	fixture := newOIDCFixture(t)
	clock := fixture.useFakeClock()
	if err := fixture.validate(t, fixture.key, "test"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.validate(t, fixture.key, "unknown-1"); err == nil {
		t.Fatal("token naming an unpublished kid accepted")
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches after the first unknown kid = %d, want 2", got)
	}
	if err := fixture.validate(t, fixture.key, "unknown-2"); err == nil {
		t.Fatal("second token naming an unpublished kid accepted")
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches after a second unknown kid within the interval = %d, want still 2", got)
	}

	// A key rotated inside the interval waits for it to pass.
	rotated := fixture.publishKey(t, "rotated")
	clock.Advance(jwksRefreshInterval - time.Second)
	if err := fixture.validate(t, rotated, "rotated"); err == nil || fixture.fetches() != 2 {
		t.Fatalf("rotated key within the interval: err %v after %d fetches, want rejection without a fetch", err, fixture.fetches())
	}
	clock.Advance(time.Second)
	if err := fixture.validate(t, rotated, "rotated"); err != nil {
		t.Fatalf("rotated key once the interval passed: %v", err)
	}
	if got := fixture.fetches(); got != 3 {
		t.Fatalf("JWKS fetches once the interval passed = %d, want 3", got)
	}
}

// An issuer outage during a forced refetch must not discard the keys that
// still verify current sessions' tokens.
func TestValidateIDTokenKeepsCachedKeysWhenJWKSRefetchFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "server error", status: http.StatusInternalServerError, body: "unavailable"},
		{name: "malformed body", status: http.StatusOK, body: "{"},
		{name: "no usable keys", status: http.StatusOK, body: `{"keys":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newOIDCFixture(t)
			fixture.useFakeClock()
			if err := fixture.validate(t, fixture.key, "test"); err != nil {
				t.Fatal(err)
			}
			rotated := fixture.publishKey(t, "rotated")
			fixture.answerJWKS(tc.status, tc.body)
			if err := fixture.validate(t, rotated, "rotated"); err == nil {
				t.Fatal("token naming a kid the failed refetch could not supply was accepted")
			}
			if err := fixture.validate(t, fixture.key, "test"); err != nil {
				t.Fatalf("token signed by a cached key after the failed refetch: %v", err)
			}
			if err := fixture.validate(t, rotated, "rotated"); err == nil {
				t.Fatal("unknown kid accepted within the interval after a failed refetch")
			}
			if got := fixture.fetches(); got != 2 {
				t.Fatalf("JWKS fetches = %d, want the first fill and one failed refetch", got)
			}
		})
	}
}

// A forced refetch starts the interval for every request, so a request whose
// own context ends (an aborted callback, an invocation deadline) must not cut
// that refetch short and leave later sign-ins rejected until the interval
// passes, though the issuer was never asked.
func TestValidateIDTokenRefetchesJWKSDespiteCanceledRequest(t *testing.T) {
	fixture := newOIDCFixture(t)
	clock := fixture.useFakeClock()
	if err := fixture.validate(t, fixture.key, "test"); err != nil {
		t.Fatal(err)
	}
	rotated := fixture.publishKey(t, "rotated")
	raw := signTestToken(t, fixture.claims(), rotated, jwt.SigningMethodRS256, "rotated")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.client.ValidateIDToken(ctx, raw); err != nil {
		t.Fatalf("token signed by the rotated key under a canceled request: %v", err)
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches = %d, want the first fill and one refetch the canceled request did not abort", got)
	}
	clock.Advance(jwksRefreshInterval / 2)
	if err := fixture.validate(t, rotated, "rotated"); err != nil {
		t.Fatalf("later request within the interval: %v", err)
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches after the later request = %d, want still 2", got)
	}
}

// Sign-ins that arrive together after a rotation share one refetch, and none
// is turned away by the interval that refetch started.
func TestValidateIDTokenConcurrentRotatedTokensShareOneRefetch(t *testing.T) {
	fixture := newOIDCFixture(t)
	if err := fixture.validate(t, fixture.key, "test"); err != nil {
		t.Fatal(err)
	}
	rotated := fixture.publishKey(t, "rotated")
	raw := signTestToken(t, fixture.claims(), rotated, jwt.SigningMethodRS256, "rotated")
	const requests = 16
	start := make(chan struct{})
	errs := make(chan error, requests)
	for range requests {
		go func() {
			<-start
			_, err := fixture.client.ValidateIDToken(context.Background(), raw)
			errs <- err
		}()
	}
	close(start)
	for range requests {
		if err := <-errs; err != nil {
			t.Errorf("concurrent token signed by the rotated key: %v", err)
		}
	}
	if got := fixture.fetches(); got != 2 {
		t.Fatalf("JWKS fetches = %d, want the first fill and one shared refetch", got)
	}
}
