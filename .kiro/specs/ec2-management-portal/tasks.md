# Implementation Plan: EC2 Management Portal

## Overview

Implement a protected administration portal (`/login`, `/auth/callback`, and `/mgmt/*`) on top of the existing Go + Templ + HTMX portfolio app. Authentication is fully delegated to AWS Cognito via the OAuth 2.0 Authorization Code + PKCE flow — the application never sees operator passwords. The portal reuses `internal/session` AES-256-GCM cookie encryption and `httpx` helpers, modeled directly after `internal/soccer`. All AWS calls go through thin client interfaces for testability. The portal is entirely opt-in: routes are only registered when `cfg.PortalEnabled()` is true.

## Tasks

- [x] 1. Extend config and add portal constants
  - [x] 1.1 Add portal constants to `internal/config/constants.go`
    - Add `PortalSessionCookieName = "mgmt_session"`, `PortalCookiePath = "/"`, `PortalSessionTTL = 12 * time.Hour`, `DefaultPortalAWSRegion = "us-east-1"`, `PortalOAuthStateCookieName = "mgmt_oauth_state"`, `PortalOAuthStateCookieTTL = 10 * time.Minute`
    - _Requirements: 10.1, 1.3, 4.6_

  - [x] 1.2 Extend `internal/config/config.go` with portal fields and loading logic
    - Add fields to `Config`: `PortalSessionKey []byte`, `PortalCognitoDomain string`, `PortalCognitoClientID string`, `PortalCognitoRedirectURI string`, `PortalCognitoLogoutURI string`, `PortalAWSRegion string`, `portalEnabled bool`
    - Add `PortalEnabled() bool` method — returns true only when `PortalSessionKey` is valid AND `PortalCognitoDomain` is non-empty AND `PortalCognitoClientID` is non-empty
    - Extend `config.Load()`: read `MGMT_SESSION_KEY` (absent/empty → disable silently; invalid 64-char hex → disable + WARN); read `MGMT_COGNITO_DOMAIN` and `MGMT_COGNITO_CLIENT_ID` (either absent while key is valid → disable + WARN); read `MGMT_COGNITO_REDIRECT_URI`, `MGMT_COGNITO_LOGOUT_URI` (optional); read `MGMT_AWS_REGION` defaulting to `"us-east-1"`; emit single INFO log: `portal_enabled`, `aws_region`
    - _Requirements: 4.6, 10.1, 10.2, 10.3, 10.5, 10.6, 10.7_

  - [x] 1.3 Write property tests for config loading
    - **Property 12: Portal config errors never break non-portal routes** — for any combination of invalid portal env vars, `GET /` returns HTTP 200; use `rapid.Custom` for config variations
    - **Property 13: Invalid MGMT_SESSION_KEY always disables portal with warning** — for any `rapid.String` set as `MGMT_SESSION_KEY` that is not 64-char lowercase hex, `PortalEnabled = false` and a WARN log entry is emitted
    - **Validates: Requirements 10.4, 10.5**

  - [x] 1.4 Write unit tests for config edge cases
    - Test: absent key → portal disabled, no warning
    - Test: 64-char hex key + Cognito domain + client ID → portal enabled
    - Test: invalid hex key → portal disabled + warning logged
    - Test: valid key but missing `MGMT_COGNITO_DOMAIN` → portal disabled + warning
    - Test: valid key but missing `MGMT_COGNITO_CLIENT_ID` → portal disabled + warning
    - Test: missing `MGMT_AWS_REGION` → defaults to `"us-east-1"`
    - _Requirements: 10.3, 10.5, 10.6, 10.7_

