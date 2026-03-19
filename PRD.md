# Feature: User-Mediated LPS Authenticated Data Import

## Overview

Build a replacement for the current server-proxied LPS login flow that does not
depend on LPS reCAPTCHA, private keys, or site-owned authentication behavior.
Instead, the user signs in directly on letsplaysoccer.com, copies a JWT and one
or more player IDs from publicly accessible browser/network data, then imports
that information into the portfolio soccer page for the current session only.

Why now: the current branch assumes a reCAPTCHA flow that cannot be validated on
this site without LPS cooperation. The JWT-based import path is already proven
by manual curl testing against the public LPS API.

## Success Criteria

- [ ] All tasks complete
- [ ] All tests passing
- [ ] Build succeeds
- [ ] No blockers
- [ ] Soccer page supports importing a JWT and player IDs without using LPS login
- [ ] Imported auth data is kept only for the current session
- [ ] Invalid or expired JWTs produce actionable user feedback
- [ ] ICS export continues to work for schedules fetched with imported auth data

## Tasks

### Task-001: Define Import Contract And Session Boundaries

**Priority**: High  
**Estimated Iterations**: 1

**Acceptance Criteria**:

- [ ] Replace the current product assumption of server-side LPS sign-in with a
      documented user-mediated import flow
- [ ] Define the MVP artifact contract as: pasted JWT plus one or more manual
      player IDs
- [ ] Define session scope as current-session-only storage using existing secure
      session infrastructure where appropriate
- [ ] Define the user-facing extraction workflow as guided DevTools/network
      instructions, not automation or scraping
- [ ] Confirm non-goals for MVP are documented to prevent scope creep

**Verification**:

```bash
# Documentation exists and is consistent with the implementation plan
test -f PRD.md && test -f PROGRESS.md
```

### Task-002: Implement Secure JWT Import Flow

**Priority**: High  
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] Remove the MVP dependency on `LPS_RECAPTCHA_SITE_KEY` for authenticated LPS
      schedule access
- [ ] Add a server endpoint and UI flow that accepts a pasted bearer JWT and one
      or more player IDs
- [ ] Validate JWT input format before making outbound LPS API requests
- [ ] Store imported auth data only for the current session using encrypted,
      HttpOnly cookie storage or an equivalent existing secure session mechanism
- [ ] Do not persist imported JWTs across browser restarts or long-term app state
- [ ] Add clear logout/clear-import behavior

**Verification**:

```bash
# Build succeeds after import flow changes
just build
```

### Task-003: Fetch, Normalize, And Export Authenticated Schedules

**Priority**: High  
**Estimated Iterations**: 1-2

**Acceptance Criteria**:

- [ ] Use the imported JWT as a bearer token when calling
      `GET /players/{id}/upcoming_games`
- [ ] Support one or more manually supplied player IDs in the MVP
- [ ] Normalize the live LPS response shape observed in manual testing
      (`UGameID`, `field_name`, `SchedGameDateTime`, `facilityName`, nested team
      objects, and null end time cases)
- [ ] Continue deduping and merging games across players where appropriate
- [ ] Keep ICS export working from authenticated schedule data fetched with the
      imported token
- [ ] Return actionable errors for 401/403 responses, malformed tokens, invalid
      player IDs, and upstream outages

**Verification**:

```bash
# Automated tests pass
go test ./...
```

### Task-004: Update Soccer UX, Guidance, And Tests

**Priority**: Medium  
**Estimated Iterations**: 1

**Acceptance Criteria**:

- [ ] Replace login modal copy and controls so the page explains the manual
      import workflow instead of prompting for LPS credentials
- [ ] Provide guided extraction instructions for JWT and player IDs using plain
      language
- [ ] Maintain accessibility for the new import form and feedback states
- [ ] Add or update tests for import validation, session lifecycle, upstream
      auth failures, and authenticated ICS export
- [ ] Update README or inline project docs for required environment variables and
      the new authenticated workflow

**Verification**:

