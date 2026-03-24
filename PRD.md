# Feature: Refactor main.go into Multi-File package main

## Overview

Split the monolithic `main.go` (~4,000 lines, 182 functions, 25 types) into well-scoped files within `package main`. The file already has clear logical domains, marked by section comments, that map directly to target files. While splitting, standardize repeated patterns (cookie helpers, body-size limits, template rendering, error classification) and remove redundancies (unnecessary type aliases, duplicated constant values, wrapper-only functions).

This refactor preserves every exported and unexported symbol in `package main`. No new packages are introduced. Tests continue to run unmodified after each task because only the file boundaries move — visibility and import paths do not change.

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing (`just test`)
- [ ] Lint passing (`just lint`)
- [ ] Build succeeds (`just build`)
- [ ] `main.go` contains only `main()`, route registration, and server startup (~100 lines)
- [ ] Every other `.go` file in the root has a single clear purpose evident from its filename
- [ ] Repeated patterns (cookies, body limits, template render, LPS status classification) are consolidated
- [ ] No redundant type aliases remain
- [ ] Magic numbers replaced with named constants
- [ ] `main_test.go` split into domain-aligned test files

## Target File Layout

After the refactor, the root directory will contain these Go source files:

| File | Purpose | Approx Lines |
| ------ | --------- | ------------- |
| `main.go` | `main()`, route registration, server startup | ~100 |
| `config.go` | `serverConfig`, `loadServerConfig()`, env parsing, listen address, `publicBindEnabled()` | ~130 |
| `session.go` | AES-GCM encrypt/decrypt, session cookie CRUD, rate limiter | ~220 |
| `cookies.go` | Shared cookie builder, Google connection cookie, OAuth state cookie, HTTPS detection, base URL | ~140 |
| `handlers_portfolio.go` | Home, About, Experience, Skills, Projects, Education, Contact handlers + HTMX partials | ~150 |
| `data_portfolio.go` | `experienceData()`, `skillsData()`, `projectsData()`, `gravatarURL()` | ~430 |
| `handlers_soccer.go` | Soccer page, import, logout, session, schedule fetch/download/subscribe handlers | ~350 |
| `google_oauth.go` | Google OAuth connect/callback/disconnect handlers, state management, token refresh | ~300 |
| `google_calendar.go` | Google Calendar API calls, event sync/insert/update, calendar listing/selection | ~350 |
| `lps_client.go` | All LPS API request building, HTTP calls, `doLPSAPIRequest()` | ~120 |
| `lps_schedule.go` | `lpsScheduleResolver`, player/team/facility fetching, response decoding | ~450 |
| `schedule.go` | Game merging, sorting, normalization, deduplication, upcoming filter | ~200 |
| `schedule_time.go` | Time parsing (flexible, mislabeled Zulu), formatting, Mountain TZ loader | ~130 |
| `schedule_ics.go` | ICS builder, `canonicalGameEvent()`, RFC 5545 line folding | ~200 |
| `schedule_errors.go` | `lpsFetchError`, `lpsErrorKind`, `scheduleErrorDetails`, feedback/download error mapping | ~170 |
| `helpers.go` | `fullName()`, `firstNonEmptyString()`, `firstPositiveInt()`, `sortedUniqueIDs()`, IP/proxy detection | ~130 |
| `lps_decode.go` | `decodeLPSUserPlayers()`, `decodeLPSGames()`, `extractGameMaps()`, `mapLPSGame()`, map-access helpers | ~200 |

Test files mirror the source files:

| Test File | Covers |
| ----------- | -------- |
| `session_test.go` | Encrypt/decrypt round-trip, rate limiter |
| `handlers_soccer_test.go` | Import, logout, session, schedule fetch/download handlers |
| `google_oauth_test.go` | Connect, callback, calendar selection handlers |
| `google_calendar_test.go` | Google add, event matching, sync actions |
| `lps_schedule_test.go` | Player/team resolution, facility caching, upcoming games |
| `lps_decode_test.go` | User-check decoding, HTTP failure classification |
| `schedule_time_test.go` | Flexible time parsing, mislabeled Zulu, RFC3339 |
| `schedule_ics_test.go` | ICS building, line folding, canonical event formatting |
| `helpers_test.go` | Client IP, HTTPS detection |
| `test_helpers_test.go` | `testJWT()`, `testMislabelledLPSZuluTime()`, `configureGoogleTestRuntime()`, `unfoldICS()` |

## Tasks

### Task-001: Extract constants, config, and helpers

**Priority**: High
**Estimated Iterations**: 1-2

**Description**: Create the foundational files that everything else depends on. Move config loading, constants, helper utilities, and error types first because they have no handler dependencies — only other code depends on them.

