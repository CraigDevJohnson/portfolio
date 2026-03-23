# Progress Log

## Completed

- [x] Task-001: Enrich Schedule Data for Exact Event Fields
- [x] Task-002: Build One Canonical Event Formatter for Google and ICS
- [x] Task-002 follow-up: Add cancelled Google payload regression coverage
- [x] Task-003: Update Existing Google Events Only When Game IDs Match
- [x] Task-004: Harden Regression Coverage for Calendar Exports

## Current Iteration

- Iteration: 8
- Working on: Awaiting next task
- Started: 2026-03-23T05:00:15Z

## Last Completed

- Task-004: Harden Regression Coverage for Calendar Exports
- Tests: ✅ `just test`
- Build: ✅ `just build`
- Key decisions:
  - Added direct canonical formatter assertions so shared Google/ICS event fields stay locked together
  - Regression tests now cover selected-game filtering for exports and invalid manual team selection handling
  - Google add coverage now confirms unselected games do not trigger lookups or updates

## Blockers

- None

## Notes for Next Iteration

- Verification flow remains `just test && just build`
- Regression coverage spans enriched schedule fetching, canonical formatting, Google add/update semantics, timezone normalization, and ICS line folding
- Legacy Google events without the same canonical game ID are intentionally left untouched
