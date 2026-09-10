<!-- markdownlint-disable MD013 -->
# Soccer Scraper Decommission and Repository Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire the standalone Soccer scraper only after the replacement production portfolio is proven, preserve a verified recovery set, remove its dedicated AWS resources, archive and ultimately delete its GitHub repository, and offer separately approved local-checkout cleanup without affecting the surviving portfolio Soccer workflow.

**Architecture:** The surviving `portfolio` repository owns the decommission evidence and runbook. Execution is gate-driven: prove production and close the Amplify rollback dependency, preserve source and data, freeze deployment and scheduled work, observe a reversible quiet period, cut off the final browser caller, delete only exact dedicated resources, archive GitHub for 90 days, permanently remove the GitHub repository after a fresh deletion review, and treat local-checkout cleanup as an optional separately approved follow-on.

**Tech Stack:** Go, AWS Lambda, EventBridge Scheduler, DynamoDB, SNS, Cognito Identity, IAM, CloudWatch Logs, GitHub Actions, GitHub CLI, AWS CLI, Task, jq, Markdown

**Spec:** `docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md`

## Global Constraints

- This document is a plan, not authorization for any AWS, Cloudflare, GitHub, subscriber-message, or filesystem mutation.
- Do not begin the reversible shutdown until the replacement production cutover and its complete seven-day observation gate pass.
- Keep the standalone service available while Amplify remains the approved production rollback origin; its Vue Soccer bundle directly invokes `soccer_schedule_scraper`.
- Before shutdown, formally close Amplify as an approved production rollback target. Acceptance of broken legacy Soccer behavior is not a substitute for closing that rollback dependency.
- Use `portfolio-deployer` only for the existing production-cutover gate. First provision a bounded read-only auditor, use it to create the private manifest, and only then separately approve a temporary non-root `soccer-retirement` operator. Account-scope discovery/list permissions may use `Resource: "*"` only when AWS requires it and must remain read-only, while every mutation is limited to exact private-manifest resources and guarded by explicit conditions/denies. Simulate allowed mutations and denial outside the manifest, then remove the mutable assignment after teardown. Never fall back to the root user.
- Use `soccer-retirement-auditor` only as a time-bounded, read-only assignment/session for inventory, absence, backup-state, and cost verification. It may list/get/describe only, has no mutation permission, and is removed/read back absent after each approved checkpoint; do not retain a dedicated audit IAM resource for the backup's lifetime.
- Do not use `tofu destroy`, `aws-nuke`, wildcard deletion, prefix-only deletion, unresolved globs, or automatic approval. The standalone resources are not owned by the replacement OpenTofu roots.
- Treat the Scheduler `default` group and AWS-managed IAM policies as shared. Detach managed policies from dedicated identities; never delete the managed policies.
- Never commit or print subscriber email endpoints, access-key identifiers, secret values, raw Lambda environment values, or raw CloudWatch logs. The log group may contain email addresses.
- Preserve the dirty checkout at `/Users/craigjohnson/repos/soccer_scraper`, the clean checkout at `/Users/craigjohnson/Projects/soccer_scraper`, their complete `.git` directories, reflogs, Codex refs, and working-tree state before either local path is moved.
- Keep `updates` and PR #37 intact until their metadata and refs are backed up. Do not merge unfinished work merely to preserve it.
- Every external message and every mutable surface has a separate approval gate: production rollback closure, subscriber notice, GitHub Actions freeze, IAM-key deactivation, schedules, Cognito, Lambda, SNS, DynamoDB, logs, IAM identities, GitHub secrets, PR closure, GitHub archive, GitHub deletion, and each local Trash operation.
- Prefer reversible changes first. A failed read-back or unexpected consumer stops the plan; it never authorizes broad cleanup.
- Store only redacted logical labels, counts, and cryptographic hashes in Git; the exact account, pool, topic, credential, and deletion manifest stays in the encrypted owner-only archive unless public disclosure is separately approved. Store recovery artifacts under `/Users/craigjohnson/Documents/Codex/decommission-archives/soccer_scraper` only after re-verifying FileVault and owner-only permissions.
- Irreversible AWS deletion requires a second complete encrypted recovery copy at an exact destination approved during execution. Verify the artifacts themselves there, not only a checksum. Time Machine is not currently configured, so the local archive alone is insufficient; the same independently verified copy is also required for permanent GitHub deletion.
- Retain the recovery set for at least one year after permanent GitHub deletion. Deleting that recovery set requires a new exact approval.
- Retain the final DynamoDB on-demand backup as an explicit teardown exception for at least one year after permanent GitHub deletion. Record its owner, monthly storage-cost estimate, and exact review/deletion date; deleting it requires a new exact approval.
- Use `Taskfile.yaml` as the portfolio command source of truth and run the repository-required checks for portfolio changes.
- Treat every SHA, identity count, item count, metric total, and patch digest in the planning snapshot as a revalidation expectation, not a permanent invariant. Drift must be explained and reconciled before execution.

---

## Planning Snapshot — 2026-08-30

Current lifecycle: **Active — high confidence**. Owner-directed disposition: **pending decommission**. Deletion recommendation: **No (not eligible yet)**. Deletion execution readiness: **blocked**.

| ID | Status | Evidence |
|---|---|---|
| F-001 | Verified | `craigdevjohnson.com` still serves the Amplify Vue application and its Soccer bundle invokes `soccer_schedule_scraper` through a dedicated unauthenticated Cognito identity pool. |
| F-002 | Verified | `dev.craigdevjohnson.com` uses the independent Go portfolio implementation. The replacement production root is configured but has not cut public production traffic over. |
| F-003 | Verified | Two enabled Scheduler jobs account for exactly 14/60/180 Lambda invocations over 7/30/90 days. No excess frontend invocation was observed in those windows. |
| F-004 | Verified | Ten SNS topics retain 11 confirmed email subscriptions. DynamoDB contains ten 2.6 KB schedule records; TTL and PITR are disabled and no backup exists. |
| F-005 | Verified | The AWS resource set is dedicated except for the Scheduler `default` group and AWS-managed IAM policies. No API Gateway integration, Function URL, Lambda resource policy, event-source mapping, EventBridge Rule, alarm, or log subscription exists. |
| F-006 | Verified | GitHub `CraigDevJohnson/soccer_scraper`, repository ID `940342051`, has open PR #37 and branch `updates`; pushes to unprotected `main` automatically deploy using one active key for IAM user `github_soccer_scraper`. |
| F-007 | Verified | The second local clone has unstaged `go.mod`/`go.sum` changes. A mirror or Git bundle alone would not preserve both working trees, reflogs, or non-commit Codex refs. |
| G-001 | Blocking | The production observation recorder currently probes a Go Tailwind asset on the Vue Amplify rollback origin, which returns 404. The production observation gate cannot pass until a separate production-plan correction uses a real recorded Vue asset. |
| G-002 | Blocking for permanent deletion | GitHub Packages, Projects V2, and wiki content require fresh verification or export with suitable access before permanent repository deletion. |
| G-003 | Blocking for permanent deletion | FileVault is currently enabled, but no Time Machine destination or second approved encrypted recovery destination is configured. |

