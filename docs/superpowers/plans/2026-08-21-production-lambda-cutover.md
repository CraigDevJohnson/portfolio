# Production Lambda Cutover Implementation Plan

<!-- markdownlint-disable MD013 MD010 -->

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Promote the development-tested Lambda image digest to an isolated production environment, move the apex and `www` behavior off Amplify, and retain a fully recorded rollback until production proves stable.

**Architecture:** Instantiate the proven service module under a dedicated production backend with protected DynamoDB tables, 90-day logs, confirmed alarms, and the same immutable ECR digest accepted in development. Build and validate the API, certificate, and custom domains alongside Amplify, then change only reviewed Cloudflare rules and records.

**Tech Stack:** OpenTofu, AWS Lambda, API Gateway HTTP API, ECR, DynamoDB, SSM, ACM, CloudWatch, Cloudflare, Google OAuth, GitHub

**Spec:** `docs/superpowers/specs/2026-08-21-aws-lambda-platform-migration-design.md`

## Global Constraints

- Start only after the accepted [2026-08-29 development App Runner retirement design](../specs/2026-08-29-development-app-runner-retirement-design.md) is reviewed, the preserved failed development evidence remains unchanged, and current public development health reports the same release SHA recorded for promotion.
- Promote the accepted ECR digest; do not rebuild an image for production.
- Keep Amplify, its `main` branch, apex association, certificate-validation record, and current Cloudflare rollback coordinates intact.
- Use fresh production DynamoDB tables and a fresh production session key. Do not copy legacy encrypted Google rows in this plan.
- Do not expose secret values in OpenTofu, shell arguments, files, terminal output, Git, or evidence documents.
- Every AWS apply consumes a reviewed saved plan and every Cloudflare mutation names the exact record or rule.
- Keep Cloudflare proxying only after dynamic paths are confirmed uncached and the origin is verified.
- Production table deletion protection and point-in-time recovery remain enabled.
- Amplify retirement is not part of this plan.
- Approval of this plan does not approve a future live mutation. At every
  **Approval gate**, stop and present the exact commands, saved plan checksum,
  record/rule changes, URL purge list, or GitHub text; continue only after
  approval in that execution session.
- Never rely on an AWS profile or release variable from an earlier task. Every
  AWS block selects `portfolio-deployer` explicitly, reruns the non-root guard
  before mutation, and re-derives the digest or endpoint it consumes.

---

### Task 1: Revalidate prerequisites and capture immutable release coordinates

**Files:**

- No repository changes
- Read-only evidence in the execution log

**Interfaces:**

- Consumes: merged replacement PR, accepted development alias, image digest, and health revision
- Produces: immutable production input record and verified non-root identity

- [ ] **Step 1: Verify current main, accepted development retirement prerequisite, and health**

```bash
git fetch origin
git switch main
git pull --ff-only
test -z "$(git status --porcelain)"

export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-dev-init
release_sha=$(curl --fail --show-error --silent \
  https://dev.craigdevjohnson.com/healthz | jq -er .revision)
release_record="docs/deployment/evidence/releases/${release_sha}.json"
test -f "$release_record"
evidence_file=$(jq -er .development.observation_evidence "$release_record")
test -f "$evidence_file"
test "$(jq -er .source_sha "$release_record")" = "$release_sha"
test "$(jq -er .development.healthz_revision "$release_record")" = "$release_sha"
test "$(jq -er .development.observation_completed_at "$release_record")" = "null"
test "$(git rev-parse HEAD:docs/deployment/evidence/development-observation.jsonl)" = \
  "$(git rev-parse 927a11835dc217cc228361b383e54565af73c2cb:docs/deployment/evidence/development-observation.jsonl)"
jq -e '.passed == false and .unresolved_blockers == ["rollback origin failed"]' "$evidence_file"
git merge-base --is-ancestor "$release_sha" origin/main
```

Expected: main is clean; the accepted retirement design is the governing
development prerequisite; the failed evidence remains byte-for-byte unchanged
with its recorded blocker; and current public health, the release record, and
the promotion SHA agree. Stop if the release JSON or evidence is absent,
inconsistent, or altered.

- [ ] **Step 2: Read the live development alias and digest**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
dev_function=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
dev_alias_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias --function-name "$dev_function" --name live --query FunctionVersion --output text)
dev_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function --function-name "$dev_function" --qualifier "$dev_alias_version" --query Code.ImageUri --output text)
case "$dev_image_uri" in
  *@sha256:*) ;;
  *) echo "Development function is not digest pinned" >&2; exit 1 ;;
