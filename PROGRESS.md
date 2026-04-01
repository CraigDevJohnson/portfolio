# Progress Log

## Completed

- [x] Task-001: Extract `internal/config` package
- [x] Task-002: Introduce App struct — convert all test files to use App struct pattern
- [x] Task-003: Eliminate portfolio wrapper files
- [x] Task-004: Eliminate schedule/time/ICS wrapper files
- [x] Task-005: Eliminate LPS client and helpers wrapper files (commits: f622be4, 988849b)
- [x] Task-006: Expand `internal/lps` with resolver, decode, errors, and JWT helpers (commits: b6a0138, 017b126)
- [x] Task-007: Extract `internal/soccer` handlers (commit: 1d1628c)
- [x] Task-008: Extract `internal/google` (commits: 2acf530, 8e6d13c)
- [x] Task-009: Migrate tests to domain packages
- [x] Task-010: Clean internal/app to routing-only
- [x] Task-011: Update docs and validate final structure
- [x] Task-011B: Archive stale pre-refactor references in plan.md

## Last Completed

- Portfolio polish and maintainability sweep across frontend, accessibility,
  backend cleanup, and docs refresh
- Verification rerun on current HEAD after the refactor sweep
- Key decisions:
  - Consolidated shared UI primitives into `cmd/web/static/css/styles.css`
  - Standardized JS accessibility behavior for modals, filters, and focus
  - Kept soccer login on JWT import with automatic player discovery
  - Simplified soccer/google coordination without changing external behavior

## Current Iteration

- Iteration: 15
- Working on: Complete — frontend polish, accessibility hardening, backend cleanup
- Started: 2026-03-31T00:00:00Z

## Blockers

- None

## Final Verification Evidence (Iteration 15)

- Date: 2026-03-31
- Commands:
  - `just ci` -> ✅ exit code 0
  - `just test` -> ✅ exit code 0
  - `just build` -> ✅ exit code 0
  - `just vet` -> ✅ exit code 0
- Notes:
  - Responsive, WCAG, modal/table, backend simplification, and architecture
    follow-up changes were revalidated on current HEAD.
  - The soccer flow remains JWT import with server-side player discovery,
    schedule fetch, ICS download, and optional Google Calendar add.
