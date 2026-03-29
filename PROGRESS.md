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

- Iteration: 7
- Working on: Task-005 review fix: move `NonEmptyStrings` to `internal/lps`

## Blockers

- None

## Notes for Next Iteration

- Task-005 work moved shared LPS value helpers into `internal/lps/helpers.go`
- Task-005 work introduced `internal/soccer/form.go` for form parsing helpers used by soccer and Google handlers
- `internal/app/helpers.go` and `internal/app/lps_client.go` were removed; local `just test` and `just build` passed
- RalphReviewer follow-up: `NonEmptyStrings` was moved from `internal/soccer/form.go` to `internal/lps/helpers.go` to keep generic helpers out of the soccer form package