- [x] 2. Create `internal/portal` package skeleton
  - [x] 2.1 Create `internal/portal/handler.go` — Handler struct and constructor
    - Define `Handler` struct: `Config *config.Config`, `OIDC *OIDCClient`, `EC2 EC2ClientIface`, `CloudWatch CloudWatchClientIface`, `Logs CloudWatchLogsClientIface`, `Logger *slog.Logger`
    - Implement `NewHandler(cfg, oidc, ec2, cw, logs, logger)` constructor following soccer handler pattern
    - Add private `setHTMLContentType` helper
    - _Requirements: 10.2_

  - [x] 2.2 Create `internal/portal/ec2.go` — AWS client interfaces and view models
    - Define `EC2ClientIface` interface: `DescribeInstances`, `StartInstances`, `StopInstances`
    - Define `CloudWatchClientIface` interface: `GetMetricStatistics`
    - Define `CloudWatchLogsClientIface` interface: `FilterLogEvents`
    - Define view models: `InstanceSummary{ID, Name, State, InstanceType, AZ}`, `MetricPoint{Timestamp, CPUPercent}`, `LogEvent{Timestamp, Message}`
    - Compile `instanceIDRegex = regexp.MustCompile(`^i-[0-9a-f]{8,17}$`)` at package init
    - _Requirements: 5.1, 5.2, 6.6, 7.1, 7.5, 8.1, 8.6_

  - [x] 2.3 Create `internal/portal/session.go` — PortalSession and OAuthState types and helpers
    - Define `PortalSession{Username string, ExpiresAt time.Time}`
    - Implement `IsValid() bool`: returns true iff `Username != ""` AND `time.Now().Before(ExpiresAt)`
    - Define `OAuthState{State string, CodeVerifier string}`
    - Implement `Handler.setSession`, `Handler.loadSession`, `Handler.clearSession` using `session.EncryptJSONValue`/`DecryptJSONValue` and `httpx.NewSecureCookie` with `config.PortalSessionCookieName` and `config.PortalCookiePath`
    - Implement `Handler.setOAuthState`, `Handler.loadOAuthState`, `Handler.clearOAuthState` for the `mgmt_oauth_state` cookie with `PortalOAuthStateCookieTTL`
    - _Requirements: 1.3, 2.1, 2.4, 4.6, 4.8_

  - [x] 2.4 Write property tests for session validity
    - **Property 3: Session validity is strictly determined by username and expiry** — test all four combos of (empty/non-empty username) × (past/future expiry); `IsValid()` must return true iff both conditions hold
    - **Validates: Requirements 4.8**

  - [x] 2.5 Write unit tests for session helpers
    - Test `setSession` → `loadSession` round-trip returns equal `PortalSession`
    - Test `clearSession` sets Max-Age to -1 and Expires to epoch
    - Test `loadSession` with no cookie returns `nil, nil`
    - Test `loadSession` with corrupted ciphertext returns error
    - Test `setOAuthState` → `loadOAuthState` round-trip preserves `State` and `CodeVerifier`
    - Test `clearOAuthState` sets Max-Age to -1
    - _Requirements: 1.3, 2.1, 4.4_

- [x] 3. Checkpoint — ensure `task build` passes with package skeleton
  - Ensure all tests pass, ask the user if questions arise.

