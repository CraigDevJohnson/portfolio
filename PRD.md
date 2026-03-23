# Feature: Google Calendar and ICS Event Format Parity

## Overview

Update the soccer schedule export flow so both Google Calendar events and the downloaded ICS file match the event field format defined in `event_format.md` exactly. This requires enriching upstream schedule data with player-team context and facility addresses, routing both outputs through shared event formatting logic, and updating existing Google events only when they can be matched to the same game ID.

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing (`just test`)
- [ ] Build succeeds (`just build`)
- [ ] Google Calendar inserts and updates match `event_format.md` for summary, description, location, start, end, status, ID, and private game ID
- [ ] ICS downloads match `event_format.md` for summary, description, location, start, end, status, and stable game identity
- [ ] Existing Google events are updated only when they match the same game ID value
- [ ] No blockers

## Tasks

### Task-001: Enrich Schedule Data for Exact Event Fields

**Priority**: High
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] Schedule resolution returns the exact upstream data needed to build the target event fields: `UGameID`, player team name, opponent team name, division name, facility name, facility address, city, state, ZIP, field name, result, and start time
- [ ] Discovered-player schedule resolution follows the `event_format.md` flow when needed for parity: resolve player teams from `/players/<PLAYER_ID>/my_teams`, then fetch `/teams/<TEAM_ID>` so the selected player's team and opponent can be identified correctly
- [ ] Manual team-ID schedule resolution preserves the same enriched game data shape used by the discovered-player path
- [ ] Facility enrichment fetches `/facilities/<FacilityID>` and captures `Address`, `City`, `State`, `ZIP`, and canonical facility name for downstream event formatting
- [ ] Facility lookups are cached within a request by `FacilityID` so repeated games at the same facility do not trigger duplicate upstream fetches
- [ ] Shared schedule/domain data is extended in existing repo patterns (`main.go` plus `types/types.go` if needed) without introducing a new package split
- [ ] Upstream lookup failures surface explicit schedule export errors consistent with current fetch/download handling instead of silently falling back to incorrect event content
- [ ] Automated tests cover player-team resolution, facility enrichment, and duplicate facility lookup behavior

**Verification**:

```bash
just test
```

### Task-002: Build One Canonical Event Formatter for Google and ICS

**Priority**: High
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] Google event payload generation and ICS `VEVENT` generation both use one shared formatting path so they stay in lockstep
- [ ] Summary is formatted exactly as `PlayerTeamName vs OpponentTeamName - Field`
- [ ] Description is formatted exactly as `PlayerTeamName is playing OpponentTeamName\nDivision: DivisionName\nFacility: FacilityName\nField: FieldName\nResult: Result`
- [ ] Location is formatted from the facility lookup as `Facility.Address, Facility.City, Facility.State, Facility.ZIP`
- [ ] Start is derived from `SchedGameDateTime` and End is exactly 45 minutes after the start time when no explicit upstream end time is provided
- [ ] Google event `ID` is the raw `UGameID` string and `ExtendedProperties.Private["game_id"]` is set to the same game ID value
- [ ] Google event status is `cancelled` when the upstream result is `cancelled`; otherwise it is `confirmed`
- [ ] ICS output mirrors the same canonical fields with `SUMMARY`, `DESCRIPTION`, `LOCATION`, `STATUS`, `DTSTART`, `DTEND`, and `UID` aligned to the same game identity
- [ ] The previous season-only description and hashed/prefixed Google event identity are fully removed from the new event formatter path
- [ ] Automated tests assert the exact Google payload fields and ICS lines for representative games, including cancelled games

**Verification**:

```bash
just test && just build
```

### Task-003: Update Existing Google Events Only When Game IDs Match

**Priority**: High
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] Adding selected games to Google Calendar updates an existing calendar event when the existing event maps to the same `UGameID` value
- [ ] Matching logic uses the same game ID value carried by the Google event ID and/or private `game_id` field; non-matching events are left untouched
- [ ] Existing matching events are refreshed with the canonical summary, description, location, start, end, status, source, and private game ID fields instead of being counted as duplicates
- [ ] Matching cancelled events are restored or updated back to confirmed when the upstream game is no longer cancelled
- [ ] Matching confirmed events are updated to cancelled when the upstream result is `cancelled`
- [ ] Legacy Google events that do not expose the same game ID value are not migrated or mutated by this change
- [ ] User feedback from the Google add flow reports added, updated/restored, and skipped counts accurately
- [ ] Automated tests cover insert, update, restore, cancel, and skip scenarios for matching versus non-matching Google events

**Verification**:

```bash
just test
```

Manual test: Connect a test Google Calendar, add one selected game, change the upstream fixture so the same game ID has different event fields, add it again, and confirm the existing event is updated rather than duplicated.

### Task-004: Harden Regression Coverage for Calendar Exports

**Priority**: Medium
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] `main_test.go` includes regression coverage for enriched schedule fetching, canonical event formatting, Google add/update semantics, and ICS field generation
- [ ] Existing export behaviors that must remain stable continue to be covered: selected-game filtering, invalid selection handling, timezone normalization, and ICS line folding
- [ ] New tests use the repo's existing `httptest` + fake upstream server patterns instead of adding new test frameworks
- [ ] Verification commands for the change are documented in the PRD/progress flow so a Ralph loop can stop only after tests and build pass cleanly

**Verification**:

- Automated: `just test && just build`
- Manual test: Download an ICS file and compare one event against the Google Calendar event for the same game ID to confirm matching summary, description, location, timing, and cancelled/confirmed state

## Technical Constraints

- Language: Go 1.26.1
- Framework: Standard `net/http` server with Templ/HTMX UI; keep the existing monolithic `main.go` application layout
- Testing: Go `testing` package via `just test`
- Build: `just build`
- Style: Follow existing Go patterns in `main.go`, shared models in `types/types.go`, and existing handler tests in `main_test.go`

## Architecture Notes

- `event_format.md` is the source of truth for the target event fields and upstream lookup sequence
- Current Google and ICS event builders live in `main.go`; this work should replace duplicated field derivation with one shared formatter/helper path
- The current discovered-player flow uses player-based schedule fetching, but exact parity may require unifying both player and manual flows around team/facility enrichment before export formatting
- The Google add flow already has insert/conflict/restore behavior; extend that path to update matching events rather than only restoring cancelled ones or skipping duplicates
- Keep changes focused inside the existing architecture: handler orchestration in `main.go`, shared field storage in `types/types.go`, and regression coverage in `main_test.go`
- Preserve existing HTMX/UI behavior unless minimal feedback copy updates are required to report added versus updated events

## Out of Scope

- Migrating legacy Google events whose stored IDs or private properties do not match the same `UGameID` value
- Changing the soccer page UX, auth flow, or manual import workflow beyond export-status messaging required by this feature
- Adding new calendar providers or subscription mechanisms
- Refactoring the repo into additional Go packages