Expected lifecycle transitions:

1. Remain **Active** through production cutover and the seven-day Amplify rollback window.
2. Become **Stale Candidate** after production is independent, deployment/schedules are frozen, and the owner confirms notification retirement.
3. Become **Stale and Deletion-Eligible** only after the quiet-window and caller-cutoff gates pass, recovery is verified, and a fresh consumer/retention review finds no material unknowns.
4. Archive GitHub for 90 days before considering permanent deletion.

---

### Task 1: Pass the production exit gate

**Files:**

- Read: `docs/superpowers/plans/2026-08-21-production-lambda-cutover.md`
- Read: the source-SHA release record under `docs/deployment/evidence/releases/`
- Read: `docs/deployment/evidence/production-observation.jsonl`
- No decommission files or live resources change in this task

**Interfaces:**

- Consumes: corrected production observation contract, deployed `portfolio-lambda-prod`, public cutover, and complete production observation evidence
- Produces: a signed-off prerequisite record proving the old service is no longer required for production or rollback

- [ ] **Step 1: Require the production-plan correction before cutover**

Verify that the separate production change replaces the hard-coded Amplify `/static/css/tailwind.css` rollback probe in `scripts/record-lambda-observation.sh`, its checker, and its contract test with an exact asset captured from the Vue deployment. Correct the production cutover plan itself to use top-level `.image.uri`, remove any direct call to internal task `_lambda-identity-check`, and require merged evidence rather than only a local zero exit. Require the approved production operator to have the read-only Amplify and Cognito Identity permissions needed by production Task 2.

Run the focused observation contract test and the authoritative repository gate from the corrected production branch:

```bash
cd /Users/craigjohnson/Projects/portfolio
test -x .tools/tailwind/tailwindcss || task install-tailwind
sh tests/lambda-observation-window.sh
task ci
git diff --check
```

Expected: all commands pass and the corrected test proves both the Go production asset and the real Vue rollback asset independently.

- [ ] **Step 2: Prove the public production release**

```bash
cd /Users/craigjohnson/Projects/portfolio
release_sha=$(curl --fail --show-error --silent https://craigdevjohnson.com/healthz | jq -er .revision)
release_record="docs/deployment/evidence/releases/${release_sha}.json"
test -f "$release_record"
jq -e --arg sha "$release_sha" '
  .source_sha == $sha and
  .production != null and
  .production.healthz_revision == $sha and
  (.production.live_alias_target | type == "string") and
  (.image.uri | contains("@sha256:"))
' "$release_record"
```

Verify `/`, `/soccer`, `/healthz`, the versioned CSS, a representative image, OAuth connect/callback, calendar add, and result sync through the public apex and the approved `www` behavior. The public `/soccer` response must contain the Go/HTMX route contract and must not load the Vue `SoccerView` chunk or an AWS Lambda browser client.

- [ ] **Step 3: Pass the seven-day production observation gate**

```bash
cd /Users/craigjohnson/Projects/portfolio
release_sha=$(curl --fail --show-error --silent https://craigdevjohnson.com/healthz | jq -er .revision)
release_record="docs/deployment/evidence/releases/${release_sha}.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
EVIDENCE_FILE=docs/deployment/evidence/production-observation.jsonl
task lambda-prod-observation-gate \
  RELEASE_RECORD="$release_record" \
  EVIDENCE_FILE="$EVIDENCE_FILE"
```

The public task invokes the internal identity check itself; do not call `_lambda-identity-check` directly. A local zero exit is necessary but insufficient. Require at least eight passing samples, at least 604800 elapsed seconds, no gap over 26 hours, stable release coordinates, healthy alarms and rollback origin, and successful cutover/day-seven OAuth, Secure-cookie, add, and sync evidence. Also require `production.observation_completed_at` to be non-null, the evidence JSON/JSONL to be committed, hosted checks to pass, and the evidence PR to be merged into current `origin/main` as required by the production cutover plan.

- [ ] **Step 4: Close the Amplify rollback dependency**

Present a separate approval stating that Amplify is no longer the production rollback target. Record whether Amplify is disabled/retired under its own legacy-platform plan or retained only as historical infrastructure with broken legacy Soccer behavior explicitly accepted.

Do not proceed while either of these remains true:

- production traffic can return to Amplify as an approved rollback; or
- the old Cognito browser role must retain `lambda:InvokeFunction` for that rollback.

- [ ] **Step 5: Prove both surviving environments are independent**

Search only executable portfolio source and infrastructure, excluding historical plans/evidence:

```bash
cd /Users/craigjohnson/Projects/portfolio
if rg -n 'soccer_schedule_scraper|soccer-schedules|soccer-schedule-' \
  cmd internal infra/lambda go.mod go.sum; then
  echo "Unexpected standalone dependency remains" >&2
  exit 1
fi
```

Read back both portfolio execution roles and use IAM policy simulation to require `implicitDeny` or `explicitDeny` for invoking the standalone Lambda. Verify public dev and production `/healthz` revisions and complete a representative `/soccer` fetch/download workflow in both environments.

**Approval gate:** Present the production release SHA/digest/version, observation evidence, Amplify rollback disposition, public route results, and IAM simulation results. Approval starts Task 2 only; it does not approve a live shutdown.

---

### Task 2: Capture exact retirement inventory and a verified recovery set

**Files:**

- Create: `docs/deployment/evidence/soccer-scraper-retirement-inventory.json`
- Create: `scripts/check-soccer-scraper-retirement-evidence.sh`
- Create: `tests/soccer-scraper-retirement-evidence.sh`
- Create outside Git: `/Users/craigjohnson/Documents/Codex/decommission-archives/soccer_scraper/`
- Preserve: `/Users/craigjohnson/Projects/soccer_scraper`
- Preserve: `/Users/craigjohnson/repos/soccer_scraper`

**Interfaces:**

- Consumes: merged Task 1 production evidence and fresh read-only AWS/GitHub/local inventory
- Produces: a redacted public evidence record, encrypted exact manifest, independently restorable source/runtime snapshots, GitHub metadata export, and a tested pre-shutdown DynamoDB recovery point

- [ ] **Step 1: Establish a clean evidence branch and guard the portfolio checkout**

Fetch current `origin/main` only after the production evidence PR merges. Record branch, upstream, divergence, all worktrees, status, and staged paths. If the saved portfolio checkout is not clean, create an isolated worktree; do not absorb or overwrite unrelated changes.

```bash
cd /Users/craigjohnson/Projects/portfolio
git fetch origin main
git status --short --branch
git worktree list --porcelain
test -z "$(git status --porcelain)"
git switch --create codex/soccer-scraper-retirement origin/main
```

