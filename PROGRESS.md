# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files
- [x] Task-005: Eliminate LPS client and helpers wrapper files (commits: f622be4, 988849b)

## Last Completed

- Task-005: Eliminate LPS client and helpers wrapper files
- Tests: ✅ All 80 passing
- Build: ✅ Success
- Key decisions:
  - Deleted `internal/app/lps_client.go` and `internal/app/helpers.go`
  - Moved LPS value helpers to `internal/lps/helpers.go`
  - Added focused form parsing helpers in `internal/soccer/form.go`
  - Replaced app-local IP/base URL wrappers with direct `internal/httpx` usage
  - Reviewer-required fix moved `NonEmptyStrings` from `internal/soccer` to `internal/lps`

## Current Iteration

- Iteration: 8
- Working on: Task-006: Expand `internal/lps` with schedule resolver and decode

## Blockers

- None

## Notes for Next Iteration

- Task-006 implementation moved resolver, decode, JWT, and schedule error logic into `internal/lps`
- `internal/app/handlers_soccer.go` and the Task-006 tests now call `internal/lps` directly
- Reviewer follow-up removed stale tracked `internal/app/lps_schedule.go`, `internal/app/lps_decode.go`, and lingering `internal/app/lps_client.go`
- Verified current Task-006 implementation with `just test` and `just build`; leave completion marking to coordinator/review