esac
release_sha=$(curl --fail --show-error --silent https://dev.craigdevjohnson.com/healthz | jq -er .revision)
release_record="docs/deployment/evidence/releases/${release_sha}.json"
test "$dev_image_uri" = "$(jq -er .image.uri "$release_record")"
test "$dev_alias_version" = "$(jq -er .development.live_alias_target "$release_record")"
echo "$dev_function $dev_alias_version $dev_image_uri"
```

- [ ] **Step 3: Re-run the non-root and state-bucket gates**

Use the identity, account, state-bucket versioning, and legacy-state metadata
checks from the development plan. Stop on a root ARN or any unexpected legacy
state-object change.

- [ ] **Step 4: Verify the release exists and its scan is complete**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
dev_function=$(tofu -chdir=infra/lambda/environments/dev output -raw lambda_function_name)
dev_alias_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias --function-name "$dev_function" --name live --query FunctionVersion --output text)
dev_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function --function-name "$dev_function" --qualifier "$dev_alias_version" --query Code.ImageUri --output text)
release_digest=${dev_image_uri##*@}
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ecr describe-images \
  --repository-name portfolio-lambda-releases \
  --image-ids imageDigest="$release_digest" \
  --query 'imageDetails[0].{Tags:imageTags,Digest:imageDigest,Scan:imageScanStatus.status,PushedAt:imagePushedAt}' \
  --output json
```

Expected: the digest has the accepted tag formed from `git-` plus the full 40-character source SHA, and its scan is complete. Review any critical or high findings before proceeding.

---

### Task 2: Inventory the production edge and old frontend integration

**Files:**

- Create: `docs/deployment/evidence/production-precutover.md`
- Create: `docs/deployment/evidence/production-precutover.json`
- Do not record secrets, cookies, account credentials, or decrypted environment values

**Interfaces:**

- Consumes: Cloudflare dashboard/API read access and Amplify read access
- Produces: exact rollback records, rules, cache behavior, OAuth dependencies, and public-route baseline

Before each Amplify or AWS read in this task, explicitly select
`AWS_PROFILE=portfolio-deployer`, set `AWS_REGION=us-west-2`, and run
`task _lambda-identity-check`; do not inherit the default root profile.

- [ ] **Step 1: Export Cloudflare DNS records and relevant rules**

```bash
mkdir -p docs/deployment/evidence
```

Record for apex and `www`:

- record IDs, types, names, contents, TTLs, and proxy state;
- redirect rules and their expressions/actions;
- cache rules, especially any Cache Everything or Edge TTL setting;
- SSL/TLS mode, HSTS behavior, and origin rules; and
- the ACM validation CNAME used by Amplify.

Store only non-secret configuration in the evidence document. Preserve the raw export outside Git when it contains zone identifiers or unrelated records.
Use `apply_patch` to place the exact rollback subset in the companion JSON with
`schema_version`, `environment`, `captured_at`, `public_hosts`, `dns_records`,
`redirect_rule`, `cache_rule`, and `tls` fields. Each apex and `www` DNS object
contains exact `id`, `type`, `name`, `content`, numeric `ttl`, and boolean
`proxied` values. `redirect_rule` contains nonempty `id`, `expression`, `action`,
and `target` strings plus numeric `status_code`. `cache_rule` contains nonempty
`id`, `expression`, and `action` strings plus nullable numeric
`edge_ttl_seconds` and `browser_ttl_seconds`. `tls` contains nonempty `mode` and
`minimum_version` strings plus boolean `hsts_enabled`.

- [ ] **Step 2: Identify the old Amplify frontend's public Lambda dependency**

Read the value of `VITE_LAMBDA_FUNCTION_NAME` without printing unrelated
environment values. Confirm whether the named function still exists, what public
frontend route uses it, and whether the new Go application already owns that
user-visible behavior. Record only the function name and dependency conclusion.

- [ ] **Step 3: Capture the Amplify rollback origin and status**

Record app ID `duh662t9ntbp`, branch `main`, latest successful job ID, default
domain, apex association status, and the public response's `Last-Modified`,
`ETag`, `Age`, `Cache-Control`, and redirect behavior. Do not modify the branch,
domain association, or validation record. Store the same non-secret values in
the companion JSON under `amplify`, including `rollback_origin_url` as an HTTPS
URL to the default Amplify domain.

- [ ] **Step 4: Capture route and asset baselines**

Verify and record status, content type, redirect target, and a body hash for:

```text
https://craigdevjohnson.com/
https://craigdevjohnson.com/about
https://craigdevjohnson.com/experience
https://craigdevjohnson.com/skills
https://craigdevjohnson.com/projects
https://craigdevjohnson.com/education
https://craigdevjohnson.com/contact
https://www.craigdevjohnson.com/
```

The static Amplify app may return its shell for route paths; record observed behavior rather than assuming SSR semantics.

- [ ] **Step 5: Commit the sanitized evidence**

```bash
git switch -c codex/production-lambda-cutover origin/main
jq -e '
  .schema_version == 1 and
  .environment == "production" and
  (.captured_at | fromdateiso8601) > 0 and
  .public_hosts == ["craigdevjohnson.com", "www.craigdevjohnson.com"] and
  (.amplify.rollback_origin_url | startswith("https://")) and
  (.dns_records | keys) == ["apex", "www"] and
  all(.dns_records[];
    (.id | length) > 0 and (.type | length) > 0 and (.name | length) > 0 and
    (.content | length) > 0 and (.ttl | type) == "number" and
    (.proxied | type) == "boolean") and
  (.redirect_rule.id | length) > 0 and
  (.redirect_rule.expression | length) > 0 and
  (.redirect_rule.action | length) > 0 and
  (.redirect_rule.target | length) > 0 and
  (.redirect_rule.status_code | type) == "number" and
  (.cache_rule.id | length) > 0 and
  (.cache_rule.expression | length) > 0 and
  (.cache_rule.action | length) > 0 and
  (.cache_rule.edge_ttl_seconds == null or
    (.cache_rule.edge_ttl_seconds | type) == "number") and
  (.cache_rule.browser_ttl_seconds == null or
    (.cache_rule.browser_ttl_seconds | type) == "number") and
  (.tls.mode | length) > 0 and
  (.tls.minimum_version | length) > 0 and
  (.tls.hsts_enabled | type) == "boolean"' \
  docs/deployment/evidence/production-precutover.json
git add -- docs/deployment/evidence/production-precutover.md docs/deployment/evidence/production-precutover.json
git commit -m "docs: record production cutover baseline"
```

**Approval gate:** Present the evidence diff, exact branch and commit, draft PR
title/body, repository, and base. Push and open the draft production-cutover PR
only after current-session approval. All later production configuration commits
in Task 6 update this reviewed branch; nothing is pushed directly from `main`.

---

### Task 3: Revalidate the protected production root already merged by the replacement PR

**Files:**

- Review only: `infra/lambda/environments/prod/`
- Review only: `infra/lambda/modules/service/`, `Taskfile.yaml`, and `.github/workflows/ci.yml`
- No production-root creation or direct push from this plan

**Interfaces:**

- Consumes: the production root and commands reviewed in the replacement PR
- Produces: proof that merged `main` owns backend key
  `portfolio-lambda-http-api/prod/terraform.tfstate` and retains all safety inputs

- [ ] **Step 1: Prove the merged replacement PR contains the complete root**

```bash
for path in \
  infra/lambda/environments/prod/backend.hcl \
  infra/lambda/environments/prod/versions.tf \
  infra/lambda/environments/prod/providers.tf \
  infra/lambda/environments/prod/variables.tf \
  infra/lambda/environments/prod/main.tf \
  infra/lambda/environments/prod/outputs.tf \
  infra/lambda/environments/prod/prod.auto.tfvars \
  infra/lambda/environments/prod/.terraform.lock.hcl
do
  git cat-file -e "origin/main:$path"
done
git diff --exit-code origin/main -- infra/lambda Taskfile.yaml .github/workflows/ci.yml
```

- [ ] **Step 2: Recheck the backend and production safety contract**

Require the exact production backend key, `portfolio-lambda-prod` name prefix,
29-second timeout, 90-day logs, PITR and deletion protection enabled, apex and
`www` domain names, and both domain activation flags false. Require
`alarm_action_arns`, `ecr_repository_url`, and `image_digest` to have no defaults.

- [ ] **Step 3: Validate the merged root offline**

```bash
export TF_VAR_ecr_repository_url=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases
export TF_VAR_image_digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export TF_VAR_alarm_action_arns='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'

tofu -chdir=infra/lambda/environments/prod init -backend=false -input=false
tofu -chdir=infra/lambda/environments/prod fmt -check
tofu -chdir=infra/lambda/environments/prod validate
task ci
git diff --exit-code
```

- [ ] **Step 4: Verify hosted replacement checks, not a new direct push**

Read the merged replacement PR check rollup and require both application and
infrastructure jobs to have succeeded. If any production-root correction is
needed, stop and open a focused `codex/` PR; do not commit or push directly from
`main`.

---

### Task 4: Establish production alerts and SecureStrings

**Files:**

- Create: `docs/deployment/evidence/production-alarm-delivery.json`
- Create or select one confirmed SNS notification path
- Create three production SSM SecureStrings

**Interfaces:**

- Produces: nonempty `TF_VAR_alarm_action_arns`, sanitized alarm-delivery proof,
  and `/portfolio/lambda/prod/*`
- Preserves: all legacy parameter values and Google records

- [ ] **Step 1: Select the non-root identity and inspect the dedicated topic**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' \
  --output text)
printf '%s\n' "$alarm_topic_arn"
if [ "$alarm_topic_arn" != "None" ]; then
  subscriptions=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
    sns list-subscriptions-by-topic --topic-arn "$alarm_topic_arn" --output json)
  subscription_fingerprint=$(printf '%s' "$subscriptions" | \
    jq -cS '[.Subscriptions[] | {Protocol,Endpoint,SubscriptionArn}] | sort_by(.Protocol,.Endpoint,.SubscriptionArn)' | \
    shasum -a 256 | awk '{print $1}')
  printf 'subscription_fingerprint=%s\n' "$subscription_fingerprint"
  printf '%s' "$subscriptions" | jq \
    '[.Subscriptions[] | {Protocol,Status:.SubscriptionArn}]'
