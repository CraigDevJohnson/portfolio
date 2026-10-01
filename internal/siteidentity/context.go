// Package siteidentity exposes the request's verified site principal and current page grants.
package siteidentity

import (
	"context"
	"time"
)

// Grant names the two independently authorized private page families.
type Grant string

const (
	GrantSoccer     Grant = "soccer"
	GrantManagement Grant = "management"
)

// Principal identifies a validated Cognito subject in one environment.
type Principal struct {
	Issuer  string
	Subject string
	Email   string
}

type contextKey struct{}

type requestIdentity struct {
	principal Principal
	grants    map[Grant]bool
	returnTo  string
}

// WithRequestIdentity adds a validated principal and freshly evaluated grants to a request context.
func WithRequestIdentity(ctx context.Context, principal *Principal, grants []Grant, returnTo string) context.Context {
	identity := requestIdentity{returnTo: returnTo}
	if principal != nil {
		identity.principal = *principal
		identity.grants = make(map[Grant]bool, len(grants))
		for _, grant := range grants {
			identity.grants[grant] = true
		}
	}
	return context.WithValue(ctx, contextKey{}, identity)
}

type signInAvailabilityKey struct{}

// WithSignInAvailable records whether this environment can start site sign-in.
func WithSignInAvailable(ctx context.Context, available bool) context.Context {
	return context.WithValue(ctx, signInAvailabilityKey{}, available)
}

// SignInAvailable reports whether shared navigation may offer site sign-in.
func SignInAvailable(ctx context.Context) bool {
	available, _ := ctx.Value(signInAvailabilityKey{}).(bool)
	return available
}

type sessionTimesKey struct{}

// sessionTimes says when the request's site session was issued and when this
// browser last signed out explicitly. Either is zero when unknown.
type sessionTimes struct {
	issuedAt    time.Time
	lastSignOut time.Time
}

// WithSessionTimes records when the request's site session was issued and
// when this browser last signed out explicitly. Pass zero for either one the
// request does not carry.
func WithSessionTimes(ctx context.Context, issuedAt, lastSignOut time.Time) context.Context {
	return context.WithValue(ctx, sessionTimesKey{}, sessionTimes{issuedAt: issuedAt, lastSignOut: lastSignOut})
}

// SessionIssuedAt returns when the request's site session was issued, or zero
// when the request has none or its session predates issue times.
func SessionIssuedAt(ctx context.Context) time.Time {
	times, _ := ctx.Value(sessionTimesKey{}).(sessionTimes)
	return times.issuedAt
}

// RevokedBySignOut reports whether browser state authorized by a site session
// issued at authorizedAt predates this browser's latest explicit sign-out, so
// that sign-out ended it. A site-session timeout records no sign-out, so state
// it withholds is never revoked by it. A zero authorizedAt, from a session
// without an issue time, counts as before any recorded sign-out.
func RevokedBySignOut(ctx context.Context, authorizedAt time.Time) bool {
	times, _ := ctx.Value(sessionTimesKey{}).(sessionTimes)
	return !times.lastSignOut.IsZero() && !authorizedAt.After(times.lastSignOut)
}

// PrincipalFromContext returns the verified site principal, if any.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	identity, ok := ctx.Value(contextKey{}).(requestIdentity)
	return identity.principal, ok && identity.principal.Issuer != "" && identity.principal.Subject != ""
}

// HasGrant reports a current request's grant decision.
func HasGrant(ctx context.Context, grant Grant) bool {
	identity, ok := ctx.Value(contextKey{}).(requestIdentity)
	return ok && identity.grants[grant]
}

// HasGrantForOwner checks the current page grant and exact Cognito owner coordinates.
func HasGrantForOwner(ctx context.Context, grant Grant, issuer, subject string) bool {
	if issuer == "" || subject == "" || !HasGrant(ctx, grant) {
		return false
	}
	principal, ok := PrincipalFromContext(ctx)
	return ok && principal.Issuer == issuer && principal.Subject == subject
}

// SoccerPrivateAllowed reports whether this request may use private Soccer
// actions: it carries a verified principal holding the current soccer grant.
// A request without site identity is refused.
func SoccerPrivateAllowed(ctx context.Context) bool {
	return HasGrant(ctx, GrantSoccer)
}

// SoccerOwnerAllowed reports whether owner-bound private Soccer state, such as
// imported LPS access, a Google connection, or pending Google consent, belongs
// to this request's principal and that principal holds the current soccer
// grant. Ownerless state and requests without site identity are refused.
func SoccerOwnerAllowed(ctx context.Context, issuer, subject string) bool {
	return HasGrantForOwner(ctx, GrantSoccer, issuer, subject)
}

// ForeignOwner reports owner-bound private state that this request's site
// identity can never use: the state has no owner, or a different principal is
// signed in. State whose owner is signed out, or signed in without a current
// grant, is withheld rather than foreign, so its owner can use it again.
func ForeignOwner(ctx context.Context, issuer, subject string) bool {
	if issuer == "" || subject == "" {
		return true
	}
	principal, signedIn := PrincipalFromContext(ctx)
	return signedIn && (principal.Issuer != issuer || principal.Subject != subject)
}

// EmailForNavigation returns the current account label for shared navigation.
func EmailForNavigation(ctx context.Context) string {
	principal, ok := PrincipalFromContext(ctx)
	if !ok {
		return ""
	}
	return principal.Email
}

// WithNavigationReturnTo replaces the page shared navigation returns to after
// sign-in, keeping the request's principal and grants. The sign-in landing uses
// its own destination so navigation never nests the landing inside itself.
func WithNavigationReturnTo(ctx context.Context, returnTo string) context.Context {
	identity, _ := ctx.Value(contextKey{}).(requestIdentity)
	identity.returnTo = returnTo
	return context.WithValue(ctx, contextKey{}, identity)
}

// ReturnToForNavigation returns the current local page path.
func ReturnToForNavigation(ctx context.Context) string {
	identity, ok := ctx.Value(contextKey{}).(requestIdentity)
	if !ok || identity.returnTo == "" {
		return "/"
	}
	return identity.returnTo
}
