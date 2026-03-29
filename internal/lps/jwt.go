package lps

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"portfolio/internal/config"
)

// JWTExpiry returns the exp claim from a JWT, or the zero time when it is unavailable.
func JWTExpiry(token string) time.Time {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}

	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}
	}

	return time.Unix(claims.Exp, 0)
}

// NormalizeImportedJWT trims an imported bearer token, validates its shape, and rejects expired tokens.
func NormalizeImportedJWT(raw string) (string, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return "", errors.New("Paste the bearer JWT from your Let's Play Soccer browser session.")
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return "", errors.New("Paste a single JWT value without extra spaces or line breaks.")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
	}
	for _, segment := range parts[:2] {
		if segment == "" {
			return "", errors.New("The imported value must be a JWT with three dot-separated sections.")
		}
		if _, err := base64.RawURLEncoding.DecodeString(segment); err != nil {
			return "", errors.New("The imported JWT format is not valid base64url data.")
		}
	}

	expiresAt := JWTExpiry(token)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return "", errors.New("This JWT has expired. Copy a fresh bearer token from letsplaysoccer.com and import it again.")
	}

	return token, nil
}

// ImportedSessionExpiry clamps the session lifetime to the earlier of the JWT expiry or the default session TTL.
func ImportedSessionExpiry(token string) time.Time {
	deadline := time.Now().Add(config.DefaultSessionTTL)
	expiresAt := JWTExpiry(token)
	if expiresAt.IsZero() || expiresAt.After(deadline) {
		return deadline
	}
	return expiresAt
}