fi
```

- [ ] **Step 2: Create or repair the notification path only when needed**

If the query returned `None`, stop and obtain the intended notification endpoint
from the user. Prepare exactly one topic create and one subscription create. If
the topic exists but has no confirmed subscription, prepare only one subscription
create against that ARN. If it already has a confirmed subscription, skip this
step. If an existing pending subscription belongs to a different redacted
endpoint or protocol, stop and obtain direction instead of adding another one.

**Approval gate:** Present the account, region, observed topic state, exact action
(`create-and-subscribe` or `subscribe-existing`), topic name or ARN, protocol,
redacted endpoint, and the reviewed subscription fingerprint (`absent` when the
topic does not exist). After current-session approval, set `SNS_TOPIC_ACTION`
and `APPROVED_SUBSCRIPTIONS_FINGERPRINT` to those exact reviewed values and run
the guarded explicit-profile block. Do not continue until the user completes
the out-of-band subscription confirmation. The `subscribe-existing` branch
cannot create a topic, rejects any subscription-list change, and rejects a
pending subscription for a different endpoint or protocol. The
`create-and-subscribe` branch fails if a topic appeared after review.

```bash
set -euo pipefail
: "${ALERT_PROTOCOL:?set the approved protocol}"
: "${ALERT_ENDPOINT:?set the approved notification endpoint}"
: "${SNS_TOPIC_ACTION:?set create-and-subscribe or subscribe-existing}"
: "${APPROVED_SUBSCRIPTIONS_FINGERPRINT:?set absent or the reviewed fingerprint}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' \
  --output text)
case "$SNS_TOPIC_ACTION" in
  create-and-subscribe)
    test "$alarm_topic_arn" = "None"
    test "$APPROVED_SUBSCRIPTIONS_FINGERPRINT" = "absent"
    alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns create-topic \
      --name portfolio-lambda-prod-alerts --query TopicArn --output text)
    ;;
  subscribe-existing)
    test "$alarm_topic_arn" != "None"
    subscriptions=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
      sns list-subscriptions-by-topic --topic-arn "$alarm_topic_arn" --output json)
    subscription_fingerprint=$(printf '%s' "$subscriptions" | \
      jq -cS '[.Subscriptions[] | {Protocol,Endpoint,SubscriptionArn}] | sort_by(.Protocol,.Endpoint,.SubscriptionArn)' | \
      shasum -a 256 | awk '{print $1}')
    test "$subscription_fingerprint" = "$APPROVED_SUBSCRIPTIONS_FINGERPRINT"
    confirmed_count=$(printf '%s' "$subscriptions" | jq \
      '[.Subscriptions[] | select(.SubscriptionArn != "PendingConfirmation" and .SubscriptionArn != "Deleted")] | length')
    test "$confirmed_count" = "0"
    pending_other_count=$(printf '%s' "$subscriptions" | jq \
      --arg protocol "$ALERT_PROTOCOL" --arg endpoint "$ALERT_ENDPOINT" \
      '[.Subscriptions[] | select(.SubscriptionArn == "PendingConfirmation" and (.Protocol != $protocol or .Endpoint != $endpoint))] | length')
    test "$pending_other_count" = "0"
    ;;
  *)
    printf 'unsupported SNS_TOPIC_ACTION: %s\n' "$SNS_TOPIC_ACTION" >&2
    exit 2
    ;;
esac
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns subscribe \
  --topic-arn "$alarm_topic_arn" \
  --protocol "$ALERT_PROTOCOL" \
  --notification-endpoint "$ALERT_ENDPOINT" >/dev/null
```

- [ ] **Step 3: Assert confirmation and prove delivery**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' \
  --output text)
test "$alarm_topic_arn" != "None"
subscriptions=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-subscriptions-by-topic \
  --topic-arn "$alarm_topic_arn" --output json)
printf '%s' "$subscriptions" | jq -e \
  '[.Subscriptions[] | select(.SubscriptionArn != "PendingConfirmation" and .SubscriptionArn != "Deleted")] | length > 0' >/dev/null
export TF_VAR_alarm_action_arns="[\"$alarm_topic_arn\"]"
```

**Approval gate:** Present the exact topic and a non-sensitive test subject/body.
Publish one test notification only after current-session approval, and require
the user to return its unique receipt token within five minutes before production
infrastructure is planned. Record topic ARN, confirmed count, SNS message ID,
sent time, and receipt-confirmed time without subscriber endpoints. The approved
publish uses:

```bash
: "${SNS_RECEIPT_TOKEN:?set the displayed approved receipt token}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
test "$alarm_topic_arn" != "None"
confirmed_count=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  sns list-subscriptions-by-topic --topic-arn "$alarm_topic_arn" --output json | \
  jq '[.Subscriptions[] | select(.SubscriptionArn != "PendingConfirmation" and .SubscriptionArn != "Deleted")] | length')
test "$confirmed_count" -ge 1
sns_sent_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
sns_message_id=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns publish \
  --topic-arn "$alarm_topic_arn" \
  --subject "Portfolio production alarm delivery test" \
  --message "Receipt token: $SNS_RECEIPT_TOKEN" \
  --query MessageId --output text)
printf 'message_id=%s sent_at=%s confirmed_count=%s\n' \
  "$sns_message_id" "$sns_sent_at" "$confirmed_count"
```

After the user returns the exact token, set the captured non-secret values in a
fresh shell, verify the token and five-minute bound, and calculate only its hash:

```bash
: "${SNS_RECEIPT_TOKEN:?set the original receipt token}"
: "${SNS_RETURNED_TOKEN:?set the token returned by the user}"
: "${SNS_MESSAGE_ID:?set the captured SNS message ID}"
: "${SNS_SENT_AT:?set the captured RFC3339 send time}"
: "${SNS_CONFIRMED_COUNT:?set the captured confirmed count}"
test "$SNS_RETURNED_TOKEN" = "$SNS_RECEIPT_TOKEN"
SNS_RECEIPT_CONFIRMED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
jq -en --arg sent "$SNS_SENT_AT" --arg confirmed "$SNS_RECEIPT_CONFIRMED_AT" \
  '(($confirmed | fromdateiso8601) - ($sent | fromdateiso8601)) as $elapsed |
   $elapsed >= 0 and $elapsed <= 300' >/dev/null
SNS_RECEIPT_TOKEN_SHA256=$(printf '%s' "$SNS_RECEIPT_TOKEN" | shasum -a 256 | awk '{print $1}')
printf 'receipt_confirmed_at=%s receipt_token_sha256=%s\n' \
  "$SNS_RECEIPT_CONFIRMED_AT" "$SNS_RECEIPT_TOKEN_SHA256"
```

Use `apply_patch` to create
`docs/deployment/evidence/production-alarm-delivery.json` with
`schema_version`, `environment`, `account_id`, `region`, `topic_arn`,
`confirmed_subscription_count`, `message_id`, `sent_at`,
`receipt_confirmed_at`, and `receipt_token_sha256`. Never record the raw token or
subscriber endpoints. Validate the recorded values against the captured shell
values, including the five-minute bound, then commit only that file as
`docs: record production alarm delivery proof`. Keep the commit on the existing
production cutover branch; its next approved push carries it into the draft PR.

