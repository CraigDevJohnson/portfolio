# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package

## Last Completed

- Task-001: Extract `internal/config`
- Tests: ✅ All 80 passing
- Build: ✅ Success
- Lint: ✅ 0 issues
- Key decisions:
  - Config methods use pointer receivers to satisfy gocritic hugeParam
  - `serverConfig` kept as type alias `= config.Config` for test compatibility
  - All constants moved to `internal/config/constants.go` (exported)
  - Package-level vars and sentinel errors stay in `internal/app` temporarily
  - Fixed corrupted `constants.go` from prior incomplete attempt

## Current Iteration

- Iteration: 1
- Working on: Task-001 (complete, awaiting commit)

## Blockers

- None

## Notes for Next Iteration

- `internal/config` is stable: Config struct, Load(), LoginEnabled(), GoogleEnabled(), all constants
- `serverConfig = config.Config` alias in `internal/app/config.go` lets tests construct config values without importing config
- Next task (Task-002) should introduce the `App` struct and eliminate package-level mutable vars
