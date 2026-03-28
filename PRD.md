# Feature: Extract `internal/app` into Domain Packages

## Overview

Split the monolithic `internal/app` package (~3,800 lines, 16 source files, 10 test files) into proper Go domain packages following `cmd/server + internal/...` architecture. The previous refactor (complete) moved all code from a root `package main` monolith into `internal/app` with satellite packages that already contain extracted domain logic. This refactor completes the job: it eliminates the thin wrapper files in `internal/app`, moves remaining logic into focused domain packages, introduces dependency injection via a central `App` struct, and leaves `internal/app` as a thin routing/wiring layer.

### Current State

The codebase has satellite packages with real logic already extracted:
- `internal/httpx` -- client IP, proxy trust, HTTPS detection, secure cookie builder
- `internal/session` -- AES-GCM encryption, rate limiter
- `internal/lps` -- LPS API endpoint/request construction
- `internal/schedule` -- game merge/sort/normalize, time parsing, ICS building
- `internal/portfolio` -- page handlers and static data

But `internal/app` still contains:
- ~8 thin wrapper files that delegate 1:1 to satellite packages (schedule.go, schedule_time.go, schedule_ics.go, lps_client.go, handlers_portfolio.go, data_portfolio.go, helpers.go fragments)
- ~560-line soccer handler file with JWT import, session management, schedule fetch/download
- ~400-line Google OAuth file with DynamoDB store, token management, connect/callback/disconnect
- ~600-line Google Calendar file with event insert/update/sync, API helpers
- ~500-line LPS schedule resolver with player/team discovery, facility caching
- ~300-line LPS decode file with flexible JSON parsing
- ~200-line config with package-level mutable state
- ~130-line error classification
- Route registration and server startup

### Target State

After this refactor:
- `cmd/server/main.go` -- 6 lines, calls `app.Run()`
- `internal/app` -- ~200 lines: `App` struct, route registration, `Run()`, dependency wiring
- `internal/config` -- server config, env parsing, feature toggles
- `internal/soccer` -- soccer page, JWT import, logout, session state, schedule fetch/download handlers
- `internal/google` -- OAuth connect/callback/disconnect, Calendar API, DynamoDB store, token management
- `internal/lps` -- API client + schedule resolver + decode + error classification (expanded from current 45 lines)
- `internal/schedule` -- unchanged (already complete)
- `internal/session` -- unchanged (already complete)
- `internal/httpx` -- unchanged (already complete)
- `internal/portfolio` -- unchanged (already complete)

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing (`just test`)
- [ ] Lint passing (`just lint`)
- [ ] Build succeeds (`just build`)
- [ ] `just ci` passes (fmt -> vet -> lint -> test -> build)
- [ ] `internal/app` contains only route registration, `App` struct, `Run()` (~200 lines)
- [ ] Every `internal/*` package has a clear single-domain purpose
- [ ] No thin 1:1 wrapper files remain (schedule.go, lps_client.go, etc. eliminated)
- [ ] Callers import domain packages directly
- [ ] Tests migrated to their domain packages
- [ ] Package-level mutable globals replaced with injected dependencies where practical
- [ ] All existing routes, behavior, and security properties preserved

## Tasks

### Task-001: Extract `internal/config`

**Priority**: High
**Estimated Iterations**: 3-5

**Description**: Move `serverConfig`, `loadServerConfig()`, env parsing, constants, and feature toggles into a standalone `internal/config` package. This is the foundation that every other package depends on, so it must be extracted first. Replace package-level `configData` global with an exported `Config` struct that gets passed to consumers.

**Acceptance Criteria**:

- [ ] `internal/config/config.go` contains:
  - Exported `Config` struct (replaces `serverConfig`) with fields: `SessionKey`, `LPSAPIBaseURL`, `GoogleClientID`, `GoogleClientSecret`, `GoogleConnectionTableName`
  - `Load()` function (replaces `loadServerConfig()`)
  - `NormalizeLPSAPIBaseURL()`
  - `isLoopbackHost()` (private)
  - `LoginEnabled()` (method on Config)
  - `GoogleEnabled()` (method on Config)
  - `ServerListenAddress()`, `LocalServerURL()`, `PublicBindEnabled()`
