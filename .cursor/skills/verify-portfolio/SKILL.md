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

The preview launch clears the inherited environment, binds only `127.0.0.1`, and initializes no portal AWS clients. For preview mutations, retain the preview banner, the returned `Preview only` feedback, the server log, and the browser request list. Never infer that a live AWS or Google side effect was tested.

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

Both shipped helpers are executable:

```bash
.cursor/skills/verify-portfolio/scripts/control-portfolio launch
.cursor/skills/verify-portfolio/scripts/control-portfolio doctor
.cursor/skills/verify-portfolio/scripts/prove-skills-search
.cursor/skills/verify-portfolio/scripts/control-portfolio cleanup
```

`control-portfolio` owns lifecycle and doctor checks. `prove-skills-search` is the reference browser proof and records its command transcript alongside snapshots, screenshots, assertions, request records, and console diagnostics. Read the scripts only when changing the verification harness; normal use should not require reverse-engineering them.
