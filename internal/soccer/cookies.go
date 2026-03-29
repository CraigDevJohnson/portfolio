package soccer

import (
	"net/http"

	"portfolio/internal/httpx"
)

func newSecureCookie(r *http.Request, name, value, path string, maxAge int, sameSite http.SameSite) *http.Cookie {
	return httpx.NewSecureCookie(r, name, value, path, maxAge, sameSite)
}