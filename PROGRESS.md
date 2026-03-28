# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern

## Last Completed

- Task-002: Introduce App struct — test file conversions
- Tests: ✅ All 80 passing
- Build: ✅ Success (`just build` clean)
- Vet: ✅ 0 issues
- Key decisions:
  - Converted 5 test files from `configData` global to `newTestApp(t)` pattern
  - Files converted: `google_oauth_test.go`, `google_calendar_test.go`, `handlers_soccer_test.go`, `lps_decode_test.go`, `lps_schedule_test.go`
  - Also fixed `schedule_time_test.go` (`mountainTimeLocation` → `loadMountainTimeLocation()`)
  - Removed all `previousConfig := configData` / save-restore patterns
  - Removed all `resetSoccerLoginAttempts(t)` calls
  - Removed all `configureGoogleTestRuntime` calls
  - All handler/LPS method calls now prefixed with `app.`
  - `addSessionCookie` signature updated to include `app` parameter

## Current Iteration

- Iteration: 2
- Working on: Task-002 (Introduce App struct) — complete

## Blockers

- None

## Notes for Next Iteration

- All test files now use `newTestApp(t)` or `newTestAppWithGoogle(t, ...)` helpers
- No remaining references to `configData` in test files
- `serverConfig = config.Config` type alias still in `internal/app/config.go`
- Test helper `addSessionCookie(t, app, req, session)` requires `app` as second param
- 80 tests all passing
