# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files

## Last Completed

- Task-004: Eliminate schedule/time/ICS wrapper files
- Tests: ✅ All 80 passing
- Build: ✅ Success
- Key decisions:
  - Deleted `internal/app/schedule.go` (10 wrapper functions + 1 constant alias)
  - Deleted `internal/app/schedule_time.go` (7 wrapper functions including `loadMountainTimeLocation`)
  - Deleted `internal/app/schedule_ics.go` (6 wrapper functions + 1 type alias)
  - All callers now import `internal/schedule` directly
  - `formattedGameEvent` type alias removed; callers use `schedule.FormattedGameEvent`
  - `loadMountainTimeLocation()` replaced with `schedule.MountainTimeLocation` (package-level var)
  - Updated 8 files: lps_schedule.go, handlers_soccer.go, google_calendar.go, app.go, test_helpers_test.go, schedule_ics_test.go, schedule_time_test.go, lps_schedule_test.go

## Current Iteration

- Iteration: 5
- Working on: Next task TBD

## Blockers

- None

## Notes for Next Iteration

- Portfolio routes are now decoupled from `App` struct
- Next candidates for wrapper elimination: schedule wrapper files (Task-004), LPS/helpers wrappers (Task-005)
- 80 tests all passing
