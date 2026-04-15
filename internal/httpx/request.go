package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the best available client IP for the request.
func ClientIP(request *http.Request) string {
	if ip, ok := forwardedClientIP(request); ok {
		return ip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(request.RemoteAddr)
}

func forwardedClientIP(request *http.Request) (string, bool) {
	remoteIP := remoteAddrIP(request.RemoteAddr)
	if remoteIP == nil || !isTrustedProxyIP(remoteIP) {
		return "", false
	}

	// Prefer Cloudflare's single-IP header before falling back to X-Forwarded-For.
	if ip := strings.TrimSpace(request.Header.Get("CF-Connecting-IP")); isValidIP(ip) {
		return ip, true
	}

	for _, candidate := range strings.Split(request.Header.Get("X-Forwarded-For"), ",") {
		candidate = strings.TrimSpace(candidate)
		if isValidIP(candidate) {
			return candidate, true
		}
	}

	return "", false
}

func remoteAddrIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	return net.ParseIP(host)
}

func isTrustedProxyIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func isValidIP(value string) bool {
	return net.ParseIP(strings.TrimSpace(value)) != nil
}

// RequestIsHTTPS reports whether the request reached the app over HTTPS.
func RequestIsHTTPS(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	if remoteIP := remoteAddrIP(request.RemoteAddr); remoteIP != nil && isTrustedProxyIP(remoteIP) {
		return strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}

// RequestBaseURL returns the request origin using the detected request scheme.
func RequestBaseURL(request *http.Request) string {
	scheme := "http"
	if RequestIsHTTPS(request) {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(request.Host)
}

// NewSecureCookie builds an HTTP-only cookie using the request security context.
func NewSecureCookie(request *http.Request, name, value, path string, maxAge int, sameSite http.SameSite) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   RequestIsHTTPS(request),
		SameSite: sameSite,
	}
}
