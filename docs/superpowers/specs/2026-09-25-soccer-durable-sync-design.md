# Durable LPS soccer history sync design

**Status:** Approved design, 2026-09-25. Implementation and live activation pending.

**Scope:** [Issue #80](https://github.com/CraigDevJohnson/portfolio/issues/80).
The stats view is tracked separately in
[Issue #81](https://github.com/CraigDevJohnson/portfolio/issues/81).
This document records product decisions. It does not approve an AWS apply or a
live recurring LPS job.

## Purpose and terms

Store source-available Let's Play Soccer (LPS) player, team, season, facility,
and game facts for known teams. The first read use is a team's completed game
history and win/loss/draw record by season. Collection is bounded by teams
linked to a valid imported player or entered by Team ID. It does not claim to
enumerate the LPS catalog.

An **LPS season** is the upstream `Season` identifier, with division context
when supplied. It is distinct from the temporary imported JWT browser session.
A **known team** has a Team ID found through a valid player import or supplied
manually and accepted by LPS. An **enrolled team** is a known team admitted to
daily refresh. **Membership evidence** is an exact player ID, team ID, and LPS
season association obtained through an authenticated player-team lookup, with
its source and observation time recorded. An entered Team ID is collection
input, never membership evidence.

The current client calls authenticated `/users/check` and
`/players/{id}/my_teams`, then fetches known `/teams/{id}` and
`/facilities/{id}` without the imported JWT. Its team response models a season
ID but no join or leave date. The existing `soccer-sessions` DynamoDB table
holds an expiring import baseline, not durable game history. See
[`internal/lps/resolver.go`](../../../internal/lps/resolver.go),
[`internal/lps/types.go`](../../../internal/lps/types.go), and
[`infra/lambda/modules/service/dynamodb.tf`](../../../infra/lambda/modules/service/dynamodb.tf).

## Accepted collection contract

- The player import requires a current site session and `soccer` grant under
  [Issue #83](https://github.com/CraigDevJohnson/portfolio/issues/83).
  [Issue #84](https://github.com/CraigDevJohnson/portfolio/issues/84) owns the
  reusable import flow. The LPS JWT proves linked-player access, not site
  identity. Bind retained import and membership provenance to the validated
  Cognito issuer and subject for the current environment, never an email alone.
- On every successful player import, discover team and season associations for
  every player returned by LPS, including players the visitor did not select.
  Capture the associations while the JWT is valid. Retain the available player
  IDs, names, player flags, and observed membership evidence. Do not store the
  JWT in the durable dataset.
- Accept a manually entered Team ID as a collection seed only after LPS accepts
  it. Persist source-available team, season, division, facility, and game fields
  for known teams, including source IDs, home and away team IDs, scheduled
  times, field, facility, and result. Preserve every game returned by a known
  team's schedule response. Record which team and season responses were
  actually fetched; older seasons not returned by LPS remain unknown.
- Attempt daily refresh for every enrolled ID while LPS continues accepting
  it, including dormant teams and manually entered IDs. A later invalid ID
  stops future polling but does not erase history. A temporary network error,
  rate limit, or upstream failure does not make an ID invalid. An empty or old
  schedule is not an inactivity signal.
- Admit new IDs only within a measured request and cost limit. Keep daily
  attempts for already enrolled IDs; reject further enrollment clearly and
  alert when capacity is full. Reserve new enrollment capacity for teams
  discovered through authenticated player imports before accepting more
  anonymous manual Team IDs. Pace and cap upstream requests. The numeric
  limit remains an activation gate because no enrolled-team census or source
  rate limit has been established.
- Identify games by stable upstream game ID, deduplicate games seen from both
  teams, and replace mutable fields with the latest fetched values when LPS
  corrects a score, time, or team. Record fetch time. A full version history
  is not required. Do not delete a stored game merely because one later
  response omits it. Record per-team success, failure, and coverage.

## Accepted read and privacy contract

Every stats request requires a current site session, the current `soccer`
grant, and a valid same-owner LPS import that confirmed the player ID. Site
identity uses the validated Cognito issuer and subject. The LPS import may be
restored to the same site owner within its allowed lifetime under #84; the
visitor need not paste the JWT for every read. Site sign-out or JWT expiry
ends access until the required sessions are valid again.

The player may read a team's stored season only when exact player, team, and
LPS season membership evidence is available from the current authenticated
lookup or an earlier authenticated observation. The earlier observation may
prove a former association after a valid same-owner import confirms the
player ID. A prior observation is not a prerequisite when the current lookup
provides proof.
Season membership is sufficient; game-day roster or attendance proof is not
required. Name matches, manually entered Team IDs, and game dates do not
establish membership. If a season lacks positive proof, keep its game data but
deny that player's stats read and report membership as unverified.

For an authorized team season, provide its completed game history and a
team-relative win/loss/draw record calculated from games with numeric scores.
Label the record as calculated from scored games, not as official LPS
standings. Treat canceled games, `Final` without a score, and unparseable or
missing results as unclassified; report incomplete coverage or results.
Include the team, season, source freshness, and collection status so #81 can
distinguish no games from missing or failed collection. Do not infer player
goals, assists, appearances, or attendance from team scores.

Retain the durable player identity, membership evidence, and team and game
facts indefinitely, without the imported-session TTL. The import UI must
explain that the JWT access is temporary while linked-player and team facts
will be stored and refreshed indefinitely. The user explicitly accepts this
collection through the disclosed import action.

A verified removal request deletes that player's identity and membership
proofs throughout the shared dataset, including owner associations. Team and
game facts remain. A later valid import may collect the player again. The
request must verify the current site identity and `soccer` grant and confirm
authority over the player before global deletion.

## Proposed implementation shape

This is design guidance for a later, separately reviewed implementation plan:

1. Add a dedicated durable DynamoDB data store. Keep it separate from the
   current TTL-bound soccer import audit table. Model indexed reads for due
   teams, game history by team and season, and player-to-team-season proof;
   avoid table-wide scans on ordinary job and stats paths.
2. Capture player-team-season evidence during the #84 import workflow,
   after checking the #83 site session and `soccer` grant. Bind the observation
   to the validated issuer and subject. Enqueue or otherwise checkpoint
   accepted Team IDs. A dedicated worker,
   separate from the 29-second HTTP Lambda, refreshes known public team and
   facility endpoints on a daily schedule. Retries and repeated invocations
   must converge on the same records.
3. Bound the worker's total LPS requests, rate, and retry work. Handle invalid
   IDs separately from transient 429, 5xx, and network errors. Preserve
   successful teams when another team fails. Report missed daily attempts
   rather than declaring a partial run complete.
4. Expose an authenticated read contract for #81. Check the current #83
   site session and `soccer` grant on every request, then verify a valid
   same-owner LPS import, player ID, and exact season membership on the
   server for each requested team season. Saved manual workflow Team IDs
   confer no authorization.
5. Update the import explanation before enabling durable collection. Provide
   a verified global player-data removal path.

## Acceptance evidence

An implementation should demonstrate:

- A multi-player import discovers and records every linked player's teams,
  while the durable records contain no JWT.
- Valid manually entered IDs collect data without granting stats access.
- All returned games, stable IDs, source fields, corrected results, and
  per-team/season coverage survive repeated and partial syncs.
- Daily attempts include valid dormant and manual teams. Invalid IDs stop
  polling without deleting history; temporary failures retry and report.
- Enrollment at the reviewed capacity limit fails visibly without dropping
  existing enrolled teams' daily work.
- A current site session and `soccer` grant plus a valid same-owner LPS
  import and exact current or saved authenticated membership proof grant only
  that player and team season. A different site owner, missing grant, expired
  import, or unproven season fails closed.
- Calculated records use numeric score results and identify missing or
  unclassified games rather than claiming official standings.
- Import disclosure and verified global player removal work as specified;
  team/game history remains, and a later valid import can recollect the player.

## Source and activation gates

- No separate LPS guidance for automated daily collection or long-term storage
  has been supplied. Review the applicable
  [LPS terms](https://www.letsplaysoccer.com/terms-and-conditions?lang=en),
  published API guidance, and acceptable request behavior before activation.
  The public terms do not state a numerical rate limit for this job.
- Verify the actual Team ID response contract. The current client accepts a
  decodable 2xx response, including an empty schedule, but that alone may not
  establish a usable team. Confirm how LPS signals invalid IDs and historical
  season coverage before defining production validation.
- Measure distinct enrolled teams, facility lookups, response sizes, changed
  games, retry rates, worker duration, and retained bytes. Itemize DynamoDB,
  worker, schedule, logs, alarms, and failure-queue costs. Set and review the
  numerical admission and request ceilings from those observations before
  any live job. A billing alert is not an immediate traffic limit.
- Complete the #83 site identity and #84 reusable import contracts before
  activating private collection and reads. Review environment-specific
  infrastructure, IAM, and a saved plan before creating AWS resources or
  enabling collection. Development and production activation require separate
  authorization. A design, local tests, or a passing plan is not proof of a
  running job.
- An official LPS standings page exposes a partial player leaderboard. Its
  embedded division-season ID is not established as the same identifier as
  the current client's `Season`. Investigate exact-ID mapping and source
  stability before adding public standings as optional positive membership
  proof or importing leaderboard statistics. No name-only proof is allowed.

## Issue boundaries

[Issue #80](https://github.com/CraigDevJohnson/portfolio/issues/80) owns
collection, durable storage, authorization, deletion, and the read contract.
[Issue #81](https://github.com/CraigDevJohnson/portfolio/issues/81) owns the
stats presentation and remains blocked by #80. Its presentation choices,
including how to open stats and select among linked players and teams, remain
open there. [Issue #79](https://github.com/CraigDevJohnson/portfolio/issues/79)
and its [implementation spec #84](https://github.com/CraigDevJohnson/portfolio/issues/84)
own the reusable LPS import, reactive schedule planner, game selection, and
calendar controls. [Issue #83](https://github.com/CraigDevJohnson/portfolio/issues/83)
owns the site session, stable issuer/subject, and `soccer` page grant. #80
consumes these contracts for collection and every private stats read.
