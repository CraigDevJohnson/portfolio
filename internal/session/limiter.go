package session

import (
	"sync"
	"time"
)

type loginAttempt struct {
	Count       int
	WindowStart time.Time
}

// LoginRateLimiter tracks login attempts per key within a fixed time window.
type LoginRateLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	maxKeys     int
	window      time.Duration
	attempts    map[string]loginAttempt
	stop        chan struct{}
	closeOnce   sync.Once
}

// NewLoginRateLimiter tracks login attempts per key within a fixed time window.
func NewLoginRateLimiter(maxAttempts int, window time.Duration, maxKeys int) *LoginRateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	if maxKeys <= 0 {
		maxKeys = 1
	}

	limiter := &LoginRateLimiter{
		maxAttempts: maxAttempts,
		maxKeys:     maxKeys,
		window:      window,
		attempts:    make(map[string]loginAttempt),
		stop:        make(chan struct{}),
	}
	go limiter.periodicCleanup()
	return limiter
}

// Close stops the background cleanup goroutine.
func (limiter *LoginRateLimiter) Close() {
	limiter.closeOnce.Do(func() { close(limiter.stop) })
}

// Allow reports whether the provided key may make another login attempt.
// Empty keys bypass rate limiting for internal fallback paths.
func (limiter *LoginRateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	attempt := limiter.attempts[key]
	if limiter.isWindowExpired(attempt, now) {
		attempt = loginAttempt{WindowStart: now}
	}
	if attempt.Count >= limiter.maxAttempts {
		return false
	}

	_, exists := limiter.attempts[key]
	isNewKey := !exists
	isAtCapacity := len(limiter.attempts) >= limiter.maxKeys
	if isNewKey && isAtCapacity && !limiter.ensureCapacityForNewKey(now) {
		return false
	}

	attempt.Count++
	limiter.attempts[key] = attempt
	return true
}

func (limiter *LoginRateLimiter) isWindowExpired(attempt loginAttempt, now time.Time) bool {
	return now.Sub(attempt.WindowStart) > limiter.window
}

func (limiter *LoginRateLimiter) ensureCapacityForNewKey(now time.Time) bool {
	for candidate, attempt := range limiter.attempts {
		if limiter.isWindowExpired(attempt, now) {
			delete(limiter.attempts, candidate)
		}

		if len(limiter.attempts) < limiter.maxKeys {
			break
		}
	}

	return len(limiter.attempts) < limiter.maxKeys
}

func (limiter *LoginRateLimiter) periodicCleanup() {
	ticker := time.NewTicker(limiter.window)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			limiter.mu.Lock()
			now := time.Now()
			for key, attempt := range limiter.attempts {
				if limiter.isWindowExpired(attempt, now) {
					delete(limiter.attempts, key)
				}
			}
			limiter.mu.Unlock()
		case <-limiter.stop:
			return
		}
	}
}
