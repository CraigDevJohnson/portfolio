# Soccer OAuth and Workflow State Follow-Through Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Soccer's JWT-to-calendar workflow truthful and durable across Google OAuth, refreshes, calendar changes, and HTMX updates, with one canonical Import action.

**Architecture:** Separate configured capability from runtime readiness, use OAuth-compatible Lax cookies for the encrypted LPS session and Google connection identifier, and retain the encrypted server-rendered workflow snapshot as the restoration source. Connections owns the only visible Import launcher; Stage 1 reports and advances the selected source without duplicating setup controls.

**Tech Stack:** Go, encrypted HTTP-only cookies, DynamoDB connection store, Google OAuth 2.0, Templ, HTMX, vanilla JavaScript, handler integration tests, Chrome.

**Spec:** `docs/superpowers/specs/2026-08-18-portfolio-feedback-adjustments-design.md`

## Global Constraints

- Never expose JWTs, OAuth tokens, cookie values, or credentials in UI, logs, tests, or browser output.
- Cross-site OAuth restoration may use `SameSite=Lax`; state-changing Soccer actions remain POST-only.
- A new import or explicit clear resets downstream workflow. Google connect, reconnect, disconnect, and calendar selection never reset it.
- `task portal-preview` must remain safe and must not advertise a real Google connection when its persistent store is intentionally unavailable.
- Real OAuth verification runs under `task run`, which loads `.env` and initializes the real Google connection store.
- Preserve native ICS, Google add/sync behavior, selection fingerprints, and unrelated worktree changes.

---

### Task 1: Make Google availability reflect runtime store readiness

**Files:**
- Modify: `internal/google/oauth.go`
- Modify: `internal/google/oauth_flow.go`
- Modify: `internal/google/calendar_handlers.go`
- Modify: `internal/soccer/handler.go`
- Modify: `internal/soccer/page.go`
- Modify: `internal/google/oauth_test.go`
- Modify: `internal/google/test_helpers_test.go`
- Modify: `internal/app/server_portal_preview_test.go`

**Interfaces:**
- Adds: `GoogleHooks.GoogleAvailable() bool` and `google.Handler.GoogleAvailable() bool`.
- Changes: Connect/add/sync/calendar handlers gate on runtime readiness, not configuration strings alone.

- [ ] **Step 1: Write failing readiness tests**

  Require a configured handler with its initial no-op store to report unavailable and redirect connect to `/soccer?google=unavailable`. After `SetStore(fakeStore)`, require it to report available and render a connect action. Require portal preview to render Google as unavailable rather than entering a false connected loop.

- [ ] **Step 2: Verify red**

  Run: `go test ./internal/google ./internal/app -run 'Google.*Available|Connect.*Unavailable|PortalPreview.*Google' -count=1`

  Expected: FAIL because configuration currently implies availability even when the store is no-op.

- [ ] **Step 3: Implement readiness as a thread-safe handler property**

  Keep the existing store mutex. `SetStore` marks a non-noop store ready; `GoogleAvailable` combines config validity with readiness. Use this method in OAuth and calendar handlers and through Soccer's Google hook.

- [ ] **Step 4: Verify green**

  Run: `go test ./internal/google ./internal/app -run 'Google|PortalPreview' -count=1`

  Expected: PASS.

### Task 2: Preserve encrypted session and connection cookies through OAuth return

