<!-- markdownlint-disable MD013 -->
# LPS history sync activation readiness

**Issue:** #104 under #80. **Checkpoint:** `1f0ad35a` on September 26, 2026, with #101–#103 merged; the initial packet was committed at `d3262cf1`. **Recommendation:** blocked for live collection. This packet is an offline review artifact, not a source-use approval, AWS plan, or activation record.

## Evidence boundaries

The checkout has an owner-bound import and membership write, public Team ID enrollment, a durable archive adapter, a private team-season read, verified global player removal, and a separate daily worker entrypoint. `infra/lambda/modules/service/dynamodb.tf` declares a non-TTL `soccer_history` table with the `due-teams` GSI. The module names it `portfolio-lambda-{dev,prod}-soccer-history` in `us-west-2`. Declaration and passing tests do not establish that either table exists live. The HTTP Lambda still has a 29-second timeout; the separate worker is configurable for 30–900 seconds and is absent by default.

No live LPS request, AWS state read, backend initialization, state lock, plan, resource change, or scheduled invocation was performed for this packet. Current enrolled team count, source response distribution, facility fanout, changed-game frequency, retained bytes, AWS usage, and worker runtime are **unknown live**. Local fake measurements appear below and are not estimates of LPS production behavior.

## Source-use finding: unresolved

