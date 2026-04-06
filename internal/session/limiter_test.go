package session

import (
	"sync"
	"testing"
	"time"
)

func newTestLimiter(maxAttempts int, window time.Duration, maxKeys int) *LoginRateLimiter {
	limiter := NewLoginRateLimiter(maxAttempts, window, maxKeys)
	return limiter
}

func TestAllow_EmptyKeyAlwaysAllowed(t *testing.T) {
	limiter := newTestLimiter(1, time.Minute, 10)
	defer limiter.Close()

	for range 100 {
		if !limiter.Allow("") {
			t.Fatal("empty key must always be allowed")
		}
	}
}

func TestAllow_WithinLimit(t *testing.T) {
	limiter := newTestLimiter(3, time.Minute, 10)
	defer limiter.Close()

	for i := range 3 {
		if !limiter.Allow("user1") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
}

func TestAllow_ExceedsLimit(t *testing.T) {
	limiter := newTestLimiter(3, time.Minute, 10)
	defer limiter.Close()

	for range 3 {
		limiter.Allow("user1")
	}

	if limiter.Allow("user1") {
		t.Fatal("4th attempt should be denied")
	}
}

func TestAllow_IndependentKeys(t *testing.T) {
	limiter := newTestLimiter(2, time.Minute, 10)
	defer limiter.Close()

	for range 2 {
		limiter.Allow("user1")
	}

	if limiter.Allow("user1") {
		t.Fatal("user1 should be rate-limited")
	}
	if !limiter.Allow("user2") {
		t.Fatal("user2 should still be allowed")
	}
}

func TestAllow_WindowResetsAfterExpiry(t *testing.T) {
	window := 50 * time.Millisecond
	limiter := newTestLimiter(2, window, 10)
	defer limiter.Close()

	for range 2 {
		limiter.Allow("key")
	}
	if limiter.Allow("key") {
		t.Fatal("should be denied before window expires")
	}

	time.Sleep(window + 10*time.Millisecond)

	if !limiter.Allow("key") {
		t.Fatal("should be allowed after window expires")
	}
}

func TestAllow_MaxKeysEnforcement(t *testing.T) {
	limiter := newTestLimiter(5, time.Minute, 2)
	defer limiter.Close()

	if !limiter.Allow("a") {
		t.Fatal("first key should be allowed")
	}
	if !limiter.Allow("b") {
		t.Fatal("second key should be allowed")
	}
	if limiter.Allow("c") {
		t.Fatal("third key should be denied when maxKeys=2 and no stale entries")
	}
}

func TestAllow_MaxKeysEvictsStale(t *testing.T) {
	window := 50 * time.Millisecond
	limiter := newTestLimiter(5, window, 2)
	defer limiter.Close()

	limiter.Allow("old")
	limiter.Allow("current")

	time.Sleep(window + 10*time.Millisecond)

	limiter.Allow("current")

	if !limiter.Allow("newcomer") {
		t.Fatal("newcomer should be allowed after stale eviction of 'old'")
	}
}

func TestClose_Idempotent(t *testing.T) {
	limiter := newTestLimiter(5, time.Minute, 10)
	limiter.Close()
	limiter.Close()
}

func TestAllow_ConcurrentAccess(t *testing.T) {
	limiter := newTestLimiter(100, time.Minute, 50)
	defer limiter.Close()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			for range 10 {
				limiter.Allow(key)
			}
		}(string(rune('A' + i)))
	}
	wg.Wait()
}

func TestPeriodicCleanup_RemovesExpired(t *testing.T) {
	window := 50 * time.Millisecond
	limiter := newTestLimiter(5, window, 10)
	defer limiter.Close()

	limiter.Allow("stale")

	time.Sleep(window*2 + 20*time.Millisecond)

	limiter.mu.Lock()
	count := len(limiter.attempts)
	limiter.mu.Unlock()

	if count != 0 {
		t.Fatalf("expected 0 stale entries after cleanup, got %d", count)
	}
}