**Acceptance Criteria**:

- [ ] `config.go` contains `serverConfig`, `loadServerConfig()`, `normalizeLPSAPIBaseURL()`, `isLoopbackHost()`, `publicBindEnabled()`, `serverListenAddress()`, `localServerURL()`, and all package-level constants (`careerStartYear`, cookie names, TTLs, `mountainTimeZoneID`, `defaultLPSAPIBaseURL`)
- [ ] `config.go` contains the package-level `var` block: `configData`, `lpsHTTPClient`, `mountainTimeLocation`, `soccerLoginAttempts`, sentinel errors, `googleConnectionsMu`, `googleConnections`, Google OAuth/Calendar URLs
- [ ] `helpers.go` contains `fullName()`, `firstNonEmptyString()`, `firstPositiveInt()`, `intString()`, `stringPointerValue()`, `sortedUniqueIDs()`, `nonEmptyStrings()`, `parseSelectedIDs()`, `parsePlayerIDs()`, `splitDelimitedValues()`, `parseTeamIDs()`, `clientIP()`, `forwardedClientIP()`, `remoteAddrIP()`, `isTrustedProxyIP()`, `isValidIP()`
- [ ] `schedule_errors.go` contains `lpsErrorKind`, `lpsFetchError`, `newLPSFetchError()`, `scheduleErrorDetails`, `scheduleFetchFeedback()`, `scheduleDownloadError()`, `scheduleErrorDetail()`
- [ ] Magic numbers are extracted into named constants:
  - `const maxRequestBodySize = 1 << 20` (replaces 9 occurrences of `1<<20`)
  - `const maxLPSResponseBodySize = 2 << 20` (replaces 5 occurrences of `2<<20`)
  - `const defaultGameDuration = 45 * time.Minute`
  - `const soccerCookiePath = "/soccer"` (replaces 6 hardcoded `"/soccer"` cookie paths)
  - `const rateLimiterMaxKeys = 10000` (already exists, move to `config.go`)
- [ ] All type aliases (`Experience = types.Experience`, etc.) are removed; call sites use `types.Experience` directly or the file-local usage is clear enough without the alias
- [ ] `main.go` is trimmed to only `main()` with route registration and server startup
- [ ] `just test && just build` pass with zero changes to test logic

**Verification**:

```bash
just test && just build
```

### Task-002: Extract session and cookie management

**Priority**: High
**Estimated Iterations**: 1-2

**Description**: Move all encryption, session CRUD, and cookie helpers into dedicated files. Consolidate the repeated cookie-building pattern.

**Acceptance Criteria**:

- [ ] `session.go` contains `encryptJSONValue()`, `decryptJSONValue()`, `encryptSession()`, `decryptSession()`, `getSession()`, `loadSoccerSession()`, `setSession()`, `clearSession()`, the `loginRateLimiter` type and all its methods, and the `loginAttempt` type
- [ ] `cookies.go` contains `getGoogleConnectionID()`, `setGoogleConnectionCookie()`, `clearGoogleConnectionCookie()`, `setGoogleOAuthStateCookie()`, `getGoogleOAuthStateCookie()`, `clearGoogleOAuthStateCookie()`, `requestIsHTTPS()`, `requestBaseURL()`
- [ ] A shared `newSecureCookie()` helper is added to `cookies.go` that builds an `http.Cookie` with `HttpOnly`, `Secure` (based on `requestIsHTTPS()`), `SameSite`, `Path`, and `MaxAge`. All 6 cookie-setting call sites use it instead of duplicating cookie struct construction
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-003: Extract portfolio handlers and data

**Priority**: High
**Estimated Iterations**: 1-2

**Description**: Move portfolio page handlers and their hardcoded data into separate files.

**Acceptance Criteria**:

- [ ] `handlers_portfolio.go` contains `homeHandler()`, `aboutHandler()`, `experienceHandler()`, `experienceTimelineHandler()`, `skillsHandler()`, `skillsGridHandler()`, `skillsFilteredHandler()`, `skillsDetailHandler()`, `projectsHandler()`, `projectsGridHandler()`, `educationHandler()`, `contactHandler()`, and `getFeaturedSkills()`
- [ ] `data_portfolio.go` contains `experienceData()`, `skillsData()`, `projectsData()`, `gravatarURL()`, and any SVG icon constants used by `skillsData()`
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-004: Extract soccer handlers

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Move soccer page handlers, schedule fetch/download, and login-state rendering into a dedicated file.

**Acceptance Criteria**:

- [ ] `handlers_soccer.go` contains `soccerHandler()`, `soccerSessionHandler()`, `soccerImportHandler()`, `soccerLogoutHandler()`, `fetchSchedulesHandler()`, `subscribeHandler()`, `downloadICSHandler()`, `loginEnabled()`, `googleEnabled()`, `soccerLoginStateProps()`, `renderSoccerLoginState()`, `renderSoccerLoginFeedback()`, `populateScheduleProps()`, `resolveScheduleData()`, `resolveScheduleGames()`, `handleScheduleDownloadError()`, `requestedScheduleGames()`, `selectedScheduleGames()`, `googleAddScheduleErrorMessage()`
- [ ] `soccerGoogleFlashKind()` and `soccerGoogleFlashMessage()` move to `handlers_soccer.go` (used only by `soccerHandler`)
- [ ] All `http.MaxBytesReader` calls in soccer handlers use the `maxRequestBodySize` constant from Task-001
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-005: Extract Google OAuth and Calendar

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Move Google OAuth handlers and Calendar API integration into dedicated files.

**Acceptance Criteria**:

- [ ] `google_oauth.go` contains `soccerGoogleConnectHandler()`, `soccerGoogleCallbackHandler()`, `soccerGoogleDisconnectHandler()`, `renderGoogleDisconnectFeedback()`, `redirectSoccerWithGoogleStatus()`, `newRandomHex()`, `newGoogleOAuthState()`, `googleOAuthConfigForRequest()`, `googleHTTPContext()`, `encryptGoogleToken()`, `decryptGoogleToken()`, `loadGoogleConnectionRecord()`, `deleteGoogleConnection()`, `currentGoogleToken()`, `syncGoogleCalendarSelection()`
- [ ] `google_oauth.go` also contains the `googleOAuthState` type, `googleConnectionRecord`, `googleConnectionStore` interface, and both `dynamoGoogleConnectionStore` + `noopGoogleConnectionStore` implementations with all their methods
- [ ] `google_calendar.go` contains `soccerGoogleAddHandler()`, `soccerGoogleCalendarHandler()`, `googleAPIResponseError()`, `insertGoogleCalendarEvents()`, `syncGoogleCalendarEvent()`, `refreshGoogleCalendarEvent()`, `googleFindCalendarEventByGameID()`, `googleEventMatchesGameID()`, `decodeGoogleEvent()`, `decodeGoogleEventList()`, `newGoogleAPIRequest()`, `googleInsertCalendarEvent()`, `googleGetCalendarEvent()`, `googleUpdateCalendarEvent()`, `googleListCalendarEventsByPrivateGameID()`, `readGoogleAPIError()`, `googleListCalendarsWithToken()`, `googleListCalendars()`, `preferredGoogleCalendar()`, `googleCalendarSummary()`, `googleEventPayload()`
- [ ] `google_calendar.go` also contains all Google API types: `googleCalendarListResponse`, `googleEventListResponse`, `googleCalendar`, `googleEventDateTime`, `googleEvent`, `googleEventSource`, `googleAPIError`, `googleCalendarEventAction`
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-006: Extract LPS client and schedule logic

**Priority**: High
**Estimated Iterations**: 2-3

**Description**: Move all LPS API interaction and schedule processing into dedicated files.

**Acceptance Criteria**:

- [ ] `lps_client.go` contains `lpsAPIEndpoint()`, `newLPSAPIRequest()`, `validateLPSAPIRequest()`, `doLPSAPIRequest()`
- [ ] `lps_schedule.go` contains `lpsScheduleResolver` and all its methods (`fetchPlayerTeams()`, `fetchTeamGames()`, `fetchTeamSchedule()`, `mapTeamScheduleGame()`, `fetchFacility()`), plus `newLPSScheduleResolver()`, `lpsFetchUserPlayers()`, `lpsFetchGamesForPlayers()`, `lpsFetchGamesForTeams()`, `lpsFetchUpcomingGames()`, `lpsFetchTeamGames()`, and all LPS response types (`lpsUserPlayerDiscovery`, `lpsUserCheckResponse`, `lpsTeamSummary`, `lpsTeamScheduleGame`, `lpsTeamScheduleResponse`, `lpsFacility`), plus `resolveSelectedTeamMatchup()`
- [ ] `lps_decode.go` contains `decodeLPSUserPlayers()`, `decodeLPSGames()`, `extractGameMaps()`, `mapLPSGame()`, `firstString()`, `firstInt()`, `anyToString()`
- [ ] `schedule.go` contains `mergeGames()`, `stableGameFields()`, `fallbackGameID()`, `gameKey()`, `gameStartTime()`, `normalizeScheduleGames()`, `mergeScheduleGames()`, `sortScheduleGames()`, `compareScheduleGames()`, `upcomingScheduleGames()`
- [ ] `schedule_time.go` contains `loadMountainTimeLocation()`, `parseScheduleTime()`, `normalizeLPSScheduleTime()`, `parseMislabelledLPSZuluTime()`, `parseFlexibleTime()`, `formatGameDateTime()`, `scheduleTimes()`
- [ ] `schedule_ics.go` contains `buildICS()`, `canonicalGameEvent()`, `canonicalGameLocation()`, `canonicalGameStatus()`, `escapeICSText()`, `writeICSLine()`, and the `formattedGameEvent` type
- [ ] All `io.LimitReader` calls use `maxRequestBodySize` or `maxLPSResponseBodySize` as appropriate
- [ ] `just test && just build` pass