```bash
: "${SNS_TOPIC_ARN:?set the reviewed topic ARN}"
: "${SNS_MESSAGE_ID:?set the captured message ID}"
: "${SNS_SENT_AT:?set the captured send time}"
: "${SNS_RECEIPT_CONFIRMED_AT:?set the captured confirmation time}"
: "${SNS_CONFIRMED_COUNT:?set the captured confirmed count}"
: "${SNS_RECEIPT_TOKEN:?set the returned and matched receipt token}"
receipt_token_sha256=$(printf '%s' "$SNS_RECEIPT_TOKEN" | shasum -a 256 | awk '{print $1}')
alarm_evidence=docs/deployment/evidence/production-alarm-delivery.json
jq -e \
  --arg topic "$SNS_TOPIC_ARN" \
  --arg message "$SNS_MESSAGE_ID" \
  --arg sent "$SNS_SENT_AT" \
  --arg confirmed "$SNS_RECEIPT_CONFIRMED_AT" \
  --arg token_hash "$receipt_token_sha256" \
  --argjson count "$SNS_CONFIRMED_COUNT" '
    .schema_version == 1 and
    .environment == "production" and
    .account_id == "180294223248" and
    .region == "us-west-2" and
    .topic_arn == $topic and
    .confirmed_subscription_count == $count and $count >= 1 and
    .message_id == $message and
    .sent_at == $sent and
    .receipt_confirmed_at == $confirmed and
    .receipt_token_sha256 == $token_hash and
    ((($confirmed | fromdateiso8601) - ($sent | fromdateiso8601)) as $elapsed |
      $elapsed >= 0 and $elapsed <= 300)' "$alarm_evidence"
git add -- "$alarm_evidence"
git commit -m "docs: record production alarm delivery proof"
```

- [ ] **Step 4: Create a fresh production session key without printing it**

**Approval gate:** Present the exact session-key target path and that its value
will be newly generated. Continue only after approval of that one SecureString
create.

```bash
set -euo pipefail
set +x
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
for name in LPS_SESSION_KEY CLIENT_ID_KEY CLIENT_SECRET_KEY; do
  test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "/portfolio/lambda/prod/$name" --query Parameter.Name --output text 2>/dev/null || true)" = ""
done
openssl rand -hex 32 | \
  jq -Rn '{Name:"/portfolio/lambda/prod/LPS_SESSION_KEY",Type:"SecureString",Overwrite:false,Value:input}' | \
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm put-parameter --cli-input-json file:///dev/stdin >/dev/null
```

- [ ] **Step 5: Copy the existing OAuth client through a pipe**

**Approval gate:** Re-present the two exact source-to-target path mappings and
the target-absence check. Copy them only after separate current-session approval.
If any earlier create partially succeeded, inventory the exact existing targets
and obtain fresh approval for only the missing paths; never set overwrite true.

```bash
set -euo pipefail
set +x
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
for name in CLIENT_ID_KEY CLIENT_SECRET_KEY; do
  test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "/portfolio/lambda/prod/$name" --query Parameter.Name --output text 2>/dev/null || true)" = ""
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm get-parameter \
    --name "/portfolio/$name" --with-decryption --output json | \
  jq --arg target "/portfolio/lambda/prod/$name" \
    '{Name:$target,Type:"SecureString",Overwrite:false,Value:.Parameter.Value}' | \
  aws --profile "$AWS_PROFILE" --region "$AWS_REGION" ssm put-parameter --cli-input-json file:///dev/stdin >/dev/null
done
```

- [ ] **Step 6: Verify metadata only**

Use the explicit deployment profile with `describe-parameters` and require
exactly three production SecureStrings. Do not copy the seven legacy Google
table records; production users reconnect after cutover.

---

### Task 5: Plan, apply, and prove the production direct endpoint

**Files:**

- No repository changes
- Create production AWS resources only from the reviewed saved plan

**Interfaces:**

- Consumes: accepted development digest and confirmed alarm topic
- Produces: protected production Lambda/API/tables/IAM/logs/alarms with no DNS change

- [ ] **Step 1: Initialize the production backend and export exact inputs**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-prod-init
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
release_sha=$(jq -er .source_sha "$RELEASE_RECORD")
release_digest=$(jq -er .image.digest "$RELEASE_RECORD")
repository_url=$(jq -er .image.repository_url "$RELEASE_RECORD")
image_uri=$(jq -er .image.uri "$RELEASE_RECORD")
test "$(jq -er .image.tag "$RELEASE_RECORD")" = "git-$release_sha"
test "$image_uri" = "$repository_url@$release_digest"
test "$(tofu -chdir=infra/lambda/artifacts output -raw ecr_repository_url)" = "$repository_url"
export TF_VAR_ecr_repository_url="$repository_url"
export TF_VAR_image_digest="$release_digest"
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
test "$alarm_topic_arn" != "None"
```

- [ ] **Step 2: Create a private saved plan**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-prod-init
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
release_digest=$(jq -er .image.digest "$RELEASE_RECORD")
image_uri=$(jq -er .image.uri "$RELEASE_RECORD")
alarm_topic_arn=$(aws --profile portfolio-deployer --region us-west-2 sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
test "$alarm_topic_arn" != "None"
plan_dir=$(mktemp -d)
prod_plan="$plan_dir/prod.tfplan"
prod_plan_json="$prod_plan.json"
task lambda-prod-plan \
  PLAN_FILE="$prod_plan" \
  IMAGE_DIGEST="$release_digest" \
  ALARM_ACTION_ARN="$alarm_topic_arn"

tofu -chdir=infra/lambda/environments/prod show -json "$prod_plan" > "$prod_plan_json"
jq -r '.resource_changes[] | [.address, (.change.actions | join(","))] | @tsv' "$prod_plan_json"
task lambda-plan-check \
  PLAN_JSON="$prod_plan_json" \
  ENVIRONMENT=prod \
  NAME_PREFIX=portfolio-lambda-prod \
  IMAGE_URI="$image_uri" \
  ALARM_ACTIONS_JSON="[\"$alarm_topic_arn\"]"
```

- [ ] **Step 3: Enforce mechanical production-plan assertions**

The checker must prove:

- no delete or replace action;
- no App Runner, Amplify, legacy ECR, or legacy table address;
- each name-bearing Lambda, IAM, DynamoDB, managed log-group, API, and alarm
  field starts with `portfolio-lambda-prod`, excluding `$default`, domain names,
  ARNs, and provider-owned identifiers;
- image URI equals the accepted development digest;
- both tables enable PITR and deletion protection;
- log retention equals 90 days;
- all five alarms have exactly `[$alarm_topic_arn]` as their action list; and
- no secret value appears in plan JSON or human output.

- [ ] **Step 4: Apply only the exact approved plan**

**Approval gate:** Stop and present the saved plan's absolute path and checksum,
complete action list, exact development digest, confirmed alarm topic, and all
mechanical assertions. Run only after the user approves that exact saved plan
in the current execution session:

```bash
: "${APPROVED_PROD_PLAN:?set the exact approved production plan path}"
test -f "$APPROVED_PROD_PLAN"
test -f "$APPROVED_PROD_PLAN.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-prod-init
task lambda-prod-apply PLAN_FILE="$APPROVED_PROD_PLAN"
```

- [ ] **Step 5: Verify function, alias, data protection, and alarms**

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
prod_function=$(tofu -chdir=infra/lambda/environments/prod output -raw lambda_function_name)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda wait function-active-v2 --function-name "$prod_function"
prod_version=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias \
  --function-name "$prod_function" --name live --query FunctionVersion --output text)
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-alias --function-name "$prod_function" --name live --output json
actual_image_uri=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" lambda get-function \
  --function-name "$prod_function" --qualifier "$prod_version" \
  --query 'Code.ImageUri' --output text)