- [ ] `internal/config/constants.go` contains all constants:
  - `CareerStartYear`, `DefaultLPSAPIBaseURL`
  - `LPSSessionCookieName`, `GoogleConnectionCookieName`, `GoogleOAuthStateCookieName`
  - `DefaultSessionTTL`, `GoogleConnectionCookieTTL`, `GoogleOAuthStateTTL`
  - `MountainTimeZoneID`, `RateLimiterMaxKeys`
  - `MaxRequestBodySize`, `MaxLPSResponseBodySize`, `DefaultGameDuration`, `SoccerCookiePath`
- [ ] `internal/app/config.go` is deleted; references updated to use `config.Config`
- [ ] Type aliases (`Game`, `LPSPlayer`, `SessionData`) either moved to consuming packages or removed; callers use `types.*` directly
- [ ] Package-level `var configData` in `internal/app` replaced with a `Config` value passed through the `App` struct (Task-002 will wire this)
- [ ] Sentinel errors (`errSessionExpired`, `errPlayerSessionRequired`, `errInvalidTeamSelection`, `errScheduleSelection`) move to the package that owns the behavior (session errors -> `internal/session`, LPS errors -> `internal/lps`, schedule selection -> `internal/soccer`)
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-002: Introduce `App` struct in `internal/app`

**Priority**: High
**Estimated Iterations**: 3-5

**Description**: Create a central `App` struct that holds all runtime dependencies (config, HTTP client, rate limiter, Google connection store, timezone location). Wire it in `Run()` and convert handlers from package-level function references to methods on `App` (or closures that capture `App`). This enables dependency injection and eliminates package-level mutable state.

**Acceptance Criteria**:

- [ ] `internal/app/app.go` contains:
  - `App` struct with fields: `Config config.Config`, `LPSClient *http.Client`, `LoginLimiter *session.LoginRateLimiter`, `MountainTZ *time.Location`, `GoogleStore` (store interface)
  - `New(cfg config.Config) *App` constructor
  - All handler registrations use `app.methodName` or closures
- [ ] `Run()` creates `App` via `New()`, registers routes, starts server
- [ ] Package-level `var` block in config.go is eliminated:
  - `configData` -> `app.Config`
  - `lpsHTTPClient` -> `app.LPSClient`
  - `soccerLoginAttempts` -> `app.LoginLimiter`
  - `mountainTimeLocation` -> `app.MountainTZ`
  - `googleConnections` -> `app.GoogleStore`
- [ ] Google OAuth/Calendar URLs become constants (they never change at runtime)
- [ ] All 80 tests still pass (tests may need to construct `App` in setup)
- [ ] `just ci` passes

**Verification**:

```bash
just ci
```

### Task-003: Eliminate portfolio wrapper files

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Remove `internal/app/handlers_portfolio.go` and `internal/app/data_portfolio.go`. Route registration in `server.go` should call `internal/portfolio` handlers directly, passing `config.CareerStartYear` as needed.

**Acceptance Criteria**:

- [ ] `internal/app/handlers_portfolio.go` deleted
- [ ] `internal/app/data_portfolio.go` deleted
- [ ] Route registration calls `portfolio.HomeHandler`, `portfolio.AboutHandler`, etc. directly (via closures that inject `careerStartYear`)
- [ ] `internal/portfolio/handlers.go` signatures are reviewed: if the only injected value is `careerStartYear`, pass it as a parameter; no need for a full dependency struct
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-004: Eliminate schedule/time/ICS wrapper files

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Remove `internal/app/schedule.go`, `internal/app/schedule_time.go`, and `internal/app/schedule_ics.go`. All callers in `internal/app` (and later `internal/soccer`, `internal/lps`) import `internal/schedule` directly.

**Acceptance Criteria**:

- [ ] `internal/app/schedule.go` deleted
- [ ] `internal/app/schedule_time.go` deleted
- [ ] `internal/app/schedule_ics.go` deleted
- [ ] All call sites updated to use `schedule.MergeGames()`, `schedule.ParseScheduleTime()`, `schedule.BuildICS()`, etc.
- [ ] `formattedGameEvent` type alias removed; callers use `schedule.FormattedGameEvent`
- [ ] `fieldLocationPrefix` constant reference replaced with `schedule.FieldLocationPrefix`
- [ ] `mountainTimeLocation` initialization uses `schedule.MountainTimeLocation` directly
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-005: Eliminate LPS client and helpers wrapper files

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Remove `internal/app/lps_client.go` and the IP/proxy wrapper functions in `internal/app/helpers.go` that delegate to `internal/httpx`. The non-wrapper helpers (`fullName`, `firstNonEmptyString`, `parseSelectedIDs`, etc.) will be evaluated: move to `internal/httpx` if generally useful, or to the consuming domain package if domain-specific.

**Acceptance Criteria**:

- [ ] `internal/app/lps_client.go` deleted
- [ ] Callers use `lps.NewAPIRequest()`, `lps.DoAPIRequest()` directly, passing `app.Config.LPSAPIBaseURL`
- [ ] IP/proxy wrappers (`clientIP`, `forwardedClientIP`, `remoteAddrIP`, `isTrustedProxyIP`, `isValidIP`) deleted from `helpers.go`; callers use `httpx.ClientIP()` etc. directly
- [ ] `requestIsHTTPS()` and `requestBaseURL()` in `cookies.go` replaced with direct `httpx` calls
- [ ] Remaining helper functions in `helpers.go` are relocated:
  - `fullName()` -> `internal/lps` (used by LPS decode)
  - `parseSelectedIDs()`, `parsePlayerIDs()`, `parseTeamIDs()`, `splitDelimitedValues()` -> `internal/soccer` (form parsing)
  - `firstNonEmptyString()`, `firstPositiveInt()`, `intString()`, `stringPointerValue()`, `sortedUniqueIDs()`, `nonEmptyStrings()` -> `internal/lps` or a shared `internal/stringutil` if needed by multiple packages
- [ ] `internal/app/helpers.go` deleted or contains zero functions
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-006: Expand `internal/lps` with schedule resolver and decode

**Priority**: High
**Estimated Iterations**: 4-6

**Description**: Move `lps_schedule.go`, `lps_decode.go`, and `schedule_errors.go` from `internal/app` into `internal/lps`. This consolidates all LPS API interaction (client, request building, player/team discovery, schedule resolution, JSON decoding, error classification) into one package. The `lpsScheduleResolver` struct and its methods move here. Dependencies on config (base URL, HTTP client) are injected through constructor parameters.

**Acceptance Criteria**:

- [ ] `internal/lps/resolver.go` (or similar) contains `ScheduleResolver` (renamed from `lpsScheduleResolver`) with all methods: `FetchPlayerTeams`, `FetchTeamGames`, `FetchTeamSchedule`, `MapTeamScheduleGame`, `FetchFacility`
- [ ] `internal/lps/resolver.go` also contains: `NewScheduleResolver()`, `FetchUserPlayers()`, `FetchGamesForPlayers()`, `FetchGamesForTeams()`, `FetchUpcomingGames()`, `FetchTeamGames()` (package-level convenience functions that create resolvers internally)
- [ ] `internal/lps/decode.go` contains: `DecodeLPSUserPlayers()`, `DecodeLPSGames()`, `ExtractGameMaps()`, `MapLPSGame()`, and map-access helpers
- [ ] `internal/lps/errors.go` contains: `ErrorKind`, `FetchError`, `NewFetchError()`, `ScheduleFetchFeedback()`, `ScheduleDownloadError()`, `ScheduleErrorDetail()`
- [ ] `internal/lps/types.go` contains all LPS response types: `UserPlayerDiscovery`, `UserCheckResponse`, `TeamSummary`, `TeamScheduleGame`, `TeamScheduleResponse`, `Facility`
- [ ] `internal/lps/jwt.go` contains: `JWTExpiry()`, `NormalizeImportedJWT()`, `ImportedSessionExpiry()`
- [ ] All LPS functions receive dependencies (baseURL, httpClient) as parameters instead of reading package globals
- [ ] `internal/app/lps_schedule.go` deleted
- [ ] `internal/app/lps_decode.go` deleted
- [ ] `internal/app/schedule_errors.go` deleted
- [ ] `just test && just build` pass

**Verification**:

```bash
just ci
```

### Task-007: Extract `internal/soccer` handlers

**Priority**: High
**Estimated Iterations**: 5-7

