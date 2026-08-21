# Soccer Resilience and Schedule UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve Soccer workflow state across Google OAuth and page changes, clarify connection/player/team state, and replace the horizontally scrolling schedule with a full-width responsive match board and local action feedback.

**Architecture:** Persist submitted player/team/source state in the existing encrypted LPS session and reconstruct it on full-page loads. Keep per-game checkbox choices as non-sensitive `sessionStorage` state keyed by a confirmed-team fingerprint. Split connection readiness from the numbered workflow, remove obsolete sub-team classification, and move Review & Output to a full-width responsive list.

**Tech Stack:** Go, encrypted JSON cookies, Templ, HTMX, vanilla JavaScript, Tailwind CSS v4 source files, Google OAuth, LPS schedule client, Go handler/contract tests.

**Spec:** `docs/superpowers/specs/2026-08-18-portfolio-feedback-adjustments-design.md`

## Global Constraints

- Never store the LPS JWT, Google token, player names, or calendar credentials in browser storage.
- A new JWT import and explicit “Clear import” reset downstream workflow; Google connect/disconnect/calendar changes do not.
- Native `.ics` download and existing Google event add/sync semantics remain intact.
- Keep LPS and Google credentials encrypted and preserve current cookie security attributes.
- The schedule must have no horizontal scrollbar or horizontal-scroll guidance at any supported viewport.
- Do not edit generated `*_templ.go` or generated `tailwind.css` by hand.
- Preserve unrelated dirty-worktree changes.

---

### Task 1: Add a validated workflow snapshot to the encrypted Soccer session

**Files:**
- Modify: `types/types.go`
- Modify: `internal/soccer/auth.go`
- Modify: `internal/soccer/schedule.go`
- Modify: `internal/soccer/form_test.go`
- Modify: `internal/app/session_test.go`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`

**Interfaces:**
- Produces:

  ```go
  type SoccerWorkflowState struct {
      Source            string    `json:"source,omitempty"`
      SelectedPlayerIDs []int     `json:"selected_player_ids,omitempty"`
      AvailableTeams    []LPSTeam `json:"available_teams,omitempty"`
      SelectedTeamIDs   []int     `json:"selected_team_ids,omitempty"`
  }
  ```

- Extends: `SessionData` with `Workflow SoccerWorkflowState`.
- Valid sources: empty, `imported`, and `manual`.

- [ ] **Step 1: Write session round-trip and normalization tests**

  Cover backward-compatible decoding of a cookie without `workflow`, encrypted
  round-trip of all workflow fields, stable positive unique ID normalization,
  rejection of IDs not present in the imported player/team sets, and clearing
  state on a fresh import.

- [ ] **Step 2: Run session and form tests and confirm missing-state failures**

  Run: `go test ./internal/app ./internal/soccer -run 'Session|Workflow|Form'`

  Expected: compile/test failure until the workflow type and helpers exist.

- [ ] **Step 3: Implement the workflow type and helpers**

  Add focused helpers to normalize positive unique IDs, clear downstream team
  state when player IDs change, and save the encrypted session with the current
  cookie security settings. Keep JSON fields optional for old cookies.

- [ ] **Step 4: Persist player discovery state**

  After `DiscoverTeamsHandler` validates the chosen players and resolves their
  current teams, set source `imported`, store selected player IDs and available
  teams, clear confirmed team IDs, then write the session before rendering the
  team fragment. If the cookie write fails, render actionable feedback and do
  not claim the selection was saved.

- [ ] **Step 5: Persist successful schedule selection**

  After `FetchSchedulesHandler` successfully resolves games, save confirmed
  team IDs and source. For manual input, store parsed team IDs with source
  `manual`; for imported selection, retain selected players and available teams
  with source `imported`. A failed fetch leaves the last successful snapshot
  intact.

- [ ] **Step 6: Verify focused state tests**

  Run: `task fmt`

  Run: `go test ./internal/app ./internal/soccer -run 'Session|Workflow|DiscoverTeams|FetchSchedules'`

  Expected: PASS.

- [ ] **Step 7: Commit workflow persistence**

  ```sh
  git add types/types.go internal/soccer/auth.go internal/soccer/schedule.go internal/soccer/form_test.go internal/app/session_test.go internal/app/handlers_soccer_test.go internal/app/handlers_soccer_schedule_test.go
  git commit -m "feat(soccer): persist workflow selections"
  ```

### Task 2: Reconstruct the completed workflow after Google OAuth or reload

**Files:**
- Modify: `internal/soccer/page.go`
- Modify: `cmd/web/pages/soccer.templ`
- Modify: `cmd/web/partials/soccer_player_select.templ`
- Modify: `cmd/web/partials/soccer_team_select.templ`
- Modify: `cmd/web/partials/soccer_viewmodels.go`
- Modify: `cmd/web/partials/soccer_viewmodels_test.go`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/google/oauth_test.go`