test "$actual_image_uri" = "$(jq -er .image.uri "$RELEASE_RECORD")"

for table in \
  "$(tofu -chdir=infra/lambda/environments/prod output -raw google_connection_table_name)" \
  "$(tofu -chdir=infra/lambda/environments/prod output -raw soccer_session_table_name)"
do
  test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" dynamodb describe-table \
    --table-name "$table" --query Table.DeletionProtectionEnabled --output text)" = "True"
  test "$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" dynamodb describe-continuous-backups \
    --table-name "$table" --query ContinuousBackupsDescription.PointInTimeRecoveryDescription.PointInTimeRecoveryStatus --output text)" = "ENABLED"
done

alarm_names=()
while IFS= read -r alarm_name; do
  alarm_names+=("$alarm_name")
done < <(tofu -chdir=infra/lambda/environments/prod output -json alarm_arns | \
  jq -r '.[] | split(":")[-1]')
test "${#alarm_names[@]}" = "5"
alarm_json=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch describe-alarms \
  --alarm-names "${alarm_names[@]}" --output json)
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
printf '%s' "$alarm_json" | jq -e --arg topic "$alarm_topic_arn" \
  '(.MetricAlarms | length) == 5 and
   all(.MetricAlarms[]; (.StateValue == "OK" or .StateValue == "INSUFFICIENT_DATA") and .AlarmActions == [$topic])' >/dev/null
```

Expected: the exact development-tested image URI is live, both tables are
protected with PITR, and exactly five healthy-or-new alarms point only at the
confirmed topic.

- [ ] **Step 6: Exercise the direct endpoint**

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
release_sha=$(jq -er .source_sha "$RELEASE_RECORD")
prod_url=$(tofu -chdir=infra/lambda/environments/prod output -raw api_default_url)
test "$(curl --fail --show-error --silent "$prod_url/healthz" | jq -er .revision)" = "$release_sha"

check_content() {
  url=$1
  expected=$2
  test "$(curl --show-error --silent --output /dev/null --write-out '%{http_code}' "$url")" = "200"
  content_type=$(curl --show-error --silent --output /dev/null --write-out '%{content_type}' "$url")
  case "$content_type" in
    "$expected"*) ;;
    *) echo "unexpected content type for $url: $content_type" >&2; return 1 ;;
  esac
}

check_content "$prod_url/healthz" "application/json"
check_content "$prod_url/" "text/html"
check_content "$prod_url/about" "text/html"
check_content "$prod_url/soccer" "text/html"
check_content "$prod_url/static/css/tailwind.css" "text/css"
check_content "$prod_url/static/images/backgrounds/home-hero.jpg" "image/jpeg"
```

Expected: revision matches development and all requests succeed.

- [ ] **Step 7: Require an empty convergence plan and unchanged legacy metadata**

Create and inspect a second plan with the same record and alarm inputs:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
release_digest=$(jq -er .image.digest "$RELEASE_RECORD")
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
convergence_dir=$(mktemp -d)
convergence_plan="$convergence_dir/prod-convergence.tfplan"
task lambda-prod-plan \
  PLAN_FILE="$convergence_plan" \
  IMAGE_DIGEST="$release_digest" \
  ALARM_ACTION_ARN="$alarm_topic_arn"
tofu -chdir=infra/lambda/environments/prod show -json "$convergence_plan" | \
  jq -e '[.resource_changes[]? | select(.change.actions != ["no-op"])] | length == 0'
rm "$convergence_plan"
rmdir "$convergence_dir"

: "${APPROVED_PROD_PLAN:?set the exact applied production plan path}"
test "$(basename "$APPROVED_PROD_PLAN")" = "prod.tfplan"
rm -- "$APPROVED_PROD_PLAN" "$APPROVED_PROD_PLAN.json"
rmdir -- "$(dirname "$APPROVED_PROD_PLAN")"
```

Re-run the legacy `portfolio/terraform.tfstate` metadata check and compare it to
Task 1. The cleanup removes only the explicitly reloaded applied plan, its JSON,
and the fresh convergence plan.

---

### Task 6: Prove production OAuth and activate custom domains without DNS traffic

**Files:**

- Modify: `infra/lambda/environments/prod/prod.auto.tfvars`
- Modify externally: Google OAuth redirect allowlist and Cloudflare ACM-validation records

**Interfaces:**

- Consumes: accepted direct endpoint
- Produces: issued apex/`www` ACM certificate and Regional API mappings not yet receiving public DNS traffic

- [ ] **Step 1: Add the execute-api callback and complete one production OAuth workflow**

Re-derive `prod_url` from the production root:

```bash
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-prod-init
prod_url=$(tofu -chdir=infra/lambda/environments/prod output -raw api_default_url)
printf '%s\n' "${prod_url%/}/soccer"
```

**Approval gate:** Present the
complete current redirect list and the one exact `${prod_url%/}/soccer` addition.
Add it only after current-session approval without removing any URI.

Complete connect/callback and calendar selection. Before writing Google Calendar,
present the exact calendar, selected game, expected event ID, and result-sync
scope. **Approval gate:** perform the chosen add and sync only after separate
current-session approval. This creates fresh production records encrypted with
the new production session key.

- [ ] **Step 2: Request the production certificate**

Change only `request_custom_domain = true` and commit
`feat(infra): request production API certificates` on the draft production
cutover branch. Create a saved plan using the reviewed release record and alarm
ARN; require ACM certificate creation only:

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-prod-init
release_digest=$(jq -er .image.digest "$RELEASE_RECORD")
image_uri=$(jq -er .image.uri "$RELEASE_RECORD")
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
certificate_plan_dir=$(mktemp -d)
certificate_plan="$certificate_plan_dir/prod-certificate.tfplan"
certificate_plan_json="$certificate_plan.json"
task lambda-prod-plan PLAN_FILE="$certificate_plan" \
  IMAGE_DIGEST="$release_digest" ALARM_ACTION_ARN="$alarm_topic_arn"
tofu -chdir=infra/lambda/environments/prod show -json "$certificate_plan" > \
  "$certificate_plan_json"
task lambda-plan-check PLAN_JSON="$certificate_plan_json" ENVIRONMENT=prod \
  NAME_PREFIX=portfolio-lambda-prod IMAGE_URI="$image_uri" \
  ALARM_ACTIONS_JSON="[\"$alarm_topic_arn\"]"
jq -e '[.resource_changes[] | select(.change.actions != ["no-op"])] as $changes |
  ($changes | length) == 1 and
  ($changes[0].address | test("aws_acm_certificate")) and
  $changes[0].change.actions == ["create"]' "$certificate_plan_json"
```

**Approval gate:** Present and obtain separate approvals for the exact GitHub
push and exact saved AWS plan. Wait for hosted PR checks before applying. No
Lambda, alias, API, table, or alarm replacement is allowed.

```bash
: "${APPROVED_PROD_CERTIFICATE_PLAN:?set the exact approved certificate plan path}"
test -f "$APPROVED_PROD_CERTIFICATE_PLAN"
test -f "$APPROVED_PROD_CERTIFICATE_PLAN.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-prod-init
task lambda-prod-apply PLAN_FILE="$APPROVED_PROD_CERTIFICATE_PLAN"
```

- [ ] **Step 3: Create only ACM validation CNAMEs in Cloudflare**

