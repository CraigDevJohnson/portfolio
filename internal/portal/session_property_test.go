package portal

// Feature: ec2-management-portal, Property 3: Session validity is strictly determined by username and expiry

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestSessionValidity_Property3 verifies that IsValid() returns true if and only if
// the session has a non-empty username AND an ExpiresAt that is strictly in the future.
// All four combinations of (empty/non-empty username) × (past/future expiry) are exercised.
//
// Validates: Requirements 4.8
func TestSessionValidity_Property3(t *testing.T) {
	t.Run("empty username and past expiry is always invalid", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			// Generate a duration between 1 second and 365 days in the past.
			pastOffset := rapid.Int64Range(1, int64(365*24*time.Hour)).Draw(rt, "past_offset_ns")
			session := &PortalSession{
				Username:  "",
				ExpiresAt: time.Now().Add(-time.Duration(pastOffset)),
			}
			if session.IsValid() {
				rt.Fatalf("expected IsValid()=false for empty username + past expiry, got true: %+v", session)
			}
		})
	})

	t.Run("empty username and future expiry is always invalid", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			// Generate a duration between 1 second and 365 days in the future.
			futureOffset := rapid.Int64Range(int64(time.Second), int64(365*24*time.Hour)).Draw(rt, "future_offset_ns")
			session := &PortalSession{
				Username:  "",
				ExpiresAt: time.Now().Add(time.Duration(futureOffset)),
			}
			if session.IsValid() {
				rt.Fatalf("expected IsValid()=false for empty username + future expiry, got true: %+v", session)
			}
		})
	})

	t.Run("non-empty username and past expiry is always invalid", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			// Draw a non-empty username string (1–64 characters).
			username := rapid.StringN(1, 64, -1).Draw(rt, "username")
			// Generate a duration between 1 second and 365 days in the past.
			pastOffset := rapid.Int64Range(1, int64(365*24*time.Hour)).Draw(rt, "past_offset_ns")
			session := &PortalSession{
				Username:  username,
				ExpiresAt: time.Now().Add(-time.Duration(pastOffset)),
			}
			if session.IsValid() {
				rt.Fatalf("expected IsValid()=false for non-empty username + past expiry, got true: %+v", session)
			}
		})
	})

	t.Run("non-empty username and future expiry is always valid", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			// Draw a non-empty username string (1–64 characters).
			username := rapid.StringN(1, 64, -1).Draw(rt, "username")
			// Generate a duration between 1 second and 365 days in the future.
			futureOffset := rapid.Int64Range(int64(time.Second), int64(365*24*time.Hour)).Draw(rt, "future_offset_ns")
			session := &PortalSession{
				Username:  username,
				ExpiresAt: time.Now().Add(time.Duration(futureOffset)),
			}
			if !session.IsValid() {
				rt.Fatalf("expected IsValid()=true for non-empty username + future expiry, got false: %+v", session)
			}
		})
	})

	t.Run("nil session is always invalid", func(t *testing.T) {
		var session *PortalSession
		if session.IsValid() {
			t.Fatal("expected IsValid()=false for nil session, got true")
		}
	})
}