**Description**: Move soccer page rendering, JWT import, logout, session state, schedule fetch, ICS download, and subscribe handlers from `internal/app/handlers_soccer.go` into a new `internal/soccer` package. The soccer package depends on `internal/config`, `internal/session`, `internal/lps`, `internal/schedule`, and `internal/httpx`. It receives its dependencies via a `Handler` struct or similar pattern.

**Acceptance Criteria**:

- [ ] `internal/soccer/handler.go` contains a `Handler` struct with dependencies: `Config *config.Config`, `LPSClient *http.Client`, `LoginLimiter *session.LoginRateLimiter`, `MountainTZ *time.Location`, `GoogleStore` (interface from google package)
- [ ] `internal/soccer/handler.go` contains constructor `NewHandler(...) *Handler`
- [ ] `internal/soccer/page.go` contains `(h *Handler) SoccerPage()` (the main soccer page handler)
- [ ] `internal/soccer/auth.go` contains `ImportHandler()`, `LogoutHandler()`, `SessionHandler()`
- [ ] `internal/soccer/schedule.go` contains `FetchSchedulesHandler()`, `DownloadICSHandler()`, `SubscribeHandler()`
- [ ] `internal/soccer/helpers.go` contains form parsing helpers (`parseSelectedIDs`, `parsePlayerIDs`, etc.), `loginEnabled()`, `renderSoccerLoginState()`, `renderSoccerLoginFeedback()`, `populateScheduleProps()`, `resolveScheduleData()`, `resolveScheduleGames()`, etc.
- [ ] `internal/soccer/session.go` contains soccer-specific session operations: `getSession()`, `loadSoccerSession()`, `setSession()`, `clearSession()`
- [ ] `internal/app/handlers_soccer.go` deleted
- [ ] `internal/app/session.go` deleted (session crypto stays in `internal/session`; soccer session cookie ops move to `internal/soccer`)
- [ ] `internal/app/cookies.go` split: soccer cookie ops -> `internal/soccer`, Google cookie ops -> `internal/google`
- [ ] Route registration in `app.go` uses `soccer.NewHandler(...).SoccerPage` etc.
- [ ] `just ci` passes

**Verification**:

```bash
just ci
```

### Task-008: Extract `internal/google`

**Priority**: High
**Estimated Iterations**: 5-7

**Description**: Move Google OAuth handlers, Calendar API integration, token management, and DynamoDB connection store from `internal/app` into a single `internal/google` package. This consolidates `google_oauth.go` and `google_calendar.go`.

**Acceptance Criteria**:

- [ ] `internal/google/oauth.go` contains:
  - `ConnectHandler()`, `CallbackHandler()`, `DisconnectHandler()`
  - `newRandomHex()`, `newGoogleOAuthState()`, `googleOAuthConfigForRequest()`
  - `googleHTTPContext()`, `encryptGoogleToken()`, `decryptGoogleToken()`
  - `loadGoogleConnectionRecord()`, `deleteGoogleConnection()`, `currentGoogleToken()`
  - `syncGoogleCalendarSelection()`
  - `renderGoogleDisconnectFeedback()`, `redirectSoccerWithGoogleStatus()`
  - `googleOAuthState` type
- [ ] `internal/google/calendar.go` contains:
  - `AddHandler()`, `CalendarHandler()`
  - `insertGoogleCalendarEvents()`, `syncGoogleCalendarEvent()`, `refreshGoogleCalendarEvent()`
  - `googleFindCalendarEventByGameID()`, `googleEventMatchesGameID()`
  - All Google API request/decode helpers
  - All Google Calendar/Event types
- [ ] `internal/google/store.go` contains:
  - `ConnectionStore` interface (replaces `googleConnectionStore`)
  - `ConnectionRecord` type (replaces `googleConnectionRecord`)
  - `DynamoStore` implementation
  - `NoopStore` implementation
  - `NewConnectionStore()` factory
- [ ] Google cookie operations (`getGoogleConnectionID`, `setGoogleConnectionCookie`, `clearGoogleConnectionCookie`, Google OAuth state cookie ops) move to `internal/google`
- [ ] `internal/app/google_oauth.go` deleted
- [ ] `internal/app/google_calendar.go` deleted
- [ ] Google handlers receive dependencies via a struct (config, session key, HTTP client, store)
- [ ] `just ci` passes

