# Lambda HTTP API infrastructure

The OpenTofu roots for the portfolio in the workloads account. Each root has
its own state key in `portfolio-tofu-state-<account>` and an `aws_account_id`
input that its provider enforces through `allowed_account_ids`.

- `ci-roles` is the account root: the GitHub OIDC CI roles, the
  `PortfolioLambdaExecutionBoundary` permissions boundary and the state bucket.
  Apply it first. See its [README](ci-roles/README.md).
- `artifacts` owns only the immutable release repository.
- `environments/dev` and `environments/prod` call `modules/service` with
  distinct backend keys and settings.
- `modules/service` contains no backend or provider configuration.
- `auth/dev` is the planned development Cognito pool; it is not provisioned.
- `auth/site/dev` and `auth/site/prod` are the planned, separate site sign-in
  pools built from `auth/site/modules/pool`; neither is provisioned. See
  [site identity](../../docs/deployment/site-identity.md).

Saved plans can contain sensitive configuration, so keep them outside the
checkout. Plan and apply are separate commands; see
[DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md).

## Soccer history collection and daily refresh

`modules/service` can plan durable Soccer history in three stages, each off by
default and not wired into either environment root:

1. `enable_soccer_history` plans only the history table and the HTTP runtime's
   table grant.
2. `activate_soccer_history_collection` with `soccer_history_limits` hands the
   HTTP runtime the table and the reviewed limits, so visitor Team ID lookups
   and granted player imports enroll teams within the admission capacity. It
   adds an alarm on rejected enrollment and needs `alarm_action_arns`.
3. `activate_soccer_history_schedule` with a once-daily UTC
   `soccer_history_schedule_expression` adds the scheduled worker: a separate
   Lambda running the release image in `SOCCER_HISTORY_MODE=scheduled`, its
   EventBridge Scheduler schedule and roles, a failure queue, and error,
   incomplete-run and failure-queue alarms.

With `soccer_history_limits` unset there is no collection or schedule, and the
application itself refuses to build a history store or worker without every
reviewed limit. Supplying limits alone activates nothing.

Before any stage is applied, the #104 activation review must settle source use,
measured traffic and cost, the numeric limits and the schedule.
`max_requests_per_run` must be at least `max_enrolled_teams` times the measured
requests one team costs: its team request, one lookup per facility its games
use, and retries. The module and the application check only one request per
team, so below that a full archive cannot reach every enrolled team each day,
and each run reports the teams it left.

The review must also change settings and resources this module does not own:

- `PortfolioLambdaExecutionBoundary` (`ci-roles/boundary.tf`) grants none of
  the history table, its `due-teams` index, the worker and Scheduler roles, the
  worker log group or the failure queue. The worker and Scheduler roles attach
  the boundary, so they can do nothing until it grants them.
- The CI roles read only the existing tables, the service function and its
  alarms. A plan with the history table, worker, schedule, queue or history
  alarms needs matching read grants, and releases need the deployers to update
  the worker's image too.
- `scripts/check-lambda-plan.sh` accepts a release plan only when the service
  function image and `live` alias change. Once the worker exists, its image
  changes with every release, so the check must also accept that change.
- The worker reserves one concurrent execution, so a repeated delivery never
  runs alongside the first. Lambda keeps 100 executions unreserved, and the
  workloads account's concurrency limit is still 10 (aws-setup #30), which is
  why both environments run the app unreserved. The schedule stage can't be
  applied until that limit covers 100 unreserved plus every reservation: prod's
  planned 10 and the worker's 1.

Local OpenTofu tests use a mocked provider and create no resources.

The [history-sync readiness packet](../../docs/deployment/2026-09-26-lps-history-activation-readiness.md)
records the candidate limits, fake dry run, source-use questions, cost model,
and blockers before any environment can activate collection or scheduling.