- [ ] **Step 2: Create the owner-only archive root**

```bash
archive_root=/Users/craigjohnson/Documents/Codex/decommission-archives/soccer_scraper
test "$(fdesetup status)" = "FileVault is On."
install -d -m 700 "$archive_root"
test "$(stat -f '%Lp' "$archive_root")" = "700"
```

- [ ] **Step 3: Draft the private action manifest and provision scoped operators**

**Approval gate A:** First create/assign only a time-bounded `soccer-retirement-auditor` session. Use its read-only list/get/describe access to capture fresh inventory privately; do not print sensitive fields. Build the first private action manifest from that inventory and derive the mutation action/resource matrix from it, then remove/read back the audit assignment absent. Discovery/list actions that AWS cannot resource-scope may use account scope only as read-only actions; all backup, restore, update, detach, and delete actions must name exact resources and use available condition keys or explicit denies to guard unrelated workloads.

**Approval gate B:** Only after the draft manifest is reviewed, create/assign the temporary mutable `soccer-retirement` policy. Expose the two identities only through `AWS_PROFILE=soccer-retirement` and `AWS_PROFILE=soccer-retirement-auditor`; never widen `portfolio-deployer`.

Use IAM simulation and harmless read-backs to require the mutable operator's reviewed access for Amplify/Cognito discovery, Lambda code download, DynamoDB backup/restore/delete, Scheduler, SNS, Cognito Identity, IAM, Lambda, and Logs operations, while denying unrelated mutations. Require the auditor to permit only list/get/describe and billing/cost read-back and to deny every mutation. Record policy hashes and removal procedures privately.

- [ ] **Step 4: Snapshot both local clones transactionally**

Stop editors and agents using either checkout. Before copying, capture each path's device/inode, branch/HEAD, upstream, all refs and reflog tips, index checksum, staged patch, unstaged binary patch, status, and inventory of tracked, untracked, and ignored files. Explicitly include the current ignored `.vscode/tasks.json` and `bin/scraper`, with modes, sizes, and hashes; record exact Codex ref names rather than a count.

```bash
test ! -e "$archive_root/Projects-soccer_scraper"
test ! -e "$archive_root/repos-soccer_scraper"
ditto --rsrc --extattr --acl \
  /Users/craigjohnson/Projects/soccer_scraper \
  "$archive_root/Projects-soccer_scraper"
ditto --rsrc --extattr --acl \
  /Users/craigjohnson/repos/soccer_scraper \
  "$archive_root/repos-soccer_scraper"
```

Repeat the complete source inventory immediately after copying and fail on drift. Treat the planning-snapshot patch digest and SHAs as expected values to reconcile freshly. After Task 4 freezes deployment, repeat this comparison and refresh every changed snapshot, patch, and manifest before key deactivation.

- [ ] **Step 5: Preserve all GitHub source and metadata surfaces**

```bash
test ! -e "$archive_root/github-mirror.git"
git clone --mirror \
  git@github.com:CraigDevJohnson/soccer_scraper.git \
  "$archive_root/github-mirror.git"
git -C "$archive_root/github-mirror.git" fetch origin \
  '+refs/pull/*/head:refs/pull/*/head' \
  '+refs/pull/*/merge:refs/pull/*/merge'
git -C "$archive_root/github-mirror.git" fsck --full
```

Freshly validate `main`, `updates`, PR #37 head and merge refs, both local-only states, and every named Codex ref. Inventory Git LFS; if used, run `git lfs fetch --all`, otherwise record verified absence. Mirror the wiki or record verified absence. Download user-uploaded attachments referenced by repository Markdown, issues, and PRs.

Export timestamped API JSON for repository settings, branches/tags, issues/PRs and all discussion/review surfaces, Actions, environments/deployments, Pages/releases, integrations/keys/collaborators, security/SBOM, traffic, forks/stars/watchers, packages, and Projects V2. Preserve only the three live secret names. Record accurately that run `22026488614` logs return HTTP 410, artifacts are zero, retained job steps are empty, Go `1.24.0` is configuration-derived, and the workflow has one deploy job with no test/lint/vet job.

If a package exists, map consumers/downloads, export versions/manifests/attestations, and require a separate retain/deprecate/delete decision. If a user-level Project references the repository, require a separate unlink/archive/retain decision. Repository deletion remains blocked until each surface is verified absent or its disposition is complete.

- [ ] **Step 6: Capture a restorable AWS configuration set**

Using `soccer-retirement`, save the deployed Lambda ZIP from its signed code location and verify its SHA-256 against `CodeSha256`; if reproducibly rebuilding instead, require the rebuilt ZIP to match. Export exact Lambda configuration without displaying environment values, Scheduler target JSON, Cognito pool/role configuration, every IAM trust/inline/attached-policy document, all versions of the customer-managed Cognito policy, and current inbound-integration discovery results. Keep these exact artifacts only in the encrypted archive.

Immediately before and after eventual deletion, repeat discovery for API Gateway REST/HTTP integrations, Function URL, Lambda resource policy, event-source mappings, EventBridge Rules/targets, and any other inbound integration. A stale integration blocks teardown even if the Lambda itself can be deleted.

- [ ] **Step 7: Create the pre-shutdown DynamoDB recovery point**

This is a recovery point, not the final deletion backup. **Approval gate:** `dynamodb create-backup` is a live AWS mutation. Present its exact table and release-derived name before running it with `soccer-retirement`.

Wait for `AVAILABLE`; record its private ARN, source ARN, item count/size, owner, expected storage cost, and retention date. Under a second approval, restore it to an exact temporary table, compare key schema, TTL configuration, non-secret content digest, and item count, then separately approve deletion of that temporary table. If restore rehearsal is declined or fails, call the backup untested and block irreversible teardown.

- [ ] **Step 8: Write separate private and public manifests**

The encrypted manifest contains exact AWS identifiers, configurations, recovery locations, private hashes, and deletion targets. The committed inventory contains only schema version, timestamps, production proof, logical resource labels, counts, redacted suffixes, non-secret digests, verified absences, GitHub public metadata, and recovery-owner/retention fields. It must not contain account/pool/topic identifiers, endpoints, access-key identifiers, raw environment values, or raw logs.

- [ ] **Step 9: Implement evidence contracts before relying on them**

Write `tests/soccer-scraper-retirement-evidence.sh` first with failing fixtures for malformed inventory, duplicate timestamps, insufficient samples, excessive gaps, incomplete `coverage_end_at`, non-zero post-baseline invocations, and missing receipt fields. Implement `scripts/check-soccer-scraper-retirement-evidence.sh` to validate inventory JSON, quiescence JSONL, 48-hour cutoff JSONL, and the teardown receipt. The checker must reject duplicate keys/timestamps, email addresses, `AKIA`/`ASIA` access-key IDs, standalone 12-digit AWS account IDs, AWS ARNs, Cognito pool UUID formats, SNS endpoint/topic identifiers, and exact actor/provider identifiers. Tests must include one rejecting fixture for every pattern.

