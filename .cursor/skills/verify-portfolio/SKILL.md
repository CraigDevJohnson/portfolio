---
name: verify-portfolio
description: Verify Craig Johnson's Go/Templ portfolio web UI, HTMX interactions, Soccer workflow, and loopback-only management preview with an isolated local server and Playwright CLI evidence.
---
<!-- markdownlint-disable MD013 -->

# Verify Portfolio

Use this skill whenever a portfolio change needs proof from the rendered browser surface. The primary surface is the public web UI. The same binary also exposes the Soccer tool and, only under the loopback-safe preview launch below, a mock management portal and closed Soccer fixture routes.

Read [`features/README.md`](./features/README.md) before driving a feature. Its entry points are the maintained verification contract.

## Launch

Run from the repository root in a dedicated terminal or long-lived command session. Pick a run ID once and keep it for every command. The default port is `18181`; set another unused loopback port before launch when a concurrent verification run already owns it.

```bash
export VERIFY_RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"
export VERIFY_PORT=18181
.cursor/skills/verify-portfolio/scripts/control-portfolio launch
```

`launch` intentionally remains in the foreground after the build. Keep that terminal or command session alive and run doctor, browser proof, and cleanup from another shell. In Codex, retain the yielded command-session ID until cleanup ends the server. A background child is not reliable here because the command runner reaps descendants when the launching shell exits.

`launch` runs the repository-authoritative `task build`, records the binary digest and current revision, and starts that binary with an empty environment except for `PATH` and these explicit settings:

```text
HOST=127.0.0.1
APP_BIND_ALL=false
MGMT_LOCAL_PREVIEW=true
PORT=$VERIFY_PORT
LOG_FORMAT=text
LOG_LEVEL=info
```

The helper refuses an occupied port or a live PID already registered to the run ID. Wait for the `server listening` log line, then require doctor to confirm readiness through both `/` and `/mgmt`. The server PID, port, and URL live under `output/playwright/verify-portfolio/$VERIFY_RUN_ID/state/`; build output, metadata, and server logs live under the sibling `evidence/` directory.

Two runs can coexist when both `VERIFY_RUN_ID` and `VERIFY_PORT` differ. Never drive a URL unless its run ID passes `doctor`; a shared or unowned server is not a valid target.

## Doctor

Run the read-only doctor before browser work and whenever the page, HTMX, or preview behavior looks wrong:

```bash
.cursor/skills/verify-portfolio/scripts/control-portfolio doctor
```

Doctor requires the recorded process to be the exact repository binary, confirms that PID owns the recorded loopback listener, compares the current binary digest with the launched build, and checks the Home and management-preview identities over HTTP. A digest mismatch means the checkout was rebuilt after launch: clean up and launch a new run instead of driving stale code.

## Drive

Use the repository's Playwright CLI wrapper and a run-scoped browser session:

```bash
export PORTFOLIO_PWCLI=/Users/craigjohnson/.codex/skills/playwright/scripts/playwright_cli.sh
export PORTFOLIO_PW_SESSION="portfolio-$VERIFY_RUN_ID"
export VERIFY_URL="$(<"output/playwright/verify-portfolio/$VERIFY_RUN_ID/state/url")"
```

The wrapper requires `npx`; verify it with `command -v npx >/dev/null 2>&1`. Open the real server, take a snapshot before using an element reference, and prefer the stable selectors documented in the feature map:

```bash
"$PORTFOLIO_PWCLI" --session "$PORTFOLIO_PW_SESSION" open "$VERIFY_URL/"
"$PORTFOLIO_PWCLI" --session "$PORTFOLIO_PW_SESSION" resize 1440 1000
"$PORTFOLIO_PWCLI" --session "$PORTFOLIO_PW_SESSION" snapshot
```

Snapshot again after navigation, an HTMX swap, a mobile-menu state change, or a detail-panel load. Do not use screen coordinates. The current handles are semantic landmarks, `aria-label` values, route paths, and repo-owned `data-*` attributes.

For the proven starter path, run:

```bash
.cursor/skills/verify-portfolio/scripts/prove-skills-search
```

That helper uses literal Playwright CLI operations to navigate from Home to Skills, enter `Terraform`, wait for the HTMX result, assert URL-backed state and the one visible skill, and capture the before/action/result evidence described below.

## Evidence

Keep proof under:

```text
output/playwright/verify-portfolio/$VERIFY_RUN_ID/evidence/<feature-id>/
```

Every browser proof must include:

- an accessibility snapshot and screenshot before the user action;
- the exact command transcript for the action;
- an accessibility snapshot and full-page screenshot of the resulting state;
- explicit assertions for the route, visible heading/status, and affected control state;
- browser warnings/errors and relevant browser request records;
- a side-effect check when the feature mutates anything.

Exercise a real user entry point, not a handler call or internal setter. Public portfolio and Skills proofs must use `/`, the visible navigation or CTA, and the production routes. The loopback preview is acceptable only for the production boundaries it replaces: Cognito/AWS and LPS/Google data. Label such artifacts `preview`; they prove the application behavior around that boundary, not live AWS, LPS, Google OAuth, or public connectivity.

