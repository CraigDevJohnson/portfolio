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

## Last Completed

- Task-009: Migrate tests to domain packages
- Tests: 80 passing, `just ci` green
- Key decisions:
  - Schedule tests moved to internal/schedule (time_test.go, ics_test.go)
  - LPS tests moved to internal/lps (decode_test.go, schedule_test.go)
  - Google tests already in internal/google (oauth_test.go, calendar_test.go, add_handler_test.go)
  - httpx tests already in internal/httpx; deleted duplicate from app
  - Soccer integration tests kept in internal/app (test through App wiring)
  - Session integration tests kept in internal/app (test session encrypt through App)
  - Removed corrupted empty stub test files (session/crypto_test.go, soccer/handler_test.go)
  - Deleted duplicate internal/app/helpers_test.go (exact copy of httpx/request_test.go)
  - Fixed gocritic rangeValCopy in google/test_helpers_test.go

## Current Iteration

- Iteration: 12
- Working on: Task-010
- Started: 2026-03-29T14:00:00Z

## Blockers

- None
