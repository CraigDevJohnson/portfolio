# Feature: Auto-Discover Players From /users/check

## Overview

Replace the manual player ID entry in the soccer JWT import flow with automatic
player discovery via the LPS `/users/check` endpoint. When a user pastes their
JWT, the server calls `/users/check` with that token, extracts all linked player
IDs (and real names) from the response, stores them in the session, and presents
a pre-selected player list. The user optionally deselects players, then clicks
fetch to retrieve schedules.

Why: The current flow requires users to manually find and paste player IDs from
DevTools alongside their JWT. The `/users/check` endpoint already returns the
full list of linked players with names, making manual entry unnecessary and
error-prone. This also enables showing real player names instead of placeholder
"Player 12345" labels.

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing (`go test ./...`)
- [ ] Build succeeds (`just build`)
- [ ] No blockers
- [ ] JWT import no longer asks for manual player IDs
- [ ] Players are auto-discovered from `/users/check` with real names
- [ ] Player select shows all discovered players pre-selected
- [ ] Auth failures from `/users/check` produce actionable user feedback
- [ ] Schedule fetch and ICS export continue to work with discovered player IDs
- [ ] Existing session lifecycle (encrypted cookie, clear import) unchanged

## Tasks

### Task-001: Add /users/check API Client

**Priority**: High
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] New function `lpsFetchUserPlayers(ctx, jwt) ([]LPSPlayer, error)` calls
      `GET {LPSAPIBaseURL}/users/check` with `Authorization: Bearer {jwt}`
- [ ] Parses the `players` array from the response, mapping each entry to the
      existing `LPSPlayer` struct fields:
      - `UPlayerID` from `UPlayerID`
      - `FirstName` from `FirstName`
      - `LastName` from `LastName`
      - `IsMainPlayer` from `is_main_player`
- [ ] Detects auth failure response `{"authFailure":true,"error":"..."}` and
      returns an `lpsFetchError` with kind `lpsErrorUnauthorized`
- [ ] Handles HTTP error codes (401, 403, 5xx) with appropriate `lpsFetchError`
      kinds matching the existing error classification pattern
- [ ] Respects the existing `lpsHTTPClient` (15s timeout) and response size
      limit (2 MiB)
- [ ] Filters out entries where `deleted` is true (from the `user_players`
      cross-reference) if that field is present
- [ ] Unit tests cover: successful parse, auth failure JSON, HTTP errors,
      malformed response body

**Verification**:

```bash
go test -run TestLPSFetchUserPlayers ./...
```

### Task-002: Refactor Import Handler To Auto-Discover Players

**Priority**: High
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] `soccerImportHandler` no longer reads `player_ids` from the form
- [ ] After JWT validation, handler calls `lpsFetchUserPlayers` to discover
      players
- [ ] On success, stores discovered players (with real names and
      `is_main_player` flag) in the session via `setSession`
- [ ] On auth failure from `/users/check`, returns actionable error message:
      "The JWT was rejected by Let's Play Soccer. Copy a fresh bearer token and
      try again."
- [ ] On upstream error, returns: "Could not reach Let's Play Soccer to look up
      your players. Try again in a moment."
- [ ] On empty player list (valid JWT but zero players), returns: "No linked
      players found for this account."
- [ ] Session `UserName` is populated from the `/users/check` response
      `first_name` + `last_name` instead of the placeholder
      "Current browser session"
- [ ] Existing rate limiting, JWT format validation, and session encryption
      remain unchanged
- [ ] Update existing import handler tests to remove player_ids dependency
      and mock `/users/check` responses instead

**Verification**:

```bash
go test -run TestSoccerImport ./...
just build
```

### Task-003: Update Import Modal UI — Remove Player ID Input

**Priority**: High
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] Remove the `player_ids` textarea and its label/hint from
      `soccer_login_modal.templ`
- [ ] Update modal title/description copy to reflect JWT-only import:
      - Description: "Paste the bearer JWT from your signed-in Let's Play Soccer
        browser session. Your linked players will be discovered automatically."
- [ ] Update instruction step 3 from "Copy one or more player IDs..." to
      something like "Click Import — your linked players will be loaded
      automatically."
- [ ] Run `just generate` to regenerate Templ output
- [ ] Verify the import form submits only `jwt` (no `player_ids` field)

**Verification**:

```bash
just generate && just build
```

