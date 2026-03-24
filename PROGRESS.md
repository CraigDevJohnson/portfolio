# Progress Log

## Completed

- [x] Task-001: Extract constants, config, and helpers
- [x] Task-002: Extract session and cookie management

## Current Iteration

- Iteration: 3
- Working on: Task-003 - Extract portfolio handlers and data
- Started: pending

## Last Completed

- Task-002: Extract session and cookie management
- Tests: ✅ 80/80 passing
- Build: ✅ Compiles cleanly
- Key decisions:
  - `session.go` holds AES-GCM encrypt/decrypt, session CRUD, loginRateLimiter type and all methods, loginAttempt type
  - `cookies.go` holds Google connection/OAuth state cookie CRUD, requestIsHTTPS(), requestBaseURL()
  - Added `newSecureCookie(r, name, value, path, maxAge, sameSite)` helper in cookies.go
  - Accepted SameSite as a parameter (not hardcoded Lax) because session cookies use SameSiteStrictMode while OAuth state uses SameSiteLaxMode
  - Refactored 6 cookie-setting call sites (setSession, clearSession, setGoogleConnectionCookie, clearGoogleConnectionCookie, setGoogleOAuthStateCookie, clearGoogleOAuthStateCookie) to use newSecureCookie()
  - Removed crypto/aes, crypto/cipher, sync imports from main.go (no longer needed there)

## Blockers

- None

## Notes for Next Iteration

- session.go and cookies.go are fully extracted
- `newSecureCookie()` consolidates the repeated cookie struct boilerplate
- `setSession` and `clearSession` in session.go depend on `newSecureCookie` from cookies.go (same package, no issue)
- `setGoogleOAuthStateCookie` in cookies.go depends on `encryptJSONValue` from session.go (same package)
- main.go still has: handlers, portfolio data, soccer logic, Google OAuth/Calendar, LPS client, schedule/ICS code
- Next task: Task-003 (extract portfolio handlers and data)