**Interfaces:**
- Extends: `SoccerPlayerSelectProps` with `SelectedPlayerIDs []int`.
- Extends: `SoccerTeamSelectProps` with `SelectedTeamIDs []int`.
- Produces: `restoreSoccerWorkflow(ctx, session)` returning optional team props,
  optional schedule props, and non-destructive restore feedback.

- [ ] **Step 1: Write the OAuth-return restore test**

  Seed an encrypted session with imported players, available teams, and
  confirmed teams; request `/soccer?google=connected`; require selected player
  inputs, selected team inputs, Upcoming/Past content from the stubbed LPS
  response, and the Google success message in one full-page response.

- [ ] **Step 2: Write partial-state restore tests**

  Cover imported-only, players-selected, teams-confirmed, manual-source,
  expired-session, and upstream schedule failure. Upstream restore failure must
  keep connection/player/team UI and show a retryable schedule message rather
  than clearing the workflow.

- [ ] **Step 3: Run the focused handler tests**

  Run: `go test ./internal/app ./internal/google -run 'SoccerPage|OAuth|Restore'`

  Expected: failure because full-page rendering currently uses only auth state.

- [ ] **Step 4: Add selection-aware view models**

  Player checkboxes are checked only when their IDs are in the restored set;
  when no prior player choice exists, all imported players remain checked.
  Team checkboxes are checked only when their IDs are in the restored confirmed
  set; before first confirmation, all available teams are checked.

- [ ] **Step 5: Restore team and schedule props in `SoccerPage`**

  Rebuild player groups from `session.Workflow.AvailableTeams`. When confirmed
  team IDs exist, fetch all games for those IDs using the existing resolver and
  populate `InitialResults`. Apply a request-scoped timeout no longer than the
  existing LPS client timeout. Do not mutate or clear the snapshot on restore
  failure.

- [ ] **Step 6: Regenerate and verify**

  Run: `task generate`

  Run: `task fmt`

  Run: `go test ./internal/app ./internal/google -run 'SoccerPage|OAuth|Restore'`

  Expected: PASS.

- [ ] **Step 7: Commit restore behavior**

  ```sh
  git add internal/soccer/page.go cmd/web/pages/soccer.templ cmd/web/partials/soccer_player_select.templ cmd/web/partials/soccer_team_select.templ cmd/web/partials/soccer_viewmodels.go cmd/web/partials/soccer_viewmodels_test.go internal/app/handlers_soccer_test.go internal/google/oauth_test.go
  git commit -m "fix(soccer): restore workflow after oauth"
  ```

### Task 3: Move connection readiness before the four-stage workflow

**Files:**
- Modify: `cmd/web/pages/soccer.templ`
- Modify: `cmd/web/partials/soccer_login_state.templ`
- Modify: `cmd/web/partials/soccer_viewmodels.go`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `internal/soccer/page.go`
- Modify: `internal/google/calendar_handlers.go`
- Modify: `internal/google/oauth_flow.go`
- Modify: `internal/app/soccer_presentation_contract_test.go`
- Modify: `internal/app/soccer_style_contract_test.go`
- Modify: `internal/app/soccer_interaction_contract_test.go`

**Interfaces:**
- Produces: `#soccer-connections`, `#soccer-lps-connection`, and
  `#soccer-google-connection` stable fragment targets.
