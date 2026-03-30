package session

import (
	"sync"
	"time"
)

type LoginAttempt struct {
	Count       int
	WindowStart time.Time
}

type LoginRateLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	maxKeys     int
	window      time.Duration
	attempts    map[string]LoginAttempt
	stop        chan struct{}
	closeOnce   sync.Once
}

// NewLoginRateLimiter tracks login attempts per key within a fixed time window.
func NewLoginRateLimiter(maxAttempts int, window time.Duration, maxKeys int) *LoginRateLimiter {
	limiter := &LoginRateLimiter{
		maxAttempts: maxAttempts,
		maxKeys:     maxKeys,
		window:      window,
		attempts:    make(map[string]LoginAttempt),
		stop:        make(chan struct{}),
	}
	go limiter.periodicCleanup()
	return limiter
}

func (limiter *LoginRateLimiter) Close() {
	limiter.closeOnce.Do(func() { close(limiter.stop) })
}

// Allow reports whether the provided key may make another login attempt.
// Empty keys bypass rate limiting.
func (limiter *LoginRateLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	now := time.Now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	attempt := limiter.attempts[key]
	if now.Sub(attempt.WindowStart) > limiter.window {
		attempt = LoginAttempt{WindowStart: now}
	}
	if attempt.Count >= limiter.maxAttempts {
		return false
	}

	if _, exists := limiter.attempts[key]; !exists && len(limiter.attempts) >= limiter.maxKeys {
		for candidate, saved := range limiter.attempts {
			if now.Sub(saved.WindowStart) > limiter.window {
				delete(limiter.attempts, candidate)
			}
			if len(limiter.attempts) < limiter.maxKeys {
				break
			}
		}
		if len(limiter.attempts) >= limiter.maxKeys {
			return false
		}
	}

	attempt.Count++
	limiter.attempts[key] = attempt
	return true
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
