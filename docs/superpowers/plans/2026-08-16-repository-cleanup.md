# Repository Cleanup Plan

> **Execution note:** This plan applies directly to the existing dirty workspace because the cleanup targets are local evidence, caches, duplicated snapshots, and Codex turn-history objects. A second worktree would duplicate those targets. Existing source edits must not be reset, staged, or committed.

**Goal:** Reduce the repository from roughly 17 GiB to a practical development size without losing current source work or the authoritative Final14 browser evidence.

**Retention boundary:** Keep all live source, configuration, tracked history, the Task12 and Task13 accepted source snapshots/review documents, and the complete `output/playwright/task-13/runs/2026-08-16-final-14` run. Remove only regenerable or superseded local artifacts.

## 1. Recovery and baseline

- Record the working-tree status, disk usage, Git object statistics, Codex turn-diff refs, and exact cleanup targets.
- Create a source-only recovery archive in `/private/tmp`, excluding secrets, Git objects, test evidence, tool caches, generated files, and binaries.
- Verify the archive opens and contains current source paths but not `.env`.

## 2. Prevent recurrence

- Add `.playwright-cli/` and `/output/playwright/` to `.gitignore`.
- Preserve the accepted Final14 run on disk even though browser evidence is ignored going forward.

## 3. Remove superseded artifacts

- Remove `.playwright-cli/` browser sessions and traces.
- Remove all older `output/playwright/*` task directories and all Task13 runs except Final14.
- Remove duplicated `.superpowers` source snapshots before Task12, while retaining `task-12-after`, `task-13-after`, reports, ledgers, manifests, review packages, and progress history.
- Remove the generated `portfolio-server` binary if present.

## 4. Compact internal Git history

- Record the eight `refs/codex/turn-diffs/*` names and object IDs.
- Delete only that internal ref namespace; preserve branches, remotes, session refs, tags, and the stash.
- Expire unreachable reflog entries and run an immediate Git garbage collection so deleted browser evidence is no longer retained in packfiles.

## 5. Verify

- Confirm the authoritative Final14 result, screenshot, and artifact counts still exist.
- Confirm source status was preserved, nothing was staged, and the diff has no whitespace errors.
- Run the repository formatter, tests, linter, and build using Taskfile commands; record the known lint loader result honestly if it persists.
- Remove the regenerated `portfolio-server` after the successful build.
- Re-measure repository and Git sizes, verify Git object integrity, and report exact reclaimed space and the recovery archive path.