- Replaces: the five-item legend with Source, Players, Teams, Review & Output.
- Preserves: import modal, Google connect/reconnect/disconnect/calendar actions,
  and their existing handler endpoints.

- [ ] **Step 1: Write connection-panel and stage-order contracts**

  Require Connections before stage 1; require four legend entries in the exact
  order; require no fifth Calendar Output stage. Test connected and disconnected
  cards for explicit text plus `data-connection-state="connected|disconnected"`.

- [ ] **Step 2: Write non-reset OOB contracts**

  Google calendar selection, reconnect, and disconnect responses may replace
  only the Google connection card and adjacent feedback. Import success may
  replace the LPS card and intentionally reset downstream player/team/schedule
  state. No Google action may emit `soccer-workflow-reset`.

- [ ] **Step 3: Run focused Soccer contracts**

  Run: `go test ./internal/app ./internal/google -run 'Soccer.*(Presentation|Style|Interaction)|CalendarHandler|Disconnect'`

  Expected: failure until targets and ordering change.

- [ ] **Step 4: Split connection components**

  Refactor the current combined login/calendar state into two compact cards
  inside a Connections section above the planner. Keep security copy with the
  LPS card. Use a mint border, tinted background, check icon, and “Imported for
  this session” or “Connected to [calendar]” text for configured states.

- [ ] **Step 5: Simplify the numbered workflow**

  Make source stage 1 choose imported context or manual IDs, Players stage 2,
  Teams stage 3, and Review & Output stage 4. Remove the fifth calendar section;
  its connection controls now live above stage 1 and its output actions remain
  in the schedule sections.

- [ ] **Step 6: Update fragment responses atomically**

  Adjust import/logout/Google handlers, OOB targets, and related view-model flags
  together. Calendar changes refresh only the Google card. New import/logout
  refresh LPS state and the appropriate downstream regions.

- [ ] **Step 7: Regenerate and verify**

  Run: `task generate`

  Run: `task fmt`

  Run: `go test ./internal/app ./internal/google -run 'Soccer|CalendarHandler|Disconnect'`

  Expected: PASS.

- [ ] **Step 8: Commit workflow reordering**

  ```sh
  git add cmd/web/pages/soccer.templ cmd/web/partials/soccer_login_state.templ cmd/web/partials/soccer_viewmodels.go cmd/web/tailwind/soccer.css internal/soccer/page.go internal/google/calendar_handlers.go internal/google/oauth_flow.go internal/app/soccer_presentation_contract_test.go internal/app/soccer_style_contract_test.go internal/app/soccer_interaction_contract_test.go
  git commit -m "refactor(soccer): surface connection readiness"
  ```

### Task 4: Highlight the primary player row and remove sub-team classification