**Files:**
- Modify: `internal/soccer/auth.go`
- Modify: `internal/google/oauth_flow.go`
- Modify: `internal/app/session_test.go`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/google/oauth_test.go`

**Interfaces:**
- Changes: `lps_session` and `google_connection` to `SameSite=Lax` while retaining HttpOnly, request-aware Secure, path `/soccer`, encryption for LPS data, and the existing expiry policy.

- [ ] **Step 1: Write failing response-cookie contracts**

  Require imported LPS cookies and callback connection cookies to be Lax. Add a callback-follow-up test that sends both returned cookies to `/soccer?google=connected` and proves the rendered page contains imported players, restored selected teams/schedule, and a connected Google card.

- [ ] **Step 2: Verify red**

  Run: `go test ./internal/app ./internal/google -run 'Session.*OAuth|Callback.*Restores|Import.*Cookie' -count=1`

  Expected: FAIL because both durable cookies are currently Strict and no end-to-end callback render is asserted.

- [ ] **Step 3: Change only the two OAuth-surviving cookie policies**

  Leave the short-lived OAuth state cookie Lax. Change the LPS session and Google connection identifier cookies to Lax; keep their clearing cookies consistent.

- [ ] **Step 4: Verify green and existing security behavior**

  Run: `go test ./internal/app ./internal/google -run 'Session|Import|OAuth|Callback|Connection' -count=1`

  Expected: PASS with encrypted/HttpOnly/Secure behavior unchanged.

### Task 3: Establish one canonical visible Import action

**Files:**
- Modify: `cmd/web/pages/soccer.templ`
- Modify: `cmd/web/partials/soccer_login_state.templ`
- Modify: `cmd/web/tailwind/soccer.css`
- Modify: `internal/app/soccer_presentation_contract_test.go`

**Interfaces:**
- Preserves: the Connections Import button and modal submit button.
- Removes: the second visible Stage 1 launcher.

- [ ] **Step 1: Write the failing render contract**

  In the disconnected full-page state, require exactly one visible element with `data-open-login-modal`. Require Stage 1 to reference the Connections panel and report `available`, `connected`, or `unavailable` without rendering another launcher.

- [ ] **Step 2: Verify red**

  Run: `go test ./internal/app -run 'Soccer.*Import.*Action' -count=1`

  Expected: FAIL because Connections and Stage 1 both open the modal.

- [ ] **Step 3: Remove the Stage 1 launcher and simplify its card**

  Keep a single status/next-step message in Stage 1. When imported, its Continue to players link remains; when disconnected, it points users to the clearly visible Connections panel without duplicating the action.

- [ ] **Step 4: Regenerate and verify green**

  Run: `task generate`

  Run: `go test ./internal/app -run 'Soccer.*(Import|Presentation)' -count=1`

  Expected: PASS.

### Task 4: Verify state-preserving Google and calendar transitions

**Files:**
- Modify if tests expose gaps: `internal/google/calendar_handlers.go`
- Modify if tests expose gaps: `internal/soccer/page.go`
- Modify: `internal/google/oauth_test.go`
- Modify: `internal/google/oauth_connection_test.go`
- Modify: `internal/app/handlers_soccer_test.go`
- Modify: `internal/app/handlers_soccer_schedule_test.go`

**Interfaces:**
- Preserves: `SessionData.Workflow`, selected players, available teams, confirmed team IDs, schedule reconstruction, and per-game browser selection fingerprint.

- [ ] **Step 1: Add table-driven transition tests**

  Seed a full workflow snapshot and prove that Google callback, calendar selection, Google disconnect, HTMX Google refresh, and ordinary page reload do not clear or replace the LPS workflow regions. Prove new import and logout still reset them.

- [ ] **Step 2: Verify red for any uncovered transition**

  Run: `go test ./internal/app ./internal/google -run 'Workflow.*(OAuth|Calendar|Reload)|Google.*Preserves' -count=1`

- [ ] **Step 3: Fix only demonstrated destructive transitions**

  Keep Google fragment responses scoped to `#soccer-google-connection` and adjacent feedback. Do not emit `soccer-workflow-reset` from Google actions.

- [ ] **Step 4: Verify green**

  Run: `go test ./internal/app ./internal/google -run 'Soccer|Google|Workflow' -count=1`

  Expected: PASS.

### Task 5: Run the real local flow and finish the presentation

**Files:**
- Modify if live evidence requires: `cmd/web/pages/soccer.templ`
- Modify if live evidence requires: `cmd/web/partials/soccer_login_state.templ`
- Modify if live evidence requires: `cmd/web/partials/soccer_table_fragment.templ`
- Modify if live evidence requires: `cmd/web/tailwind/soccer.css`
- Modify if live evidence requires: `cmd/web/static/js/main.js`

**Interfaces:**
- Consumes: `task run` for real integrations and Chrome at 390, 768, 1119, 1121, 1440, and native wide width.
- Produces: one truthful, non-looping path from Import through calendar output.

- [ ] **Step 1: Start the real local runtime**

  Stop the existing mock portal preview process and run `task run` on loopback. Confirm logs show the Google connection store initialized before exposing Connect.

- [ ] **Step 2: Verify the complete safe fixture path**

  Use automated handlers/fixtures to exercise JWT import, players, teams, schedule, callback, calendar selection, add, sync, refresh, and selection restoration without exposing secrets.

- [ ] **Step 3: Verify the live OAuth boundary**

  Use the existing Chrome tab. If Google requires a fresh consent action, stop immediately before granting access and ask the user for that single interaction. After return, confirm the connected card, imported state, selections, and schedule remain present.

- [ ] **Step 4: Iterate visual hierarchy across required widths**

  Keep Connections compact, make each workflow stage visually subordinate to the current action, keep feedback adjacent, and verify no horizontal overflow or duplicate action labels.

- [ ] **Step 5: Run all completion gates**

  Run: `task generate`

  Run: `task fmt`

  Run: `task lint`

  Run: `task test`

  Run: `task vet`

  Run: `task build`

  Run: `node --check cmd/web/static/js/main.js`

  Run: `git diff --check`

  Expected: all pass, with `task lint` reporting zero issues.