**Verification**:

```bash
just ci
```

### Task-009: Migrate tests to domain packages

**Priority**: Medium
**Estimated Iterations**: 4-6

**Description**: Move tests from `internal/app/*_test.go` to their corresponding domain packages. This is the test equivalent of the code migrations in Tasks 006-008. Test helpers that are shared across packages need to be extracted to a test helper package or duplicated where minimal.

**Acceptance Criteria**:

- [ ] `internal/lps/*_test.go` contains tests from:
  - `lps_schedule_test.go` -- resolver, player/team discovery, facility caching
  - `lps_decode_test.go` -- user-check decoding, HTTP failure classification
- [ ] `internal/soccer/*_test.go` contains tests from:
  - `handlers_soccer_test.go` -- import, logout, session, schedule fetch/download
  - `session_test.go` -- soccer session cookie round-trip tests (crypto tests stay in `internal/session`)
- [ ] `internal/google/*_test.go` contains tests from:
  - `google_oauth_test.go` -- connect, callback, calendar selection
  - `google_calendar_test.go` -- add handler, event matching
- [ ] `internal/schedule/*_test.go` contains tests from:
  - `schedule_time_test.go` -- time parsing tests
  - `schedule_ics_test.go` -- ICS building tests
- [ ] `internal/app/helpers_test.go` tests redistributed to packages where the functions moved
- [ ] `internal/app/test_helpers_test.go` shared helpers (`testJWT()`, `testMislabelledLPSZuluTime()`, `configureGoogleTestRuntime()`, `unfoldICS()`) extracted to an `internal/testutil` package or duplicated in consuming test packages
- [ ] All 80 tests still pass
- [ ] `just ci` passes

**Verification**:

```bash
just ci
```

### Task-010: Clean up `internal/app` to routing-only

**Priority**: Medium
**Estimated Iterations**: 2-3

**Description**: After all domain extractions, `internal/app` should contain only the `App` struct, `Run()`, route registration, and dependency wiring. Remove any remaining wrapper functions, unused imports, or orphaned files.

**Acceptance Criteria**:

- [ ] `internal/app` contains at most 2-3 files:
  - `app.go` -- `App` struct, `New()`, dependency wiring
  - `server.go` -- `Run()`, route registration, MIME types, static file serving, server startup
- [ ] All other files in `internal/app/` are deleted
- [ ] No wrapper functions remain -- every route handler comes from a domain package
- [ ] `internal/app` imports: `config`, `soccer`, `google`, `portfolio`, `session`, `schedule` -- never the reverse
- [ ] Total line count for `internal/app` <= 250 lines
- [ ] `just ci` passes

**Verification**:

```bash
just ci
wc -l internal/app/*.go
```

### Task-011: Update docs and validate final structure

**Priority**: Medium
**Estimated Iterations**: 1-2

**Description**: Update README.md, copilot-instructions.md, and any other documentation to reflect the new package structure. Run full validation suite.

**Acceptance Criteria**:

- [ ] `README.md` updated with new package layout
- [ ] `.github/copilot-instructions.md` architecture section reflects new structure
- [ ] No stale file references in documentation
- [ ] Each `internal/*` package has a doc comment on its primary file
- [ ] `just ci` passes
- [ ] Final structure matches target:

```
cmd/server/main.go           -- entry point (~6 lines)
internal/
  app/                        -- routing, wiring, server startup (~200 lines)
  config/                     -- env parsing, feature toggles, constants
  soccer/                     -- soccer page, auth, schedule, download handlers
  google/                     -- OAuth, Calendar API, DynamoDB store
  lps/                        -- API client, resolver, decode, errors, JWT
  schedule/                   -- game logic, time parsing, ICS building
  session/                    -- AES-GCM crypto, rate limiter
  httpx/                      -- client IP, HTTPS detection, secure cookies
  portfolio/                  -- page handlers, static data
types/types.go                -- shared models
cmd/web/                      -- Templ templates, static assets
```

**Verification**:

```bash
just ci
find internal -name '*.go' -not -name '*_test.go' | sort
wc -l internal/app/*.go
```

## Technical Constraints