```bash
sh tests/soccer-scraper-retirement-evidence.sh
git diff --check
```

- [ ] **Step 10: Rehearse recovery, create a detached manifest, and commit**

Restore the mirror and both snapshots into fresh `mktemp -d` directories, run `git fsck --full`, validate all refreshed refs/SHAs, recreate the dirty worktree, and compare ignored-file hashes. Build a detached checksum manifest containing relative path, mode, size, and SHA-256 for every recovery artifact except itself; hash or sign that manifest separately so it is not self-referential.

Run the repository secret scanner against the entire staged evidence diff, not just the inventory. Format only named files, inspect the full diff, and stage only the inventory, checker, and contract test.

```bash
git add docs/deployment/evidence/soccer-scraper-retirement-inventory.json \
  scripts/check-soccer-scraper-retirement-evidence.sh \
  tests/soccer-scraper-retirement-evidence.sh
sh scripts/check-soccer-scraper-retirement-evidence.sh --inventory \
  docs/deployment/evidence/soccer-scraper-retirement-inventory.json
git diff --cached --check
git commit -m "docs: record soccer scraper retirement inventory"
```

Before any irreversible AWS deletion, copy the complete source, Lambda/configuration, GitHub, DynamoDB-restore, and exact-manifest recovery set to a second approved encrypted destination. Verify every detached-manifest entry from that destination, and require the sanitized evidence PR to be pushed and merged with hosted checks passing. Any exception requires explicit recorded risk acceptance and blocks a default Task 7 execution. Re-verify both complete copies before permanent GitHub deletion.

**Approval gate:** Present the redacted diff, private archive path and manifest hash, restore results, pre-shutdown backup/restore result, operator/auditor policy hashes, and exact branch/commit. Pushing or opening a portfolio PR requires separate approval.

---

### Task 3: Remove the surviving portfolio's broken future repository link

**Files:**

- Modify: `internal/portfolio/data/projects.json:29-38`
- Modify: `internal/portfolio/data_test.go:58-106`

**Interfaces:**

- Consumes: verified plan to delete `CraigDevJohnson/soccer_scraper`
- Produces: a portfolio project card whose `/soccer` demo remains valid without linking to the retiring repository

- [ ] **Step 1: Write the failing content contract**

Update the Soccer project expectation in `internal/portfolio/data_test.go` so `githubURL` is empty and `demoURL` remains `/soccer`. Add a focused assertion that no project URL contains either `CraigDevJohnson/soccer-scraper` or `CraigDevJohnson/soccer_scraper`.

- [ ] **Step 2: Run the focused test and require failure**

```bash
go test ./internal/portfolio -run 'Test.*Project' -count=1
```

Expected: failure because `projects.json` still contains the retiring repository URL.

- [ ] **Step 3: Remove only the retiring `github_url` field**

Use `apply_patch` to remove the Soccer card's `github_url`. Preserve its name, copy, image, technology list, category, and `/soccer` demo link. This plan does not retire the replacement portfolio Soccer route.

- [ ] **Step 4: Verify and commit the portfolio change**

Reconfirm branch/upstream/status/staged files before formatting. If unrelated changes appeared, stop or move this work to an isolated worktree. After `task fmt`, inspect every changed path and stage only the two named files.

```bash
go test ./internal/portfolio -count=1
task fmt
task lint
task test
task build
git diff --check
git add internal/portfolio/data/projects.json internal/portfolio/data_test.go
git diff --cached --name-only
git commit -m "chore(portfolio): remove retired soccer scraper link"
```

Expected: all checks pass and no public portfolio link targets the retiring repository.

**Approval gate:** Present the content diff, test results, commit, PR title/body, and hosted-check plan. Push, PR creation, and merge are separately approved; complete this merge before permanent GitHub deletion.

---

### Task 4: Decide subscriber communication and freeze new deployments

**Files:**

- Modify: `docs/deployment/evidence/soccer-scraper-retirement-inventory.json`
- No standalone repository commit

**Interfaces:**

- Consumes: 11 confirmed email subscriptions across ten exact SNS topics and the verified recovery set
- Produces: recorded communication decision, disabled deployment workflow, and reversibly inactive deployment credential

- [ ] **Step 1: Present the subscriber notice decision**

Recommended notice text:

```text
Soccer schedule email notifications will be retired during the final standalone
service shutdown. No action is required. Your subscription will be removed as
part of that shutdown. The replacement schedule tool does not send automatic
email notifications.
```

Present the exact message, subject `Soccer schedule notification retirement`, ten topic ARNs, expected maximum of 11 deliveries, and the fact that a person subscribed to multiple teams may receive more than one copy.

**Approval gate:** Send nothing until the user explicitly approves this external message. If notice is declined, record `subscriber_notice.decision` as `no_notice`; do not substitute direct email or export addresses.

- [ ] **Step 2: Send an approved notice once per exact topic**

If approved, use the reviewed topic list from the inventory and publish once to each ARN. Record message IDs and timestamps but never endpoints. Re-read topic subscription counts afterward. If the inventory differs, stop and present the change before publishing.

- [ ] **Step 3: Disable the deployment workflow before any standalone Git change**

Require no queued or in-progress deployment run, then present and approve disabling workflow ID `220210440` in `CraigDevJohnson/soccer_scraper`:

```bash
gh workflow disable 220210440 --repo CraigDevJohnson/soccer_scraper
gh api repos/CraigDevJohnson/soccer_scraper/actions/workflows/220210440 \
  --jq '{id,name,state}'
```

Expected: workflow ID/name are unchanged, state is `disabled_manually`, and no queued or in-progress run exists. Do not attempt a manual dispatch as a test: that is an unnecessary mutation attempt. Do not modify standalone `main`, because any push before this read-back would deploy.

After workflow disablement, repeat the two-checkout transactional inventory from Task 2. If either checkout, ref, ignored file, or patch changed, refresh the affected snapshot and detached manifest and rehearse it again before deactivating the deployment key.

- [ ] **Step 4: Deactivate the dedicated deployment key reversibly**

Read the key count for IAM user `github_soccer_scraper` without printing its identifier. Require exactly one active key, require its last service/region to remain Lambda/`us-west-2`, and confirm the user has no group, console, or non-retirement consumer.

**Approval gate:** Present the user ARN, key count/status/last-use metadata, and rollback (`Active`). After approval, set only that selected key to `Inactive` and verify it remains present but unusable.

- [ ] **Step 5: Commit the sanitized decision record**

Use `apply_patch` to record the subscriber decision/message IDs if applicable, workflow state, and key status without endpoints or identifiers. Re-run the sensitive-pattern scan and commit:

```bash
git add docs/deployment/evidence/soccer-scraper-retirement-inventory.json
git commit -m "docs: record soccer scraper shutdown decision"
```

---

### Task 5: Quiesce scheduled work and observe for seven days

**Files:**

- Create: `docs/deployment/evidence/soccer-scraper-quiescence.jsonl`
- Modify: `docs/deployment/evidence/soccer-scraper-retirement-inventory.json`
- Modify: `scripts/check-soccer-scraper-retirement-evidence.sh`
- Modify: `tests/soccer-scraper-retirement-evidence.sh`

**Interfaces:**

- Consumes: disabled deployment workflow, inactive deployment key, production exit proof, and optional final subscriber notice
- Produces: seven full days with both schedules disabled and no unexpected Lambda caller

- [ ] **Step 1: Capture the pre-disable baseline**

Record the current states and complete target objects for:

- `soccer-schedule-check-morning`;
- `soccer-schedule-check-afternoon`; and
- Lambda invocation/error/throttle counts over the preceding 24 hours.

The target objects are required because Scheduler updates must preserve expression, timezone, flexible window, target ARN, role ARN, retry policy, and DLQ state. Define `quiescence_start_at` as the later successful `DISABLED` read-back timestamp. Record the preceding 24-hour CloudWatch totals using 60-second periods and a sanitized Lambda log-derived invocation count; metrics may be delayed, but no post-start time is excluded from the gate.

- [ ] **Step 2: Disable only the two exact schedules**

**Approval gate:** Present both before/after states and the exact `update-schedule` requests. After approval, update each to `DISABLED` while passing every unchanged required field from its fresh `get-schedule` response.

Read back both schedules. The Scheduler `default` group remains untouched.

- [ ] **Step 3: Start daily quiescence sampling**

Use the product's recurring monitoring mechanism to append one sanitized sample at least daily for seven full days. Every sample uses a unique, strictly increasing UTC RFC3339 timestamp and records:

- observed time and elapsed seconds;
- `coverage_start_at` and `coverage_end_at` for the exact closed metric interval;
- schedule states;
- Lambda invocation/error/throttle delta since disable;
- function state and code SHA;
- SNS topic count and confirmed/pending totals, without endpoints;
- DynamoDB item count;
- production/dev health revisions; and
- production `/soccer` success and absence of the old bundle identifiers.

Do not use a blocking sleep. Wait at least 15 minutes before closing each CloudWatch interval. The first sample sets `coverage_start_at = quiescence_start_at`; each later sample sets `coverage_start_at` to the preceding sample's `coverage_end_at`, and every `coverage_end_at = sample_time - 15 minutes`. Treat intervals as half-open `[coverage_start_at, coverage_end_at)` and query interval-local 60-second buckets only, with no overlaps or gaps; the 15 minutes is reporting delay, not an ignored invocation window. Count sanitized log START events for that same interval without exporting raw messages. A new invocation, schedule re-enable, topic/subscriber change, table recreation, production regression, or workflow re-enable freezes retirement and triggers investigation.

- [ ] **Step 4: Close the seven-day gate**

Require the evidence checker to prove at least eight samples, first sample between 15 and 30 minutes after `quiescence_start_at`, no observation gap above 26 hours, no duplicate key or timestamp, continuous `DISABLED` read-backs, and zero metric/log invocations at or after the exact start time. Calculate gate duration from `quiescence_start_at` through the final `coverage_end_at`, not through `sample_time`; require contiguous non-overlapping coverage and `coverage_end_at >= quiescence_start_at + 604800 seconds`. The final sample therefore occurs at least 15 minutes after the seven-day endpoint.

Use `apply_patch` to set `quiescence.scheduler_window_passed_at` in the inventory, validate JSON/JSONL, and commit:

```bash
sh tests/soccer-scraper-retirement-evidence.sh
sh scripts/check-soccer-scraper-retirement-evidence.sh --quiescence \
  docs/deployment/evidence/soccer-scraper-quiescence.jsonl \
  --minimum-seconds 604800 --minimum-samples 8 --maximum-gap-seconds 93600
git add docs/deployment/evidence/soccer-scraper-retirement-inventory.json \
  docs/deployment/evidence/soccer-scraper-quiescence.jsonl \
  scripts/check-soccer-scraper-retirement-evidence.sh \
  tests/soccer-scraper-retirement-evidence.sh
git commit -m "docs: record soccer scraper quiescence"
```

**Approval gate:** Present the complete sanitized window. Passing permits the browser caller cutoff; it does not authorize deletion.

---

### Task 6: Cut off the legacy browser caller and prove production for 48 hours

**Files:**

- Modify: `docs/deployment/evidence/soccer-scraper-quiescence.jsonl`
- Modify: `docs/deployment/evidence/soccer-scraper-retirement-inventory.json`
- Modify: `scripts/check-soccer-scraper-retirement-evidence.sh`
- Modify: `tests/soccer-scraper-retirement-evidence.sh`

**Interfaces:**

- Consumes: seven-day zero-invocation window and closed Amplify rollback dependency
- Produces: revoked browser invocation permission and 48 hours of healthy replacement production

- [ ] **Step 1: Revalidate the dedicated Cognito chain**

Require all current facts to remain true:

- identity pool `unauth_soccer_schedule_scrape_pool` has no login providers and allows unauthenticated identities;
- its sole assigned role is `unauth_soccer_schedule_scraper_role`;
- the private-manifest `browser-invoke-policy` is attached only to that role; and
- the policy's only workload permission is invocation of `soccer_schedule_scraper`.

Stop if another app, role, provider, or workload is present.

- [ ] **Step 2: Detach the browser invoke policy reversibly**

**Approval gate:** Present the exact private pool, role, policy, current attachment, expected browser effect, and rollback reattachment. After approval, detach the private-manifest browser invoke policy from `unauth_soccer_schedule_scraper_role`; do not delete the pool, role, identities, or policy yet.

Use IAM simulation to prove invocation is denied. Do not call Cognito `GetId` or otherwise create a new anonymous identity for this test. Prefer policy simulation or reuse a known test identity without persisting data; if any test changes identity count, re-inventory it immediately. Verify production and dev `/soccer` remain healthy.

- [ ] **Step 3: Observe replacement production for 48 hours**

Define `caller_cutoff_start_at` as the successful policy-detachment read-back. Append at least five unique, strictly increasing samples, with the first between 15 and 30 minutes after cutoff and no observation gap above 13 hours. Each sample records `coverage_start_at`/`coverage_end_at` under the same complete 60-second metric/log contract as Task 5. Require contiguous coverage through `coverage_end_at >= caller_cutoff_start_at + 172800 seconds`; the final sample therefore occurs at least 15 minutes after the 48-hour endpoint. Require zero standalone invocations, accepted public health revisions, successful representative Soccer fetch/download behavior, no old bundle on public production, and no production/dev IAM permission to invoke the standalone Lambda.

Close the window only with an exit-zero checker:

```bash
sh scripts/check-soccer-scraper-retirement-evidence.sh --caller-cutoff \
  docs/deployment/evidence/soccer-scraper-quiescence.jsonl \
  --minimum-seconds 172800 --minimum-samples 5 --maximum-gap-seconds 46800
```

- [ ] **Step 4: Create and restore-test the final DynamoDB backup**

After the 48-hour gate and immediately before teardown, re-scan the live table, calculate a non-secret canonical content digest, and compare schema, TTL, item count, and size to the private manifest. **Approval gate:** Create a second release-derived on-demand backup using `soccer-retirement`; this final backup supersedes the pre-shutdown backup for deletion eligibility.

Wait for `AVAILABLE`. Under separate approval, restore the final backup to an exact temporary table and verify key schema, item count, TTL expectation, and the same content digest. Under another separate approval, delete only the temporary table and verify it absent. After the final restore succeeds, separately approve deletion of the earlier pre-shutdown backup so exactly one final recovery backup remains. Keep that final backup `AVAILABLE` as the explicit retained exception, with owner, cost estimate, and one-year review/deletion date.

- [ ] **Step 5: Persist and merge the caller-cutoff/backup proof**

Use `apply_patch` to record `caller_cutoff.passed_at`, final `coverage_end_at`, final-backup logical label/digest/restore result, and the superseded-backup deletion read-back. Validate and commit only the named evidence files:

```bash
sh tests/soccer-scraper-retirement-evidence.sh
sh scripts/check-soccer-scraper-retirement-evidence.sh --caller-cutoff \
  docs/deployment/evidence/soccer-scraper-quiescence.jsonl \
  --minimum-seconds 172800 --minimum-samples 5 --maximum-gap-seconds 46800
git add docs/deployment/evidence/soccer-scraper-retirement-inventory.json \
  docs/deployment/evidence/soccer-scraper-quiescence.jsonl \
  scripts/check-soccer-scraper-retirement-evidence.sh \
  tests/soccer-scraper-retirement-evidence.sh
git diff --cached --check
git commit -m "docs: record soccer scraper caller cutoff"
```

Push, PR creation, hosted checks, and merge each require their normal approvals. Task 7 remains blocked until this evidence and the portfolio link cleanup are merged into current `origin/main` and the second complete encrypted recovery copy verifies.

- [ ] **Step 6: Reclassify before irreversible deletion**

Repeat the declared consumer perimeter: local/sibling repositories, GitHub code and repository surfaces, packages/wiki/projects, domains/CDNs, AWS integrations, schedules, IAM policies, monitoring, documentation links, and owner-retention intent.

Only a **Stale and Deletion-Eligible — high confidence** result permits Task 7. Any current use, retained rollback need, new consumer, missing recovery proof, or material not-verified surface stops the plan. Require the inventory/quiescence evidence, final-backup restore proof, and detached-manifest hash to exist in approved independent storage and the portfolio evidence PR to be merged with hosted checks passing before irreversible work begins.

**Approval gate:** Present the fresh lifecycle decision, exact deletion manifest, recovery results, and every Task 7 command grouped by surface. Approval is obtained separately for each surface.

---

### Task 7: Delete the exact dedicated AWS resources

**Files:**

- Create: `docs/deployment/evidence/soccer-scraper-retirement-receipt.json`
- Modify: `docs/deployment/evidence/soccer-scraper-retirement-inventory.json`

**Interfaces:**

- Consumes: Stale and Deletion-Eligible decision, merged durable evidence, approved private manifest, restore-tested final DynamoDB backup, and 48-hour caller-cutoff proof
- Produces: no remaining standalone runtime, live table, subscriber, credential, or IAM resource; exactly one approved retained DynamoDB backup remains

- [ ] **Step 1: Freeze the manifest and repeat inbound-integration discovery**

Hash the private deletion manifest and compare every live identifier/configuration to it. Repeat exact scans for API Gateway REST and HTTP integrations, Function URL, Lambda resource policy, event-source mappings, EventBridge Rules/targets, Scheduler targets, and other Lambda integrations. Stop on any addition or drift. Record the before-scan privately and its redacted hash publicly.

- [ ] **Step 2: Delete the dedicated Cognito chain**

Freshly capture identity count and export the pool, role trust/boundary/inline/attached-policy state, and every customer-managed policy version. The current snapshot expects default `v2` and non-default `v1`; revalidate rather than assume.

**Approval gate:** Present the one exact private-manifest pool and consequences. Delete it, then read it back absent without relying on a fixed historical identity count. Never touch the other pool.

Before role deletion, require zero instance profiles and other consumers, remove any inline policy, detach only reviewed managed policies, remove any boundary, and read back zero attachments. Before policy deletion, detach it, delete every non-default version (including expected `v1` if still present), require only the default version to remain, then delete the policy and dedicated browser role under their separate approvals.

- [ ] **Step 3: Delete the two schedules and dedicated Scheduler role**

**Approval gate:** Delete only the two exact schedules in the private manifest. Enumerate every Scheduler group and call `get-schedule` for every remaining schedule because list summaries do not reliably expose `Target.RoleArn`. Require both target schedules absent, every unrelated schedule unchanged, and no remaining schedule to use `EventBridgeSchedulerRole`.

Then remove the reviewed inline policy and delete the role only after confirming zero attached managed policies, boundary, instance profiles, and other consumers. Keep the shared `default` group.

- [ ] **Step 4: Delete the Lambda and its execution role**

Freshly prove that only the target Lambda uses `soccer_schedule_scraper_role`: enumerate Lambda execution roles, Amplify apps/branches and compute/service roles, instance profiles, and other service consumers. Its trust policy includes `amplify.amazonaws.com`, so a Lambda-only search is insufficient.

**Approval gate:** Delete only `soccer_schedule_scraper`, including its current `$LATEST` and every freshly enumerated published version. Read back `ResourceNotFoundException`. Detach, but never delete, the reviewed AWS-managed policies. Remove any inline policy or boundary and delete the role only after all use/attachment checks are zero.

- [ ] **Step 5: Delete the exact SNS topics and subscriptions**

Compare the private topic list to the public count/digest and require no additions. **Approval gate:** Present the exact private list and the irreversible loss of the current confirmed subscriptions. Delete only those topics. Require each exact topic absent and every unrelated topic unchanged. Restoring notification delivery requires new opt-in confirmations.

- [ ] **Step 6: Delete the table while retaining the final backup**

Require the final backup to remain `AVAILABLE`, its source to match the target table, its restore rehearsal to pass, and its recorded digest/schema/item count to match the final live scan. **Approval gate:** Delete only `soccer-schedules` and wait for not found.

Do not delete the final on-demand backup. Record it as the sole intentional data-retention exception, with owner, expected storage cost, review date, and a deletion date no earlier than one year after permanent GitHub deletion. Re-read it as `AVAILABLE` after table deletion.