- Manual test: Sign in on letsplaysoccer.com, paste a valid JWT and player ID,
  fetch schedules, then export ICS successfully
- Manual test: Paste an expired or invalid JWT and verify the UI explains how to
  refresh it
- Automated: `go test ./...`

## Technical Constraints

- Language: Go 1.23+
- Framework: Go net/http, Templ, HTMX, minimal client-side JavaScript
- Testing: `go test ./...`
- Style: `gofmt`, `go vet`, existing Templ and CSS conventions
- Security: No private LPS keys, no reCAPTCHA dependency, no browser scraping,
  no long-term JWT persistence
- Session model: Current-session-only authenticated import

## Architecture Notes

- Product assumption change for MVP: this feature no longer attempts server-side
      LPS sign-in from the portfolio site. The only supported authenticated path is
      a user-mediated import of artifacts the user can already access in their own
      browser session on letsplaysoccer.com.
- Design pattern: User-mediated artifact import instead of third-party auth proxy
- Key libraries: Go standard library, existing AES-GCM session cookie helpers,
  existing Templ page/partial structure
- Data flow:
  1. User signs in directly on letsplaysoccer.com
  2. User copies a JWT and one or more player IDs from public browser/network data
  3. Portfolio app validates and stores the imported auth data for the current session
  4. Server calls LPS `upcoming_games` endpoints with the imported bearer token
  5. Response is normalized into existing `Game` models and used for table + ICS output
- Integration points:
  - Existing soccer page and HTMX fragments
  - Existing encrypted session cookie infrastructure
  - LPS endpoint: `https://lps-api-prod.lps-test.com/players/{id}/upcoming_games`

## MVP Import Contract

- Imported artifact set:
      - One pasted LPS bearer JWT copied by the user from their own authenticated
            letsplaysoccer.com browser session
      - One or more manually entered player IDs supplied by the user
- Contract rules:
      - The portfolio app treats the JWT as an opaque bearer token and does not try
            to mint, refresh, or derive a new token
      - Player IDs are entered manually for MVP; the app does not attempt automatic
            player discovery
      - The imported JWT and player IDs are valid only for the active browser
            session and are discarded on explicit logout or session end
      - Any future import sources beyond pasted JWT plus manual player IDs are out of
            scope unless added by a later PRD task

## Session Boundaries

- Storage model: current-session-only storage using the existing encrypted
      session cookie infrastructure where appropriate
- Security boundary:
      - Keep imported auth data in secure server-managed session state backed by the
            existing cookie/session approach
      - Do not persist imported JWTs in localStorage, sessionStorage, URL params, or
            any durable database record
      - Do not preserve imported auth across browser restarts or long-lived
            remembered sessions
- Session end conditions:
      - User explicitly clears the imported auth state
      - Browser session ends and the session cookie is no longer present
      - Imported JWT expires or becomes invalid upstream

## User Extraction Workflow

- MVP guidance is instructional only. The product explains how the user can
      retrieve their own auth artifacts with browser tooling; it does not automate,
      scrape, or proxy this process.
- Expected user flow:
      1. Sign in directly on letsplaysoccer.com in the browser
      2. Open browser DevTools and inspect authenticated network requests
      3. Copy the bearer JWT from a request or response the user can access in that
             session
      4. Identify one or more relevant player IDs from the same visible network data
             or request paths
      5. Paste the JWT and manually enter the player IDs into the portfolio soccer
             page import flow
- Explicit exclusions:
      - No browser automation, extensions, injected scripts, or scraping helpers
      - No popup-based sign-in mediated by the portfolio app
      - No automatic token harvesting from cookies, storage, or another tab

## Non-Goals For MVP

- Direct username/password sign-in to LPS from this site
- Any flow that depends on LPS reCAPTCHA or LPS-managed site keys
- Automatic JWT extraction from another browser tab, popup, cookie jar, or local
  storage
- Automatic player discovery for MVP
- Background token refresh or long-lived remembered auth
- Mobile/native browser automation or extension-based import