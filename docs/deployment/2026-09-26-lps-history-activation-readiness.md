<!-- markdownlint-disable MD013 -->
# LPS history sync activation readiness

**Issue:** #104 under #80. **Checkpoint:** `0a552e90` on September 26, 2026, before the #103 daily-worker branch. **Recommendation:** blocked for live collection. This packet is an offline review artifact, not a source-use approval, AWS plan, or activation record.

## Evidence boundaries

The checkout has an owner-bound import and membership write, public Team ID enrollment, a durable archive adapter, an on-demand refresh worker, a private team-season read, and verified global player removal. `infra/lambda/modules/service/dynamodb.tf` declares a non-TTL `soccer_history` table with the `due-teams` GSI. The current application module names it `portfolio-lambda-{dev,prod}-soccer-history` in `us-west-2`. Declaration and passing tests do not establish that either table exists live. The HTTP Lambda has a 29-second timeout; a separate scheduled worker and its resource/IAM contract are pending #103.

No live LPS request, AWS state read, backend initialization, state lock, plan, resource change, or scheduled invocation was performed for this packet. Current enrolled team count, response distribution, facility fanout, changed-game frequency, retained bytes, and worker runtime are **unknown live**.

## Source-use finding: unresolved

LPS [Terms and Conditions, §3](https://www.letsplaysoccer.com/terms-and-conditions?lang=en) discuss use of its APIs and published documentation subject to the terms and Acceptable Use Policy. The same section discusses suspension for unusual traffic and service impact. I did not find a published AUP or endpoint-specific rate/retention guidance in the reviewed public pages. That text does **not** establish that the `/teams/{id}`, `/facilities/{id}`, `/users/check`, or `/players/{id}/my_teams` endpoints are a published API available for unattended daily retrieval. Endpoint reachability and existing browser use establish no such permission. The [LPS Privacy Policy](https://www.letsplaysoccer.com/privacy-policy?lang=en) describes the company's own personal-data retention; it does not authorize this site's indefinite retention of player and membership evidence.

Before any live collection, obtain and record LPS's applicable AUP/API documentation or a direct written answer covering these exact endpoints, daily request and retry rates, indefinite retention of linked-player identity and team-season evidence, and deletion expectations. Review the answer with the operator; do not infer permission from silence. The app's disclosed import action and verified removal path are useful product controls, but do not resolve this source-use gate by themselves.

## Candidate traffic and admission envelope

This is a **planning hypothesis, not a configured or approved limit**. #103 must expose and enforce the chosen values, and the values must be revisited after source-use guidance and a permitted measurement sample.

| Input | Candidate assumption or present bound | Evidence gap |
| --- | --- | --- |
| Enrolled teams | 25 total; hold 10 new slots for authenticated player-linked enrollment, at most 15 manual slots | Actual enrollment and growth unknown; current branch has no enforced admission ceiling |
| Daily LPS calls | One team response plus at most two distinct facility responses per team: `25 × (1 + 2) = 75`; reserve 25 retry calls, absolute candidate budget 100/day | Actual facility IDs and 429/5xx rate unknown; current on-demand worker has no request cap or pacing |
| Response bytes | Planning case: 64 KiB/team response and 4 KiB/facility response, `25 × (64 + 2 × 4) = 1,800 KiB/day` | Unmeasured; `internal/lps/client.go` reads at most 2 MiB per response, so 100 calls could still read up to 200 MiB/day without a tighter cap |
| Games and seasons | 20 returned games and two returned seasons per team; allow two previously known omitted seasons | Live distributions unknown; a later omission retains old facts and changes coverage |
| Changed games | Unknown; estimate writes from **all** returned games because `SaveTeamSnapshot` currently upserts each game and team-game edge on every successful refresh | A low changed-game rate does not lower current DynamoDB write volume |
| Worker duration | Separate 512 MiB worker, candidate 300-second invocation ceiling; 100 paced calls at ≥1 second apart already take ≥100 seconds before DynamoDB work | Runtime and latency unmeasured; the existing 29-second HTTP Lambda is unsuitable |

At 25 teams for 30 days, the candidate is at most 3,000 LPS calls/month. A team response can reference more than two facilities; in that case the worker must stop or checkpoint within the approved budget rather than silently exceed it. Existing enrolled teams retain priority over new admissions; player-linked admissions use their reserved capacity before additional anonymous Team IDs. None of these behaviors is proven to be enforced on this pre-#103 branch.

## Itemized cost model, provisional

The arithmetic below is an illustrative **gross** monthly scenario, before shared free tiers, in the target account's 30-day month. The published AWS pages show example rates; they are not a verified `us-west-2` quote. Reprice in the [AWS Pricing Calculator](https://calculator.aws/) for each environment after #103 fixes the worker shape. Region, actual item sizes, account-wide free-tier use, retries, logs, backups, and data transfer can change the bill.

| Component | Scenario arithmetic | Reference charge/month |
| --- | --- | ---: |
| DynamoDB writes + due index | Scenario: 91 base item puts/team/day including full game re-upsert and three team-game edges/game, 4 write units/item, plus one due-index update/team/day: `(25 × 30 × 91 × 4 + 25 × 30 × 4) × $0.625/M` | $0.17 |
| DynamoDB reads | About 28 strongly consistent 4 KiB reads/team/day: `25 × 30 × 28 × $0.125/M` | <$0.01 |
| Durable table storage | Assumed 0.1 GB average, including index and accumulated history: `0.1 × $0.25/GB-month` | $0.03 |
| Production PITR | Enabled by `prod.auto.tfvars`; assumed 0.1 GB: `0.1 × $0.20/GB-month` | $0.02 |
| Development PITR | Disabled by `dev.auto.tfvars` | $0.00 |
| Worker Lambda | 30 runs × 300 seconds × 0.5 GB × $0.0000166667/GB-second, plus 30 × $0.20/M requests | $0.08 |
| Scheduler | 30 invocations × $1/M before the shared 14M/month free allowance | <$0.01 |
| CloudWatch logs | Assumed 0.1 GB ingested and retained for the month: `0.1 × ($0.50 + $0.03)/GB` | $0.05 |
| Two low-cardinality custom metrics | Two × $0.30/month reference; no team-ID dimensions | $0.60 |
| Three standard alarms | Three × $0.10/month reference | $0.30 |
| Failure handling | Placeholder for a bounded DLQ/replay path and request charges; resource and regional price pending #103 | $0.01 placeholder |

**Reference subtotal:** about **$1.26/month in production** or **$1.24/month in development**, assuming no other charges. These small values are not a spend authorization or a sound ceiling. The largest uncertainty is not the example AWS bill: it is permitted LPS use, fanout, all-game write amplification, worker duration, and indefinite history growth. A failed invocation, redrive, larger item, or extra metric dimension changes the model. Sources: [DynamoDB pricing](https://aws.amazon.com/dynamodb/pricing/), [Lambda pricing](https://aws.amazon.com/lambda/pricing/), [EventBridge Scheduler pricing](https://aws.amazon.com/eventbridge/pricing/), [CloudWatch pricing](https://aws.amazon.com/cloudwatch/pricing/), and [SQS pricing](https://aws.amazon.com/sqs/pricing/). AWS's example DynamoDB and CloudWatch rates are region-sensitive; obtain the `us-west-2` quote before approval.

## Offline behavior evidence at this checkpoint

The following focused commands passed locally at `0a552e90` with fake Cognito, LPS, and DynamoDB boundaries:

```text
go test ./internal/app -run 'Test(SoccerImportDisclosesAndCapturesEveryLinkedPlayersMembership|ManualTeamLookupArchivesSourceFactsAndReportsEnrollment|SoccerHistoryReadCalculatesOnlyNumericScoredGamesForProvenTeamSeason|SoccerPlayerRemovalRequiresFreshSameOwnerProofAndAllowsLaterImport)$' -count=1
go test ./internal/soccerarchive -run 'Test(RefreshWorkerAppliesCorrectionsAndRetainsOmittedGames|RefreshWorkerReportsInvalidTransientAndSuccessfulTeamsIndependently|DynamoArchivePersistsOwnerBoundPlayerSeasonEvidenceAndTeamEnrollment|DynamoArchiveRemovesOnePlayerGloballyAndRetainsSharedFacts)$' -count=1
```

Those tests separately prove enrollment, exact owner-bound membership, correction/omission refresh, authorized read, and verified global removal. They do not yet make one continuous enrollment → membership → scheduled refresh → read → removal journey against one shared fake archive; #103's daily entrypoint is absent. Build that dry run after the worker branch merges, and record its request counts, fake response bytes, GetItem/PutItem/Query/Delete counts, item-size distribution, retries, retained bytes, and elapsed duration. Fake success cannot establish LPS permission, live invalid-ID behavior, DynamoDB capacity/cost, or an AWS deployment.

## Current resource and approval map

| Environment | Checked-in archive declaration | Current IAM and observed gap |
| --- | --- | --- |
| Development | `portfolio-lambda-dev-soccer-history`, on-demand DynamoDB, `due-teams` GSI, no PITR/deletion protection, `us-west-2` | HTTP Lambda policy includes archive `GetItem`, `PutItem`, `Query`, `DeleteItem`; no separate daily worker, scheduler, or failure path is declared in this checkpoint |
| Production | `portfolio-lambda-prod-soccer-history`, on-demand DynamoDB, `due-teams` GSI, PITR and deletion protection, `us-west-2` | Same HTTP actions; production root remains plan-only under its existing launch decisions; no daily worker/scheduler activation is authorized |

After #103 merges, enumerate exact proposed Lambda function, execution role and boundary, table and index ARNs, scheduler target role, schedule disabled/enabled state, log group/retention, fixed-cardinality metrics and alarms, failure destination and redrive authority, and each `dev`/`prod` plan action. The worker role should receive only the archive table/index operations, scoped logs/metrics, and any reviewed failure-destination action it actually needs. Public-HTTP egress to approved LPS endpoints is a separate network/code control, not an IAM action. Avoid granting the worker player-removal or unrelated portfolio permissions. A daily schedule must remain disabled until the separate live source-use, numeric limit, AWS plan, and activation approvals are recorded.

For a later saved-plan review, first obtain distinct approval for the exact environment and state-lock write. The repo's `DEPLOY-INSTRUCTIONS.md` and `Taskfile.yaml` require the reviewed non-root profile in `us-west-2`, an absolute unused `PLAN_FILE`, a digest-qualified image, the exact lock URI, and environment-specific inputs. `task lambda-dev-plan` and `task lambda-prod-plan` use AWS-backed state locks and are **not** part of this packet. Once separately authorized, retain the private saved plan and SHA-256, inspect both the human plan and `tofu show -json` through `task lambda-plan-check`, verify only expected create/update/no-op actions and no schedule activation, and seek a separate approval before any apply. Offline `task lambda-infrastructure-ci` validates configuration without the backend, but does not replace the saved-plan or live-resource review.

## Remaining gates

1. Obtain LPS's applicable AUP/API and written answer for these endpoints, daily volume/rate, and indefinite player evidence retention.
2. Merge #103, inspect its actual bound/pacing/retry/admission controls and complete the one-archive fake journey; replace the candidate assumptions with measured fake counters and permitted live sample data.
3. Quote `us-west-2` AWS rates and itemize the final worker, scheduler, failure handling, alarms, and table/index costs for the exact plan; choose a numeric cap with a clear overflow behavior and alert.
4. Review an environment-specific saved plan only after approval for its AWS state lock; compare exact resources, IAM, disabled schedule, and plan hash. Obtain separate creation and later activation authorizations.

Until those gates are met, **do not activate daily LPS polling**.
