package google

import "errors"

// ErrOAuthStateExpired indicates the OAuth state cookie has expired.
var ErrOAuthStateExpired = errors.New("oauth state expired")
