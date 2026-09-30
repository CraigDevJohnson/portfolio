# Lambda HTTP API infrastructure

The OpenTofu roots for the portfolio in the workloads account. Each root has
its own state key in `portfolio-tofu-state-<account>` and an `aws_account_id`
input that its provider enforces through `allowed_account_ids`.

- `ci-roles` is the account root: the GitHub OIDC CI roles, the
  `PortfolioLambdaExecutionBoundary` and `PortfolioLambdaHistoryExecutionBoundary`
  permissions boundaries and the state bucket. Apply it first. See its
  [README](ci-roles/README.md).
- `artifacts` owns only the immutable release repository.
- `environments/dev` and `environments/prod` call `modules/service` with
  distinct backend keys and settings.
- `modules/service` contains no backend or provider configuration.
- `auth/site/dev` and `auth/site/prod` are the planned, separate site sign-in
  pools built from `auth/site/modules/pool`; neither is provisioned. Their
  plans and state hold the Google client secret, so Craig plans, applies and
  exports them only with the private `cognito-site-<env>-*` tasks. See
  [site identity](../../docs/deployment/site-identity.md).

Saved plans can contain sensitive configuration, so keep them outside the
checkout. Plan and apply are separate commands; see
[DEPLOY-INSTRUCTIONS.md](../../DEPLOY-INSTRUCTIONS.md).

## Soccer history collection and daily refresh

`modules/service` can plan durable Soccer history in three stages. Both
environment roots pass every history input from their `*.auto.tfvars`, never
with `-var`, and every stage is off in both:

1. `enable_soccer_history` plans only the history table and the HTTP runtime's
   table grant.
2. `activate_soccer_history_collection` with `soccer_history_limits` hands the
   HTTP runtime the table and the reviewed limits, so visitor Team ID lookups
   and granted player imports enroll teams within the admission capacity. It
   adds an alarm on refused player-linked teams, plus a
   `ManualAdmissionRejected` metric with no alarm for refused visitor Team ID
   lookups, and needs `alarm_action_arns`. Both metric filters match the
   refusal line's top-level `source`, which relies on the HTTP function keeping
   `LOG_ADD_SOURCE=false`.
3. `activate_soccer_history_schedule` with a once-daily UTC
   `soccer_history_schedule_expression` adds the scheduled worker: a separate
   Lambda running the release image in `SOCCER_HISTORY_MODE=scheduled`, its
   EventBridge Scheduler schedule and roles, a failure queue, and error,
   incomplete-run and failure-queue alarms.

With `soccer_history_limits` unset there is no collection or schedule, and the
application itself refuses to build a history store or worker without every
reviewed limit. Supplying limits alone activates nothing.

Before any stage is applied, the #104 activation review must settle source use,
measured traffic and cost. Craig accepted the numeric limits and the 10:30 UTC
daily run on September 30, 2026: both environments carry the limits, and
production also carries the schedule expression. Whether development collects
or runs the schedule is undecided, so it has no expression and keeps its empty
`alarm_action_arns`.
`max_requests_per_run` must be at least `max_enrolled_teams` times the measured
requests one team costs: its team request, one lookup per facility its games
use, and retries. The module and the application check only one request per
team, so below that a full archive cannot reach every enrolled team each day,
and each run reports the teams it left.

Settings and resources outside this module already cover every stage; the
account root must be applied before any stage is planned:

- `PortfolioLambdaExecutionBoundary` (`ci-roles/boundary.tf`) lets each HTTP
  execution role get, put, query and delete items in its own history table.
  The worker and Scheduler roles attach a separate
  `PortfolioLambdaHistoryExecutionBoundary`, which grants the worker its table,
  `due-teams` index, log group and failure queue, and the Scheduler role only
  invoking the worker and reporting to the queue. Both grants in one policy
  would exceed IAM's 6,144-character managed-policy limit. IAM enforces that
  limit only at apply, never in a plan, so `ci-roles/tests/policies.tftest.hcl`
  asserts it offline for both.
- The CI roles read the history table, metric filters, worker and Scheduler
  roles, worker, log group, alarms, event invoke configuration, failure queue
  and schedule, so a release plan can refresh them. The deployers may also
  publish the worker's code.
- `scripts/check-lambda-plan.sh` accepts a release plan that also moves the
  worker's image, only to the release image. `scripts/verify-lambda-release.sh`
  checks every alarm the environment's outputs name, so the history alarms
  join release verification once a stage is applied. The failure-queue alarm
  stays in ALARM while any message sits in the 14-day failure queue, so after
  a failed daily run or delivery every release in that environment fails
  verification until an operator records the cause and drains the queue as
  `workloads-admin` (see the Alarms section of `DEPLOY-INSTRUCTIONS.md`).
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