**Files:**
- Modify: `types/types.go`
- Modify: `internal/soccer/auth.go`
- Modify: `internal/soccer/schedule.go`
- Modify: `internal/soccer/store.go`
- Modify: `cmd/web/partials/soccer_player_select.templ`
- Modify: `cmd/web/partials/soccer_team_select.templ`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`
- Modify: `internal/app/soccer_presentation_contract_test.go`

**Interfaces:**
- Removes: `SessionData.KnownTeams`, `LPSTeam.IsSubTeam`, baseline known-team
  import fetching, `TeamsJSON` baseline persistence, and all `Sub` UI.
- Produces: `data-primary-player="true"` on the complete main-player label.

- [ ] **Step 1: Write removal and primary-row tests**

  Assert that import makes no `FetchPlayerTeams` calls, `LPSTeam` JSON has no
  `is_sub_team`, team markup contains no “Sub,” every first-render team checkbox
  is checked, and the main player's label owns the primary-state marker and
  “Primary player” supporting text.

- [ ] **Step 2: Run focused import/team tests**

  Run: `go test ./internal/app ./internal/soccer -run 'Import|Primary|Sub|DiscoverTeams'`

  Expected: failure against the baseline classification and detached chip.

- [ ] **Step 3: Remove classification data and import work**

  Delete `KnownTeams`, `IsSubTeam`, `fetchKnownTeams`, baseline team JSON
  persistence, and comparison logic. `resolvePlayerTeams` now returns every
  currently discovered positive team without classification.

- [ ] **Step 4: Simplify team markup**

  Remove the explanatory sub paragraph, badges, and conditional `checked`.
  Render all teams checked unless Task 2 supplies a restored confirmed set.

- [ ] **Step 5: Restyle the primary player**

  Apply the primary marker to the full label, use a mint border and quiet mint
  surface, and place “Primary player” below the name. Remove the detached pill.
  Preserve checkbox and label hit-target behavior.

- [ ] **Step 6: Regenerate, format, and verify**

  Run: `task generate`

  Run: `task fmt`

  Run: `go test ./internal/app ./internal/soccer -run 'Import|Primary|Sub|DiscoverTeams'`

  Expected: PASS.

- [ ] **Step 7: Commit player/team cleanup**

  ```sh
  git add types/types.go internal/soccer/auth.go internal/soccer/schedule.go internal/soccer/store.go cmd/web/partials/soccer_player_select.templ cmd/web/partials/soccer_team_select.templ cmd/web/tailwind/soccer.css internal/app/handlers_soccer_test.go internal/app/handlers_soccer_schedule_test.go internal/app/soccer_presentation_contract_test.go
  git commit -m "fix(soccer): simplify player and team selection"
  ```

### Task 5: Replace horizontally scrolling tables with the full-width match board

**Files:**
- Modify: `cmd/web/pages/soccer.templ`
- Modify: `cmd/web/partials/soccer_table_fragment.templ`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `cmd/web/partials/ui_types_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`
- Modify: `internal/app/soccer_presentation_contract_test.go`
- Modify: `internal/app/soccer_style_contract_test.go`

**Interfaces:**
- Replaces: `SoccerGamesTable` and `.table-wrapper` with
  `SoccerMatchList(sectionID string, games []types.Game)`.
- Preserves: `name="selected"`, game IDs, `data-game-checkbox`,
  `data-game-group`, forms, native download, Google include behavior, and
  select-all semantics.

- [ ] **Step 1: Write no-horizontal-scroll markup contracts**

  Require a semantic ordered list and one checkbox per game. Require combined
  matchup and location fields. Assert absence of `<table class="games-table">`,
  `.table-wrapper`, both horizontal-scroll messages, and any schedule
  `overflow-x: auto` or fixed `min-width`.

- [ ] **Step 2: Write desktop/mobile CSS contracts**

  Require a desktop grid with columns for select, date/time, matchup, location,
  season, and result; require a narrow card layout with visible field labels;
  require Upcoming rows to omit empty result markup. Keep minimum 44-pixel
  checkbox targets and visible row focus.

- [ ] **Step 3: Run schedule presentation tests**

  Run: `go test ./cmd/web/partials ./internal/app -run 'Soccer.*(Schedule|Presentation|Style)|Games'`

  Expected: failure against the current eight-column table and scroll rules.

- [ ] **Step 4: Move Review & Output outside the planner rail**

  Place stages 1–3 in the existing rail/workspace layout and render stage 4 as
  a separate full-width section immediately below. Keep heading IDs and stage
  navigation targets valid.

- [ ] **Step 5: Implement the semantic match list**

  Render one row/card per game. Combine Home/Away into Matchup and Field/Venue
  into Location while preserving all text. Keep Season and parsed Result.
  Upcoming entries do not render a blank Result block.

- [ ] **Step 6: Replace table CSS with responsive row/card CSS**

  Use a wide grid at sufficient container width and a labeled stacked card
  below it. Do not create a local horizontal scroll region. Preserve alternating
  emphasis, past-result tone, hover/focus, field/season/result badges, and form
  density.

- [ ] **Step 7: Regenerate and verify**

  Run: `task generate`

  Run: `go test ./cmd/web/partials ./internal/app -run 'Soccer.*(Schedule|Presentation|Style)|Games'`

  Expected: PASS.

- [ ] **Step 8: Commit the match board**

  ```sh
  git add cmd/web/pages/soccer.templ cmd/web/partials/soccer_table_fragment.templ cmd/web/tailwind/soccer.css cmd/web/partials/ui_types_test.go internal/app/handlers_soccer_schedule_test.go internal/app/soccer_presentation_contract_test.go internal/app/soccer_style_contract_test.go
  git commit -m "refactor(soccer): add responsive match board"
  ```

### Task 6: Make Add/Sync progress and results local and visible

**Files:**
- Modify: `cmd/web/partials/soccer_table_fragment.templ`
- Modify: `cmd/web/partials/soccer_login_feedback.templ`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `cmd/web/static/js/main.js`
- Modify: `internal/google/calendar_handlers.go`
- Modify: `internal/google/add_handler_test.go`
- Modify: `internal/app/soccer_interaction_contract_test.go`
- Modify: `internal/app/soccer_presentation_contract_test.go`

**Interfaces:**
- Produces: `#upcoming-calendar-feedback` and `#past-results-feedback`, each
  directly below its toolbar and marked with `data-soccer-feedback`.
