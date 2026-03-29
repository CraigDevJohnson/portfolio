# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files
- [x] Task-005: Eliminate LPS client and helpers wrapper files (commits: f622be4, 988849b)
- [x] Task-006: Expand `internal/lps` with resolver, decode, errors, and JWT helpers (commits: b6a0138, 017b126)
- [x] Task-007: Extract `internal/soccer` handlers (commit: 1d1628c)
- [x] Task-008: Extract `internal/google` (commits: 2acf530, 8e6d13c)
- [x] Task-009: Migrate tests to domain packages
- [x] Task-010: Clean internal/app to routing-only

## Last Completed

- Task-010: Clean internal/app to routing-only
- Tests: 80 passing, `just ci` green
- Key decisions:
  - Moved all test-compatibility wrappers from soccer_bridge.go to bridges_test.go
  - Deleted dead Google type aliases and cookie bridges (config.go, cookies.go)
  - Removed dead loginAttempt type alias
  - internal/app production files: app.go (34), server.go (142), soccer_bridge.go (87) = 263 lines
  - Test wrappers preserved in _test.go for integration tests

## Current Iteration

- Iteration: 12
- Working on: Complete
- Started: 2026-03-29T14:00:00Z

## Blockers

- None
