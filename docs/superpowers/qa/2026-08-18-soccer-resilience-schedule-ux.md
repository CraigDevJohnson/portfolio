# Soccer resilience and schedule UX QA — 2026-08-18

Status: PASS for encrypted workflow persistence, local selection restoration,
responsive match-board behavior, and fixture-backed connection/action states.

## Source verification

| Command | Result |
| --- | --- |
| `task generate` | PASS |
| `task fmt` | PASS |
| `task lint` | PASS — `0 issues. All linters passed!` |
| `task test` | PASS |
| `task build` | PASS |
| `node --check cmd/web/static/js/main.js` | PASS |
| `git diff --check` | PASS |

## Workflow-state verification

Go handler/session tests verify:

- backward-compatible encrypted-session decoding;
- normalized manual and imported workflow snapshots;
- selected-player and discovered-team persistence;
- confirmed-team persistence and schedule reconstruction on `/soccer`;
- Google-return reconstruction of players, teams, checked choices, and games;
- preservation of player/team choices when schedule refresh fails;
- intentional reset behavior for a new import, logout, invalid input, and an
  expired session; and
- unchanged secure cookie attributes.

The browser exercised the versioned
`portfolio:soccer:selection:<team-fingerprint>` session-storage payload. A
partial selection survived reload and a page round trip to the connected
Google fixture, then cleared on `soccer-workflow-reset`. The stored JSON
contained only version `1` and upcoming/past opaque game-ID arrays.

## Responsive and state evidence

Every Soccer evidence case asserted all of the following before capture:

- no page-level horizontal overflow;
- no descendant schedule region with `overflow-x: auto` or `scroll`;
- no sticky `.games-section-top`; and
- readable semantic match rows/cards at the active width.

| State | Widths | Evidence | Result |
| --- | --- | --- | --- |
| Combined upcoming/past | 390, 768, 1119, 1121, 1440 | `.playwright-cli/qa-soccer-combined-{width}.png` | PASS |
| Player selection | 390, 1440 | `.playwright-cli/qa-soccer-players-{width}.png` | PASS |
| Team selection | 768 | `.playwright-cli/qa-soccer-teams-768.png` | PASS |
| Google connected | 1119, 1121 | `.playwright-cli/qa-soccer-connected-{width}.png` | PASS |
| Add success | 1440 | `.playwright-cli/qa-soccer-add-success-1440.png` | PASS |
| Sync error | 390 | `.playwright-cli/qa-soccer-sync-error-390.png` | PASS |
| No games | 390 | `.playwright-cli/qa-soccer-no-games-390.png` | PASS |
| Expired session | 1440 | `.playwright-cli/qa-soccer-expired-1440.png` | PASS |
| Loading | 768 | `.playwright-cli/qa-soccer-loading-768.png` | PASS |
| Manual-only regression | 390, 1440 | `.playwright-cli/qa-soccer-manual-{width}.png` | PASS |

Additional focused evidence:

- `.playwright-cli/page-2026-08-18T13-38-10-373Z.png` — full-width connected
  upcoming/past board with opaque, non-sticky toolbars
- `.playwright-cli/page-2026-08-18T13-38-44-133Z.png` — stacked mobile match
  cards with zero overflowing rows
- `.playwright-cli/page-2026-08-18T13-39-56-601Z.png` — one whole-row primary
  player highlight and no sub-team label

At 1440px the Review & Output section measured 1232px wide, the results panel
1166px, and each match row 1132px. At 390px the document and viewport were both
390px, no match row overflowed, and no horizontal scroll region existed.
Upcoming cards label the last field `Season`; past cards label it
`Season / result`, so upcoming games do not imply a blank result.

## Connection and action feedback

- LPS and Google are explicit Connections cards above the numbered workflow.
- Visible status copy says `Not imported`, `Imported for this session`,
  `Not connected`, or `Connected to ...`; state is not conveyed by color alone.
- Review & Output is stage 4 and spans the page below stages 1–3.
- Add and Sync each target the live feedback region immediately below their
  own toolbar.
- Busy buttons switch to count-aware `Adding N games…` or
  `Syncing N results…` labels, remain disabled/`aria-busy`, and restore their
  original selection-aware state after completion.
- Upcoming and past use semantic lists, not an eight-column table, and retain
  date/time, matchup, location, season, parsed result, and 44px checkbox targets.

## Validation boundary

No real LPS credential, Google OAuth authorization, calendar write, calendar
disconnect, or external account mutation was performed. Those paths were
verified with local preview fixtures plus stubbed LPS/Google handler and OAuth
callback tests. The evidence therefore proves local UI/state handling and
server contracts, not live third-party availability.

