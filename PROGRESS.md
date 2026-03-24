# Progress Log

## Completed

- [x] Task-001: Extract constants, config, and helpers

## Current Iteration

- Iteration: 1
- Working on: Task-001 - Extract constants, config, and helpers
- Status: Complete, pending commit

## Last Completed

- Task-001: Extract constants, config, and helpers
- Tests: ✅ 80/80 passing
- Build: ✅ Compiles cleanly
- Key decisions:
  - Kept type aliases for Game, LPSPlayer, SessionData in config.go (main_test.go uses bare names)
  - Removed aliases for Experience, Skill, SkillCategory, Project, GoogleCalendarOption, LambdaGamesResponse (call sites updated to types.X)
  - Magic numbers replaced: maxRequestBodySize (9), maxLPSResponseBodySize (5), defaultGameDuration (1), soccerCookiePath (6)
  - rateLimiterMaxKeys moved to config.go
  - loginEnabled() and googleEnabled() moved to config.go (Task-004 will move to handlers_soccer.go)
  - lpsAPIEndpoint() kept in main.go (Task-006 will move to lps_client.go)

## Blockers

- None

## Notes for Next Iteration

- config.go, helpers.go, schedule_errors.go are fully extracted
- main.go import block lost "os" (moved to config.go), all others retained
- Type aliases for Game/LPSPlayer/SessionData are in config.go; future tasks moving those types can update references
- loadMountainTimeLocation() remains in main.go; it depends on mountainTimeZoneID from config.go
- Next task: Task-002 (session and cookie management)
