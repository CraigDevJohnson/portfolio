# Progress Log

## Completed

- [x] Task-001: Enrich Schedule Data for Exact Event Fields
- [x] Task-002: Build One Canonical Event Formatter for Google and ICS
- [x] Task-002 follow-up: Add cancelled Google payload regression coverage
- [x] Task-003: Update Existing Google Events Only When Game IDs Match

## Current Iteration

- Iteration: 6
- Working on: Task-004: Harden Regression Coverage for Calendar Exports
- Started: TBD

## Last Completed

- Task-003: Update Existing Google Events Only When Game IDs Match
- Tests: ✅ `just test`
- Build: ✅ `just build`
- Key decisions:
  - Preflight Google event matches now use only the raw event `ID` and `extendedProperties.private.game_id`
  - Matching events are refreshed in place, preserving any existing Google event ID when only the private `game_id` matches
  - Google add feedback now reports added, updated/restored, and safely skipped counts separately

## Blockers

- None

## Notes for Next Iteration

- Regression coverage now includes insert, update, restore, cancel, and skip scenarios for Google add flows
- Canonical event formatting remains shared by `canonicalGameEvent`, `googleEventPayload`, and `buildICS`
- Legacy Google events without the same canonical game ID are intentionally left untouched
