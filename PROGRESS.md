# Progress Log

## Completed

- [x] Task-001: Reorder input sections — manual team IDs first
- [x] Task-002: Add loading/progress indicators to async soccer actions

## Current Iteration

- Iteration: 3
- Working on: Awaiting next assigned task
- Started: 2026-03-22T00:00:00Z

## Last Completed

- Task-002: Add loading and disabled states for in-scope async soccer actions
- Validation: `just generate`, `just test`, and `just build` ✅
- Key decisions:
  - Used `hx-disabled-elt` and inline `.htmx-indicator` spinners on the affected HTMX forms and buttons so duplicate requests are blocked without backend changes
  - Added small `main.js` helpers to toggle `aria-busy` and loading classes for the JWT modal submit button and the full-page Google connect/reconnect links
  - Preserved the existing `#soccer-auth-panel`, `#games-container`, `#loading-indicator`, and feedback targets so current handler behavior and swaps stay intact

## Blockers

- None

## Notes

- Ralph loop initialized
- PRD created: 2026-03-22
- Three tasks: reorder inputs, add loading indicators, visual polish
- Task-001 keeps the right-side import instructions in place and uses card-level section styling for clearer separation