Read `acm_validation_records` and prepare the exact record IDs or creates with
type, name, target, TTL, and `proxied=false`. **Approval gate:** create only that
reviewed list after current-session approval, preserving the existing Amplify
validation record. Use the explicit deployment profile and wait for `ISSUED` on
both names.

- [ ] **Step 4: Activate Regional custom domains and mappings**

Change only `activate_custom_domain = true` and commit
`feat(infra): activate production API domains` on the same draft PR. Create a
saved plan requiring only certificate validation, two Regional domain resources,
and two mappings:

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-artifacts-init
task lambda-prod-init
release_digest=$(jq -er .image.digest "$RELEASE_RECORD")
image_uri=$(jq -er .image.uri "$RELEASE_RECORD")
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
activation_plan_dir=$(mktemp -d)
activation_plan="$activation_plan_dir/prod-domain-activation.tfplan"
activation_plan_json="$activation_plan.json"
task lambda-prod-plan PLAN_FILE="$activation_plan" \
  IMAGE_DIGEST="$release_digest" ALARM_ACTION_ARN="$alarm_topic_arn"
tofu -chdir=infra/lambda/environments/prod show -json "$activation_plan" > \
  "$activation_plan_json"
task lambda-plan-check PLAN_JSON="$activation_plan_json" ENVIRONMENT=prod \
  NAME_PREFIX=portfolio-lambda-prod IMAGE_URI="$image_uri" \
  ALARM_ACTIONS_JSON="[\"$alarm_topic_arn\"]"
jq -e '[.resource_changes[] | select(.change.actions != ["no-op"])] as $changes |
  ($changes | length) == 5 and
  all($changes[];
    (.address | test("aws_acm_certificate_validation|aws_apigatewayv2_domain_name|aws_apigatewayv2_api_mapping")) and
    .change.actions == ["create"])' "$activation_plan_json"
```

**Approval gate:** Present and obtain separate approvals for the GitHub push and
exact saved AWS plan. Wait for hosted checks before apply. No function, alias,
table, or alarm replacement is allowed.

```bash
: "${APPROVED_PROD_ACTIVATION_PLAN:?set the exact approved domain-activation plan path}"
test -f "$APPROVED_PROD_ACTIVATION_PLAN"
test -f "$APPROVED_PROD_ACTIVATION_PLAN.json"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-prod-init
task lambda-prod-apply PLAN_FILE="$APPROVED_PROD_ACTIVATION_PLAN"
```

- [ ] **Step 5: Add canonical OAuth callbacks**

Prepare exactly `https://craigdevjohnson.com/soccer` and
`https://www.craigdevjohnson.com/soccer`. **Approval gate:** present the entire
before/after redirect lists and add them only after current-session approval.
Preserve both direct callbacks through the observation window.

- [ ] **Step 6: Test the custom-domain target without public DNS change**

For each API Gateway target, use `curl --connect-to` so TLS SNI and Host remain
the intended public domain. Verify certificate names, `/healthz`, `/`, `/soccer`,
CSS, and HTTPS OAuth redirect construction. Do not change public DNS.

- [ ] **Step 7: Merge the reviewed production configuration before DNS cutover**

Require the draft PR to contain only sanitized evidence and the two reviewed
domain flags. Pin the reviewed local and remote head before approval:

```bash
production_pr_url=$(gh pr view --json url --jq .url)
production_pr_head_sha=$(git rev-parse HEAD)
gh pr checks "$production_pr_url" --watch --interval 10
test "$(gh pr view "$production_pr_url" --json headRefOid --jq .headRefOid)" = \
  "$production_pr_head_sha"
git diff --check origin/main..."$production_pr_head_sha"
```

**Approval gate:** Present the PR number, head SHA, exact diff, merge method, and
rollback before marking ready or merging. After approval, run:

```bash
: "${APPROVED_PRODUCTION_PR_URL:?set the exact reviewed production PR URL}"
: "${APPROVED_PRODUCTION_PR_HEAD_SHA:?set the exact reviewed production PR head SHA}"
test "$(gh pr view "$APPROVED_PRODUCTION_PR_URL" --json headRefOid --jq .headRefOid)" = \
  "$APPROVED_PRODUCTION_PR_HEAD_SHA"
gh pr ready "$APPROVED_PRODUCTION_PR_URL"
test "$(gh pr view "$APPROVED_PRODUCTION_PR_URL" --json headRefOid --jq .headRefOid)" = \
  "$APPROVED_PRODUCTION_PR_HEAD_SHA"
gh pr merge "$APPROVED_PRODUCTION_PR_URL" --merge \
  --match-head-commit "$APPROVED_PRODUCTION_PR_HEAD_SHA"
```

Then prove the
development-tested source SHA remains an ancestor of `origin/main` and the
merged production root matches the applied configuration.

---

### Task 7: Prepare and execute the Cloudflare cutover

**Files:**

- Modify: `docs/deployment/evidence/production-precutover.md` with approved before/after records
- Modify: `docs/deployment/evidence/production-precutover.json` with the same
  machine-readable approved cutover coordinates
- Modify: the source-SHA-named JSON release record
- Create: `docs/deployment/evidence/production-observation.jsonl` at cutover and
  continue it during Task 8
- Modify externally: one cache rule and the apex/`www` traffic records

**Interfaces:**

- Consumes: issued custom domains, tested target, exported old configuration
- Produces: Lambda-backed apex and preserved `www` redirect behavior

- [ ] **Step 1: Define the exact cache and client-identity contract**

Prepare one reviewed cache rule for both portfolio hosts:

```text
If the request path does not start with /static/ and is not /favicon.ico,
bypass Cloudflare cache.
```

Export the prior rule ID, expression, action, and TTLs. Because the application
intentionally does not trust `CF-Connecting-IP`, also record that proxied traffic
may share a Cloudflare edge IP for logs and the login rate limiter. Exercise the
import flow from two independent clients before cutover. If legitimate use is
blocked or the limitation is not accepted, stop and implement a bounded
Cloudflare-origin trust design; do not weaken header trust ad hoc.

- [ ] **Step 2: Recheck rollback origins and exact current values**

Verify the Amplify default domain and current public apex both return 200.
Require the saved cache rule, apex, and `www` record IDs and values to match live
state. Stop and re-plan if another actor changed any input.

- [ ] **Step 3: Apply the cache bypass rule before the origin switch**

**Approval gate:** Present zone, rule ID, full prior expression/action, full new
expression/action, and rollback. Change only that rule after current-session
approval. Confirm current Amplify HTML reports `CF-Cache-Status: BYPASS` or
`DYNAMIC` before changing an origin.

- [ ] **Step 4: Switch the apex record first**

Prepare one exact record mutation from the saved Amplify value to the OpenTofu
API Gateway target, preserving apex flattening, TTL Auto, and reviewed proxy
state. **Approval gate:** present record ID and complete before/after values;
apply only after current-session approval. Leave `www` unchanged until apex
verification passes.

- [ ] **Step 5: Purge an exact apex URL list and verify externally**

Build an explicit list containing every primary apex route plus the exact CSS,
JavaScript, image, favicon, and other static URLs referenced by the accepted
release. Save the reviewed list in sanitized cutover evidence. Never use a
zone-wide purge.

**Approval gate:** Present the complete URL list and Cloudflare zone. Purge only
those URLs after current-session approval. Then verify TLS/HSTS, `/healthz`, all
primary routes, HTMX Skills behavior, Soccer import and Secure cookies, Google
connect/callback and the separately approved calendar workflow, all static
types, dynamic HTML cache status, logs, and alarms.

