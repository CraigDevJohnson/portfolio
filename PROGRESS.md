# Progress Log

## Completed

- [x] Task-001: Extract constants, config, and helpers
- [x] Task-002: Extract session and cookie management
- [x] Task-003: Extract portfolio handlers and data
- [x] Task-004: Extract soccer handlers
- [x] Task-005: Extract Google OAuth and Calendar
- [x] Task-006: Extract LPS client, schedule, decode, time, and ICS
- [x] Task-007: Split main_test.go into domain-aligned test files

## Current Iteration

- Iteration: 8
- Working on: None (Task-007 complete, awaiting next assignment)

## Last Completed

- Task-007: Split main_test.go into 10 domain-aligned test files
- Tests: ✅ All 80 passing
- Build: ✅ Compiles cleanly
- Key decisions:
  - test_helpers_test.go (118 lines): unfoldICS, testJWT, testMislabelledLPSZuluTime, resetSoccerLoginAttempts, addSessionCookie, fakeGoogleConnectionStore, configureGoogleTestRuntime
  - session_test.go (94 lines): encrypt/decrypt round-trip, rate limiter tests
  - lps_decode_test.go (236 lines): JWT normalization, user player mapping/classification, mislabelled Zulu timestamps
  - handlers_soccer_test.go (1025 lines): import, logout, session, fetch, download handlers
  - lps_schedule_test.go (558 lines): game mapping, player/team resolution, facility caching, filtering
  - google_oauth_test.go (213 lines): connect redirect, callback persistence, calendar selection
  - google_calendar_test.go (481 lines): event payloads, game ID matching, add/update/cancel handler
  - schedule_time_test.go (86 lines): flexible time parsing, schedule times, Mountain wall time
  - schedule_ics_test.go (187 lines): canonical events, ICS line folding, UTF-8 folding, cancelled games
  - helpers_test.go (83 lines): client IP extraction, HTTPS detection
  - main_test.go deleted (was 2939 lines, 64 functions)

## Blockers

- None

## Notes for Next Iteration

- main.go is now 101 lines with only main() and imports
- All domain logic extracted into dedicated files
- Total extracted source files: config.go, cookies.go, data_portfolio.go, session.go, helpers.go, handlers_portfolio.go, handlers_soccer.go, google_oauth.go, google_calendar.go, lps_client.go, lps_schedule.go, lps_decode.go, schedule.go, schedule_time.go, schedule_ics.go, schedule_errors.go
- Test files now mirror source structure: 10 domain-aligned test files, 3081 total lines, 80 tests
