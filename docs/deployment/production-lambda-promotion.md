# Production Lambda promotion

The [accepted Issue #75 scope](https://github.com/CraigDevJohnson/portfolio/issues/75)
governs the first launch. The production entrypoints now execute the reviewed
protected-approval and saved-plan contracts. The workflow job remains gated by
`PRODUCTION_APPLY_ENABLED`, which must be explicitly enabled after readiness
readback under the approved activation plan. A successful `production-plan` job is
rehearsal evidence, not a deployment or live-change authorization. The
[September 25 readiness review](2026-09-25-production-launch-readiness.md)
records the earlier findings. The September 27 continuation restored development
on source `b90d7cf9fc28377341b44546482415c9e42e4276` in successful Release run
[36356429868](https://github.com/CraigDevJohnson/portfolio/actions/runs/36356429868).
Production readiness and acceptance remain separate gates.

## Accepted launch contract

- Launch the public portfolio, Soccer, and Google Calendar in account
  `180294223248`, `us-west-2`. Foundation/member migration is later work and
  is not a prerequisite to this current-account launch.
- Promote the exact development-verified immutable image without rebuilding.
  The reviewed promotion PR changes only `deploy/production-release.json`;
  application source and promotion commit remain distinct.
- Use fresh production data and a fresh session key. Reconnect Google accounts;
  do not copy legacy encrypted connections.
- Serve the application on `craigdevjohnson.com`; permanently redirect `www`
  to that canonical apex. Keep Soccer authorization/callbacks on the apex.
- Defer the EC2 management portal. Its Cognito login, callback, and logout are
  not first-launch acceptance requirements.
- Accept downtime while fixing failures. Do not require a fallback origin or
  fabricate prior verified production history. Optional alias-only rollback
  requires an actual prior verified version and separate authorization.
- Require 30 uninterrupted minutes of successful public acceptance. Any
  required failure, including an upstream failure, invalidates the window;
  diagnose and repair it, then begin a fresh full window.

## Authority boundaries

- `portfolio-production-planner-ci`, trusted only by `production-plan`,
  validates the manifest and creates evidence. It cannot write production
  state or services.
- `portfolio-production-deployer-ci`, trusted only by `production`, is defined
  in source. Provisioning and use require a reviewed activation plan. Routine
  release authority covers only the exact production state/lock objects and
  approved existing Lambda release resources.
- `portfolio-deployer` is the guarded local SSO path for separately approved
  bootstrap or recovery. It does not replace protected CI promotion.
- Release automation excludes IAM/OIDC provisioning, state bootstrap, runtime
  parameter/data writes, DNS/Cloudflare, API Gateway domains, ACM, legacy
  hosting, and unrelated environments. Review those prerequisites in their
  owning operator plans.

## Readiness before activation

Retain dated, sanitized evidence for these gates without secret values:

1. Resolve the development backlog and independently revalidate the chosen
   source SHA, successful trusted development deployment, existing ECR digest,
   scan, live alias/version, and application verification.
2. Inventory production state and resources. Under an approved bootstrap plan,
   establish the function, `live` alias, API Gateway, isolated tables, encrypted
   runtime parameters, logs, alarms, and chosen HTTPS/domain route. Record any
   missing resources rather than treating source declarations as live proof.
3. Verify the exact production execution role can resolve approved runtime
   parameters. Exercise the application without recording decrypted values,
   JWTs, OAuth codes, cookies, calendar contents, or tokens in evidence.
4. Verify certificate coverage and the actual apex/`www` routing design,
   including direct-origin apex HTTPS, permanent public `www` redirect, the
   canonical Soccer Google callback allowlist, and secure session cookies.
   A separate application served at the `www` origin is not required.
5. Verify five configured alarms and their notification path, and collect
   healthy alarm/error observations. A configured topic is not inbox delivery.
6. Record exact existing and proposed DNS/Cloudflare record/rule changes and
   review the public-cutover sequence. Keeping that inventory does not require
   a fallback hosting service or authorize its retirement.
7. Complete the applicable pinned foundation baseline review, itemized
   recurring and temporary costs, and explicit exceptions with expiry/review
   triggers. Do not inherit an unrelated foundation stage's spending limit
   or logging/detection exception.

## First deployment evidence

The planner records null prior-production fields and `BOOTSTRAP_REQUIRED` when
no durable verified production deployment exists. `BOOTSTRAP_EVIDENCE_FILE`
binds the observed function, live alias, version, revision ID and immutable
image URI. Apply rechecks that alias and the absence of a verified predecessor.
The bootstrap coordinate never becomes an authorized rollback target.

The first-deployment path must bind independently reviewed bootstrap evidence
to the exact account, backend/workspace, source/digest, live function/alias
state, and saved plan. Preserve the distinction between an observed bootstrap
alias and a **verified production predecessor**. Only public acceptance can
create the first durable verified production record. Unknown or conflicting
history must fail closed; do not populate prior-success fields to pass a guard.

## Activation and approval

Prepare a concrete versioned plan before requesting live authorization. It
must name exact resource/configuration changes, source revisions, cost bounds,
work window, verification, and any baseline exceptions. Once Craig approves
that scope, carry out its necessary steps without asking again for each
command. Stop for a material expansion or failed prerequisite.

1. Review the isolated CI-role saved plan, expected role/policy changes, and
   checksum. Verify exact OIDC repository/environment/audience, separate
   builder/development/planner/deployer permissions, and representative
   allowed/denied operations.
2. Create the `production` GitHub Environment before its workflow job is
   enabled. Require Craig's review, permit self-review, restrict to protected
   branches, disable administrator bypass, and set only the verified
   `AWS_PRODUCTION_DEPLOYER_ROLE_ARN` needed by that job. Read back the result.
3. Review and merge the activation change only after its implementation and
   readiness gates pass and live activation is authorized. The job uses
   `environment: production`, `deployments: write`, `id-token: write`, and the
   non-cancelling `lambda-production` group. PR-head jobs get no AWS credentials.
4. Generate a new manifest-only promotion after renewed development evidence
   checks. Do not reuse a stale manifest solely because it once passed.
5. Bind protected approval and apply to the reviewed binary plan, its JSON/text
   and policy/checksums, exact backend/workspace, release identity, planning
   run/attempt, and actual approval identity. Recheck current `main` before
   credentials and immediately before apply; preserve state locks and
   stale-release checks.

## Public verification and failure handling

Record a protected production deployment before application, then retain the
exact final Lambda alias/version/digest and `/healthz` revision, public pages
and assets, HTTPS, canonical redirect, secure cookies, successful Soccer/Google
authorization and an authorized calendar operation, and alarm/error samples.
The 30-minute window covers the required public acceptance checks together;
separate short smoke runs do not establish an uninterrupted window. Development
verification and recovery behavior remain unchanged.

On apply or verification failure, preserve evidence, record failure or
unverified status on the original GitHub deployment, and stop. Resolve the
failure under the approved scope or a newly reviewed repair plan. Mark success
only after a complete clean acceptance window. Never apply rollback
automatically. Legacy retirement and later account migration have separate
observation periods, inventories, and approvals.

## Shared alarm delivery

Production uses an empty direct `alarm_action_arns` array. The foundation owns
`foundation-notifications-services` in Oregon, which selects the five exact
portfolio production alarms and forwards native ALARM/OK events to the Ohio
`foundation-notifications` bus. Its existing encrypted delivery path reaches
`foundation-security`. Do not substitute a direct, unencrypted application topic.

Planning, preapply and ongoing production observation inspect the enabled rule,
account and native state filters, exact alarm selectors, forwarding role and
single bus target. This read permission is restricted to the exact rule in both
production CI roles. These configuration checks do not prove inbox delivery;
retain separate native alarm-to-inbox test evidence before public cutover.

## Protected acceptance record

The production apply job consumes its own planning job's immutable artifact and
trusted plan/identity hash outputs. Protected approval must identify Craig and
the current main revision; plans and approvals must be within the eight-hour
run window. A successful apply and automatic observation leave the original
GitHub deployment `in_progress`, with `APPLIED_NOT_VERIFIED` evidence.

The `Production acceptance` workflow takes the original Release run ID and a
sanitized browser receipt. It requires protected production approval again,
verifies the immutable artifact archive digest, ties the receipt to the same
release and uninterrupted 1,800-second window, checks final ingested metrics,
and repeats public probes. The browser receipt must describe actual Soccer
access/current data and Google Calendar create/readback/delete operations at
the window boundaries, with successful connected-calendar reads between them.
Record cookie attributes and cache controls, never cookie values or tokens.

Only this complete check creates schema-version-2 production verification and
records success on the original deployment. Both workflows share the
non-cancelling `lambda-production` concurrency group. Recording rereads terminal
state before retries, refuses conflicting terminal results, and does not
silently deactivate another deployment. Failure requires investigation and a
fresh complete observation window; there is no automatic rollback.

## Approved temporary root-key exception

Craig explicitly approved extending the existing IAM-02 inactive-root-key
exception to this first portfolio launch in account `180294223248`. It expires
October 5, 2026 at 06:33 UTC. The root key must remain inactive; its separate
deletion review cannot occur before October 4 at 06:33:23 UTC, after the
seven-day observation. This is an accepted temporary gap, not IAM-02 compliance.
Foundry backup measurement still requires zero root keys. The deployment change
session still ends September 28 at 06:43:37 UTC; this exception does not extend it.
Production log retention is 30 days, matching LOG-02.
