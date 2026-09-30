// Package siteidentity exposes the request's verified site principal and current page grants.
package siteidentity

import "context"

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
