package app

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"portfolio/internal/config"
	internalsession "portfolio/internal/session"
	"portfolio/types"
)

func TestEncryptDecryptSessionRoundTrip(t *testing.T) {
	app := newTestApp(t)

	expected := types.SessionData{
		JWT:      "token-value",
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		},
		ExpiresAt: time.Unix(1_773_634_161, 0).UTC(),
	}

	encrypted := encryptTestSession(t, app, &expected)
	actual := decryptTestSession(t, app, encrypted)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("decryptSession mismatch: got %#v want %#v", actual, expected)
	}
}

func TestRateLimiterRejectsAtMaxKeys(t *testing.T) {
	limiter := internalsession.NewLoginRateLimiter(100, time.Hour, config.RateLimiterMaxKeys)
	defer limiter.Close()

	// Fill up to the max key limit
	for i := 0; i < config.RateLimiterMaxKeys; i++ {
		key := fmt.Sprintf("ip-%d", i)
		if !limiter.Allow(key) {
			t.Fatalf("expected Allow to return true for key %d", i)
		}
	}

	// The next new key should be rejected
	if limiter.Allow("overflow-ip") {
		t.Fatal("expected Allow to return false when max keys exceeded")
	}

	// An existing key should still work
	if !limiter.Allow("ip-0") {
		t.Fatal("expected Allow to return true for existing key")
	}
}

func TestRateLimiterExpiredKeysEvictedAtCapacity(t *testing.T) {
	limiter := internalsession.NewLoginRateLimiter(100, 50*time.Millisecond, config.RateLimiterMaxKeys)
	defer limiter.Close()

	// Fill to capacity
	for i := 0; i < config.RateLimiterMaxKeys; i++ {
		limiter.Allow(fmt.Sprintf("ip-%d", i))
	}

	// Wait for entries to expire
	time.Sleep(100 * time.Millisecond)

	// New key should succeed because expired entries are swept on demand
	if !limiter.Allow("fresh-ip") {
		t.Fatal("expected Allow to return true after expired entries are swept")
	}
}

func TestRateLimiterCloseIsIdempotent(t *testing.T) {
	limiter := internalsession.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys)
	limiter.Close()
	limiter.Close() // must not panic
}
