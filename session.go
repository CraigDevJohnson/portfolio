// Session management: AES-GCM encryption, session cookies, and rate limiting.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

type loginAttempt struct {
	Count       int
	WindowStart time.Time
}

type loginRateLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	window      time.Duration
	attempts    map[string]loginAttempt
	stop        chan struct{}
	closeOnce   sync.Once
}

func newLoginRateLimiter(maxAttempts int, window time.Duration) *loginRateLimiter {
	limiter := &loginRateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		attempts:    make(map[string]loginAttempt),
		stop:        make(chan struct{}),
	}
	go limiter.periodicCleanup()
	return limiter
}

func (limiter *loginRateLimiter) Close() {
	limiter.closeOnce.Do(func() { close(limiter.stop) })
}

func (limiter *loginRateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	attempt := limiter.attempts[key]
	if now.Sub(attempt.WindowStart) > limiter.window {
		attempt = loginAttempt{WindowStart: now}
	}
	if attempt.WindowStart.IsZero() {
		attempt.WindowStart = now
	}
	if attempt.Count >= limiter.maxAttempts {
		return false
	}

	// Enforce upper bound on stored keys to prevent unbounded memory growth.
	// Sweep expired entries first so legitimate requests aren't blocked by stale keys.
	if _, exists := limiter.attempts[key]; !exists && len(limiter.attempts) >= rateLimiterMaxKeys {
		for candidate, a := range limiter.attempts {
			if now.Sub(a.WindowStart) > limiter.window {
				delete(limiter.attempts, candidate)
			}
			if len(limiter.attempts) < rateLimiterMaxKeys {
				break
			}
		}
		if len(limiter.attempts) >= rateLimiterMaxKeys {
			return false
		}
	}

	attempt.Count++
	limiter.attempts[key] = attempt
	return true
}

func (limiter *loginRateLimiter) periodicCleanup() {
	ticker := time.NewTicker(limiter.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			limiter.mu.Lock()
			now := time.Now()
			for key, attempt := range limiter.attempts {
				if now.Sub(attempt.WindowStart) > limiter.window {
					delete(limiter.attempts, key)
				}
			}
			limiter.mu.Unlock()
		case <-limiter.stop:
			return
		}
	}
}

func encryptJSONValue(data any) (string, error) {
	if !loginEnabled() {
		return "", errors.New("session encryption key is not configured")
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(configData.SessionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func decryptJSONValue(value string, out any) error {
	if !loginEnabled() {
		return errors.New("session encryption key is not configured")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(configData.SessionKey)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(decoded) < gcm.NonceSize() {
		return errors.New("invalid session payload")
	}
	nonce := decoded[:gcm.NonceSize()]
	ciphertext := decoded[gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return err
	}
	return nil
}

func encryptSession(data *SessionData) (string, error) {
	return encryptJSONValue(data)
}

func decryptSession(value string) (SessionData, error) {
	var session SessionData
	if err := decryptJSONValue(value, &session); err != nil {
		return session, err
	}
	return session, nil
}

func getSession(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(lpsSessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return nil, errSessionExpired
	}
	return &session, nil
}

func loadSoccerSession(w http.ResponseWriter, r *http.Request) (*SessionData, bool) {
	session, err := getSession(r)
	if errors.Is(err, errSessionExpired) {
		clearSession(w, r)
		return nil, true
	}
	if err != nil {
		log.Printf("soccer session read failed: %v", err)
		clearSession(w, r)
		return nil, true
	}
	return session, false
}

func setSession(w http.ResponseWriter, r *http.Request, session *SessionData) error {
	encrypted, err := encryptSession(session)
	if err != nil {
		return err
	}
	http.SetCookie(w, newSecureCookie(r, lpsSessionCookieName, encrypted, soccerCookiePath, 0, http.SameSiteStrictMode))
	return nil
}

func clearSession(w http.ResponseWriter, r *http.Request) {
	cookie := newSecureCookie(r, lpsSessionCookieName, "", soccerCookiePath, -1, http.SameSiteStrictMode)
	cookie.Expires = time.Unix(0, 0)
	http.SetCookie(w, cookie)
}
