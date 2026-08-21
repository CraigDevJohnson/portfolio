package portal

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ──────────────────────────────────────────────
// PKCE pure-function helpers (task 4.1)
// ──────────────────────────────────────────────

// generateCodeVerifier returns a 32-byte random value encoded as base64url
// without padding, suitable for use as an OAuth 2.0 PKCE code_verifier.
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating code verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// codeChallenge computes the PKCE S256 code_challenge for the given verifier:
//
//	BASE64URL(SHA-256(ASCII(code_verifier)))
//
// No padding characters are included, as required by RFC 7636.
func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState returns 16 random bytes encoded as a lowercase hex string,
// suitable for use as an OAuth 2.0 state nonce.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ──────────────────────────────────────────────
// Token and Claims types (task 4.2)
// ──────────────────────────────────────────────

// TokenResponse is the JSON body returned by the Cognito token endpoint.
type TokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Claims holds the subset of JWT claims the portal cares about.
type Claims struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Username string `json:"cognito:username"`
	Issuer   string `json:"iss"`
	Audience string `json:"aud"`
	Expiry   int64  `json:"exp"`
}

// ──────────────────────────────────────────────
// JWKS cache (task 4.2)
// ──────────────────────────────────────────────

// jwk is a minimal representation of one entry in a JWKS response.
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"` // RSA modulus (base64url)
	E   string `json:"e"` // RSA public exponent (base64url)
}

// jwksResponse is the JSON envelope returned by a JWKS endpoint.
type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

// jwksCache holds cached public keys fetched from a Cognito JWKS endpoint.
// It refreshes at most once per hour.
type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]crypto.PublicKey // kid → *rsa.PublicKey
	fetchedAt time.Time
	ttl       time.Duration
	fetchFn   func(ctx context.Context) ([]jwk, error)
}

// newJWKSCache constructs a jwksCache that fetches keys from jwksURL using
// the default HTTP client.
func newJWKSCache(jwksURL string) *jwksCache {
	c := &jwksCache{
		keys: make(map[string]crypto.PublicKey),
		ttl:  time.Hour,
	}
	c.fetchFn = func(ctx context.Context) ([]jwk, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating JWKS request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching JWKS: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading JWKS response: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", resp.StatusCode)
		}
		var jwksResp jwksResponse
		if err := json.Unmarshal(body, &jwksResp); err != nil {
			return nil, fmt.Errorf("parsing JWKS response: %w", err)
		}
		return jwksResp.Keys, nil
	}
	return c
}

// getKeys returns the current set of public keys, refreshing from the remote
// endpoint if the cache is empty or older than the TTL.
func (c *jwksCache) getKeys(ctx context.Context) (map[string]crypto.PublicKey, error) {
	// Fast path: read lock.
	c.mu.RLock()
	if len(c.keys) > 0 && time.Since(c.fetchedAt) < c.ttl {
		keys := c.keys
		c.mu.RUnlock()
		return keys, nil
	}
	c.mu.RUnlock()

	// Slow path: write lock and re-fetch.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if len(c.keys) > 0 && time.Since(c.fetchedAt) < c.ttl {
		return c.keys, nil
	}

	rawKeys, err := c.fetchFn(ctx)
	if err != nil {
		return nil, err
	}

	newKeys := make(map[string]crypto.PublicKey, len(rawKeys))
	for i := range rawKeys {
		k := &rawKeys[i]
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAPublicKey(k)
		if err != nil {
			// Skip keys we cannot parse rather than failing the whole refresh.
			continue
		}
		newKeys[k.Kid] = pub
	}

	c.keys = newKeys
	c.fetchedAt = time.Now()
	return c.keys, nil
}