**Verification**:

```bash
just test && just build
```

### Task-007: Split test file

**Priority**: Medium
**Estimated Iterations**: 2-3

**Description**: Split `main_test.go` into domain-aligned test files that mirror the source files.

**Acceptance Criteria**:

- [ ] `test_helpers_test.go` contains the shared test helpers: `unfoldICS()`, `testJWT()`, `testMislabelledLPSZuluTime()`, `configureGoogleTestRuntime()`
- [ ] Tests are grouped into files matching the source file they primarily exercise:
  - `session_test.go` — encrypt/decrypt round-trip, rate limiter tests
  - `handlers_soccer_test.go` — import, logout, session, schedule fetch/download handler tests
  - `google_oauth_test.go` — connect, callback, calendar selection handler tests
  - `google_calendar_test.go` — Google add handler, event matching tests
  - `lps_schedule_test.go` — player/team resolution, facility caching, upcoming games tests
  - `lps_decode_test.go` — user-check decoding, HTTP failure classification tests
  - `schedule_time_test.go` — flexible time parsing, mislabeled Zulu, RFC3339 tests
  - `schedule_ics_test.go` — ICS building, line folding, canonical event formatting tests
  - `helpers_test.go` — client IP, HTTPS detection tests
- [ ] `main_test.go` is deleted (no tests remain in it)
- [ ] `just test` passes with all 54 tests green
- [ ] `just ci` passes (fmt, vet, lint, test, build)

**Verification**:

```bash
just ci
```

### Task-008: Final cleanup and validation

**Priority**: Medium
**Estimated Iterations**: 1

**Description**: Final pass to ensure consistency, remove leftover section-divider comments, and validate the full CI pipeline.

**Acceptance Criteria**:

- [ ] No `/* ==== Section ==== */` divider comments remain (they served as file-boundary markers and are no longer needed)
- [ ] Each file has a concise package-level comment if it contains exported or significant unexported symbols
- [ ] Import blocks in each file are minimal — only what that file uses
- [ ] `just ci` passes (fmt, vet, lint, test, build)
- [ ] `git diff --stat` confirms `main.go` is ~100 lines
- [ ] No file exceeds ~500 lines

**Verification**:

```bash
just ci
git diff --stat main~1..HEAD
wc -l *.go | sort -n
```

## Technical Constraints

- Language: Go 1.26.1
- Framework: Standard `net/http` server with Templ/HTMX UI
- Testing: Go `testing` package via `just test`
- Quality: `just ci` (fmt → vet → lint → test → build)
- Style: Follow existing Go patterns; use `just fmt` and `just lint` for formatting
- Templ: Run `just generate` before build if any `.templ` files change (this refactor should not touch `.templ` files)

## Architecture Notes

- All files remain in `package main` — no new packages, no new imports between packages
- Package-level variables (`configData`, `lpsHTTPClient`, `googleConnections`, etc.) are accessible from any file in `package main`; they are centralized in `config.go`
- The `types/types.go` package is unchanged; it continues to hold shared models
- Test helpers in `test_helpers_test.go` are accessible to all `_test.go` files because they share `package main`
- Each task is a standalone commit. If any task fails, the repo is still in a working state from the prior task

## Risks and Mitigations

| Risk | Impact | Mitigation |
| ------ | -------- | ----------- |
| Moving functions to wrong file | Build fails due to missing symbol | Run `just build` after each file move; symbols are package-scoped so any file works |
| Circular file dependencies | N/A | Files in the same package cannot have circular deps — Go packages, not files, are the compilation unit |
| Merge conflicts with in-flight branches | Medium | Do this on a clean branch; rebase other work after merge |
| Test file split breaks test isolation | Tests fail | Move test helpers first; each test file uses shared helpers from `test_helpers_test.go` |

## Out of Scope

- Creating new `internal/*` packages or changing import paths
- Introducing dependency injection or interfaces for testability
- Refactoring handler logic, changing HTTP routes, or modifying behavior
- Modifying `.templ` files, CSS, or JavaScript
- Adding new tests (beyond reorganizing existing ones)
