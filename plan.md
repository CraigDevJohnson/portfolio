# Archived plan: internal/app extraction (historical)

This document is retained for historical context only.
It is **not** an actionable implementation plan anymore.

The refactor described here has already been completed.

Current source of truth:

- `PRD.md`
- `PROGRESS.md`
- `README.md`
- `.github/copilot-instructions.md`

Current runtime structure summary:

- `cmd/server/main.go` is the thin entry point.
- `internal/app` handles route registration and dependency wiring.
- Domain packages live in:
  - `internal/config`
  - `internal/soccer`
  - `internal/google`
  - `internal/lps`
  - `internal/schedule`
  - `internal/session`
  - `internal/httpx`
  - `internal/portfolio`
- Shared models live in `types/types.go`.
- Templ and static assets live under `cmd/web`.

If new work is planned, create a new task-specific plan instead of reusing this archived file.