- Language: Go 1.26.1
- Framework: Standard `net/http` with Templ/HTMX
- Testing: Go `testing` package via `just test`
- Quality: `just ci` (fmt -> vet -> lint -> test -> build)
- Style: `just fmt` and `just lint`; follow existing Go idioms
- Templ: Run `just generate` before build if any `.templ` changes (this refactor should not touch `.templ` files)
- Build: `just build` compiles `./cmd/server`

## Architecture Notes

### Dependency Direction

```
cmd/server/main.go
  |__ internal/app
        |-- internal/config
        |-- internal/soccer
        |     |-- internal/config
        |     |-- internal/session
        |     |-- internal/lps
        |     |-- internal/schedule
        |     |__ internal/httpx
        |-- internal/google
        |     |-- internal/config
        |     |-- internal/session
        |     |__ internal/httpx
        |-- internal/portfolio
        |     |__ types
        |-- internal/lps
        |     |-- internal/schedule
        |     |__ types
        |-- internal/schedule
        |     |__ types
        |-- internal/session
        |__ internal/httpx
```

- Dependencies flow downward only -- no circular imports
- `types` is a leaf package imported by any package that needs shared models
- `internal/app` is the only package that imports `internal/soccer` and `internal/google`
- Domain packages (`soccer`, `google`, `lps`) never import each other directly; `soccer` depends on `lps` for schedule data and `google` for calendar ops, but `google` does not depend on `soccer`

### Dependency Injection Pattern

The `App` struct in `internal/app` constructs all dependencies and passes them to domain package constructors:

```go
type App struct {
    Config       config.Config
    LPSClient    *http.Client
    LoginLimiter *session.LoginRateLimiter
    MountainTZ   *time.Location
    GoogleStore  google.ConnectionStore
}
```

Domain packages define their own handler structs that accept only what they need:

```go
// internal/soccer
type Handler struct {
    Config       *config.Config
    LPSClient    *http.Client
    LoginLimiter *session.LoginRateLimiter
    MountainTZ   *time.Location
    GoogleStore  google.ConnectionStore
}
```

### Task Ordering and Dependencies

```
Task-001 (config) ---+
                     +-- Task-002 (App struct) ---+
Task-003 (portfolio wrappers) --------------------+
Task-004 (schedule wrappers) ---------------------+
Task-005 (lps/helpers wrappers) ------------------+
                                                  +-- Task-006 (expand lps) ---+
                                                  |                            +-- Task-007 (soccer) ---+
                                                  |                            +-- Task-008 (google)  --+
                                                  |                            |                        +-- Task-009 (tests)
                                                  |                            |                        +-- Task-010 (cleanup)
                                                  |                            |                        +-- Task-011 (docs)
```

- Tasks 001-005 can proceed somewhat incrementally: 001 first, then 002, then 003-005 in any order
- Tasks 006-008 require wrappers to be gone first (they move the real call sites)
- Task 009 should follow 006-008 so tests land in the right packages
- Tasks 010-011 are final cleanup

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Circular import between soccer and google | Build failure | Google package exposes interfaces; soccer depends on google, not the reverse. Google handlers that need session data receive it as parameters, not by importing soccer |
| Package-level state removal breaks test setup | Test failures | Introduce test constructors (`NewTestApp()`, `NewTestHandler()`) that wire dependencies with test defaults |
| Handler signature changes cascade widely | Large diffs | Convert one handler group at a time; keep `just ci` green after each task |
| LPS schedule resolver has deep coupling to config | Extraction difficulty | Pass `baseURL` and `*http.Client` as constructor parameters; resolver does not import config directly |
| Test helpers shared across packages | Test compilation errors | Create `internal/testutil` package with shared JWT generation, time helpers, ICS unfold |
| Google DynamoDB store init is async | Race conditions | Store is initialized in a goroutine and set behind a mutex; this pattern is preserved as-is |

## Out of Scope

- Feature additions (no new routes, no new UI, no new API contracts)
- UI changes (no `.templ`, CSS, or JavaScript modifications)
- Infrastructure/Terraform changes
- Broad business-logic rewrites
- Switching to embedded static assets (future hardening pass)
- Introducing a router library (standard `net/http` mux is preserved)
- Changing the `types` package structure
