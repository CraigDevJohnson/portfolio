# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files
- [x] Task-005: Eliminate LPS client and helpers wrapper files (commits: f622be4, 988849b)
- [x] Task-006: Expand `internal/lps` with resolver, decode, errors, and JWT helpers (commits: b6a0138, 017b126)
- [x] Task-007: Extract `internal/soccer` handlers (commit: 1d1628c)

## Last Completed

- Task-007: Extract `internal/soccer` handlers
- Tests: All 80 passing
- Build: Success
- Key decisions:
  - Created Handler struct with GoogleHooks interface bridge for temporary Google integration
  - Split cookies.go: soccer cookie via internal/soccer/cookies.go, Google ops remain in internal/app
  - Added soccer_bridge.go for Google handler compat (forwarding methods)
  - Form parsing helpers remain in internal/soccer/form.go from Task-005
  - Reviewer verified PASS on all acceptance criteria

## Current Iteration

- Iteration: 10
- Working on: Task-008: Extract `internal/google`

## Blockers

- None

## Notes for Next Iteration

- Task-008 moves google_oauth.go and google_calendar.go into internal/google
- Google cookie ops from internal/app/cookies.go move to internal/google
- ConnectionStore interface, DynamoStore, NoopStore move to internal/google/store.go
- soccer_bridge.go forwarding methods should be simplified or removed once Google is extracted
- Google handlers currently call app.loadSoccerSession and app.renderSoccerLoginState via bridge
