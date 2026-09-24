# Production environment and deployment decisions

Recorded on September 19, 2026, from the
[Weekly GitHub Status Report conversation](chatgpt-conversation://6a7eadce-8164-83e8-a9ed-0e810668d710).
The discussion took place on September 18 in `America/Boise`, September 19 UTC.
Related work is tracked in
[Issue #75: complete production Lambda rollout and protected promotion](https://github.com/CraigDevJohnson/portfolio/issues/75).

This record captures Craig's accepted direction. Implementation and live
deployment remain future work.

## Accepted decisions

The portfolio is a personal site with few visitors. Craig wants practice with
a realistic production setup, but accepts downtime of a few days while
troubleshooting. Completing the migration matters more than adding operational
complexity that this site does not need.

- Finish the production Lambda deployment in the current AWS account. The
  future AWS account structure does not block this rollout.
- Keep a separate, least-privilege production deployer role. Development and
  production remain distinct deployment targets even while they share an AWS
  account.
- Keep a protected `production` GitHub Environment with manual approval in
  GitHub Actions. Allow Craig to approve his own deployment. Keep this step for
  now and revisit it later if experience shows it is unnecessary.
- Promote the exact immutable image already verified in development. Do not
  rebuild the image for production. Keep the reviewed saved plan before apply.
- Retain checks for the apex and `www` domains, HTTPS, `/healthz`, OAuth and
  cookies, and alarms. Accepting downtime does not mean reporting an unverified
  deployment as successful.
- Drop the production rollback requirement entirely. This includes both
  rollback automation and a mandatory manual rollback procedure. Do not retain
  or restore Amplify or App Runner as fallback hosting. If production fails,
  accept the downtime, diagnose the failure, and fix the Lambda deployment.
- Treat the multi-account AWS design as separate later work. Splitting the
  organization management account from workload accounts remains a direction
  to revisit. Craig accepts that finishing production now may require migration
  work later. No account layout or migration date was selected.

The rollback choice was explicit. The suggestion to skip automation but keep a
manual rollback was rejected. The final decision was to fix failures in place,
not to require a fallback deployment.

Issue #77 subsequently retained the option to generate a policy-checked,
checksum-bound alias-only rollback plan when a previously verified production
Lambda version exists. This does not make rollback a mandatory procedure or
restore a fallback-hosting requirement. Applying such a plan requires a new
operator decision. The first cutover must explicitly represent the absence of a
previously verified production deployment rather than inventing one.

## September 22 scope and observation decisions

During the Issue #75 grilling session, Craig selected the following scope:

- The first production launch includes the public portfolio, Soccer, and Google
  Calendar integration. The EC2 management portal is deferred to separate work.
- Production acceptance includes a successful Soccer/Google authorization flow
  and calendar operation. Management-portal login is not a launch prerequisite.
- Issue #75 may close after 30 minutes of successful public-route,
  authentication, revision, and alarm verification, once its other launch
  acceptance criteria are satisfied. No next-day check or seven-day observation
  period is required for this initial production launch.
- A required verification failure invalidates the observation window. After the
  failure is understood and resolved, require a fresh, uninterrupted 30-minute
  window. A successful retry does not resume the failed window. Keep the
  deployment failed or unverified until the full verification succeeds; an
  upstream-service failure does not exempt required functionality from this rule.

Craig confirmed the shared understanding on September 23, 2026, and authorized
proceeding with the triage outcome. Live provisioning, activation, deployment,
and external-service changes retain their separate approval gates. These
decisions do not change development recovery behavior or authorize resource
retirement.

The existing choices of apex as the canonical hostname, a permanent `www` to
apex redirect, and fresh production data remain in effect. Google accounts
reconnect in production rather than migrating legacy encrypted connections.

## Branches and environments

The discussion confirmed `main` as the shared source branch. Development is a
deployment target, not a separate long-lived branch. The pipeline and GitHub
Environments control review and promotion.

The repository and GitHub configuration checked on September 19 still separate
these responsibilities:

| Environment | Responsibility and status |
| --- | --- |
| `development` | Deploys and verifies an eligible release using its development AWS role. |
| `release-review` | Records a protected review checkpoint when the release classification requires it. It grants no AWS deployment authority. |
| `production-plan` | Uses a separate planner role to generate and retain a production plan. It does not apply the plan. |
| `production` | Intended target for the manually approved production apply job. The Environment and job are still missing. |

The development review gate is conditional. Review-class changes and certain
backlog recovery cases require the protected checkpoint. Pending runtime
changes can carry forward without that checkpoint when the classifier permits
it. Routine development releases do not all need manual review. The retained
production approval is a separate decision.

## Development before production

The immediate next step discussed was to clear the protected development
release backlog and verify an intentionally selected revision of `main`.
The plain-language sequence was:

1. Select the exact revision intended for development.
2. Review that state through the existing protected release-review flow.
3. Let the normal development release deploy that same revision, without
   inserting additional code changes between review and release.
4. Verify page loads, authentication behavior, and the health endpoint.
5. Stop after development verification. Production promotion is a separate step.

This sequence describes the desired outcome. The current workflow's eligibility
and backlog checks determine the concrete recovery steps. It does not establish
that the backlog has been cleared or that a new release has been deployed.

## Follow-up work and conflicts with earlier plans

The next production planning task is to outline the smallest cutover that
implements these decisions after development is verified. It should cover the
production role and Environment, manual self-approval, the saved-plan apply,
promotion of the verified image, and the public deployment checks.

Issue #75, the
[August 21 production cutover plan](../superpowers/plans/2026-08-21-production-lambda-cutover.md),
and the
[original migration design](../superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md)
contain earlier requirements for rollback plans, evidence, or a healthy fallback
origin. Those requirements conflict with the accepted production scope above.
The September 23 triage outcome updates Issue #75; reconcile the earlier plans
and their validators during implementation. A verified Lambda coordinate and
optional alias-only rollback artifact do not restore a mandatory rollback
procedure or fallback origin.

The existing
[Lambda release workflow documentation](./aws-lambda-api-gateway.md)
also describes development rollback evidence and longer observation gates.
The September 22 decision above replaces the initial production observation
duration with 30 minutes. Development recovery behavior and authorization for
retiring existing resources remain separate from these production decisions.

At local revision `cb82040588ace85bd130491935ca989cad57ec64`,
`.github/workflows/release.yml` still ends the production path at
`production-plan`, and `scripts/plan-ci-lambda-production.sh` writes `PLAN_ONLY`.
The live GitHub Environment list also lacks `production`. Issue #75 remains
open. Issue #77's preparation subsequently merged through PR #78, defining a
dormant production deployer and additional release contracts while leaving
production application hard-disabled. This documentation does not change those
controls or establish current AWS or public-site readiness.
