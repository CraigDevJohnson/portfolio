package portal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
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
	client := NewOIDCClient("https://example.auth.us-east-1.amazoncognito.com", "client", "https://app.example/auth/callback", "")
	parsed, err := url.Parse(client.AuthorizationURL("state", "challenge"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"response_type": "code", "scope": "openid email profile", "client_id": "client",
		"redirect_uri": "https://app.example/auth/callback", "state": "state",
		"code_challenge": "challenge", "code_challenge_method": "S256",
	} {
		if query.Get(key) != want {
			t.Errorf("query %s = %q, want %q", key, query.Get(key), want)
		}
	}
}

func TestValidateIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	client := NewOIDCClient("https://issuer.example", "client", "", "")
	client.jwksCache.fetchFn = func(context.Context) ([]jwk, error) {
		return []jwk{{Kid: "test", Kty: "RSA", Alg: "RS256", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})}}, nil
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://issuer.example", "aud": "client", "sub": "sub", "email": "operator@example.com",
		"cognito:username": "operator", "exp": time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = "test"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := client.ValidateIDToken(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "operator@example.com" || claims.Audience != "client" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestValidateIDTokenRejectsIssuerAudienceAndExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	client := NewOIDCClient("https://issuer.example", "client", "", "")
	client.jwksCache.fetchFn = func(context.Context) ([]jwk, error) {
		return []jwk{{Kid: "test", Kty: "RSA", N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()), E: "AQAB"}}, nil
	}
	for name, claims := range map[string]jwt.MapClaims{
		"issuer":   {"iss": "https://wrong", "aud": "client", "exp": time.Now().Add(time.Hour).Unix()},
		"audience": {"iss": "https://issuer.example", "aud": "wrong", "exp": time.Now().Add(time.Hour).Unix()},
		"expired":  {"iss": "https://issuer.example", "aud": "client", "exp": time.Now().Add(-time.Hour).Unix()},
	} {
		t.Run(name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test"
			raw, signErr := token.SignedString(key)
			if signErr != nil {
				t.Fatal(signErr)
			}
			if _, validateErr := client.ValidateIDToken(context.Background(), raw); validateErr == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