- [ ] **Step 7: Delete the dedicated log group**

Record sanitized aggregate metrics, date range, stored-byte count, and the known application-level failure summary. Do not export raw events because they may contain email addresses. **Approval gate:** Delete only `/aws/lambda/soccer_schedule_scraper`; verify its subscriptions/filters are zero beforehand and the exact group is absent afterward.

- [ ] **Step 8: Permanently remove the deployment identity and repository secrets**

Require workflow `220210440` to remain `disabled_manually` and its one deploy key to have stayed inactive throughout both observation windows. Before deleting `github_soccer_scraper`, enumerate and require disposition of every access key, inline/attached policy, group, login profile, MFA device, signing certificate, SSH key, service-specific credential, and permissions boundary.

**Approval gate:** Delete the selected inactive key, detach but do not delete `AWSLambda_FullAccess`, remove any remaining reviewed prerequisite, and delete only the dedicated IAM user. Under a separate GitHub-secrets approval, delete only `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `LAMBDA_ROLE_ARN`; verify the names absent without attempting to read values.

- [ ] **Step 9: Repeat discovery and prove exact teardown completeness**

Repeat the Task 7 Step 1 inbound scans after deletion and require no stale integration. Require the target Lambda, schedules, Cognito chain, SNS topics, table, log group, IAM user/key/roles/policy, and matching tags/references absent; require every unrelated schedule/topic/pool and the shared Scheduler group/AWS-managed policies unchanged. Require exactly the approved final DynamoDB backup to remain `AVAILABLE`.

Under the provisioning authority, separately approve removal of the temporary `soccer-retirement` assignment/policy and any active audit assignment. Verify the operator can no longer perform retirement mutations and no dedicated audit assignment remains. Do not strand a teardown credential.

- [ ] **Step 10: Observe teardown state and trailing cost for seven days**

For each immediate, 48-hour, and seven-day checkpoint, separately approve a short-lived `soccer-retirement-auditor` assignment/session, verify its read-only boundary, run the check, then remove/read it back absent. Re-run exact absence/shared-resource invariants, require the retained backup to remain `AVAILABLE`, and inspect service-level/tagged cost and usage for trailing standalone charges without exposing account identifiers. A continuing unexpected charge or recreated resource stops GitHub archival and triggers investigation. Use the recurring monitoring mechanism; do not block with sleep.

- [ ] **Step 11: Preserve a redacted, contract-valid receipt durably**

Because the caller-cutoff evidence PR is already merged, refresh clean `origin/main` and create a new `codex/soccer-scraper-retirement-receipt` branch or isolated worktree before editing. Use `apply_patch` to create the receipt with timestamps, logical actor label and hash, approval references, per-surface before/after counts and digests, immediate/48-hour/seven-day read-backs and cost results, retained-backup metadata, exclusions, and recovery owner. Keep exact identifiers/ARNs/operator documents only in the encrypted archive.

```bash
sh scripts/check-soccer-scraper-retirement-evidence.sh --receipt \
  docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git add docs/deployment/evidence/soccer-scraper-retirement-inventory.json \
  docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git diff --cached --check
git commit -m "docs: record soccer scraper retirement"
```

Push and merge this evidence PR with passing hosted checks, or record an explicit risk acceptance for alternate independent durable storage, before GitHub archival. The AWS receipt must not exist only on a local branch.

**Approval gate:** Present the redacted diff, independent-storage receipt hash, retained-backup state/cost/owner/date, and portfolio PR state. Push, PR creation, and merge remain separately approved.

---

### Task 8: Resolve open GitHub work and archive the repository

**Files:**

- No standalone repository commit
- Update outside Git: GitHub metadata export and checksum manifest

**Interfaces:**

- Consumes: completed AWS teardown, verified backups, disabled workflow, and merged portfolio link cleanup
- Produces: a read-only GitHub archive retaining issues, PRs, branches, URLs, and settings for 90 days

- [ ] **Step 1: Reverify complete recovery and ancillary dispositions**

Require the recovery set to contain PR #37's body, commits, reviews, review threads, comments, checks, and event history; freshly validated `updates`, head/merge refs, local dirty/staged patches, untracked/ignored files, both local snapshots, LFS/wiki/attachment disposition, and restored-checkout proof. Preserve the accurate incomplete Actions evidence recorded in Task 2.

For every discovered package, separately approve retain, deprecate, or delete only after consumer/download/export review. For every linked user-level Project, separately approve unlink, archive, or retain. Require verified absence or completed disposition before permanent repository deletion; repository archival alone does not dispose of either surface.

- [ ] **Step 2: Present the exact PR closure message**

Draft:

```text
Closing without merge because the standalone Soccer scraper has been retired
after the replacement portfolio production rollout. The branch, commits,
review history, and local dependency work were preserved in the verified
retirement recovery set.
```

**Approval gate:** Closing PR #37 posts an external message. Do not close or comment until this exact text and target PR are approved. Retain `updates`; branch deletion provides no benefit before repository deletion.

- [ ] **Step 3: Decide the archive-facing retirement notice**

Separately approve either relying only on GitHub's archive banner or updating the repository description/README with a retirement notice. A README/description change is an external mutation. If `main` or repository metadata changes, refresh final SHAs, PR merge ref, GitHub export, mirror, local snapshot comparison, and detached checksum manifest before archival.

- [ ] **Step 4: Archive the exact GitHub repository**

**Approval gate:** Present repository `CraigDevJohnson/soccer_scraper`, immutable repository ID `940342051`, visibility, default branch, exact `main`/`updates` SHAs, open issue/PR count, workflow state, backup/restore results, and rollback (unarchive).

After approval, set `archived: true`. Read back:

- repository ID remains `940342051`;
- `archived` is true;
- `main` and `updates` retain their SHAs;
- issues, PRs, comments, and source remain readable;
- Actions writes/manual deployment are unavailable; and
- the independent recovery set still verifies.

- [ ] **Step 5: Observe the archive for 90 days**

Record the archive timestamp and earliest deletion-review timestamp 90 days later. Use a weekly recurring read-only check because GitHub traffic endpoints expose only a rolling 14-day window. Retain each raw response in the encrypted archive and only a sanitized count/digest publicly. Check inbound references, forks, traffic, package/download activity, owner requests, and accidental production dependency. Any credible consumer or retained-purpose request stops permanent deletion and favors continued archival.

---

### Task 9: Reassess and permanently delete GitHub only after the archive window

**Files:**

- Modify: `docs/deployment/evidence/soccer-scraper-retirement-receipt.json`
- Preserve: the external recovery set

**Interfaces:**

- Consumes: 90-day archived repository, fresh complete deletion review, and exact deletion approval
- Produces: permanently removed GitHub repository with independently verified recovery artifacts

- [ ] **Step 1: Repeat every time-sensitive deletion check**

Refresh repository identity, branches, refs, PR/issues, forks, mirrors, all weekly traffic samples, packages, Projects V2, wiki, LFS/attachments, releases, Actions, deployments, integrations, consumers, AWS absence, production health, portfolio links, ownership, planned use, retained purpose, and recovery integrity. Require every package and Project to be verified absent or to have its approved disposition completed.

Require no material unknowns. Recent clone counts alone are supporting evidence, but any credible inbound consumer, package use, owner request, or unresolved access gap blocks deletion.

- [ ] **Step 2: Rehearse recovery again**

Verify the SHA-256 manifest from both encrypted copies, restore the remote mirror and both local snapshots into fresh temporary directories, run `git fsck --full`, validate the named SHAs/refs, and reproduce the dirty `updates` worktree. Under a separately approved short-lived `soccer-retirement-auditor` session, confirm the DynamoDB backup remains `AVAILABLE`, record its planned retention end, then remove/read back the assignment absent.

- [ ] **Step 3: Present the permanent GitHub deletion**

**Approval gate:** Present the current repository metadata, exact consequences, archive duration, no-use evidence, backup locations/hashes, restore proof, exclusions, and GitHub's current restore limitations. Obtain immediate action-specific approval using a credential that has `delete_repo`; the current review token does not.

After approval, delete only `CraigDevJohnson/soccer_scraper`. Read back repository not found from an authenticated request and confirm the pre-approved package/Project dispositions remain satisfied; do not discover or decide their fate only after repository deletion.

- [ ] **Step 4: Update the durable receipt**

Refresh clean `origin/main` after the AWS receipt PR merges and create `codex/soccer-scraper-repository-deletion` (or an isolated worktree) before editing. Use `apply_patch` to record the deletion approval timestamp, logical actor/hash, prior public repository ID, final SHAs, provider response, read-back, recovery locations/hashes, and one-year retention end. Validate the changed receipt and its sensitive-pattern contract before external preservation or commit:

```bash
sh scripts/check-soccer-scraper-retirement-evidence.sh --receipt \
  docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git add docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git diff --cached --check