- Extends: loading controls with `data-loading-text` and a selected-count value.
- Removes: sticky `.games-section-top` and bottom-only
  `#google-calendar-feedback`.

- [ ] **Step 1: Write local-target and non-sticky contracts**

  Require Upcoming Add to target `#upcoming-calendar-feedback` and Past Sync to
  target `#past-results-feedback`. Assert each target immediately follows its
  toolbar, has polite live status, and the CSS contains no sticky rule for the
  section top.

- [ ] **Step 2: Write loading-label behavior tests**

  Add a small JavaScript contract or DOM fixture proving that Add changes to
  “Adding N games…” and Sync changes to “Syncing N results…” while busy, restores
  its original text after completion, remains disabled while busy, and reports
  success/error in the initiating section.

- [ ] **Step 3: Run action feedback tests**

  Run: `go test ./internal/google ./internal/app -run 'Add|Sync|Feedback|Interaction'`

  Run: `node --check cmd/web/static/js/main.js`

  Expected: contract failure until the local targets and labels exist.

- [ ] **Step 4: Move feedback beside each action**

  Give each form a unique feedback region and point its HTMX action to that
  region. Remove the shared bottom feedback block. Keep connection guidance in
  the top Connections panel instead of repeating it below both schedules.

- [ ] **Step 5: Implement selected-count loading labels**

  At request start, read the matching group count, cache the original label,
  set the action-specific busy label, spinner, `disabled`, and `aria-busy`.
  Restore the label and selection-aware disabled state at request end. Support
  multiple feedback IDs through `data-soccer-feedback` instead of a fixed
  single target ID.

- [ ] **Step 6: Remove sticky and translucent overlay behavior**

  Delete the `position: sticky` and `top` declarations for
  `.games-section-top`. Keep the toolbar in normal document flow with an opaque
  raised surface and a clear border separating it from match rows.

- [ ] **Step 7: Regenerate and verify**

  Run: `task generate`

  Run: `task fmt`

  Run: `node --check cmd/web/static/js/main.js`

  Run: `go test ./internal/google ./internal/app -run 'Add|Sync|Feedback|Interaction'`

  Expected: PASS.

- [ ] **Step 8: Commit action feedback**

  ```sh
  git add cmd/web/partials/soccer_table_fragment.templ cmd/web/partials/soccer_login_feedback.templ cmd/web/tailwind/soccer.css cmd/web/static/js/main.js internal/google/calendar_handlers.go internal/google/add_handler_test.go internal/app/soccer_interaction_contract_test.go internal/app/soccer_presentation_contract_test.go
  git commit -m "fix(soccer): show calendar action status in context"
  ```

### Task 7: Preserve compatible per-game selection across OAuth and swaps

**Files:**
- Modify: `cmd/web/partials/soccer_table_fragment.templ`
- Modify: `cmd/web/static/js/main.js`
- Modify: `internal/app/soccer_interaction_contract_test.go`

