# Progress Log

## Completed

- [x] Task-001: Enrich Schedule Data for Exact Event Fields
- [x] Task-002: Build One Canonical Event Formatter for Google and ICS

## Current Iteration

- Iteration: 3
- Working on: Task-003: Update Existing Google Events Only When Game IDs Match
- Started: 2026-03-23T04:46:00Z

## Last Completed

- Task-002: Build One Canonical Event Formatter for Google and ICS
- Tests: ✅ `just test`
- Build: ✅ `just build`
- Key decisions:
  - Added one shared canonical formatter for Google payloads and ICS VEVENT output
  - Google event IDs now use raw `UGameID` with private `game_id` parity and confirmed/cancelled status mapping
  - ICS now mirrors canonical summary, description, location, UID, status, and 45-minute default duration

## Blockers

- None

## Notes for Next Iteration

- Task-003 should match/update Google events using the raw event `ID` and `extendedProperties.private.game_id`
- Canonical event formatting now lives in `canonicalGameEvent` and is shared by `googleEventPayload` and `buildICS`
- Existing timezone normalization and ICS line folding behavior remain covered by tests