- [ ] **Step 6: Switch or confirm `www` redirect behavior**

Prepare the exact `www` record mutation while preserving the reviewed proxied
redirect rule. **Approval gate:** present the record and redirect-rule
before/after behavior, then apply only after current-session approval. Require
the same permanent apex redirect as the baseline. If it fails, freeze the
cutover and prepare the exact saved `www` restoration for approval.

- [ ] **Step 7: Create sanitized post-cutover and production release evidence**

Create `codex/production-observation-evidence` from the new `origin/main`. Update
both production precutover files with the exact approved cache, apex, `www`, and
purge before/after values. Preserve the original rollback fields in the JSON and
add an `applied` object containing:

- `cache_rule`: the final `id`, `expression`, `action`, nullable numeric
  `edge_ttl_seconds`/`browser_ttl_seconds`, and RFC3339 `changed_at`;
- `dns_records.apex` and `dns_records.www`: final `id`, `type`, `name`,
  `content`, numeric `ttl`, boolean `proxied`, and RFC3339
  `changed_or_verified_at`;
- `purge`: the exact nonempty HTTPS `urls` array and RFC3339 `completed_at`; and
- RFC3339 `cutover_completed_at`.

Use `apply_patch` to fill the
existing source-SHA release JSON's `production` object with function name,
published version, identical alias target, digest-qualified image URI, direct
endpoint, both custom domains, health revision, DNS-cutover timestamp,
`rollback_evidence` pointing to the precutover JSON,
`alarm_delivery_evidence` pointing to the committed alarm-delivery JSON,
observation JSONL path, and null observation-completion timestamp.

Validate SHA/digest/URI agreement, development and production health revisions,
both alias/version pairs, required domains/endpoints, both evidence paths and
their schemas, the Amplify rollback origin, confirmed-count/message/timestamp
alarm proof, and absence of secrets or raw state. The release JSON is the only
input index the recorder needs; it loads both referenced files and refuses a
missing or inconsistent one. Capture the sanitized Google connect, calendar-add,
and result-sync request IDs from Step 5, then append the cutover workflow proof:

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON path}"
: "${CONNECT_REQUEST_ID:?set the sanitized connect request ID from Step 5}"
: "${ADD_REQUEST_ID:?set the sanitized add request ID from Step 5}"
: "${SYNC_REQUEST_ID:?set the sanitized sync request ID from Step 5}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
task lambda-prod-init
rollback_evidence=$(jq -er .production.rollback_evidence "$RELEASE_RECORD")
alarm_evidence=$(jq -er .production.alarm_delivery_evidence "$RELEASE_RECORD")
test "$rollback_evidence" = "docs/deployment/evidence/production-precutover.json"
test "$alarm_evidence" = "docs/deployment/evidence/production-alarm-delivery.json"
test -f "$rollback_evidence"
test -f "$alarm_evidence"
apex_target=$(tofu -chdir=infra/lambda/environments/prod \
  output -json api_gateway_domain_targets | jq -er '."craigdevjohnson.com"')
jq -e --arg apex_target "$apex_target" '
  def timestamp:
    type == "string" and (fromdateiso8601 > 0);
  def dns_record:
    (.id | length) > 0 and (.type | length) > 0 and (.name | length) > 0 and
    (.content | length) > 0 and (.ttl | type) == "number" and
    (.proxied | type) == "boolean";
  def cache_rule:
    (.id | length) > 0 and (.expression | length) > 0 and
    (.action | length) > 0 and
    (.edge_ttl_seconds == null or (.edge_ttl_seconds | type) == "number") and
    (.browser_ttl_seconds == null or (.browser_ttl_seconds | type) == "number");
  .schema_version == 1 and .environment == "production" and
  (.captured_at | timestamp) and
  .public_hosts == ["craigdevjohnson.com", "www.craigdevjohnson.com"] and
  (.amplify.rollback_origin_url | startswith("https://")) and
  (.dns_records | keys) == ["apex", "www"] and
  all(.dns_records[]; dns_record) and
  (.redirect_rule.id | length) > 0 and
  (.redirect_rule.expression | length) > 0 and
  (.redirect_rule.action | length) > 0 and
  (.redirect_rule.target | length) > 0 and
  (.redirect_rule.status_code | type) == "number" and
  (.cache_rule | cache_rule) and
  (.tls.mode | length) > 0 and
  (.tls.minimum_version | length) > 0 and
  (.tls.hsts_enabled | type) == "boolean" and
  (.applied.cache_rule | cache_rule) and
  .applied.cache_rule.id == .cache_rule.id and
  (.applied.cache_rule.changed_at | timestamp) and
  (.applied.dns_records | keys) == ["apex", "www"] and
  all(.applied.dns_records[];
    dns_record and (.changed_or_verified_at | timestamp)) and
  .applied.dns_records.apex.id == .dns_records.apex.id and
  .applied.dns_records.apex.name == .dns_records.apex.name and
  .applied.dns_records.apex.content == $apex_target and
  .applied.dns_records.www.id == .dns_records.www.id and
  .applied.dns_records.www.name == .dns_records.www.name and
  (.applied.purge.urls | type) == "array" and
  (.applied.purge.urls | length) > 0 and
  all(.applied.purge.urls[];
    type == "string" and
    (startswith("https://craigdevjohnson.com/") or
      startswith("https://www.craigdevjohnson.com/"))) and
  (.applied.purge.completed_at | timestamp) and
  (.applied.cutover_completed_at | timestamp) and
  (.applied.cutover_completed_at | fromdateiso8601) >=
    (.applied.cache_rule.changed_at | fromdateiso8601) and
  (.applied.cutover_completed_at | fromdateiso8601) >=
    (.applied.dns_records.apex.changed_or_verified_at | fromdateiso8601) and
  (.applied.cutover_completed_at | fromdateiso8601) >=
    (.applied.dns_records.www.changed_or_verified_at | fromdateiso8601) and
  (.applied.cutover_completed_at | fromdateiso8601) >=
    (.applied.purge.completed_at | fromdateiso8601)' "$rollback_evidence"
alarm_topic_arn=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" sns list-topics \
  --query 'Topics[?ends_with(TopicArn, `:portfolio-lambda-prod-alerts`)].TopicArn | [0]' --output text)
alarm_names=()
while IFS= read -r alarm_name; do
  alarm_names+=("$alarm_name")
done < <(tofu -chdir=infra/lambda/environments/prod output -json alarm_arns | \
  jq -r '.[] | split(":")[-1]')
test "${#alarm_names[@]}" = "5"
alarm_json=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" cloudwatch describe-alarms \
  --alarm-names "${alarm_names[@]}" --output json)
printf '%s' "$alarm_json" | jq -e --arg topic "$alarm_topic_arn" \
  '(.MetricAlarms | length) == 5 and
   all(.MetricAlarms[]; .AlarmActions == [$topic])' >/dev/null
jq -e --arg topic "$alarm_topic_arn" '
  .schema_version == 1 and .environment == "production" and
  .account_id == "180294223248" and .region == "us-west-2" and
  .topic_arn == $topic and
  .confirmed_subscription_count >= 1 and
  (.message_id | length) > 0 and
  (.receipt_token_sha256 | test("^[0-9a-f]{64}$")) and
  ((.receipt_confirmed_at | fromdateiso8601) - (.sent_at | fromdateiso8601)) >= 0 and
  ((.receipt_confirmed_at | fromdateiso8601) - (.sent_at | fromdateiso8601)) <= 300' \
  "$alarm_evidence"
