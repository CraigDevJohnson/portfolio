package httpx

import (
	"context"
	"net/http"
	"strings"
)

type trustedOriginContextKey struct{}

// TrustedOrigin is the validated request origin supplied by a trusted adapter.
type TrustedOrigin struct {
	Scheme string
	Host   string
}

// WithTrustedOrigin attaches a validated trusted origin to the request context.
func WithTrustedOrigin(request *http.Request, origin TrustedOrigin) *http.Request {
	scheme := strings.ToLower(strings.TrimSpace(origin.Scheme))
	host := strings.TrimSpace(origin.Host)
	if request == nil || (scheme != "http" && scheme != "https") || host == "" {
		return request
	}

	origin = TrustedOrigin{Scheme: scheme, Host: host}
	ctx := context.WithValue(request.Context(), trustedOriginContextKey{}, origin)
	return request.WithContext(ctx)
}

func requestTrustedOrigin(request *http.Request) (TrustedOrigin, bool) {
	if request == nil {
		return TrustedOrigin{}, false
	}

	origin, ok := request.Context().Value(trustedOriginContextKey{}).(TrustedOrigin)
	return origin, ok
}
