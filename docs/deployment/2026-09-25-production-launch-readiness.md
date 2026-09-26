# Current-account production launch readiness

<!-- markdownlint-disable MD013 -->

Observed September 25, 2026. This is offline preparation for
[Issue #75](https://github.com/CraigDevJohnson/portfolio/issues/75), not a live
execution approval or a claim that the launch implementation is complete.
Application infrastructure remains owned here. The
[foundation handoff](https://github.com/CraigDevJohnson/aws-setup/blob/207d57c1148e0744da2bca660a16b7d43262ef71/docs/planning/foundation-migration-handoffs.md)
explicitly permits first production launch in the current account before the
later member-account migration.

## Current evidence

| Evidence | Observation | Limit |
| --- | --- | --- |
| GitHub `main` | `11166a9eaecf1a4c19ef96e51ab9915612d9b8bd`, PR #78 | Freeze a new exact source after reviewed preparation changes; this document does not select a deployable future SHA. |
| [CI 35500742610](https://github.com/CraigDevJohnson/portfolio/actions/runs/35500742610) | Successful for that main SHA | Static gates do not prove deployed behavior. |
| [Release 35500903159](https://github.com/CraigDevJohnson/portfolio/actions/runs/35500903159) | `authorize` failed before AWS credentials | Later main runs were skipped; no backlog recovery succeeded. |
| Development deployment `6179270539` | Success recorded August 31 for source `9528b784088f71fa39d1d7fce8570278c0d3acaf`, version 4 | Historical GitHub evidence; live alias, scan, digest, and public health require revalidation. |
| Recorded development digest | `sha256:affaf0a7e2ff3add63db709107785c49faf5b08a8a86959bd4719402372b5776` | The checked-in production manifest still names it; it is not evidence that the later UI changes have deployed. |
| Production GitHub deployment/Environment | Zero production deployments; no `production` Environment | Bootstrap history is absent, not a failed prior production release. |
| Production workflow | Ends at `production-plan`; apply and deploy scripts exit unconditionally | Removing those exits alone is not an implementation. |

The first launch includes the existing portfolio, Soccer, and Google Calendar.
The separate reactive Soccer/history work in #79/#80 and their child issues is
not a prerequisite named by #75 and is not included merely by shipping current
`main`. The management portal remains deferred.

## Development recovery blocker

Running the existing authorization script against the exact main SHA reproduced:

```text
Release review backlog validation failed: checkpoint recovery contains a runtime, mixed, or unknown pull request
Lambda release authorization failed: release review cannot checkpoint this development backlog
```

The reviewed ancestry resolves as follows before the validator stops:

| Source | Reviewed base | Classification |
| --- | --- | --- |
| `11166a9e` (#78) | `b729519b` | review |
| `b729519b` (#76) | `cb820405` | skip |
| `cb820405` | `02ca989a` | skip |
| `02ca989a` (#70, updated UI) | `6d015231` | development |

The range since the last verified development source also changes the production
manifest. Current `development-reviewed` recovery allows pending runtime changes
only when that manifest has not changed; current review checkpoint recovery
allows a narrowly bounded standalone promotion but rejects runtime changes.
This backlog fits neither. A workflow rerun, another docs-only PR, or a fabricated
successful review/deployment cursor will not repair it legitimately.

### Implemented offline recovery candidate

The manual input on the existing Release workflow implements one bounded path
for this backlog. The normal `workflow_run` classifier is unchanged. Its helper
`scripts/validate-development-recovery.py`:

1. Pins verified development base `9528b784`, backlog anchor `11166a9e`, and
   production-manifest SHA-256
   `d223aa79fcb7f803abbd8311a3a1027719845132688fcbcd53856387bdccad55`.
   The full selected source is an explicit dispatch input that must equal the
   run's main-branch SHA and current main. It must be the unique reviewed merge
   directly over the anchor, with only the enumerated recovery/docs/test files
   changed. Runtime, infrastructure, and production-manifest changes are refused.
2. Requires the latest trusted successful push CI for the selected source and
   the existing protected `release-review` Environment with Craig's approval,
   protected branches, and no administrator bypass. The selected input is
   checked against the run SHA before checkout; untrusted input code is not run.
3. Emits only `development-reviewed`, then uses the existing builder/development
   roles, saved-plan checks, non-cancelling development concurrency, locks,
   verification, failure handling, and trusted deployment/status records.
4. Treats the historical production manifest as ineligible for this recovery.
   It grants no production job or permission. A later new manifest-only
   promotion must bind the newly verified development source/digest/deployment.
5. Rechecks current source, CI, approval, unchanged verified development base,
   and the eight-hour run window before credentials, image push, and apply via
   existing current-main checks. Failed/cancelled or rejected runs are refused.
   Only the first attempt is eligible; a failed attempt requires a fresh
   dispatch/review. Successful real development verification changes the trusted
   base and consumes this recovery automatically, without a fabricated cursor.

Review/build/development and production planning jobs need read access to
Actions, pull requests, and deployment metadata to repeat these checks. GitHub documents read-only
Actions permission for [Environment configuration](https://docs.github.com/en/rest/deployments/environments#get-an-environment)
and [run approval history](https://docs.github.com/en/rest/actions/workflow-runs#get-the-review-history-for-a-workflow-run).
This adds no AWS permissions; the protected review job still cannot request an
OIDC token or write a deployment.

Scan retrieval recognizes this exact successful manual Release only after
revalidating its frozen source/manifest/PR scope, CI, first-attempt success, and
Craig's approval. That historical check grants no execution classification and
does not require the old source to remain current main after it is promoted.

The candidate is not merged, dispatched, or live-authorized by this document.
After review and merge, refresh the selected source to that full merge SHA;
`11166a9e` itself lacks the implementation and is deliberately ineligible. An
intervening main change requires refreshing and reviewing the frozen scope.
Before dispatch, review the exact development plan scope and temporary build/
deployment cost allowance; approve a fresh eight-hour execution window. Then:

```sh
: "${APPROVED_RECOVERY_SOURCE_SHA:?set the exact reviewed full merge SHA}"
gh workflow run release.yml --repo CraigDevJohnson/portfolio --ref main \
  -f recovery_source_sha="$APPROVED_RECOVERY_SOURCE_SHA"
```

Approve that exact run's protected `release-review` job after its metadata
preflight succeeds. A successful recovery produces an ordinary, verified
development deployment; it never creates a production deployment or a synthetic
release-review checkpoint.

## Production implementation work before activation

| Interface/files | Existing mismatch | Required correction and proof |
| --- | --- | --- |
| `scripts/plan-ci-lambda-production.sh`, `scripts/validate-ci-lambda-production-apply.sh`, `scripts/apply-ci-lambda-production.sh`, recording/deployment helpers | Planner honestly emits `BOOTSTRAP_REQUIRED` and null prior fields; apply/record paths still expect a successful predecessor. | Add an explicit first-deployment case bound to reviewed bootstrap state, alias/source/digest, plan/checksums, backend, and approval. Keep observed bootstrap state distinct from a verified predecessor; reject contradictory history. Test first success, first failure with no rollback target, alias drift, forged predecessor, and later normal promotion. |
| `scripts/verify-ci-lambda-production.sh`, production verification/recording tests | Requires management Cognito `/login`, `/callback`, `/logout` and treats both hosts as applications returning 200. | Verify canonical Soccer/Google authorization and cookie contracts, permanent `www` redirect, an authorized calendar operation, and portfolio routes/assets. Deferred management routes must not be required for acceptance. |
| Production observation entrypoint/evidence, production recording tests | Shared smoke verifier accepts only 300–900 seconds; older observation tools require seven days and a fallback origin. | Implement a production acceptance window of at least 1,800 uninterrupted seconds with required checks, stable release identity, alarm/error samples, and bounded evidence gaps. Any required failure invalidates it. Test failures near the end, upstream failures, timestamp/gap manipulation, stale identity, and restart with a fresh full window. Preserve development behavior. |
| `.github/workflows/release.yml`, `Taskfile.yaml`, activation tests | No protected apply/verify job | Add the job only after readiness and live activation approval. Bind it to the exact saved planning bundle and protected approval; preserve current-main checks, exact roles, non-cancelling concurrency, locking, and terminal status on the original deployment. |

These changes affect authorization and success reporting together. Implementing
only a longer sleep, accepting null prior fields everywhere, or removing the
hard stops would leave the release unsafe or falsely verified.

## Exact infrastructure scope to inventory

| Scope | Known coordinate | Required live evidence |
| --- | --- | --- |
| Account/Region | `180294223248`, `us-west-2` | Non-root authorized principal and current account/Region |
| Release registry | `180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases` | Existing immutable digest/tag and acceptable scan bound to the selected verified development release |
| Production service | `portfolio-lambda-prod`, alias `live` | State ownership, current function/version/alias, API, protected tables, logs, five alarms, and exact planned changes |
| Production backend | `s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate` | Versioning/encryption/recovery and default workspace; exact native `.tflock` object covered by plan approval |
| CI roles backend | Same bucket, `portfolio-lambda-http-api/ci-roles/terraform.tfstate` | Current role resources/state; reviewed plan must establish the actual diff rather than presume two creates |
| Production deployer | `arn:aws:iam::180294223248:role/portfolio-production-deployer-ci` | Exact policies and trust for `repo:CraigDevJohnson/portfolio:environment:production`, audience `sts.amazonaws.com`, one-hour sessions, allowed/denied checks |
| Local operator | Named `portfolio-deployer` and `portfolio-ci-roles-administrator` SSO profiles for their approved scopes | Current effective policies and live identities; routine `aws-setup-audit` does not grant deployment authority |
| Public route | `craigdevjohnson.com`, permanent `www.craigdevjohnson.com` redirect | Exact Cloudflare record/rule IDs and diff, certificate coverage, dynamic-path caching, canonical Google callback, direct-origin and public HTTPS |
| Runtime parameters | `/portfolio/lambda/prod/CLIENT_ID_KEY`, `/portfolio/lambda/prod/CLIENT_SECRET_KEY`, `/portfolio/lambda/prod/LPS_SESSION_KEY` | Existence/encryption/role access and fresh approved session material, without value disclosure; Google reconnection with fresh production data |

`prod.auto.tfvars` currently declares 512 MB, 29 seconds, reserved concurrency 10,
PITR and deletion protection, and 90-day logs. It leaves custom-domain request
and activation false. Those are source settings, not a live inventory.

## Baseline and cost decision package

Review against the pinned
[foundation register](https://github.com/CraigDevJohnson/aws-setup/blob/207d57c1148e0744da2bca660a16b7d43262ef71/docs/planning/initial-baseline-controls.md)
with control IDs, account/resource scope, timestamp, owner, method, redacted
evidence, pass/fail/not-tested/not-applicable, gaps, and exception references.
The current-account first-launch placement is explicitly accepted; it does not
prove that every other control is satisfied.

- `IAM-02`: a retained root key, even inactive, remains a gap. No portfolio
  exception was found in #75. A launch before deletion needs an explicit bounded
  exception or must wait for deletion. Preparation can proceed during the
  seven-day root-key observation period.
- `LOG-02`: reconcile the configured 90 days with the default 30-day requirement
  or document a workload-specific reason before the concrete plan.
- `LOG-01`/`DET-01`: determine actual current coverage and any launch-specific
  temporary exception. A bootstrap exception for a different stage is not
  blanket production approval.
- Review the remaining applicable controls rather than treating these three
  findings as a complete feature review.

No current portfolio cost ceiling or live saved-plan approval is established by
this review. Price incremental and temporary overlap for actual Lambda/API
traffic, DynamoDB/PITR, logs/alarms, ECR, parameters, notifications, and DNS or
other required supporting services. The foundation state allowance and Foundry
measurement allowance do not apply to this release. Record a range or approved
ceiling with its usage assumptions before the launch approval.

## Execution package and order

Before asking for live approval, assemble the selected development source,
reviewed recovery/activation revisions, exact saved-plan or plan-generation
envelope and checksums, resource/role/Environment changes, bootstrap evidence,
exact DNS/Cloudflare changes, Google callback changes, cost limits, exceptions,
work window, stop conditions, and verification procedure. Missing live inventory
is a preparation gap; this document is not a completed execution package.

The shortest supported sequence is:

1. Complete reviewed development recovery and production contract implementation
   in parallel with foundation/root-key work, preserving all current hard stops.
2. Obtain the concrete bounded development recovery approval and run its
   protected release flow; verify the exact source/digest in development.
3. Complete production prerequisite inventory and bootstrap/activation plan.
   Review and approve the complete live scope, including any temporary baseline
   exceptions and itemized costs. Avoid per-command confirmation within that
   approved scope.
4. Bootstrap and activate the reviewed production path, create a fresh
   manifest-only promotion, protect approval of its saved plan, apply, perform
   the exact public cutover, and pass the clean 30-minute acceptance window.
5. Retain source/digest/scan, plan/policy/checksums, approval/run/deployment,
   routing/authentication/calendar, alarm, and terminal-status evidence.

Legacy retirement waits for its separate 24-hour healthy observation, reviewed
subscriber notice, coordinated stop, and seven-day observation/destructive
review. The later member move waits for foundation controls and regional
rehearsals. Neither retirement sequence belongs in the first-launch acceptance
window.