EVIDENCE_FILE=docs/deployment/evidence/production-observation.jsonl
task lambda-prod-observation-workflow \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE" \
  PUBLIC_HOST=craigdevjohnson.com \
  CONNECT_REQUEST_ID="$CONNECT_REQUEST_ID" \
  ADD_REQUEST_ID="$ADD_REQUEST_ID" \
  SYNC_REQUEST_ID="$SYNC_REQUEST_ID" \
  OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=true SYNC_OK=true
```

The command must record IDs only, never cookies, calendar names, event bodies,
JWTs, OAuth codes, or tokens. Commit the precutover Markdown and JSON, release
JSON, and initial JSONL locally as `docs: record production Lambda cutover`. Do
not push or merge the evidence branch until the production observation gate
passes.

---

### Task 8: Observe production and exercise rollback readiness

**Files:**

- Create: `docs/deployment/evidence/production-observation.jsonl`
- Modify: the source-SHA-named JSON release record
- No platform deletion

**Interfaces:**

- Consumes: seven days of public checks, metrics, logs, and user workflow evidence
- Produces: a go/no-go record for later Amplify retirement

- [ ] **Step 1: Start the executable seven-day monitor**

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
EVIDENCE_FILE=docs/deployment/evidence/production-observation.jsonl
task lambda-prod-observation-sample \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE"
```

Use the product's recurring monitoring mechanism to run the same sample at
least daily. The recorder captures immutable release coordinates, route and
asset results, cache status, Lambda/API metrics, exact alarm states, notification
proof loaded from `.production.alarm_delivery_evidence`, Soccer restoration,
and Amplify rollback-origin health loaded from
`.production.rollback_evidence` without secrets, cookies, request bodies, query
strings, OAuth tokens, or subscriber endpoints.

- [ ] **Step 2: Exercise rollback as a read-only dry run**

Resolve `.production.rollback_evidence` from the release JSON, require the
machine-readable file, and validate its Amplify origin, apex/`www` DNS values,
redirect, cache, and TLS fields. Confirm the saved origin still serves and write
the exact ordered rollback mutations in the observation document. Do not change
traffic during the dry run.

- [ ] **Step 3: Enforce explicit freeze and rollback triggers**

Freeze later cutover actions immediately for any single integrity/security
trigger: release SHA, digest, version, alias, or health mismatch; invalid TLS;
HTTP OAuth callback; missing `Secure` on an auth cookie; dynamic route served as
Cloudflare `HIT`, `STALE`, or `UPDATING`; cross-environment data access; data
corruption; secret exposure; or apex/`www` behavior outside the approved contract.

Also freeze for a sustained trigger: a critical route/asset fails twice at least
60 seconds apart; Lambda Errors, Throttles, or API 5xx is `ALARM` in two
consecutive five-minute evaluations; Lambda Duration p95 is at least 24 seconds
or API Latency p95 is at least 25 seconds for two consecutive five-minute
periods with at least five samples each; OAuth connect/add/sync fails twice in
clean sessions; or a test alarm notification is not received within five minutes.

A trigger never authorizes automatic rollback.

- [ ] **Step 4: Present and execute rollback only after exact approval**

Capture trigger evidence, re-read live Cloudflare state, and prepare exact apex,
`www`, cache-rule, and URL-purge changes. **Approval gate:** present every
before/after value and rollback command, then stop. After current-session
approval, restore apex and verify Amplify, restore `www` and its redirect, restore
the prior cache rule, purge only the approved URLs, and verify Amplify body hash,
headers, redirects, and TLS. Keep Lambda, tables, logs, and image intact. Record
trigger, approval, mutation, and recovery timestamps.

- [ ] **Step 5: Record the day-seven full workflow proof**

After at least 604800 seconds, complete OAuth/connect, Secure-cookie inspection,
one approved calendar add, and one approved result sync in a clean browser
session. Confirm the corresponding sanitized request IDs in Lambda/API logs and
append the second workflow record:

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON}"
: "${CONNECT_REQUEST_ID:?set the sanitized day-seven connect request ID}"
: "${ADD_REQUEST_ID:?set the sanitized day-seven add request ID}"
: "${SYNC_REQUEST_ID:?set the sanitized day-seven sync request ID}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
EVIDENCE_FILE=docs/deployment/evidence/production-observation.jsonl
task lambda-prod-observation-workflow \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE" \
  PUBLIC_HOST=craigdevjohnson.com \
  CONNECT_REQUEST_ID="$CONNECT_REQUEST_ID" \
  ADD_REQUEST_ID="$ADD_REQUEST_ID" \
  SYNC_REQUEST_ID="$SYNC_REQUEST_ID" \
  OAUTH_OK=true SECURE_COOKIES_OK=true ADD_OK=true SYNC_OK=true
```

- [ ] **Step 6: Close the observation window only with the executable gate**

```bash
: "${RELEASE_RECORD:?set the reviewed source-SHA release JSON}"
export AWS_PROFILE=portfolio-deployer
export AWS_REGION=us-west-2
task _lambda-identity-check
EVIDENCE_FILE=docs/deployment/evidence/production-observation.jsonl
task lambda-prod-observation-gate \
  RELEASE_RECORD="$RELEASE_RECORD" \
  EVIDENCE_FILE="$EVIDENCE_FILE"
```

Require at least eight passing samples, 604800 elapsed seconds, no gap over 26
hours, stable release coordinates, healthy alarms and Amplify rollback origin,
and successful OAuth/Secure-cookie/add/sync checks at cutover and after seven
days. On exit zero, use `apply_patch` to set
`production.observation_completed_at` from the final sample, revalidate the JSON
and gate, and commit JSON plus JSONL as
`docs: record production Lambda observation`.

**Approval gate:** Present the sanitized evidence diff, exact branch/commit, PR
body, and merge method. Push, open the evidence PR, and merge only after separate
current-session approvals and hosted checks. Passing authorizes a retirement
proposal only; it does not authorize deleting Amplify.

For the merge approval, record `evidence_head_sha=$(git rev-parse HEAD)`, require
the PR `headRefOid` to equal it before and after marking ready, and use
`gh pr merge "$evidence_pr_url" --merge --match-head-commit "$evidence_head_sha"`.

---

### Task 9: Prepare, but do not execute, legacy retirement

**Files:**

- Create: `docs/superpowers/plans/2026-08-legacy-platform-retirement.md` with the actual completion date in its filename

**Interfaces:**

- Consumes: completed development and production observation evidence
- Produces: a fresh destructive-action proposal requiring separate approval

- [ ] **Step 1: Inventory every remaining legacy consumer**

Map App Runner custom domain/service, Amplify domain/app/branch, legacy shared
Lambda resources if present, ECR repositories/images, `/portfolio/*` parameters,
both legacy tables, legacy state addresses, PR source branches, and linked
worktrees. Record current item counts and recovery mechanisms.

- [ ] **Step 2: Classify each proposed action by recoverability**

For every item specify: retain, export then delete, disassociate only, disable,
or delete; exact AWS/GitHub/DNS address; last observed consumer; backup or export
location; reversal procedure; and whether state removal is involved.

- [ ] **Step 3: Present the retirement plan for explicit approval**

Do not disassociate domains, delete applications/services/images/parameters/
tables/branches/worktrees, empty ECR, destroy OpenTofu resources, or prune Git as
part of this production plan.