// parseRSAPublicKey converts a JWK RSA entry into an *rsa.PublicKey.
func parseRSAPublicKey(k *jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding RSA modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding RSA exponent: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// ──────────────────────────────────────────────
// OIDCClient struct and constructor (task 4.2)
// ──────────────────────────────────────────────

// OIDCClient handles Cognito OAuth 2.0 / OIDC operations.
type OIDCClient struct {
	CognitoDomain string
	ClientID      string
	RedirectURI   string
	LogoutURI     string
	jwksCache     *jwksCache
}

// NewOIDCClient constructs an OIDCClient for the given Cognito configuration.
// The JWKS cache is pre-wired to fetch from {domain}/.well-known/jwks.json.
func NewOIDCClient(domain, clientID, redirectURI, logoutURI string) *OIDCClient {
	jwksURL := strings.TrimRight(domain, "/") + "/.well-known/jwks.json"
	return &OIDCClient{
		CognitoDomain: domain,
		ClientID:      clientID,
		RedirectURI:   redirectURI,
		LogoutURI:     logoutURI,
		jwksCache:     newJWKSCache(jwksURL),
	}
}

// ──────────────────────────────────────────────
// OIDCClient methods (task 4.3)
// ──────────────────────────────────────────────

// AuthorizationURL builds the Cognito Hosted UI authorization endpoint URL
// with all required OAuth 2.0 Authorization Code + PKCE parameters.
func (c *OIDCClient) AuthorizationURL(state, challenge string) string {
	base := strings.TrimRight(c.CognitoDomain, "/") + "/oauth2/authorize"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", c.ClientID)
	params.Set("redirect_uri", c.RedirectURI)
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	return base + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for tokens by POSTing to the
// Cognito token endpoint. It returns a non-nil error for any non-200 response
// or JSON parsing failure.
func (c *OIDCClient) ExchangeCode(ctx context.Context, code, codeVerifier string) (*TokenResponse, error) {
	endpoint := strings.TrimRight(c.CognitoDomain, "/") + "/oauth2/token"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURI)
	form.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	return &tokenResp, nil
}

// ValidateIDToken verifies a raw Cognito ID token JWT. It:
//   - Fetches the JWKS from the cache (refreshing if needed)
//   - Parses the JWT and looks up the signing key by kid
//   - Verifies the RS256 signature
//   - Verifies iss == CognitoDomain
//   - Verifies aud == ClientID
//   - Verifies exp > time.Now().Unix()
//
// Returns the extracted Claims on success.
func (c *OIDCClient) ValidateIDToken(ctx context.Context, rawIDToken string) (*Claims, error) {
	keys, err := c.jwksCache.getKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching JWKS: %w", err)
	}

	keyFunc := func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in JWT header")
		}
		pub, found := keys[kid]
		if !found {
			return nil, fmt.Errorf("no public key found for kid %q", kid)
		}
		return pub, nil
	}

	mapClaims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(rawIDToken, mapClaims, keyFunc,
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithoutClaimsValidation(), // we do manual validation below
	)
	if err != nil {
		return nil, fmt.Errorf("parsing JWT: %w", err)
	}
	if !parsed.Valid {
		return nil, errors.New("invalid JWT token")
	}

	// Re-extract MapClaims from the parsed token (the one we passed in is populated).
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("unexpected claims type")
	}

	// Validate iss.
	iss, _ := mc["iss"].(string)
	if iss != c.CognitoDomain {
		return nil, fmt.Errorf("invalid issuer: got %q, want %q", iss, c.CognitoDomain)
	}

	// Validate aud — Cognito puts aud as a string.
	aud, _ := mc["aud"].(string)
	if aud != c.ClientID {
		return nil, fmt.Errorf("invalid audience: got %q, want %q", aud, c.ClientID)
	}

	// Validate exp.
	var expUnix int64
	switch v := mc["exp"].(type) {
	case float64:
		expUnix = int64(v)
	case json.Number:
		expUnix, _ = v.Int64()
	default:
		return nil, errors.New("missing or invalid exp claim")
	}
	if expUnix <= time.Now().Unix() {
		return nil, errors.New("ID token has expired")
	}

	claims := &Claims{
		Issuer:   iss,
		Audience: aud,
		Expiry:   expUnix,
	}
	if sub, ok := mc["sub"].(string); ok {
		claims.Sub = sub
	}
	if email, ok := mc["email"].(string); ok {
		claims.Email = email
	}
	if username, ok := mc["cognito:username"].(string); ok {
		claims.Username = username
	}

	return claims, nil
}

// LogoutURL returns the Cognito logout endpoint URL with client_id and
// logout_uri parameters. Returns an empty string if LogoutURI is not configured.
func (c *OIDCClient) LogoutURL() string {
	if c.LogoutURI == "" {
		return ""
	}
	base := strings.TrimRight(c.CognitoDomain, "/") + "/logout"
	params := url.Values{}
	params.Set("client_id", c.ClientID)
	params.Set("logout_uri", c.LogoutURI)
	return base + "?" + params.Encode()
}