git commit -m "docs: record soccer scraper repository deletion"
```

Preserve the same validated receipt externally immediately. Push, PR creation, and merge remain separately approved.

---

### Task 10: Optionally dispose of local checkouts, then verify the finished outcome

**Files:**

- Move only after approval: `/Users/craigjohnson/Projects/soccer_scraper`
- Move only after approval: `/Users/craigjohnson/repos/soccer_scraper`
- Preserve: `/Users/craigjohnson/Documents/Codex/decommission-archives/soccer_scraper`

**Interfaces:**

- Consumes: deleted GitHub repository, fresh restored local snapshots, and a separate owner choice to clean up local paths
- Produces: core decommission verification plus optional removal of each approved active local checkout

- [ ] **Step 1: Leave the target directories before any move**

Local checkout disposal is not part of the core AWS/GitHub decommission completion definition. If the owner selects it as a follow-on, run from `/Users/craigjohnson/Projects/portfolio`, never from inside either target. Repeat clone discovery across declared roots, Git worktree registrations, mounted volumes, CI/agent worktrees, and known other hosts. Re-read exact paths, device/inode, Git state, ignored/untracked inventory, snapshot hashes, and restore results.

- [ ] **Step 2: Move the clean checkout to Trash**

**Approval gate:** Present only `/Users/craigjohnson/Projects/soccer_scraper`, its exact SHA/ref state, recovery path, and reversible Finder Trash destination. After approval, move that one directory to Trash and verify the recovery snapshot still passes `git fsck`.

- [ ] **Step 3: Move the dirty checkout to Trash**

**Approval gate:** Separately present `/Users/craigjohnson/repos/soccer_scraper`, branch `updates`, the two modified files, patch digest, local branch/ref state, and restored dirty-worktree proof. After approval, move that one directory to Trash and verify the recovery snapshot and patch again.

Do not empty Trash as part of either operation.

- [ ] **Step 4: Run final external verification**

For the AWS assertions, separately approve a short-lived `soccer-retirement-auditor` session, run the read-only checks, then remove/read back its assignment absent.

Require all of the following:

- production and dev `/healthz` return their accepted release revision;
- production and dev `/soccer` complete representative fetch/download behavior;
- public production assets contain neither the standalone Lambda name nor old Cognito pool ID;
- the portfolio project card has no retiring repository link;
- all approved deletion targets remain absent and exactly the retained final DynamoDB backup remains `AVAILABLE`;
- the recorded immediate, 48-hour, and seven-day AWS absence/cost checks still pass under the read-only auditor;
- GitHub repository is absent while the recovery set remains restorable; and
- each separately approved local active path is absent while its Trash/recovery disposition is recorded; otherwise its intentional retention is recorded without blocking core completion.

- [ ] **Step 5: Close the plan**

Update the receipt with final public checks, cost checks, local disposition, unresolved exclusions, and recovery retention date. Run:

```bash
cd /Users/craigjohnson/Projects/portfolio
sh scripts/check-soccer-scraper-retirement-evidence.sh --receipt \
  docs/deployment/evidence/soccer-scraper-retirement-receipt.json
jq -e '.status == "complete" and .recovery.verified == true' \
  docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git diff --check
test -x .tools/tailwind/tailwindcss || task install-tailwind
task ci
git add docs/deployment/evidence/soccer-scraper-retirement-receipt.json
git diff --cached --check
```

Expected: the receipt is schema-valid, the portfolio passes its authoritative gate, no standalone runtime or GitHub repository remains, every selected local cleanup is complete, and recovery evidence plus the retained DynamoDB backup remain intact. Commit the final receipt update on `codex/soccer-scraper-repository-deletion`; push, PR creation, hosted checks, and merge remain separately approved.

---

## Explicit Exclusions

This plan does not delete or modify:

- Scheduler group `default`;
- any AWS-managed IAM policy;
- the account's other Cognito identity pool;
- any SNS topic outside the ten exact topic ARNs in the reviewed manifest;
- `portfolio-lambda-dev`, `portfolio-lambda-prod`, their API Gateways, tables, SSM paths, logs, alarms, ECR images, IAM roles, or OpenTofu state;
- Amplify itself, except that formal removal from the approved production rollback roster is a prerequisite;
- the private `craig-johnson-portfolio-vue` source repository;
- the surviving portfolio `/soccer` route; or
- the final DynamoDB on-demand backup before its separately approved retention end;
- the external recovery set before its separately approved retention end; or
- any local checkout that has not received its own follow-on Trash approval.

## Execution Handoff

Execute only after the prerequisite production-plan correction is merged and the production cutover/observation is complete. Use `superpowers:subagent-driven-development` for the portfolio code/evidence tasks or `superpowers:executing-plans` for checkpointed inline execution. Live production, AWS, GitHub, subscriber-message, and local Trash approvals remain separate even after an execution approach is selected.