**Interfaces:**
- Browser key: `portfolio:soccer:selection:<team-fingerprint>`.
- Stored value: versioned JSON containing only upcoming and past game ID arrays.
- Clear triggers: new import, logout, or a changed confirmed-team fingerprint.

- [ ] **Step 1: Write browser-state contracts**

  Test save/restore for partial selections, ignore malformed JSON, ignore IDs not
  present in the current result, separate two team fingerprints, and clear on
  `soccer-workflow-reset` and `soccer-logout`.

- [ ] **Step 2: Run the interaction contract**

  Run: `go test ./internal/app -run 'Soccer.*Selection'`

  Expected: failure because selections are currently DOM-only.

- [ ] **Step 3: Expose a non-sensitive team fingerprint**

  Render a stable fingerprint made from sorted confirmed numeric team IDs on
  the games form. Do not include the JWT, names, session ID, or calendar ID.

- [ ] **Step 4: Save and restore checkbox state**

  After user checkbox/select-all changes, save checked opaque game IDs for both
  groups. During `setupSoccerSelectAll`, restore matching IDs before computing
  counts and action disabled states. Fail closed to server defaults when
  `sessionStorage` is unavailable or malformed.

- [ ] **Step 5: Clear incompatible state**

  Remove the previous fingerprint entry on explicit workflow reset/logout. A
  new fingerprint naturally uses a separate key; prune the prior page key so
  browser storage does not accumulate stale schedules.

- [ ] **Step 6: Verify JavaScript and contracts**

  Run: `node --check cmd/web/static/js/main.js`

  Run: `go test ./internal/app -run 'Soccer.*Selection'`

  Expected: PASS.

- [ ] **Step 7: Commit selection resilience**

  ```sh
  git add cmd/web/partials/soccer_table_fragment.templ cmd/web/static/js/main.js internal/app/soccer_interaction_contract_test.go
  git commit -m "feat(soccer): restore match selections"
  ```

### Task 8: Run end-to-end Soccer verification and visual QA

**Files:**
- Create: `docs/superpowers/qa/2026-08-18-soccer-resilience-schedule-ux.md`
- Modify only if a verified defect is found: files owned by Tasks 1–7.

**Interfaces:**
- Consumes: local Soccer preview fixtures and stubbed LPS/Google handler tests.
- Produces: source-check results and viewport/state evidence.

- [ ] **Step 1: Run repository-authoritative checks**

  Run: `task generate`

  Run: `task fmt`

  Run: `task lint`

  Run: `task test`

  Run: `task build`

  Run: `node --check cmd/web/static/js/main.js`

  Run: `git diff --check`

  Expected: all pass. If lint repeats the known `no go files to analyze`
  toolchain failure, record the exact output and do not mark lint passed.

- [ ] **Step 2: Verify workflow-state transitions**

  Exercise manual-only, import, player choice, team confirmation, schedule
  fetch, Google connect callback, calendar change, reconnect, disconnect, new
  import, clear import, expired session, and restore-fetch failure. Confirm only
  intentional actions reset downstream state.

- [ ] **Step 3: Verify schedule and action states**

  Exercise Upcoming-only, Past-only, combined, empty, long venue/team names,
  none/partial/all selection, native ICS download, Add success/error, Sync
  success/error, connection expired, and restored per-game choices.

- [ ] **Step 4: Capture responsive evidence**

  Capture each principal state at 390, 768, 1119, 1121, and 1440 pixels. Confirm
  no horizontal schedule scroll, no sticky overlay, readable match fields,
  visible action progress/result, clear connection states, and primary-row
  emphasis without a detached stamp.

- [ ] **Step 5: Verify accessibility modes**

  Test keyboard-only operation, focus after HTMX swaps, live announcements,
  reduced motion, forced colors, and 200% zoom. Confirm status meaning is not
  color-only and every checkbox retains a 44-by-44 target.

- [ ] **Step 6: Write the QA report and commit evidence pointers**

  Record exact commands, results, screenshot locations, and blocked boundaries.

  ```sh
  git add docs/superpowers/qa/2026-08-18-soccer-resilience-schedule-ux.md
  git commit -m "test(soccer): verify resilient schedule workflow"
  ```