The preview launch clears the inherited environment, binds only `127.0.0.1`, and initializes no portal AWS clients. Its LPS API base URL points at the in-process fake under `/__preview/lps`, so Soccer lookups never reach Let's Play Soccer. A browser that opens `/__preview/account/soccer-linked` acts as the preview's granted account on the Soccer page and its LPS routes, which answer from their own in-process fake LPS with a preview-only session key; other browsers still receive `401` from those routes. For preview mutations, retain the preview banner, the returned `Preview only` feedback, the server log, and the browser request list. Never infer that a live AWS or Google side effect was tested.

## Cleanup

Clean up only the instance and browser session owned by this run:

```bash
.cursor/skills/verify-portfolio/scripts/control-portfolio cleanup
```

Cleanup validates the recorded PID before signaling it; it never kills by process name. It closes only `portfolio-$VERIFY_RUN_ID`, stops only the recorded repository binary, and removes only the run's `state/` directory. It deliberately retains `evidence/`, including build output and server logs.

After cleanup, confirm proof survival:

```bash
test -d "output/playwright/verify-portfolio/$VERIFY_RUN_ID/evidence"
find "output/playwright/verify-portfolio/$VERIFY_RUN_ID/evidence" -type f -print
```

Run cleanup after every failed iteration so no broken attempt strands a server, port, or browser session.

## Helpers

The shipped helpers are executable:

```bash
.cursor/skills/verify-portfolio/scripts/control-portfolio launch
.cursor/skills/verify-portfolio/scripts/control-portfolio doctor
.cursor/skills/verify-portfolio/scripts/prove-skills-search
.cursor/skills/verify-portfolio/scripts/prove-soccer-planner
.cursor/skills/verify-portfolio/scripts/prove-soccer-linked
.cursor/skills/verify-portfolio/scripts/prove-soccer-past-results
.cursor/skills/verify-portfolio/scripts/prove-soccer-access
.cursor/skills/verify-portfolio/scripts/prove-soccer-google-account
.cursor/skills/verify-portfolio/scripts/prove-soccer-history-notice
.cursor/skills/verify-portfolio/scripts/prove-portal-access
.cursor/skills/verify-portfolio/scripts/prove-site-account
.cursor/skills/verify-portfolio/scripts/control-portfolio cleanup
```

`control-portfolio` owns lifecycle and doctor checks. `prove-skills-search` is the reference browser proof and records its command transcript alongside snapshots, screenshots, assertions, request records, and console diagnostics. `prove-soccer-planner` drives the choice-first Soccer journey against the preview's fake LPS; its steps live in `scripts/soccer-planner/`, and it needs a fresh launch. `prove-soccer-linked` drives the linked-player journey as the preview's granted account from `/__preview/account/soccer-linked`: the .ics choice, an import of a fake token through the real dialog, player and team choices, the deduplicated schedule, deselection across refetch and a team change, two `.ics` downloads, and the recovery when LPS withdraws access; its steps live in `scripts/soccer-linked/`, and it also needs a fresh launch. `prove-soccer-past-results` reviews scored past games in Google mode from both schedule sources as the preview's granted account from `/__preview/account/soccer-linked`: Team IDs, then linked players after a fake-token import. A public Team ID lookup has no Google mode and shows no past results. It requires the scored past games newest first and selected, the postponed game without a score left out, the count and select-all, keyboard deselection kept across a refetch and output switching, no past results or result Sync on the .ics output, a 390px fit, and no Google Calendar request; its steps live in `scripts/soccer-past-results/`, and it tolerates a launch the other Soccer proofs already used. `prove-soccer-access` compares the signed-out, ungranted, and granted Soccer page through the preview account fixtures, then requires `401` for direct private Soccer requests from the production entry while public Team ID lookup still answers; its steps live in `scripts/soccer-access/`. `prove-soccer-google-account` chooses Google output on the Google preview fixtures and requires the suggested site account, the connected account Google reported, inert account controls, a 390px fit, `401` for direct Google requests from the production entry, and the paused-destination card that asks for a new writable calendar; its steps live in `scripts/soccer-google-account/`. `prove-soccer-history-notice` compares the signed-out, granted, and granted-with-history Soccer fixtures, requires the import dialog's indefinite-history notice and its `history_notice` field only where durable collection is wired, a 390px fit, and `401` for a direct disclosed import from the production entry; its steps live in `scripts/soccer-history-notice/`. `prove-portal-access` checks the preview dashboard, confirms the retired `/login`, `/callback`, and `/logout` routes return `404`, and captures the management access-denied page at desktop and compact widths. `prove-site-account` is the one site identity flow: it follows every public page through shared navigation, follows the preview's signed-out Sign in entry to the landing with its return destination, checks the signed-in account fixture and `Sign out` in both navigation regions, and then runs the Soccer grant-state and direct-request steps from `scripts/soccer-access/`; its own steps live in `scripts/site-account/`. The preview has no site Cognito, so it never completes sign-in and does not prove sign-in return; the `internal/app` route tests with fake Cognito do. Read the scripts only when changing the verification harness; normal use should not require reverse-engineering them.
