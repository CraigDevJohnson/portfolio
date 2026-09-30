package portal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestCodeChallengeRFC7636Vector(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// The RFC vector is ASCII; compute the expected value directly to keep this
	// test independent of the implementation under test.
	hash := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(hash[:])
	if got := codeChallenge(verifier); got != want {
		t.Fatalf("codeChallenge() = %q, want %q", got, want)
	}
}

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
}

func newOIDCFixture(t *testing.T) *oidcFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &oidcFixture{key: key, tokenStatus: http.StatusOK}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pool/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(jwksResponse{Keys: []jwk{{Kid: "test", Kty: "RSA", Alg: "RS256", Use: "sig", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: "AQAB"}}})
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
