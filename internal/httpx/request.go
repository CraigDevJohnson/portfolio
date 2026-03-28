package httpx

import (
	"net"
	"net/http"
	"strings"
)

func ClientIP(request *http.Request) string {
	if ip, ok := ForwardedClientIP(request); ok {
		return ip
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(request.RemoteAddr)
}

func ForwardedClientIP(request *http.Request) (string, bool) {
	remoteIP := RemoteAddrIP(request.RemoteAddr)
	if remoteIP == nil || !IsTrustedProxyIP(remoteIP) {
		return "", false
	}

	if ip := strings.TrimSpace(request.Header.Get("CF-Connecting-IP")); IsValidIP(ip) {
		return ip, true
	}

	for _, candidate := range strings.Split(request.Header.Get("X-Forwarded-For"), ",") {
		candidate = strings.TrimSpace(candidate)
		if IsValidIP(candidate) {
			return candidate, true
		}
	}

	return "", false
}

func RemoteAddrIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	return net.ParseIP(host)
}

func IsTrustedProxyIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func IsValidIP(value string) bool {
	return net.ParseIP(strings.TrimSpace(value)) != nil
}

func RequestIsHTTPS(request *http.Request) bool {
	if request.TLS != nil {
		return true
	}
	if remoteIP := RemoteAddrIP(request.RemoteAddr); remoteIP != nil && IsTrustedProxyIP(remoteIP) {
		return strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Proto")), "https")
	}
	return false
}

func RequestBaseURL(request *http.Request) string {
	scheme := "http"
	if RequestIsHTTPS(request) {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(request.Host)
}

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