### Task-004: Player Select Shows Real Names With All Pre-Selected

**Priority**: Medium
**Estimated Iterations**: 1

**Acceptance Criteria**:

- [ ] `SoccerPlayerSelect` component continues to work with auto-discovered
      players that now have real `FirstName`/`LastName` values
- [ ] All players are pre-selected (checkboxes checked) — this is already the
      current behavior via `checked` attribute
- [ ] Player names display as "CRAIG JOHNSON" (from API) instead of
      "Player 1669080" (old placeholder)
- [ ] `is_main_player` true entries still show the "Primary" badge
- [ ] No changes needed to `soccer_player_select.templ` if the `LPSPlayer`
      struct fields are already populated correctly — verify and confirm

**Verification**:

```bash
just build
# Manual: import a JWT → verify player select shows real names
```

### Task-005: Update Tests For End-To-End Discovery Flow

**Priority**: High
**Estimated Iterations**: 2-3

**Acceptance Criteria**:

- [ ] `TestSoccerImportHandler` tests updated: POST to `/soccer/import` now
      sends only `jwt` (no `player_ids`), mock server handles both
      `/users/check` and `/players/{id}/upcoming_games`
- [ ] New test: import with expired/invalid JWT still returns validation error
      before any API call
- [ ] New test: import with valid JWT but `/users/check` returns auth failure
      JSON → user sees actionable error
- [ ] New test: import with valid JWT, `/users/check` succeeds → session
      contains discovered players with real names
- [ ] Existing `TestFetchSchedulesHandler` tests remain passing (they use
      session with pre-set players, unchanged)
- [ ] Existing `TestLPSFetchUpcomingGames*` and `TestLPSFetchGamesForPlayers*`
      tests remain passing (unchanged)
- [ ] `go test ./...` passes with no failures
- [ ] `go vet ./...` reports no issues

**Verification**:

```bash
go test -v ./...
go vet ./...
```

### Task-006A: Remove Dead Manual Import Helpers

**Priority**: Low
**Estimated Iterations**: 1

**Acceptance Criteria**:

- [ ] Remove `parseDelimitedPlayerIDs` function if no longer called anywhere
- [ ] Remove the `importedPlayers` helper if no longer needed (players now come
      from `/users/check` with real data)
- [ ] Remove any dead Go helper code that exists only for the old manual import
      player-ID flow
- [ ] `go vet ./...` still clean
- [ ] `go test ./...` still passing

**Verification**:

```bash
go vet ./...
go test ./...
```

### Task-006B: Remove Remaining Manual Player-ID User-Facing Copy

**Priority**: Low
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] Update remaining user-facing copy in Go handlers and Templ components so
      the product no longer instructs users to manually import or copy player IDs
- [ ] Player selection copy describes discovered players rather than imported
      player IDs while preserving the current checkbox form contract
- [ ] Stale runtime feedback strings in `main.go` align with the current flow:
      clear import and import again, or select discovered players
- [ ] Run `just generate` after any `.templ` changes
- [ ] `go test ./...`, `go vet ./...`, and `just build` all pass

**Verification**:

```bash
just generate
go test ./...
go vet ./...
just build
```

## Technical Constraints

- Language: Go 1.23+
- Templates: Templ (run `just generate` after `.templ` changes)
- Testing: `go test` with `httptest` for mock LPS API
- Style: `gofmt`, `go vet`
- Existing patterns: `lpsHTTPClient`, `lpsFetchError`, `newLPSFetchError`,
  encrypted session cookies, rate limiting

## Architecture Notes

- The `/users/check` call happens server-side during import (not client-side)
  to keep the JWT off the browser's fetch path
- The `players` array in the `/users/check` response is the authoritative
  source for `LPSPlayer` data — it already matches the struct shape
- The `user_players` array provides the `deleted` flag for filtering; the
  `players` array provides the display data
- Cross-reference: match `user_players[].player_id` to `players[].UPlayerID`
  to apply the `deleted` filter
- No new environment variables or config needed — reuses existing
  `LPS_API_BASE_URL` and `LPS_SESSION_KEY`

## Out of Scope

- Refreshing the player list after initial import (user can clear and re-import)
- Fetching additional player metadata beyond what `/users/check` provides
- Caching `/users/check` responses across sessions
- Any changes to the ICS export flow (already works with player IDs from session)
- Client-side JWT extraction or automation
- Mobile/native browser automation or extension-based import