- [x] 4. Create `internal/portal/oidc.go` — OIDC client and PKCE helpers
  - [x] 4.1 Implement PKCE pure-function helpers
    - Run `go get github.com/golang-jwt/jwt/v5` to add the JWT dependency before writing any code that imports it
    - Implement `generateCodeVerifier() (string, error)`: 32 random bytes → base64url (no padding)
    - Implement `codeChallenge(verifier string) string`: SHA-256 → base64url (no padding); must satisfy RFC 7636 Appendix B test vector
    - Implement `generateState() (string, error)`: 16 random bytes → hex string
    - _Requirements: 1.1_

  - [x] 4.2 Implement `OIDCClient` struct, `jwksCache`, and token types
    - Define `OIDCClient{CognitoDomain, ClientID, RedirectURI, LogoutURI string, jwksCache *jwksCache}`
    - Implement `NewOIDCClient(domain, clientID, redirectURI, logoutURI string) *OIDCClient`
    - Define `TokenResponse{IDToken, AccessToken, RefreshToken string, ExpiresIn int}`
    - Define `Claims{Sub, Email, Username, Issuer, Audience string, Expiry int64}` (mapping `cognito:username` and `aud` JWT fields)
    - Implement `jwksCache{mu, keys, fetchedAt, ttl, fetchFn}` with 1-hour TTL; cache hit skips re-fetch; cache miss or expired triggers `fetchFn`
    - _Requirements: 2.6_

  - [x] 4.3 Implement `OIDCClient` methods
    - Implement `AuthorizationURL(state, codeChallenge string) string`: builds Cognito authorization endpoint URL with `response_type=code`, `scope=openid email profile`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`
    - Implement `ExchangeCode(ctx, code, codeVerifier string) (*TokenResponse, error)`: POST to Cognito token endpoint with `grant_type=authorization_code`; parse JSON response
    - Implement `ValidateIDToken(ctx, rawIDToken string) (*Claims, error)`: fetch JWKS (via cache), verify signature using `github.com/golang-jwt/jwt/v5`, verify `iss` matches expected Cognito issuer URL, verify `aud` matches `ClientID`, verify `exp > time.Now().Unix()`; return `*Claims`
    - Implement `LogoutURL() string`: Cognito logout endpoint with `client_id` and `logout_uri` params
    - _Requirements: 1.1, 2.2, 2.3, 3.1_

  - [x] 4.4 Write property tests for PKCE and token validation
    - **Property 6: PKCE code challenge is correctly derived** — for any `rapid.StringN` code verifier, `codeChallenge(v)` must equal `base64url(SHA-256(v))` with no padding; also verify RFC 7636 Appendix B known-good test vector
    - **Property 14: Expired ID tokens are always rejected** — for any `rapid.Custom` generating past `exp` values, `ValidateIDToken` returns an error regardless of whether the signature is valid; use a test RSA key pair, no real Cognito needed
    - **Validates: Requirements 1.1, 2.3**

  - [x] 4.5 Write unit tests for OIDCClient
    - Test: `AuthorizationURL` contains all required OAuth params (`response_type`, `scope`, `client_id`, `redirect_uri`, `state`, `code_challenge`, `code_challenge_method=S256`)
    - Test: `ValidateIDToken` with valid token + test RSA key pair → returns correct `Claims`
    - Test: `ValidateIDToken` with expired token → returns error
    - Test: `ValidateIDToken` with wrong `iss` → returns error
    - Test: `ValidateIDToken` with wrong `aud` → returns error
    - Test: `ValidateIDToken` with bad signature → returns error
    - Test JWKS cache: cache hit within TTL skips re-fetch; expired cache triggers re-fetch
    - _Requirements: 2.3, 2.6_

- [x] 5. Create `cmd/web/pages/portal_error.templ` — OAuth error page
  - [x] 5.1 Implement `PortalError(props ErrorPageProps)` component
    - No nav/footer; dark-first Tailwind theme consistent with existing portfolio pages
    - `ErrorPageProps` carries: `StatusCode int`, `Message string`
    - Render the status code and message with a "Try again" link pointing to `/login`
    - Run `task generate` after creating the file
    - _Requirements: 2.5, 9.1_

  - [x] 5.2 Write unit tests for error page rendering
    - Test: renders status code and message in output
    - Test: "Try again" link has `href="/login"`
    - _Requirements: 2.5_

- [x] 6. Implement auth handlers and middleware
  - [x] 6.1 Create `internal/portal/auth.go` — OAuth handlers and auth middleware
    - Implement `LoginPageHandler`: `GET /login` — if session valid redirect 302 `/mgmt`; else call `generateCodeVerifier`, `codeChallenge`, `generateState`; store `OAuthState` in `mgmt_oauth_state` cookie; redirect to `OIDC.AuthorizationURL(state, challenge)`; return 503 if Cognito config missing
    - Implement `CallbackHandler`: `GET /auth/callback` — read `state` query param and `mgmt_oauth_state` cookie; if cookie absent or `state` mismatches → clear cookie, return 400 with `portal_error.templ`; call `OIDC.ExchangeCode` with `code` and `code_verifier` from cookie; on failure → 401 with error page + ERROR log; call `OIDC.ValidateIDToken`; on failure → 401 with error page + ERROR log; extract `email` (fallback to `cognito:username`); set session cookie (failure → 500 + ERROR log); clear `mgmt_oauth_state` cookie; redirect 302 `/mgmt`
    - Implement `LogoutHandler`: `POST /logout` — clear session cookie; if `OIDC.LogoutURL()` non-empty redirect there; else redirect 302 `/login`; if no valid session just redirect 302 `/login`
    - Implement `requireAuth(next http.HandlerFunc) http.HandlerFunc`: decrypt session; if absent/invalid/expired → clear cookie + redirect 302 `/login` (WARN log on decrypt failure); if valid → inject username into context under typed key `portalUsernameKey` and call `next`
    - Define typed context key `type contextKey string` and `const portalUsernameKey contextKey = "portal_username"`
    - Implement `UsernameFromContext(ctx context.Context) (string, bool)` exported helper
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 2.3, 2.4, 2.5, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8_

  - [x] 6.2 Write property tests for auth middleware and callback handler
    - **Property 1: Invalid session always blocked** — for any invalid session (expired, empty username, bad ciphertext, absent cookie), `requireAuth` responds 302 to `/login` and never calls the downstream handler; use `rapid.Custom` to generate invalid sessions
    - **Property 2: Valid session always passes through with correct username** — for any `PortalSession` with `Username != ""` and `ExpiresAt` in the future, downstream handler is called and `UsernameFromContext` returns the session username; use `rapid.StringN` for username
    - **Property 7: OAuth state always validated before code exchange** — for any `rapid.String` mismatched state value (including absent cookie, tampered state, replayed state), `CallbackHandler` returns 400 and does not call `OIDCClient.ExchangeCode`
    - **Validates: Requirements 2.1, 4.2, 4.3, 4.4, 4.5, 4.8**

  - [x] 6.3 Write unit tests for auth edge cases
    - Test: `GET /login` with valid session redirects to `/mgmt`
    - Test: `GET /login` without session sets `mgmt_oauth_state` cookie and redirects to Cognito URL
    - Test: `GET /login` with Cognito config missing returns 503
    - Test: `CallbackHandler` valid state + successful exchange + valid JWT → session cookie set + 302 to `/mgmt` + `mgmt_oauth_state` cleared
    - Test: `CallbackHandler` missing `mgmt_oauth_state` cookie → 400 + no `ExchangeCode` call
    - Test: `CallbackHandler` state mismatch → 400 + no `ExchangeCode` call
    - Test: `CallbackHandler` `ExchangeCode` failure → 401 + ERROR logged
    - Test: `CallbackHandler` JWT validation failure → 401 + ERROR logged
    - Test: `POST /logout` with valid session → session cleared + redirect to Cognito logout URL
    - Test: `POST /logout` without logout URI configured → redirect to `/login`
    - Test: `POST /logout` without session → 302 to `/login`
    - _Requirements: 1.2, 1.4, 2.1, 2.2, 2.4, 2.5, 3.1, 3.2, 3.3_

- [x] 7. Checkpoint — ensure `task build` and `task test` pass
  - Ensure all tests pass, ask the user if questions arise.

- [x] 8. Create management dashboard Templ components
  - [x] 8.1 Create `cmd/web/pages/portal_mgmt.templ` — management dashboard page
    - Implement `PortalDashboard(props DashboardProps)` using Templ layout, dark-first Tailwind styling consistent with existing pages
    - `DashboardProps` carries: `Username string`, `Instances []portal.InstanceSummary`, `RetrievalError string`
    - Display `Username` in header; provide logout `<form method="POST" action="/logout">` button in header
    - Render instance table with columns: ID, Name, State, Type, AZ, Actions (start/stop/restart), Metrics, Logs
    - When `RetrievalError` non-empty, render alert in place of table
    - When `Instances` empty and no error, render "No instances found" message
    - Run `task generate` after creating the file
    - _Requirements: 5.2, 5.3, 5.4, 9.1, 9.4, 9.5_

  - [x] 8.2 Create `cmd/web/partials/portal_fragments.templ` — HTMX fragment components
    - `InstanceRow(instance portal.InstanceSummary)`: renders a single table row with `hx-post` on start/stop/restart buttons targeting `/mgmt/instances/{id}/{action}` and `hx-get` on metrics/logs load triggers; disabled attribute logic per state rules
    - `ActionResult(success bool, message string)`: success/error result fragment for instance actions
    - `MetricsTable(points []portal.MetricPoint)`: table of metric data points; timestamp in ISO 8601 UTC; CPU% rounded to 2 decimal places; "No data available" when empty
    - `LogsList(events []portal.LogEvent)`: up to 100 events in reverse-chronological order, RFC 3339 timestamps; "No recent log events" when empty
    - Run `task generate` after creating the file
    - _Requirements: 6.4, 6.5, 7.2, 7.3, 8.2, 8.3, 9.2, 9.3, 9.6, 9.7, 9.8_

  - [x] 8.3 Write property tests for Templ component rendering
    - **Property 9: All required instance fields are present in rendered output** — for any `rapid.Custom` `InstanceSummary`, rendered `InstanceRow` HTML must contain ID, Name, State, InstanceType, and AZ
    - **Property 10: Metrics timestamps and values are correctly formatted** — for any `rapid.Custom` `[]MetricPoint`, rendered `MetricsTable` HTML must contain each timestamp in ISO 8601 UTC and each CPU value rounded to exactly 2 decimal places
    - **Property 11: Log events are rendered in reverse-chronological order, capped at 100** — for any `rapid.SliceOf` `LogEvent` with count N ≥ 1, rendered `LogsList` contains `min(N, 100)` entries ordered newest-first; timestamps in RFC 3339
    - **Property 15: Dashboard HTMX attributes are present for every instance** — for any rendered `InstanceRow`, HTML must contain `hx-post` on start/stop/restart buttons and `hx-get` on metrics/logs triggers with correct instance-ID URLs
    - **Property 16: Action button disabled state matches instance state** — for any `rapid.SampledFrom` instance state, stop+restart disabled for `stopped`; start disabled for `running`; all three disabled for `pending`/`stopping`/`shutting-down`
    - **Validates: Requirements 5.2, 7.2, 8.2, 9.2, 9.3, 9.6, 9.7, 9.8**

  - [x] 8.4 Write unit tests for fragment rendering
    - Test `MetricsTable` with zero points renders "No data available"
    - Test `LogsList` with zero events renders "No recent log events"
    - Test `LogsList` with 150 events renders exactly 100 entries
    - Test `LogsList` event order is reverse-chronological
    - Test dashboard logout button has `method="POST"` and `action="/logout"`
    - _Requirements: 7.3, 8.3, 9.5_

- [x] 9. Checkpoint — run `task generate` then `task build` and `task test`
  - Ensure all tests pass, ask the user if questions arise.

- [x] 10. Implement management handlers
  - [x] 10.1 Create `internal/portal/mgmt.go` — dashboard and instance action handlers
    - Implement `DashboardHandler`: call `EC2ClientIface.DescribeInstances`; map to `[]InstanceSummary`; sort ascending by ID; use `"—"` for missing Name tag; render `PortalDashboard` with error alert on failure (log with `region`, `aws_error_code`, `error` fields)
    - Implement `InstanceActionHandler`: extract `{id}` from path; validate against `instanceIDRegex` (400 + error fragment if invalid); determine action from URL path suffix; dispatch to `StartInstances`, `StopInstances`, or restart sequence (stop first; only call `StartInstances` if stop succeeds); return HTMX-compatible `ActionResult` fragment; log each action with `operator_username`, `instance_id`, `action`, `outcome`
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 6.1, 6.2, 6.3, 6.4, 6.5, 6.6, 6.7, 6.8_

  - [x] 10.2 Write property tests for instance action handlers
    - **Property 4: Invalid instance ID always rejected before AWS call** — for any `rapid.String` not matching `^i-[0-9a-f]{8,17}$`, all action/metrics/logs handlers return HTTP 400 and no method is called on any client interface
    - **Property 5: Restart never starts after a failed stop** — for any restart request where mock `StopInstances` returns an error, `StartInstances` is never called; when `StopInstances` succeeds, `StartInstances` is called exactly once; use `rapid.Bool` for stop success/failure
    - **Property 8: Instance list is always sorted ascending by instance ID** — for any `rapid.SliceOf` shuffled instance IDs, dashboard handler renders rows in strictly ascending lexicographic order
    - **Validates: Requirements 5.1, 6.3, 6.6, 6.8, 7.5, 8.6**

  - [x] 10.3 Write unit tests for management handler edge cases
    - Test: EC2 API error → dashboard renders alert + error logged with region/aws_error_code fields
    - Test: zero instances → renders "no instances found" message
    - Test: `StopInstances` error on restart → `ActionResult` error fragment + `StartInstances` not called
    - Test: action success → 200 + `ActionResult` success fragment
    - Test: `DescribeInstances` Name tag absent → Name rendered as `"—"`
    - _Requirements: 5.3, 5.4, 6.3, 6.4, 6.5, 6.7, 6.8_

  - [x] 10.4 Create `internal/portal/metrics.go` — metrics and logs handlers
    - Implement `MetricsHandler`: validate instance ID (400 if invalid); call `CloudWatchClientIface.GetMetricStatistics` for `CPUUtilization` over past 60 minutes at 5-minute resolution; render `MetricsTable` fragment; on CloudWatch error return error alert fragment and log with `instance_id`, `error`
    - Implement `LogsHandler`: validate instance ID (400 if invalid); call `CloudWatchLogsClientIface.FilterLogEvents` for log group `/ec2/{id}` over past 30 minutes; cap at 100 events; sort reverse-chronological; render `LogsList` fragment; on `ResourceNotFoundException` return "Log group not found" fragment; on other error return error alert fragment and log with `instance_id`, `log_group`, `error`
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5, 8.1, 8.2, 8.3, 8.4, 8.5, 8.6_

  - [x] 10.5 Write unit tests for metrics and logs handlers
    - Test: valid ID + metric data points → ISO 8601 timestamps + 2 decimal place CPU values in fragment
    - Test: valid ID + zero metric data points → "No data available" fragment
    - Test: CloudWatch error → error alert fragment + error logged
    - Test: valid ID + log events → reverse-chronological RFC 3339 timestamps in fragment
    - Test: valid ID + zero log events → "No recent log events"
    - Test: `ResourceNotFoundException` → "Log group not found" (not generic error)
    - Test: other Logs error → error alert + error logged with `log_group` field
    - _Requirements: 7.2, 7.3, 7.4, 8.2, 8.3, 8.4, 8.5_

- [x] 11. Create `internal/portal/page.go` — rendering helpers
  - [x] 11.1 Implement `renderErrorPage`, `renderDashboard`, `renderFragment` thin wrappers
    - Each wrapper calls `templ.Component.Render(ctx, w)` and returns/logs any render error
    - Set `Content-Type: text/html; charset=utf-8` before rendering
    - _Requirements: 9.1_

- [x] 12. Wire portal into `internal/app`
  - [x] 12.1 Extend `internal/app/app.go` — add `PortalHandler` field and AWS client construction
    - Add `PortalHandler *portal.Handler` to `App` struct
    - In `New()`, when `cfg.PortalEnabled()` is true: construct `portal.NewOIDCClient(...)` from config fields; call `aws-sdk-go-v2/config.LoadDefaultConfig` to obtain an AWS config for `cfg.PortalAWSRegion`; construct real `ec2.NewFromConfig`, `cloudwatch.NewFromConfig`, `cloudwatchlogs.NewFromConfig` thin wrappers implementing the three interfaces; call `portal.NewHandler(...)` and assign to `app.PortalHandler`
    - _Requirements: 5.5, 5.6, 10.2, 10.4_

  - [x] 12.2 Register portal routes in `internal/app/server.go`
    - In `buildMux()`, after existing routes, add the guarded block: `if app.Config.PortalEnabled() { ... }` registering `GET /login`, `GET /auth/callback`, `POST /logout`, `GET /mgmt`, `POST /mgmt/instances/{id}/start`, `POST /mgmt/instances/{id}/stop`, `POST /mgmt/instances/{id}/restart`, `GET /mgmt/instances/{id}/metrics`, `GET /mgmt/instances/{id}/logs` — all `/mgmt` routes wrapped with `ph.requireAuth`
    - _Requirements: 4.1, 4.7, 10.4_

- [x] 13. Add `.env.example` entries for portal env vars
  - [x] 13.1 Append portal env var entries to `.env.example`
    - Add commented-out entries for `MGMT_SESSION_KEY`, `MGMT_COGNITO_DOMAIN`, `MGMT_COGNITO_CLIENT_ID`, `MGMT_COGNITO_REDIRECT_URI`, `MGMT_COGNITO_LOGOUT_URI`, `MGMT_AWS_REGION` with brief inline comments explaining each
    - _Requirements: 10.1_

- [x] 14. Final checkpoint — full CI pass
  - Run `task generate`, `task build`, `task test`, `task fmt`, `task lint`; ensure all pass. Ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation after major layers are added
- `task generate` must be run after every `.templ` file edit — the generated `*_templ.go` files must exist before any Go code that imports them will compile
- Property tests use `pgregory.net/rapid`; each test is tagged `// Feature: ec2-management-portal, Property N: <title>`
- All AWS client calls go through the three interfaces defined in `internal/portal/ec2.go`, so tests inject mocks directly without any AWS credentials
- `github.com/golang-jwt/jwt/v5` must be added via `go get` before implementing `internal/portal/oidc.go`
- `portal_login.templ` is not needed — Cognito provides the hosted login UI; only `portal_error.templ` is required for OAuth failure pages
- The `mgmt_oauth_state` cookie stores a JSON-encoded `OAuthState` (state nonce + code_verifier) encrypted with AES-256-GCM, with a 10-minute TTL; it is cleared immediately after the callback is processed (success or failure)
- `task fmt` uses `qlty fmt --all`, not `go fmt`

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3", "1.4", "2.1", "2.2", "2.3"] },
    { "id": 2, "tasks": ["2.4", "2.5", "4.1"] },
    { "id": 3, "tasks": ["4.2", "4.3"] },
    { "id": 4, "tasks": ["4.4", "4.5", "5.1"] },
    { "id": 5, "tasks": ["5.2", "6.1"] },
    { "id": 6, "tasks": ["6.2", "6.3", "8.1", "8.2"] },
    { "id": 7, "tasks": ["8.3", "8.4", "10.1", "10.4", "11.1"] },
    { "id": 8, "tasks": ["10.2", "10.3", "10.5", "12.1"] },
    { "id": 9, "tasks": ["12.2"] },
    { "id": 10, "tasks": ["13.1"] }
  ]
}
```
