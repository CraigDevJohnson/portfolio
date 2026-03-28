# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files

## Last Completed

- Task-003: Eliminate portfolio wrapper files
- Tests: ✅ All 80 passing
- Build: ✅ Success
- Key decisions:
  - Deleted `internal/app/handlers_portfolio.go` (12 handler wrappers)
  - Deleted `internal/app/data_portfolio.go` (4 data wrappers)
  - Route registration in `server.go` now calls `portfolio.*Handler` directly via closures
  - `config.CareerStartYear` injected into `HomeHandler` and `AboutHandler` closures

## Current Iteration

- Iteration: 3
- Working on: Task-003 — complete

## Blockers

- None

## Notes for Next Iteration

- Portfolio routes are now decoupled from `App` struct
- Next candidates for wrapper elimination: schedule wrapper files (Task-004), LPS/helpers wrappers (Task-005)
- 80 tests all passing