LPS [Terms and Conditions, §3](https://www.letsplaysoccer.com/terms-and-conditions?lang=en) discuss use of its APIs and published documentation subject to the terms and Acceptable Use Policy. The same section discusses suspension for unusual traffic and service impact. I did not find a published AUP or endpoint-specific rate/retention guidance in the reviewed public pages. That text does **not** establish that the `/teams/{id}`, `/facilities/{id}`, `/users/check`, or `/players/{id}/my_teams` endpoints are a published API available for unattended daily retrieval. Endpoint reachability and existing browser use establish no such permission. The [LPS Privacy Policy](https://www.letsplaysoccer.com/privacy-policy?lang=en) describes the company's own personal-data retention; it does not authorize this site's indefinite retention of player and membership evidence.

Before any live collection, obtain and record LPS's applicable AUP/API documentation or a direct written answer covering these exact endpoints, daily request and retry rates, indefinite retention of linked-player identity and team-season evidence, and deletion expectations. Review the answer with the operator; do not infer permission from silence. The app's disclosed import action and verified removal path are useful product controls, but do not resolve this source-use gate by themselves.

## Candidate traffic and admission envelope

The merged worker and Terraform now accept and enforce numeric limits, but all activation inputs still default to `null` or `false`. This is a **candidate tuple, not a configured or approved limit**:

```text
max_enrolled_teams=25, reserved_player_slots=10,
max_requests_per_run=100, max_retries_per_team=1,
min_request_interval_ms=1000, worker_timeout_seconds=300
activate_soccer_history_collection=false,
activate_soccer_history_schedule=false,
soccer_history_schedule_expression=null
```

The tuple satisfies the Terraform validation and `DailyLimits.Validate`; it has not been placed in either environment. The daily worker counts team, facility, and retry HTTP calls through one paced transport, queries the sparse due-team index, and checkpoints each completed team. The admission transaction caps new manual teams at 15 while reserving the remaining 10 slots for player-linked teams; existing enrolled teams retain refresh work. Fake tests prove those mechanics, not the chosen values' suitability.

| Input | Candidate assumption or present bound | Evidence gap |
| --- | --- | --- |
| Enrolled teams | 25 total; reserve 10 new slots for authenticated player-linked enrollment, leaving at most 15 manual slots | Actual enrolled count and growth unknown live; the counter is enforced only when collection is activated with limits |
| LPS calls per invocation | One team response plus two distinct facility responses per team: `25 × (1 + 2) = 75`, with 25 calls left for retries or extra facilities; 100 is a hard **per-invocation** ceiling | Facility fanout and 429/5xx frequency unknown live; overflow remains due and produces an incomplete report |
| Aggregate scheduled calls | Scheduler permits two delivery retries, so three deliveries could each use 100 calls: **up to 300/day** for one schedule occurrence, plus any separately authorized manual invocations | Successful team checkpoints often reduce repeat work, but there is no durable account-wide daily LPS call counter; source-use approval must cover the worst case or retry policy must change before activation |
| Response bytes | Fake continuous journey: **882 bytes** across seven LPS responses (two team, two facility, one player-team, two user checks). Planning case for a full 25-team pass: `25 × (64 + 2 × 4) = 1,800 KiB` | Fake payloads are deliberately small. `internal/lps/client.go` reads at most 2 MiB/response; 100 calls could still read up to 200 MiB/invocation without a tighter size cap |
| Games and seasons | Cost scenario: 20 returned games and two returned seasons per team; allow two previously known omitted seasons | Live distributions unknown; a later omission retains old facts and changes coverage |
| Changed games | Unknown; estimate writes from **all** returned games because `SaveTeamSnapshot` currently upserts each game and team-game edge on every successful refresh | A low changed-game rate does not lower current DynamoDB write volume |
| Retained bytes | Cost scenario: 0.1 GB average archive/index in the first year. At 100 newly retained games/team/year, 25 teams, four game/edge records at ≤4 KiB each imply about 40 MiB/year before team, coverage, index, and overhead; reserve up to 0.1 GB/year in the planning model | Indefinite retention has no enforced byte ceiling; live item-size distribution and growth need measurement |
| Worker duration | Separate 512 MiB, reserved-concurrency-one worker; candidate 300-second timeout. One-second pacing means 100 calls require at least 99 seconds between first and last starts before network/DynamoDB time | Fake clock recorded one second of pacing for two calls per due pass; real latency and timeout margin remain unmeasured |

At 25 teams for 30 days, the normal planning case is 75 requests/day or 2,250/month; the per-run ceiling is 3,000/month for one successful daily delivery. Scheduler retries make 9,000/month a conservative delivery-only ceiling at the candidate limit. A team with more than two facilities may consume the reserved calls; the transport stops at 100 and leaves remaining teams due. No permission, rate allowance, or live performance measurement supports this tuple yet.

## Itemized cost model, provisional

The arithmetic below is an illustrative **gross** 30-day scenario with one 300-second daily worker invocation and the candidate tuple. It assumes 25 teams, 20 games/team, two facilities and two seasons/team, at most two old omitted seasons, three team-game edge writes/game, ≤4 write units per item, 0.1 GB average retained table/index size, 0.1 GB worker logs, and no failure redrive. These are workload assumptions, not measured live quantities or a verified `us-west-2` quote. Reprice each environment in the [AWS Pricing Calculator](https://calculator.aws/) with the exact plan. Shared free-tier use, transaction overhead, item sizes, GSI writes, repeated deliveries, retained growth, notifications, and data transfer can change the bill.

| Component | Scenario arithmetic | Reference charge/month |
| --- | --- | ---: |
| DynamoDB writes + due index | Scenario: 91 base item puts/team/day including full game re-upsert and three team-game edges/game, 4 write units/item, plus one due-index update/team/day: `(25 × 30 × 91 × 4 + 25 × 30 × 4) × $0.625/M` | $0.17 |
| DynamoDB reads | About 28 strongly consistent 4 KiB reads/team/day plus one sparse due-index query/run: `25 × 30 × 28 × $0.125/M`, with small query/admission allowance | <$0.01 |
| Durable table storage | Assumed 0.1 GB average, including index and accumulated history: `0.1 × $0.25/GB-month` | $0.03 |
| Production PITR | Enabled by `prod.auto.tfvars`; assumed 0.1 GB: `0.1 × $0.20/GB-month` | $0.02 |
| Development PITR | Disabled by `dev.auto.tfvars` | $0.00 |
| Worker Lambda | 30 runs × 300 seconds × 0.5 GB × $0.0000166667/GB-second, plus 30 × $0.20/M requests | $0.08 |
| EventBridge Scheduler | 30 invocations × $1/M before the shared 14M/month free allowance | <$0.01 |
| CloudWatch logs | Worker group, 0.1 GB assumed ingested and retained for the month: `0.1 × ($0.50 + $0.03)/GB`; the application log group is separately declared | $0.05 |
| Two low-cardinality log-derived metrics | `DailyIncomplete` and `AdmissionRejected`, two × $0.30/month reference; no team-ID dimensions | $0.60 |
| Four standard alarms | Admission rejection, incomplete run, worker error, SQS visible messages: four × $0.10/month reference | $0.40 |
| Failure handling | One SSE-enabled SQS failure queue with 14-day retention; placeholder for low-volume send/receive/delete and any redrive | $0.01 placeholder |

**Reference subtotal:** about **$1.36/month in production** or **$1.34/month in development**, assuming no other charges. Three full 300-second deliveries/day would raise worker duration alone from about $0.08 to $0.23/month and could repeat reads, writes, and logs before checkpoints. Neither subtotal is a spend authorization or a cost ceiling. The largest uncertainty is permitted LPS use, fanout, full-game write amplification, worker duration, and indefinite history growth. Sources: [DynamoDB pricing](https://aws.amazon.com/dynamodb/pricing/), [Lambda pricing](https://aws.amazon.com/lambda/pricing/), [EventBridge Scheduler pricing](https://aws.amazon.com/eventbridge/pricing/), [CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/), and [SQS pricing](https://aws.amazon.com/sqs/pricing/). AWS's example rates are region-sensitive; obtain a `us-west-2` quote and include the reviewed SNS notification destination before approval.

## Offline behavior evidence

At the initial `0a552e90` checkpoint, focused fake Cognito/LPS/DynamoDB tests passed separately for import, manual enrollment, on-demand refresh, private read, and global removal. On the merged `1f0ad35a` base, `TestLPSHistoryReadinessJourneyFromEnrollmentThroughRemoval` now drives the real HTTP route assembly and real `DailyWorker` against one shared in-memory archive and fake LPS server. It passed with this literal journey:

1. Granted import captured one owner-bound player–team–season proof and enrolled Team ID 4101 without fetching games.
2. The first due pass made one `/teams/4101` and one `/facilities/5` call, applied a one-second fake-clock pacing interval, stored one scored game, and checkpointed the team. An immediate duplicate pass made zero LPS calls.
3. The private read returned fetched coverage and a one-win calculated record for the proven owner. After advancing the fake clock by 25 hours and changing the fake score, a second due pass made two calls and the read returned the corrected `2-0` result.
4. A fresh `/users/check` verified removal. Global player proof was erased while the retained team/game fact remained; the ordinary browser read without a new import was denied.

The fake server returned **882 bytes across seven responses**: two team, two facility, one player-team, and two user-check. Each due pass used two LPS requests under the candidate 100-request invocation ceiling; the not-due replay used zero. This is a tiny fixture, not a live response-size or worker-duration sample. The in-memory archive records logical snapshots; it does not measure DynamoDB RRU/WRU, item bytes, GSI writes, or provisioned latency. Existing `internal/soccerarchive/daily_test.go` separately exercises capacity reservation, 429/5xx retries, request-budget overflow including facility lookups, due-index pagination, duplicate delivery, and invalid-team stop behavior with the fake Dynamo adapter.

Focused commands run locally on the merged base:

```text
go test ./internal/app -run '^TestLPSHistoryReadinessJourneyFromEnrollmentThroughRemoval$' -count=1 -v
go test ./internal/soccerarchive -run 'Test(ArchiveAdmissionReservesCapacityForPlayerTeamsWithoutEvictingEnrolledTeams|DailyWorkerRetriesTransientTeamsWithinBudgetAndReportsPartialFailure|DailyRequestBudgetIncludesFacilityLookupsAndPreservesDueWork|DailyWorkerRefreshesEveryDueValidTeamAndCheckpointsDuplicateDelivery)$' -count=1
```

The local test and mocked OpenTofu contracts cannot establish LPS permission, live invalid-ID behavior, actual response bytes, DynamoDB capacity/cost, an effective IAM boundary, or deployed AWS resources. Record real enrolled count, representative response/facility fanout, item-size and read/write counts, retries, and runtime only after the separate source-use and AWS observation approvals.

The service module's backend-free `tofu test` passed **21 mocked cases**, including limits-only off, collection capacity alert, and worker/scheduler/failure contracts. It reported provider deprecation warnings for `hash_key`/`range_key`; those warnings are not a failed test or live compatibility proof.

`task lambda-infrastructure-ci` was attempted with `-backend=false` initialization and no AWS state lock. Formatting, validation, mocked auth/CI-role tests, and `go test ./infra/lambda` passed before the gate stopped in `tests/lambda-plan-contract.sh` at its **“exact GitHub Actions role plan”** fixture: `CI role names, trust, attachments, or inline policies drifted`. This failure is outside the #104 file diff, but the full offline infrastructure gate is **red** at this checkpoint. Diagnose and repair that fixture/contract independently before calling infrastructure CI green; do not treat the partial pass as validation of a live plan.

## Exact checked-in resource and IAM map

The following are Terraform addresses and names derived from `infra/lambda/modules/service/`; **none is asserted to exist live**. Both environments are in `us-west-2` and use different state roots.

| Environment | Archive and worker | Scheduler, queue, and logs |
| --- | --- | --- |
| Development (`portfolio-lambda-dev`) | `portfolio-lambda-dev-soccer-history` table and `due-teams` GSI; `portfolio-lambda-dev-soccer-history` worker Lambda; `portfolio-lambda-dev-soccer-history-execution` role and `portfolio-lambda-dev-soccer-history-runtime` inline policy. Table PITR/deletion protection off in checked-in dev vars. | `portfolio-lambda-dev-soccer-history-daily` UTC schedule; `portfolio-lambda-dev-soccer-history-scheduler` role and same-named inline policy; `portfolio-lambda-dev-soccer-history-failures` SQS queue; `/aws/lambda/portfolio-lambda-dev-soccer-history` log group, 14-day retention. |
| Production (`portfolio-lambda-prod`) | `portfolio-lambda-prod-soccer-history` table and `due-teams` GSI; `portfolio-lambda-prod-soccer-history` worker Lambda; `portfolio-lambda-prod-soccer-history-execution` role and `portfolio-lambda-prod-soccer-history-runtime` inline policy. Table PITR/deletion protection on in checked-in prod vars. | `portfolio-lambda-prod-soccer-history-daily` UTC schedule; `portfolio-lambda-prod-soccer-history-scheduler` role and same-named inline policy; `portfolio-lambda-prod-soccer-history-failures` SQS queue; `/aws/lambda/portfolio-lambda-prod-soccer-history` log group, 90-day retention. |

With `activate_soccer_history_collection=true` and reviewed limits, the HTTP Lambda environment gains the five `SOCCER_HISTORY_*` limit values plus `SOCCER_HISTORY_COLLECTION_ENABLED=true`; `module.service.aws_iam_role_policy.lambda` adds `dynamodb:UpdateItem` on the archive table for the transactional admission counter, alongside its existing `GetItem`, `PutItem`, `Query`, and `DeleteItem`. `module.service.aws_cloudwatch_log_metric_filter.history_admission_rejected[0]` and `aws_cloudwatch_metric_alarm.history_admission_rejected[0]` are created against the application log group. A nonempty reviewed `alarm_action_arns` is required. The checked-in Lambda execution boundary does not yet grant the archive table actions; its exact replacement and read-back require distinct authorization.

With `activate_soccer_history_schedule=true` **and** collection enabled and a reviewed UTC expression, `history_worker.tf` creates `aws_sqs_queue.history_dead_letter[0]` (SQS-managed encryption, 14-day retention), `aws_cloudwatch_log_group.history_worker[0]`, `aws_iam_role.history_worker[0]` and its policy, `aws_lambda_function.history_worker[0]` (same digest-qualified image, x86-64, 512 MiB at current env vars, timeout from limits, reserved concurrency one), and `aws_lambda_function_event_invoke_config.history_worker[0]` (zero Lambda async retries, failure destination the queue). It also creates `aws_iam_role.history_scheduler[0]` and policy, `aws_scheduler_schedule.history_daily[0]`, `aws_cloudwatch_log_metric_filter.history_incomplete[0]`, and three more alarms: daily incomplete, worker Lambda errors, and visible failure-queue messages. The scheduler has two target-delivery retries and points to the same queue. Both worker and scheduler roles attach the `PortfolioLambdaExecutionBoundary`, whose installed grants have not been verified for these new actions.

The worker inline policy allows archive-table `dynamodb:GetItem` and `PutItem`, `dynamodb:Query` on the exact `due-teams` index ARN, log-stream creation/write on its log group, and `sqs:SendMessage` on its queue. The scheduler role allows `lambda:InvokeFunction` on the worker and `sqs:SendMessage` on that queue, with a schedule-source ARN condition in its trust policy. Neither role has player-removal or unrelated portfolio permissions. Public-HTTP egress to LPS is a separate network/code control, not an IAM action. Each environment has two fixed-cardinality log metric filters and four history alarms after both stages are enabled; the production alarm destination ARN and a development alert destination remain unselected in this packet.

### Saved-plan review procedure and current blocker

1. Before an AWS-backed init or plan, obtain approval for the exact `dev` or `prod` root and its native S3 state-lock URI in `DEPLOY-INSTRUCTIONS.md`. Use only the reviewed non-root profile in `us-west-2`, a digest-qualified image, an absolute unused `PLAN_FILE`, and the environment's reviewed public identity and alarm inputs. `task lambda-dev-plan`/`task lambda-prod-plan` acquire AWS state locks; neither was run here. Keep the saved plan and its SHA-256 private.
2. Review a **collection-only** plan first (`soccer_history_limits` set, `activate_soccer_history_collection=true`, `activate_soccer_history_schedule=false`, expression unset). This would activate new HTTP enrollment, not daily polling. Compare the archive table state, app environment/IAM changes, admission metric filter/alarm, boundary, alerts, and all other plan actions. It still needs source-use, spend, state-lock, plan, and apply approval.
3. Separately review any **schedule** plan with an exact once-daily UTC `cron(...)` expression and `activate_soccer_history_schedule=true`. The Terraform resource has `state = "ENABLED"` and the worker/queue/roles/alarms appear only in this stage; an apply would immediately schedule live LPS requests. There is no dormant-worker plan in the current IaC. Require a distinct activation approval for the exact saved-plan hash, limits, schedule time, IAM boundary, queue redrive procedure, and expected resource actions.
4. The current closed allowlists in `scripts/check-lambda-plan.sh` include the history table but **do not yet enumerate** the conditional `UpdateItem` policy or the new worker, Scheduler, SQS, metric-filter, and alarm addresses. Expand and test that offline checker against representative collection-only and schedule plan JSON before relying on `task lambda-plan-check` or approving an AWS-backed plan. The normal plan task calls this checker after producing a saved plan; a new-resource plan would presently fail its gate. Only expected create/update/no-op actions are acceptable; no delete, import, move, or unreviewed IAM widening.

Offline mocked provider tests validate configuration without the backend, but do not replace a green full infrastructure gate, saved-plan inspection, effective IAM read-back, source-use permission, or live-resource verification. No AWS-backed plan or lock was attempted for this packet.

## Remaining gates

1. Obtain LPS's applicable AUP/API and written answer for the exact endpoints, worst-case **300 scheduled calls/day** under the candidate tuple and retries (or change delivery retries and recalculate), request pacing, and indefinite player evidence retention. Do not activate from the public terms alone.
2. Measure current enrollment, representative permitted LPS response/facility sizes, 429/5xx and changed-game rates, archive item and GSI bytes, real RRU/WRU, worker duration, and retained growth. The 882-byte fake fixture is not that measurement. Review whether a durable daily call counter or a lower Scheduler retry limit is required to enforce the approved aggregate source budget.
3. Obtain `us-west-2` AWS quotes for the exact dev/prod plan, select reviewed SNS alert destinations, and review the installed Lambda execution boundary plus worker/scheduler role grants. Extend `check-lambda-plan.sh` and its offline fixtures for #103's conditional resources, and restore the separate failing CI-role plan fixture, before any live saved-plan review.
4. Obtain distinct source-use, state-lock/plan, collection creation, and later schedule activation approvals. Inspect each environment's exact saved-plan actions and hash; verify live state and failure alerts only after an authorized apply.

Until those gates are met, **do not activate daily LPS polling**.
