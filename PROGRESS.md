# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files
- [x] Task-005: Eliminate LPS client and helpers wrapper files (commits: f622be4, 988849b)
- [x] Task-006: Expand `internal/lps` with resolver, decode, errors, and JWT helpers (commits: b6a0138, 017b126)

## Last Completed

- Task-006: Expand `internal/lps` with resolver, decode, errors, and JWT helpers
- Tests: ✅ All 80 passing
- Build: ✅ Success
- Key decisions:
  - Moved resolver, decode, error classification, and JWT helpers from `internal/app` into `internal/lps`
  - Added `internal/lps/resolver.go`, `decode.go`, `errors.go`, `types.go`, and `jwt.go`
  - Updated `internal/app/handlers_soccer.go` to call `internal/lps` directly
  - Kept scope tight by leaving soccer and Google handlers in `internal/app`
  - Reviewer follow-up confirmed stale app-side LPS files are gone from the current tree

## Current Iteration

- Iteration: 9
- Working on: Task-007: Extract `internal/soccer` handlers

## Blockers

- None

## Notes for Next Iteration

- Next task is Task-007: move soccer handlers and soccer-specific session helpers from `internal/app` into `internal/soccer`
- Reuse the small `internal/soccer/form.go` package introduced in Task-005 rather than replacing it
- Keep Google OAuth/Calendar in `internal/app` for now except for any interface boundaries needed by soccer handler construction
