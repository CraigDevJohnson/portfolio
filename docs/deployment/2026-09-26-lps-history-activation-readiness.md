<!-- markdownlint-disable MD013 -->
# LPS history sync activation readiness

**Issue:** #104 under #80. **Prepared:** September 30, 2026, against the #80
loop branch at `da915fdd`: main through the workloads-account move (#119),
plus issues #83 and #85 to #103. A Codex draft of this packet from September
26 predates the workloads move and the reworked #98 to #103; this version
replaces it.

**Decisions, September 30, 2026.** Craig decided gates 3, 4 and 5: the
candidate limits and daily time in 3.3 are accepted, the admission cap is a
lifetime cap, the admission alarm counts only refused player-linked teams, and
import latency is bounded in code (#100). He will send LPS the section 2
questions himself (gate 1, [Appendix A](#appendix-a-message-to-lps)). The
repository changes in 6.1 items 1, 3 to 7 and 9 are made, with offline tests
only; whether development collects is still open. Section 7 records the
membership first-seen note for #81.

**Recommendation: blocked.** Do not enable collection or the daily schedule in
either environment. Gates 1, 2 and 7 to 11 are open. Gates 3 and 6 are
closed except for development: whether it collects is undecided, and 6.1 item
2 waits on that. The [remaining gates](#remaining-gates) list what each needs.
This packet is a review artifact only: it does not approve source use, spend,
an AWS plan or an activation.

Nothing in this packet contacted LPS, AWS, Cognito or Google. No backend was
initialized, no state lock was taken, and nothing was planned against AWS,
applied, or scheduled. Every measurement below comes from fakes and is marked
as such.

## 1. What exists and what is off

History collection is built into the application and the service module, and
it is off in both environments at three levels:

| Level | Switch on the branch | Current state |
| --- | --- | --- |
| Table and HTTP grant | `enable_soccer_history`, set in each environment's `*.auto.tfvars` | `false` in both; the environment contract tests assert that neither plans a history table, worker, schedule or alarm |
| HTTP enrollment, admission counter, admission alarm and refused-lookup metric | `activate_soccer_history_collection` plus `soccer_history_limits` | `false`; both environments carry the accepted limits (3.3) |
| Daily worker, schedule, failure queue, worker alarms | `activate_soccer_history_schedule` plus a once-daily UTC `soccer_history_schedule_expression` | `false`; production carries `cron(30 10 * * ? *)`, development no expression |

The application refuses to build a history store or the daily worker unless
the table name and all five limits are set (`LimitsFromEnvironment`,
`NewDailyHistoryWorker`). The HTTP runtime also needs
`SOCCER_HISTORY_COLLECTION_ENABLED=true`. The module plans nothing for history
unless each stage's inputs are supplied together.

## 2. Source-use findings: unresolved

Collection would call these LPS endpoints on `lps-api-prod.lps-test.com`:

| When | Endpoint | Credential | Frequency |
| --- | --- | --- | --- |
| Daily worker, per enrolled team | `GET /teams/{id}` | none | once per team per run, plus retries |
| Daily worker, per team | `GET /facilities/{id}` | none | once per distinct facility the team's games use, per team; lookups are not shared between teams |
| Visitor Team ID lookup (already public today) | `GET /teams/{id}`, `GET /facilities/{id}` | none | per lookup; with collection on, the lookup also enrolls the team |
| Granted, disclosed import | `GET /users/check`, then `GET /players/{id}/my_teams` for each linked player | player's imported JWT | once per import, one call at a time in the order LPS lists the players, all within the import's 11 s lookup deadline (3.2) |
| History read without stored proof | `GET /players/{id}/my_teams` | imported JWT | per read |
| History team-season list | `GET /players/{id}/my_teams` | imported JWT | per list |
| Verified player removal | `GET /users/check` | imported JWT | per removal |

I reviewed these public pages (read only, September 30, 2026):

| Source | What it says | What it does not say |
| --- | --- | --- |
| [LPS Terms and Conditions](https://www.letsplaysoccer.com/terms-and-conditions?lang=en) (Let's Play Sports, Inc.) | Section 3 lets a customer use LPS "APIs and published documentation" with its applications, subject to the terms and an Acceptable Use Policy. Section 3.3 allows immediate suspension for unusual traffic spikes or use that threatens availability. The terms are written for customers holding an account. | No link to the Acceptable Use Policy, and I found no published copy. No rate limit, no request pacing, and nothing about end users reading their own data through a third-party tool, or about how long such a tool may keep it. No published API documentation for these endpoints. |
| [LPS Privacy Policy](https://www.letsplaysoccer.com/privacy-policy?lang=en) | LPS keeps personal data as long as necessary for its purposes and lists a privacy contact address. | Nothing about third parties keeping player data, and no deletion-request process for data a third party copied. |
| [LPS Messaging Policy](https://www.letsplaysoccer.com/messaging-policy?lang=en) | SMS consent and opt-out only. | Nothing relevant. |
| `https://www.letsplaysoccer.com/robots.txt` | Only a sitemap line; the disallow rules are commented out. | It covers the website, not the API host, and says nothing about permission. I did not request anything from the API host. |
| Web searches for LPS or Let's Play Sports API documentation or an Acceptable Use Policy | Nothing found beyond the terms above. | |

**Finding.** No public source permits unattended daily requests to these
endpoints or indefinite retention of player identities, owner links and
team-season membership. None of the following is permission: the endpoints
answering, the site's existing public Team ID lookup making the same
unauthenticated calls, the import disclosure a visitor accepts, or the
verified removal path. They are product controls, not LPS consent.

**Questions for LPS, in writing, before any activation:**

1. May a signed-in player's site keep their team, season, game and facility
   facts, and the player's own identity and team-season links, indefinitely,
   with a player-initiated removal path?
2. May an unattended job call `GET /teams/{id}` and `GET /facilities/{id}`
   without a token once a day for up to 40 teams: about 80 requests a day,
   120 at most per run, one request a second? Is there a rate limit, a
   required pacing, or a required identifying `User-Agent` or contact
   header?
3. Which Acceptable Use Policy applies, and where is it published?
4. How does LPS signal a deleted or invalid Team ID? The worker treats
   `400` and `404` as permanently invalid; `429`, `5xx` and network failures
   as temporary.
5. Do Team IDs persist across seasons, or does each season get new IDs? This
   decides how fast the enrollment cap fills (see 3.3).

**Decision (gate 1, September 30, 2026).** Craig will send these five
questions to LPS himself; no agent sends them.
[Appendix A](#appendix-a-message-to-lps) is a ready-to-send plain-text message.
Record LPS's written answer with the activation approval.

## 3. Traffic, storage and runtime

### 3.1 Fake-backed measurements

`TestLPSHistoryReadinessJourneyFromEnrollmentThroughRemoval`
(`internal/app/soccer_history_readiness_test.go`) drives the real route
assembly and the real daily worker against a counting fake LPS and the real
`DynamoStore` over an in-memory table that meters DynamoDB calls as on-demand
billing counts them (1 KB write units, two for a transactional write; 4 KB
strongly consistent read units, half for an index query; due-index writes
counted separately). It uses the candidate limits in 3.3. Run it with
`go test ./internal/app -run TestLPSHistoryReadinessJourney -count=1 -v`.
Output on this branch:

| Phase | LPS requests | Response bytes | DynamoDB calls | Billed units |
| --- | --- | --- | --- | --- |
| Anonymous Team ID lookup enrolls team 4301 | 1 team, 1 facility | 352 | 7 puts, 2 transactional puts, 7 gets | 11 WRU, 1 index WRU, 7 RRU |
| Disclosed import, 2 linked players | 1 account check, 2 player-team lookups | 464 | 6 puts, 4 transactional puts, 4 gets | 14 WRU, 2 index WRU, 4 RRU |
| Daily run 1: 2 teams due | 2 team, 3 facility | 1,069 | 23 puts, 15 gets, 1 index query | 23 WRU, 4 index WRU, 15.5 RRU |
| Repeated delivery of run 1 | none | 0 | 1 index query | 0.5 RRU |
| Daily run 2: 3 teams, 1 corrected score, 1 omitted game | 3 team, 3 facility | 1,181 | 27 puts, 19 gets, 1 index query | 27 WRU, 6 index WRU, 19.5 RRU |
| Daily run 3: one `503` retried, one Team ID rejected with `404` | 4 team, 2 facility | 829 | 20 puts, 15 gets, 1 index query | 20 WRU, 5 index WRU, 15.5 RRU |
| Daily run 4: 2 teams, nothing changed | 2 team, 2 facility | 829 | 19 puts, 13 gets, 1 index query | 19 WRU, 4 index WRU, 13.5 RRU |
| Verified removal of player 1001 | 1 account check | 277 | 2 queries, 3 deletes | 3 WRU, 2 RRU |

After the journey the table held 34 items, about 8.3 KB: team partitions
4,555 B, games 1,724 B, player partitions 1,095 B, facilities 861 B, the
admission counter 103 B. The fixture's games are about 150 bytes of JSON
each; real LPS games are larger (see 3.2).

Three properties the numbers establish, all from the code as it stands:

- **Every returned game is rewritten on every refresh.** Run 4 changed
  nothing and still wrote 19 items. One team refresh writes
  `F + 3G + 2S + 2` items and reads `F + G + S + 3`, where `F` is facility
  lookups, `G` returned games and `S` returned seasons (each game writes its
  game record and two team-game edges). Changed-game frequency therefore does
  not reduce DynamoDB cost; returned-game count does.
- **Facility lookups are per team.** Two teams at arena 5 cost two arena-5
  lookups in one run.
- **Admission slots are never released.** After LPS rejected team 4301 the
  admission counter still read 3. Teams leave polling when rejected, but keep
  their slot; removing a player removes no team. The enrollment cap is a
  lifetime cap on distinct Team IDs. Craig accepted that lifetime cap on
  September 30, 2026 (gate 4): there is no slot release.

### 3.2 Inputs, bounds and assumptions

No live measurement was possible. Each input below is either bounded by code
or assumed, and the assumption is stated.

| Input | Bound or assumption | Basis | Uncertainty |
| --- | --- | --- | --- |
| Enrolled teams `T` | At most 40 (candidate cap). An entered Team ID is admitted only while fewer than 10 teams of any source are enrolled | Admission transaction in `internal/soccerarchive/admission.go` | Real demand unknown; see 3.3 |
| Facility fanout `F` | 1 per team typical, 2 as the planning bound | Indoor leagues play at their home arena; the fixture has one away arena | Unmeasured |
| Team response size | Assumed 30 games at about 2 KB each, about 60 KB; hard cap 2 MiB per response (`MaxLPSResponseBodySize`) | Fixture games are about 150 B; real LPS games nest both teams | Unmeasured; 120 responses could read up to 240 MiB in the worst case |
| Retry rate | At most one retry per team (`max_retries_per_team = 1`), retrying only `429`, `5xx` and network errors; a retry repeats the whole team fetch, facilities included | `retryingTeamSource` in `daily.go` | Unmeasured; the fixture retried once in 10 team refreshes, by design |
| Changed-game writes | Equal to returned games, not changes: `G` game writes and `2G` edge writes per team per day | 3.1, run 4 | Exact for the current code |
| Retained bytes | Game item about 2.3 KB plus two edges of about 0.2 KB, and DynamoDB's 100 bytes of overhead per item; about 40 new games per team per year (five sessions of eight games); 40 teams give about 5 MB in year one and about 25 MB after five years | Item layout in `dynamo.go`, fixture sizes scaled to 2 KB games | Indefinite retention has no byte ceiling; dormant teams add no games |
| Worker duration | About 80 to 120 s per run at 80 requests paced one a second; at most the 300 s timeout, after which the run stops starting teams and reports the rest | `pacedTransport`, the run deadline logic in `daily.go` | LPS and DynamoDB latency unmeasured |
| Runs per day | One scheduled run. EventBridge Scheduler invokes Lambda asynchronously and retries only when that hand-off fails, so its two retries do not repeat a run that started. Delivery is at least once, so rare duplicates can run; a duplicate refreshes only teams still due, because a success keeps a team from being due for 4 hours and a temporary failure for 15 minutes | [Lambda with Scheduler](https://docs.aws.amazon.com/lambda/latest/dg/with-eventbridge-scheduler.html), [EventBridge FAQ](https://aws.amazon.com/eventbridge/faqs/), `refreshedTeamGuard` and `retryableFailureDelay` | No durable daily request counter exists; the bound is per run, so a duplicate after a budget-limited run can spend another 120 requests |
| Import latency with collection on | Bounded in code. The `1 + P` LPS calls for `P` linked players, one at a time in the order LPS lists them, share one 11 s deadline counted from the request's arrival. A player whose `my_teams` lookup has not finished by then is skipped for that import: it keeps its identity and owner link without memberships, as a player LPS no longer finds does, its teams are not enrolled, and the import's notice names it. The players already listed keep their evidence and the import completes. At most 10 s of history writes (run under their own timeout, not the spent lookup deadline) and 3 s for the import record follow. Rendering the response then checks the owner's Google connection (a DynamoDB read, a possible token refresh and write, a calendar list and a selection save); that check shares the import's 24 s deadline, also counted from arrival, and one cut short keeps the connection. The handler's work therefore ends within 24 s of API Gateway's 29 s, the budget the Google handlers keep. An account lookup that misses the deadline fails the import, as an unreachable LPS always has. Imports that collect no history keep only the 15 s client timeouts | `DefaultHistoryImportLookupBudget`, `DefaultHistoryImportBudget` and `discoverImportedPlayerTeams` in `internal/soccer/auth.go`; `internal/app/soccer_import_deadline_test.go` | Real `my_teams` latency is unmeasured. A slow LPS now costs history, not the import: skipped players wait for a later import. Lookups keep LPS's order on every import, so an LPS that stays slow, rather than briefly slow, can skip the same last players each time, and the notice says only that a later import may collect them. Cold-start initialization (bounded at 8 s) runs before the deadline starts, so a cold start at its bound coinciding with a full 24 s import could still pass 29 s |

Both environments would call the same LPS. If dev and prod both ran the
schedule, the source traffic doubles; any source approval must cover the sum.

### 3.3 Candidate limits

```hcl
soccer_history_limits = {
  max_enrolled_teams      = 40
  reserved_player_slots   = 30
  max_requests_per_run    = 120
  max_retries_per_team    = 1
  min_request_interval_ms = 1000
  worker_timeout_seconds  = 300
}
soccer_history_schedule_expression = "cron(30 10 * * ? *)" # 10:30 UTC, 03:30 Pacific daylight time
```

**Accepted (gate 3, September 30, 2026).** Craig accepted these limits and
the daily time exactly. Both `*.auto.tfvars` carry the limits with every
history switch off; production also carries the expression and development
none, because whether development collects or runs the schedule is still open.
Accepting them activates nothing.

- **40 teams.** Player-linked demand is assumed to be 2 to 4 linked players
  on 1 or 2 teams in about five sessions a year. If LPS issues new Team IDs
  each session, that is 10 to 40 new IDs a year. Because slots are never
  released, 40 lasts roughly one to four years. A player-linked team refused
  at 40 is the trigger to review the cap, and the admission alarm now fires on
  exactly that (next point).
- **30 reserved for player-linked teams.** Any visitor can enter a Team ID
  without signing in, and an entered ID is admitted only while fewer than 10
  teams of any source are enrolled. At most 10 IDs are ever enrolled that way,
  each polled for good, and once 10 teams of any kind are enrolled, entered
  IDs are refused permanently.
- **The admission alarm counts only player refusals.** Every refusal logs
  `soccer_history_admission_rejected` with its `source`
  (`internal/soccer/schedule.go`). The manual limit is 40 − 30 = 10
  (`admissionLimit`), so once 10 teams of any source are enrolled, every
  visitor lookup of an unenrolled Team ID logs a `manual` refusal. As first
  built, the stage 1 metric filter counted every refusal, so one anonymous
  lookup every 5 minutes could hold
  `portfolio-lambda-{env}-soccer-history-admission-rejected` in ALARM and hide
  the player refusals it exists for. Following gate 4 (6.1 item 6, done), the
  `AdmissionRejected` filter matches
  `{ $.msg = "soccer_history_admission_rejected" && $.source = "player" }`,
  and a separate `ManualAdmissionRejected` metric, with no alarm, counts
  refused lookups. Both patterns match the refusal line's top-level `source`
  attribute, which is unambiguous only while the HTTP function keeps
  `LOG_ADD_SOURCE=false`; slog would otherwise also write its code-location
  `source` key. The service contract asserts that setting.
- **120 requests a run.** This is 40 teams × 3: every team with its team
  request and two facility lookups, or with one facility and spare budget to
  retry 20 teams. The application requires at least one request per team.
- **One retry, one request a second.** About 80 requests a day at the cap,
  120 at most per run: about 2,400 a month typical and 3,600 at the per-run
  ceiling, per environment that runs the schedule.
- **300 s timeout.** The module's validation needs at least 163 s for this
  tuple: 119 paced gaps, two attempts of one team at 15 s plus pacing, one
  second of backoff, and 10 s to report.

Offline checks of the tuple:

- The journey test builds the store and the daily worker with it, so
  `Limits.Validate` accepts it.
- The service module's committed contract
  (`infra/lambda/modules/service/tests/service_contract.tftest.hcl`) plans
  the tuple with its mocked provider as a dev collection-only stage and a prod
  schedule stage, accepts the 163 s floor, and rejects 39 requests a run and a
  162 s timeout. `go test ./infra/lambda` runs it as part of
  `task infrastructure-ci`; to run it alone:

  ```text
  tofu -chdir=infra/lambda/modules/service init -backend=false -input=false
  tofu -chdir=infra/lambda/modules/service test
  ... candidate_collection_stage_dev ... pass
  ... candidate_schedule_stage_prod ... pass
  ... candidate_accepts_the_shortest_covering_timeout ... pass
  ... candidate_rejects_a_timeout_below_pacing ... pass
  ... candidate_rejects_a_budget_below_one_request_per_team ... pass
  ```

  Changing 163 to 162, or either rejected value to an accepted one, fails the
  matching run.
- The environment contracts
  (`infra/lambda/environments/{dev,prod}/tests/environment_contract.tftest.hcl`)
  assert that both `*.auto.tfvars` carry exactly this tuple with every switch
  off, that production carries the expression and development none, and that
  neither root plans a history table, worker, schedule or alarm. A further run
  in each root switches every stage on to prove the root passes all five
  inputs to the module.

## 4. Itemized AWS cost

Planning assumptions for one environment running both stages at the cap: 40
teams, one facility each, 30 returned games of about 2 KB in two seasons per
team response, one run a day, 30-day month, no duplicate runs. Prices are the
list prices each service's pricing page showed on September 30, 2026, for its
default region (US East); get a us-west-2 quote before approval. Free tiers
are shared with the rest of the workloads account and are ignored unless
noted.

| Item | Arithmetic | Per month |
| --- | --- | ---: |
| DynamoDB writes | Per team refresh: 1 facility + 30 games × 3 WRU + 60 edges × 1 + 4 season items + 1 coverage + 2 team + 3 due-index = 161 WRU. × 40 teams × 30 days = 193,200 WRU at $0.625 per million | $0.12 |
| DynamoDB reads | Per team refresh: 36 strongly consistent reads of items under 4 KB = 36 RRU. × 40 × 30 + one index query a day = 43,230 RRU at $0.125 per million | $0.01 |
| DynamoDB HTTP enrollment, reads and removal | A few imports, lookups and reads a month; the journey shows tens of units each | < $0.01 |
| DynamoDB storage | About 5 MB in year one, 25 MB after five years, at $0.25 per GB-month (25 GB free tier) | < $0.01 |
| Point-in-time recovery | Production only (`enable_pitr = true`); 5 to 25 MB at $0.20 per GB-month | < $0.01 |
| Worker Lambda | 512 MB × 120 s × 30 = 1,800 GB-s (bound: 300 s, 4,500 GB-s) at $0.0000166667 per GB-s, plus 30 requests | $0.03 (bound $0.08) |
| EventBridge Scheduler | 30 invocations at $1 per million, inside the 14 million free | $0.00 |
| CloudWatch Logs | About 5 KB a run (one report line with up to 40 team results plus Lambda platform lines), 0.15 MB a month at $0.50 per GB ingested and $0.03 per GB stored | < $0.01 |
| Log-derived metrics | Each publishes only when a line matches (no default value) and costs at most $0.30 per metric-month. `ManualAdmissionRejected` publishes in any month a visitor looks up an unenrolled Team ID once 10 teams are enrolled (3.3); `AdmissionRejected` (player refusals only) and `DailyIncomplete` are quiet in a good month | $0.30 (bound $0.90) |
| Alarms | 4 standard alarms (admission rejected, incomplete run, worker errors, failure queue) at $0.10; the 10 existing environment alarms already use the account's 10 free alarms | $0.40 |
| Failure handling | SQS standard queue with SQS-managed encryption: a message only per failed run or delivery, well inside 1 million free requests; alarm notifications go to aws-setup's `alerts` topic (email: first 1,000 free) | $0.00 |
| Data transfer | LPS responses are inbound; requests are small | $0.00 |
| **Total per environment** | Typical: `ManualAdmissionRejected` active, `AdmissionRejected` and `DailyIncomplete` quiet, 120 s runs | **about $0.86** |
| **Bound per environment** | All three metrics active all month, 300 s runs | **about $1.52** |

Sensitivity: 100 games of 4 KB per team response raises writes to about 711
WRU per team refresh (853,200 a month, $0.53) and reads to about $0.03, so the
bound becomes about $1.95 per environment. Running the schedule in both
environments doubles every line except storage. Sources:
[DynamoDB on-demand](https://aws.amazon.com/dynamodb/pricing/on-demand/),
[Lambda](https://aws.amazon.com/lambda/pricing/),
[EventBridge](https://aws.amazon.com/eventbridge/pricing/),
[CloudWatch](https://aws.amazon.com/cloudwatch/pricing/) and
[SQS](https://aws.amazon.com/sqs/pricing/). An AWS budget alert is not a
traffic limit; the request budget and pacing are the only enforced limits.

## 5. Fake-backed journey and what stays unproven

The journey test proves, at the real HTTP route assembly and the real worker
over one table:

1. An anonymous Team ID lookup enrolls a team and proves no membership.
2. A granted, disclosed import records owner-bound player–team–season proof
   for both linked players, with one team lookup per player and no schedule
   fetch; another owner and the entered Team ID have no proof.
3. Daily runs refresh due teams under pacing, skip them on a repeated
   delivery, follow a corrected score, keep an omitted game, retry a `503`
   within budget, stop polling a rejected ID while keeping its facts, and
   report a run with a rejected team as incomplete.
4. The owner's read of the proven season returns coverage and a scored record
   from stored proof without another LPS call; the entered Team ID's season is
   refused with `403`.
5. Verified removal erases the player's partition for every owner with one
   fresh LPS check, keeps team, game and facility facts and the other player,
   and ends the import, so the next read is refused with `401`.

The grant matrix now includes `GET /soccer/history` and
`GET /soccer/history/team-seasons`
(`internal/app/soccer_grant_matrix_test.go`), so signed-out, expired,
ungranted and revoked visitors are shown to be refused there too.

`internal/app/soccer_import_deadline_test.go` drives the same route
assembly with a fake LPS that stalls, under a 200 ms stand-in for the lookup
budget, over an in-memory table that fails any call whose context has ended,
as DynamoDB does. When one player's `my_teams` lookup stalls, the import
returns within twice the budget, names the skipped player, stores the other
player's evidence, and enrolls none of the skipped player's teams. When the
first player stalls, LPS is never asked for the second, so the lookups share
one deadline rather than each having its own. When `/users/check` stalls,
the import fails within twice the budget with nothing stored. When the
owner's fake Google calendar list stalls, the import still completes within
its 400 ms stand-in budget with every player's history saved, and the
Google connection is kept. Imports that collect no history are not bound by
the lookup budget.

**Unproven live, source:** permission (section 2); whether `/teams/{id}` and
`/facilities/{id}` keep answering unauthenticated requests from the Lambda
(the deployed public lookup makes the same calls today, not re-checked for
this packet); LPS's invalid-ID contract; its `429` behaviour and any rate
limit; real response sizes, game counts, facility fanout and latency; whether
Team IDs persist across seasons; whether `my_teams` returns past seasons.

**Unproven live, AWS:** that any history resource exists; the effective
permissions once the boundary changes; real item sizes and billed units (the
meter approximates DynamoDB's sizing rules); duplicate delivery by Scheduler
and Lambda's asynchronous queue with reserved concurrency 1; metric filters
matching the deployed JSON logs; alarm delivery to `alerts`; the monthly bill.

## 6. Infrastructure actions

On September 30, 2026, 6.1 items 1, 3 to 7 and 9 were made in the
repository and item 8 was decided; each is marked below. Nothing has been
planned or applied against AWS. Each code change is an ordinary pull request
with offline tests; each apply needs Craig's approval of that specific saved
plan.

### 6.1 Repository changes before any plan

Each item opens with the state before the September 30 changes. A **Done**,
**Open** or **Decided** note gives its state on this branch; an item without
one is still open.

1. **Wire the environment roots.** Before September 30,
   `environments/{dev,prod}` did not pass
   `enable_soccer_history`, `soccer_history_limits`,
   `activate_soccer_history_collection`, `activate_soccer_history_schedule`
   or `soccer_history_schedule_expression` to the module, and the
   environment contract tests asserted that no history table was
   planned. The change: add the variables and set them in each
   `*.auto.tfvars`, not with `-var`, so an apply of the saved plan sees
   identical inputs, and update those contract tests.
   **Done.** Both roots declare the five inputs with no defaults and pass them
   to the module; the values are in each `*.auto.tfvars` (3.3). The contract
   tests now assert the values, that neither root plans a history table,
   worker, schedule or alarm, and that every stage's inputs reach the module.
2. **Choose a dev alert destination.** Collection requires a nonempty
   `alarm_action_arns`, and `dev.auto.tfvars` has `[]`. Either dev uses the
   workloads `alerts` topic (`arn:aws:sns:us-west-2:793680745829:alerts`), or
   dev does not collect. **Open** with the development part of gate 3.
   Development keeps `alarm_action_arns = []`, so its live alarms are
   unchanged, and it does not collect.
3. **Grant the history runtime in the execution boundaries**
   (`ci-roles/boundary.tf`). **Done** as the tables below describe; the worker
   and Scheduler roles attach the new boundary. Every
   statement below has `Effect = "Allow"` and the same condition shape as
   `boundary.tf`: `ArnEquals` on `aws:PrincipalArn` naming the role, with
   `{Env}` as `Dev` or `Prod` and `{env}` as `dev` or `prod`. ARNs are in
   account 793680745829, region us-west-2.

   **Stage 1, in `PortfolioLambdaExecutionBoundary`**
   (`/portfolio/boundaries/`), one statement per environment:

   | Sid | Actions | Resource | Principal |
   | --- | --- | --- | --- |
   | `{Env}SoccerHistory` | `dynamodb:DeleteItem`, `GetItem`, `PutItem`, `Query` | `arn:aws:dynamodb:us-west-2:793680745829:table/portfolio-lambda-{env}-soccer-history` | `role/portfolio-lambda-{env}-execution` |

   That covers collection, read and removal. The admission transaction is two
   conditional puts, which IAM authorizes as `dynamodb:PutItem`; there is no
   separate `TransactWriteItems` action
   ([DynamoDB transactions and IAM](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/transaction-apis-iam.html)).

   **Stage 2, in a new `PortfolioLambdaHistoryExecutionBoundary`**
   (`/portfolio/boundaries/`), five statements per environment. The worker
   role is `portfolio-lambda-{env}-soccer-history-execution` and the
   Scheduler role `portfolio-lambda-{env}-soccer-history-scheduler`:

   | Sid | Actions | Resource | Principal |
   | --- | --- | --- | --- |
   | `{Env}HistoryWorkerTable` | `dynamodb:GetItem`, `PutItem` | `arn:aws:dynamodb:us-west-2:793680745829:table/portfolio-lambda-{env}-soccer-history` | worker |
   | `{Env}HistoryWorkerDueIndex` | `dynamodb:Query` | `arn:aws:dynamodb:us-west-2:793680745829:table/portfolio-lambda-{env}-soccer-history/index/due-teams` | worker |
   | `{Env}HistoryWorkerLogs` | `logs:CreateLogStream`, `PutLogEvents` | `arn:aws:logs:us-west-2:793680745829:log-group:/aws/lambda/portfolio-lambda-{env}-soccer-history:*` | worker |
   | `{Env}HistoryFailures` | `sqs:SendMessage` | `arn:aws:sqs:us-west-2:793680745829:portfolio-lambda-{env}-soccer-history-failures` | worker and Scheduler (a two-value `ArnEquals` list) |
   | `{Env}HistoryInvoke` | `lambda:InvokeFunction` | `arn:aws:lambda:us-west-2:793680745829:function:portfolio-lambda-{env}-soccer-history` | Scheduler |

   These mirror the worker and Scheduler role policies in
   `history_worker.tf`. The same change points both roles'
   `permissions_boundary` at
   `arn:aws:iam::793680745829:policy/portfolio/boundaries/PortfolioLambdaHistoryExecutionBoundary`
   and updates the service contract's boundary assertion.

   **Size limit.** IAM's managed-policy limit is 6,144 characters. Rendered
   with `jsonencode` (compact JSON, the form `aws_iam_policy.policy` takes)
   in a scratch copy of `ci-roles` under its mocked provider, whose account
   ID has the same length as the real one:

   | Policy | Characters |
   | --- | ---: |
   | `PortfolioLambdaExecutionBoundary` without the stage 1 statements | 4,791 |
   | With the stage 1 statements | 5,470 |
   | With the stage 1 and stage 2 statements in the one policy | 8,772 |
   | `PortfolioLambdaHistoryExecutionBoundary` alone | 3,340 |

   The first three rows include each environment's `SITE_SESSION_KEY` read
   (#85), which adds 342 characters to each; before it they were 4,449,
   5,128 and 8,430. The committed boundary leaves 674 characters of
   headroom.

   So the worker and Scheduler grants need the second policy. Other
   statement shapes give other totals (one statement per role and action
   group renders larger), so the PR must render its own statements the same
   way. IAM enforces the limit only when an apply calls
   `CreatePolicy` or `CreatePolicyVersion`; `tofu plan` never submits the
   document, so no plan catches an overflow. `ci-roles/tests/policies.tftest.hcl`
   asserts that both boundaries fit 6,144 characters, so
   `task infrastructure-ci` fails first. The committed statements render at
   the sizes in the table above.
4. **Grant the CI roles the reads and release writes** (`ci-roles/main.tf`).
   Before September 30, the CI roles read only the existing tables, the
   service function, its execution role, its log groups and the five alarms.
   Once history resources are in an environment's state, every release plan
   refreshes them and would fail without the grants below. Each role gets its
   own environment's ARNs: `{env}` is `dev` for the dev deployer
   `portfolio-development-deployer-ci`
   (policy `portfolio-development-runtime-release`) and `prod` for the prod
   planner `portfolio-production-planner-ci` (`portfolio-production-read-only-plan`)
   and the prod deployer `portfolio-production-deployer-ci`
   (`portfolio-production-runtime-release`). In the table `…` stands for
   `us-west-2:793680745829` and `F` for `portfolio-lambda-{env}`.

   | Stage | Statement | Actions | Resources to add |
   | --- | --- | --- | --- |
   | 1 | `TableRead` (existing) | unchanged: `dynamodb:DescribeContinuousBackups`, `DescribeTable`, `DescribeTimeToLive`, `ListTagsOfResource` | `arn:aws:dynamodb:…:table/F-soccer-history` |
   | 1 | `MetricFilterRead` (new) | `logs:DescribeMetricFilters` | `arn:aws:logs:…:log-group:/aws/lambda/F`, and the same ARN with `:*` |
   | 1 | `AlarmRead` (existing) | unchanged: `cloudwatch:DescribeAlarms`, `ListTagsForResource` | `arn:aws:cloudwatch:…:alarm:F-soccer-history-admission-rejected` |
   | 2 | `MetricFilterRead` | as above | `arn:aws:logs:…:log-group:/aws/lambda/F-soccer-history`, and the same ARN with `:*` |
   | 2 | `AlarmRead` | as above | `arn:aws:cloudwatch:…:alarm:F-soccer-history-incomplete`, and the same with `-errors` and `-dead-letter` in place of `-incomplete` |
   | 2 | `ExecutionRoleRead` (existing) | unchanged: `iam:GetRole`, `GetRolePolicy`, `ListAttachedRolePolicies`, `ListRolePolicies`, `ListRoleTags` | `arn:aws:iam::793680745829:role/F-soccer-history-execution`, `arn:aws:iam::793680745829:role/F-soccer-history-scheduler` |
   | 2 | `LambdaRead` (existing) | unchanged: `lambda:GetAlias`, `GetFunction`, `GetFunctionCodeSigningConfig`, `GetFunctionConcurrency`, `GetFunctionConfiguration`, `GetPolicy`, `GetRuntimeManagementConfig`, `ListTags`, `ListVersionsByFunction` | `arn:aws:lambda:…:function:F-soccer-history`, and the same ARN with `:*` |
   | 2 | `LogGroupRead` (existing) | unchanged: `logs:ListTagsForResource` | `arn:aws:logs:…:log-group:/aws/lambda/F-soccer-history`, and the same ARN with `:*` |
   | 2 | `HistoryWorkerInvokeConfigRead` (new) | `lambda:GetFunctionEventInvokeConfig` | `arn:aws:lambda:…:function:F-soccer-history`, and the same ARN with `:*` |
   | 2 | `HistoryQueueRead` (new) | `sqs:GetQueueAttributes`, `sqs:ListQueueTags` | `arn:aws:sqs:…:F-soccer-history-failures` |
   | 2 | `HistoryScheduleRead` (new) | `scheduler:GetSchedule` | `arn:aws:scheduler:…:schedule/default/F-soccer-history-daily` |
   | 2 | `DevelopmentHistoryWorkerReleaseWrite` / `ProductionHistoryWorkerReleaseWrite` (new, deployers only) | `lambda:PublishVersion`, `lambda:UpdateFunctionCode` | `arn:aws:lambda:…:function:F-soccer-history`, with the same four `aws:ResourceTag` conditions as the existing release write |

   The prod planner gets every read row and no write. Stage 2 rows can land
   with stage 1: they name resources that do not exist yet. Rendered the same
   way as item 3, the three inline policies grow from 4,894, 4,229 and 4,914
   characters to 7,443, 6,385 and 7,483 (dev deployer, prod planner, prod
   deployer), inside IAM's 10,240-character limit that
   `policies.tftest.hcl` already asserts. **Done** with exactly those sizes.
   For the table, function, roles, log
   group and alarms the actions are the ones the existing grants already
   prove; the metric filter, event invoke config, queue and schedule are new
   resource types for these roles. The first release plan after each stage is
   the cross-check: a missing read fails its refresh with `AccessDenied`
   before anything changes.
5. **Teach `scripts/check-lambda-plan.sh` the worker image.** Before
   September 30, it accepted only `aws_lambda_function.app` `image_uri` and
   the `live` alias, so a release plan that also moved
   `module.service.aws_lambda_function.history_worker[0]` to the release
   image was rejected; `tests/release-scripts.sh` held that as a reject case
   ("a history worker image update"). The change: allow exactly that
   attribute, to the same image, turn that case into an accept case, and add
   reject cases for a different worker image and another worker attribute.
   **Done.** The checker accepts the worker's `image_uri` moving to the
   release image only, and `tests/release-scripts.sh` holds that accept case
   and the two reject cases.
6. **Scope the admission alarm to player refusals** (recommended; section
   3.3). Change the `history_admission_rejected` metric filter pattern to
   `{ $.msg = "soccer_history_admission_rejected" && $.source = "player" }`
   and update the service contract's two pattern assertions. Optionally add
   a second metric filter for `source = "manual"` with no alarm, to count
   refused lookups. The pattern relies on the HTTP function keeping
   `LOG_ADD_SOURCE=false`, since slog would otherwise also write its
   code-location `source` key. The alternative is to accept an alarm that any
   visitor can hold in ALARM once 10 teams are enrolled. This is a change to
   the #80 rule "reject and alert on new enrollment at capacity", so it needs
   Craig's decision (gate 4). **Done** after that decision, with the manual
   metric and no alarm on it; the service contract asserts both patterns and
   `LOG_ADD_SOURCE=false`.
7. **Verify the history alarms on release** (recommended, after item 6).
   Before September 30, `scripts/verify-lambda-release.sh` checked only the
   five named alarms and failed a release when any was in ALARM. The
   recommendation was to add the history alarms only once the admission
   alarm counted player refusals alone, because until then any refused
   visitor lookup in the previous 5 minutes would fail the release.
   **Done.** Verification checks every alarm the environment's `alarm_names`
   output names: the five service alarms always, and the history alarms once
   a stage is applied. With history off it asks CloudWatch only for the five,
   so a release works before and after the account root grants the history
   alarm reads.

   **The failure-queue alarm holds releases until the queue is drained.** The
   admission, incomplete-run and worker-error alarms return to OK after 5
   minutes without a new failure (`notBreaching`). The
   `portfolio-lambda-{env}-soccer-history-dead-letter` alarm instead stays in
   ALARM while any message is visible in
   `portfolio-lambda-{env}-soccer-history-failures`
   (`ApproximateNumberOfMessagesVisible` maximum of at least 1), and that
   queue keeps a message for 14 days. A message lands there when a worker run
   fails (an error, a timeout or a crash; an incomplete run is not a
   failure), when a run's event waits more than an hour to start, or when
   Scheduler cannot deliver the daily event. From then on, every release
   in that environment applies its plan and then fails verification, for up
   to 14 days, until an operator inspects the messages, records the cause,
   and deletes them or purges the queue
   ([DEPLOY-INSTRUCTIONS.md, Alarms](../../DEPLOY-INSTRUCTIONS.md#alarms)).
   Once development runs the schedule, a failed development verification
   also stops `production-plan`, which needs the development job. The branch
   keeps this gate on purpose, so a failed daily run is looked at before the
   next release ships; no CI role can receive or purge the queue. Releasing
   past an undrained queue instead, by checking only that this alarm exists,
   would need Craig's decision and a `tests/release-scripts.sh` case.
8. **Decide the admission policy** (section 3.1: slots are never released).
   Accept the cap as a lifetime cap, or add a way to release slots of
   rejected or long-dormant teams. **Decided** (gate 4): a lifetime cap, so no
   code change.
9. **Bound import latency with collection on** (section 3.2). Before
   September 30, the options were one overall discovery deadline, or
   measuring `my_teams` latency and accepting the risk. **Done** under #100
   after Craig chose to bound it in code (gate 5): an import that collects
   history gives its LPS lookups one 11 s deadline and skips players not
   listed by then, and its response's Google check ends by the import's 24 s
   deadline.

### 6.2 Prerequisites outside this repository

- **Lambda concurrency.** The worker reserves one execution. The workloads
  account limit is still 10 (aws-setup #30) and Lambda keeps 100 unreserved,
  so the schedule stage cannot be applied until the limit covers 100 plus
  every reservation: prod's planned 10 and one per environment that runs the
  worker (111, or 112 with both).
- **Site identity.** Player-linked collection needs site sign-in and the
  `soccer` grant in the environment. No environment supplies `SITE_*` yet
  (#88 for production). Anonymous Team ID enrollment works without it.
- **LPS permission** (section 2).

### 6.3 Resources and IAM per environment

Names come from `modules/service`; the workloads account is 793680745829,
region us-west-2. Development is `portfolio-lambda-dev` (14-day logs, no PITR,
no deletion protection, state key `portfolio-lambda-http-api/dev/terraform.tfstate`);
production is `portfolio-lambda-prod` (30-day logs, PITR and deletion
protection, state key `portfolio-lambda-http-api/prod/terraform.tfstate`),
both in `portfolio-tofu-state-793680745829`.

**Stage 1, collection only** (`enable_soccer_history`, limits,
`activate_soccer_history_collection`, schedule off). Expected plan actions:

| Address | Action | Detail |
| --- | --- | --- |
| `module.service.aws_dynamodb_table.soccer_history[0]` | create | `portfolio-lambda-{env}-soccer-history`, on demand, `pk`/`sk`, `due-teams` index (`due_pk`/`due_sk`, all attributes), SSE, no TTL; PITR and deletion protection per environment |
| `module.service.aws_iam_role_policy.lambda` | update | adds `dynamodb:GetItem`, `PutItem`, `Query`, `DeleteItem` on the table |
| `module.service.aws_lambda_function.app` | update | adds `SOCCER_HISTORY_COLLECTION_ENABLED=true`, `SOCCER_ARCHIVE_TABLE_NAME` and the five `SOCCER_HISTORY_*` limits; publishes a new version |
| `module.service.aws_lambda_alias.live` | update | `live` moves to that version |
| `module.service.aws_cloudwatch_log_metric_filter.history_admission_rejected[0]` | create | `AdmissionRejected` in `Portfolio/SoccerHistory` on `/aws/lambda/portfolio-lambda-{env}`, counting refused player-linked teams only |
| `module.service.aws_cloudwatch_log_metric_filter.history_manual_admission_rejected[0]` | create | `ManualAdmissionRejected` in `Portfolio/SoccerHistory` on the same log group, counting refused visitor Team ID lookups; no alarm |
| `module.service.aws_cloudwatch_metric_alarm.history_admission_rejected[0]` | create | `portfolio-lambda-{env}-soccer-history-admission-rejected`, ≥ 1 in 5 minutes, to `alarm_action_arns` |

Stage 1 starts enrollment through visitor Team ID lookups and granted imports.
It starts no daily polling.

**Stage 2, daily schedule** (adds `activate_soccer_history_schedule` and the
expression). Expected creates, all with `portfolio-lambda-{env}-soccer-history`
names: `aws_sqs_queue.history_dead_letter[0]` (`-failures`, 14-day retention,
SSE-SQS); `aws_cloudwatch_log_group.history_worker[0]`;
`aws_iam_role.history_worker[0]` (`-execution`) and
`aws_iam_role_policy.history_worker[0]` (`-runtime`: table `GetItem`/`PutItem`,
index `Query`, log stream writes, queue `SendMessage`);
`aws_lambda_function.history_worker[0]` (release image,
`SOCCER_HISTORY_MODE=scheduled`, 512 MB, timeout 300 s, reserved concurrency
1); `aws_lambda_function_event_invoke_config.history_worker[0]` (no retries,
one-hour event age, failures to the queue); `aws_iam_role.history_scheduler[0]`
and its policy (`lambda:InvokeFunction` on the worker, queue `SendMessage`,
trusted only for schedule `-daily`); `aws_scheduler_schedule.history_daily[0]`
(`-daily`, UTC, `ENABLED`, two delivery retries, dead-letter queue);
`aws_cloudwatch_log_metric_filter.history_incomplete[0]`; alarms `-incomplete`,
`-errors` and `-dead-letter`. **Applying stage 2 starts live LPS polling at the
next scheduled time;** there is no dormant-worker stage.

### 6.4 Offline validation evidence

- `task infrastructure-ci` passes on this branch (offline: formatting,
  validation with `-backend=false`, the mocked `tofu test` suites including the
  service module's 29 runs, fifteen of them history contracts, release script
  tests and operator plan tests). It contacts no AWS account.
- The service module's committed history contracts cover: nothing planned
  without limits; limits alone activate nothing; collection needs the table,
  limits and an alert destination; collection adds only the admission alarm;
  the schedule needs collection and an expression; the worker is bounded,
  reserved to one, and monitored; the worker and Scheduler roles are scoped
  and inside the boundary. Five of them plan the candidate tuple (section
  3.3): a dev collection stage, a prod schedule stage, the 163 s floor, and
  the rejected 162 s timeout and 39-request budget.
- `ci-roles/tests/policies.tftest.hcl` asserts every statement of both
  boundaries and that each fits IAM's 6,144-character limit (5,470 and 3,340
  rendered), which a plan cannot check (6.1 item 3). It also asserts each CI
  role's history reads and worker release write, and that the three inline
  policies fit 10,240 characters (7,443, 6,385 and 7,483; 6.1 item 4). The
  sizes are measured from the mocked plan, whose account ID has the real
  one's length.
- The service contract asserts the player-only admission alarm, the manual
  refusal metric with no alarm, `LOG_ADD_SOURCE=false`, and the worker and
  Scheduler roles inside `PortfolioLambdaHistoryExecutionBoundary` (6.1
  items 3 and 6).
- `tests/release-scripts.sh` shows that the plan checker accepts a release
  plan that also moves the history worker to the release image and rejects a
  different worker image or another worker attribute (6.1 item 5), and that
  release verification checks exactly the alarms an environment's outputs
  name, failing on a firing or missing one (6.1 item 7).

### 6.5 Reviewing a saved plan

Each step that touches AWS needs its own approval from Craig before it runs,
because `plan` against a real backend takes the S3 state lock and reads live
state, and `apply` changes AWS.

1. **Merge the #80 runtime.** Merge the #80 loop branch and the 6.1 changes
   to main after review, with `task infrastructure-ci` green and history
   still off in both `*.auto.tfvars`. Main does not have the history runtime
   today: `cmd/lambda` has no `SOCCER_HISTORY_MODE` entry point and
   `modules/service` has no history resources. A stage 1 plan against an
   image built without it would set `SOCCER_HISTORY_*` variables that nothing
   reads, and stage 2 would create a worker with no scheduled mode.
2. **Account root.** `aws sso login`, `task lambda-ci-roles-init`,
   `task lambda-ci-roles-plan PLAN_FILE=/absolute/path/ci-roles.tfplan`.
   Expect only an update to `aws_iam_policy.lambda_execution_boundary`, a
   create of `aws_iam_policy.lambda_history_execution_boundary`
   (`PortfolioLambdaHistoryExecutionBoundary`), and updates to the three CI
   role policies (`aws_iam_role_policy.environment["dev"]`,
   `aws_iam_role_policy.environment["prod"]` and
   `aws_iam_role_policy.production_deployer`). Review it with the listing in step 5 (run with
   `-chdir=infra/lambda/ci-roles`), and apply with
   `task lambda-ci-roles-apply` only after approval. The site identity
   release order ([DEPLOY-INSTRUCTIONS.md, Release order](../../DEPLOY-INSTRUCTIONS.md#release-order),
   decision 1) applies the account root from the same branch before its
   merge, to let both environments read `SITE_SESSION_KEY`. If that apply
   already carried these grants, a new plan here shows no changes and this
   step is done. Otherwise the boundary update here also adds those
   `SITE_SESSION_KEY` reads, which `boundary.tf` already carries.
3. **Release that merge to the target environment.** Let the Release
   workflow deploy the merge commit, with history still off. Its
   verification (`scripts/verify-lambda-release.sh`) checks that the `live`
   alias runs the released digest. Before planning, confirm that the
   environment's `live` alias still runs that digest: the image URI of the
   alias's version ends in `@sha256:<release digest>`. This is the same
   read-only `aws lambda get-alias` and `get-function` check the script
   makes. If the release plan also changed infrastructure, the checker stops
   the release and Craig applies that first, as for any release.
4. **Stage 1 in one environment.** After a reviewed change sets
   `enable_soccer_history` and `activate_soccer_history_collection` to `true`
   in its `*.auto.tfvars` (the limits are already there), plan with the digest
   confirmed in step 3, so the plan shows only history changes:
   `task lambda-dev-plan IMAGE_DIGEST=sha256:<release digest from step 3> PLAN_FILE=/absolute/path/dev-history-collection.tfplan`
   (production: `task lambda-prod-plan`, which also sets the `alerts` topic).
5. Review the saved plan without printing secrets:

   ```sh
   shasum -a 256 /absolute/path/dev-history-collection.tfplan
   tofu -chdir=infra/lambda/environments/dev show -json /absolute/path/dev-history-collection.tfplan |
     jq -r '.resource_changes[] |
       select(.change.actions != ["no-op"] or .previous_address != null or
         .change.importing != null or .deposed != null) |
       "\(.change.actions | join(",")) \(.address) from=\(.previous_address // "-") importing=\(.change.importing != null) deposed=\(.deposed // "-")"'
   ```

   The filter keeps pure moves and imports, which `show -json` records as
   `no-op` with `previous_address` or `change.importing` set. It matches
   the checks in `scripts/check-lambda-plan.sh`. Reject the plan if any row
   has `from`, `importing` or `deposed` set, or any delete or replace, or any
   IAM change beyond the table grant. Accept only the stage 1 rows in 6.3.
   Check the limits in the function's planned environment.
6. Apply exactly that file with `task lambda-dev-apply PLAN_FILE=…` after
   approval of that plan hash. Right before applying, run
   `shasum -a 256` on the file again and compare it with the approved hash.
   The CI plan checker rejects such plans by design; infrastructure changes
   are applied by Craig, then the Release workflow resumes. Its next plan is
   the cross-check of the CI read grants in 6.1 item 4.
7. **Stage 2 is a separate decision** with its own plan, hash, review against
   the stage 2 list, and approval that explicitly authorizes live LPS polling
   at the reviewed time. Watch the first run's report line and the alarms. A
   failed run leaves a message in the failure queue, and its alarm then fails
   every release verification in that environment until the queue is drained
   (6.1 item 7).

## 7. Note for #81: membership first-seen time

Each granted import rewrites a membership's `observed_at` with the time of
that import (`internal/soccerarchive/membership.go`), so the store keeps the
latest observation, not when the membership was first seen. Decision 10
(September 30, 2026): keep a first-seen time only if the #81 stats view needs
it. #81 is undecided, so nothing changed. If #81 needs it, write the
first-seen time only when the membership record is created. A conditional put
needs no new IAM action; an `UpdateItem` with `if_not_exists` would need
`dynamodb:UpdateItem` in the runtime policy and both boundaries. Records
written before that change have no first-seen time, and verified removal
deletes it with the rest of the player's partition.

## Remaining gates

Collection and scheduling stay blocked until every gate below is closed.
Owner in brackets.

1. **LPS permission** for unattended daily requests and indefinite retention,
   with answers to the questions in section 2. Craig decided on September 30,
   2026 to send the five questions himself
   ([Appendix A](#appendix-a-message-to-lps)); open until LPS answers in
   writing. [Craig]
2. **Live source behaviour** measured under that permission: invalid-ID
   responses, rate limiting, response sizes, games per response, facility
   fanout, latency, and whether Team IDs persist across seasons. Recompute
   sections 3 and 4 from them. [Craig to authorize; agent to measure]
3. **Numeric limits and schedule: decided September 30, 2026, except for
   development.** Craig accepted the candidate in 3.3 exactly:
   `max_enrolled_teams` 40, `reserved_player_slots` 30,
   `max_requests_per_run` 120, `max_retries_per_team` 1,
   `min_request_interval_ms` 1000, `worker_timeout_seconds` 300 and
   `cron(30 10 * * ? *)`. Both `*.auto.tfvars` carry them with every switch
   off. **Still open:** whether development collects or runs the daily
   schedule, and so its alert destination (6.1 item 2). Until Craig decides,
   development does not collect, has no schedule and keeps
   `alarm_action_arns = []`. [Craig]
4. **Admission policy: decided September 30, 2026.** The cap is a lifetime cap
   on distinct Team IDs with no slot release; entered IDs are refused once 10
   teams are enrolled. The admission alarm counts only refused player-linked
   teams (6.1 item 6, done); refused visitor lookups have a metric with no
   alarm.
5. **Import latency** with collection on: closed. Craig chose on September
   30, 2026 to bound it in code. The import's LPS lookups now share an 11 s
   deadline, and its response's Google check ends by the import's 24 s
   deadline (3.2, 6.1 item 9). [done]
6. **Repository changes** in 6.1 items 1 to 7. Done on September 30, 2026:
   items 1 (environment wiring), 3 (boundary grants within the IAM size
   limit), 4 (CI role grants), 5 (the plan checker), 6 (the admission alarm
   scope) and 7 (release verification). Item 2, the development alert
   destination, waits on the open part of gate 3. [agent, reviewed by Craig]
7. **The #80 runtime merged and released**: the #80 loop branch and the 6.1
   changes merged to main, and the target environment's `live` alias running
   a release image built from that merge, confirmed by digest, before that
   environment's stage 1 plan (6.5 steps 1 and 3). Main has no history
   runtime today. [Craig]
8. **Lambda concurrency** limit of at least 111 (112 if both environments run
   the worker) before stage 2 (aws-setup #30). [aws-setup]
9. **Site identity** in the environment before player-linked collection
   (#88 for production). [Craig]
10. **Cost acceptance** from a us-west-2 quote of the reviewed plan. [Craig]
11. **Separate approvals** for the account root plan and apply, each
    environment's stage 1 plan and apply, and each stage 2 plan and apply,
    each for an exact saved-plan hash. [Craig]

Until then, **do not activate collection or daily LPS polling.**

## Appendix A: message to LPS

Craig sends this himself (gate 1). It holds no credentials or personal data
beyond his name and public site. Plain text, ready to send to LPS support:

```text
Subject: Questions about using the LPS API from a personal website

Hello LPS support,

I run a small personal website, craigdevjohnson.com. Its Soccer page shows
LPS team schedules: a Team ID lookup calls GET /teams/{id} and
GET /facilities/{id} without a token, and a player who chooses to import
their own LPS account calls GET /users/check and GET /players/{id}/my_teams
with that player's token.

Before I turn on a feature that keeps a history of past seasons, I would
like your written guidance on five questions:

1. May my site keep a signed-in player's team, season, game and facility
   details, and the player's own identity and team-season links,
   indefinitely, if the player can remove that data from the site at any
   time?

2. May an unattended job call GET /teams/{id} and GET /facilities/{id}
   without a token once a day for up to 40 teams? That is about 80
   requests a day, at most 120 per run, at one request per second. Is
   there a rate limit or a required pacing, and do you require an
   identifying User-Agent or contact header?

3. Which Acceptable Use Policy applies to this use, and where is it
   published?

4. How does the API signal a deleted or invalid Team ID? My job treats
   400 and 404 as permanent, and 429, 5xx and network failures as
   temporary.

5. Do Team IDs stay the same across seasons, or does each season get new
   IDs?

I will not turn this feature on until I hear from you, and I am glad to
change the schedule, volume or anything else you prefer.

Thank you,
Craig Johnson
craigdevjohnson.com
```
