#!/bin/sh
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

for editorconfig_file in \
  infra/lambda/ci-roles/tests/policies.tftest.hcl \
  scripts/apply-ci-roles-plan.sh \
  scripts/authorize-ci-lambda-release.sh \
  scripts/build-ci-lambda-release.sh \
  scripts/check-ci-aws-identity.sh \
  scripts/check-ci-roles-plan.sh \
  scripts/check-ci-state-bucket.sh \
  scripts/check-current-main.sh \
  scripts/check-lambda-plan.sh \
  scripts/classify-release-change.sh \
  scripts/create-ci-lambda-release-plan.sh \
  scripts/create-ci-lambda-rollback-plan.sh \
  scripts/create-ci-roles-plan.sh \
  scripts/deploy-ci-lambda-development.sh \
  scripts/plan-ci-lambda-production.sh \
  scripts/record-ci-lambda-development.sh \
  scripts/resolve-development-release-base.sh \
  scripts/resolve-reviewed-release-base.sh \
  scripts/validate-production-release.sh \
  scripts/verify-lambda-release.sh \
  tests/fixtures/release-fake-cli.sh \
  tests/lambda-plan-contract.sh \
  tests/release-automation.sh; do
  awk '
    index($0, "\t") || length($0) > 120 {
      printf "EditorConfig violation: %s:%d\n", FILENAME, FNR > "/dev/stderr"
      exit 1
    }
  ' "$root_dir/$editorconfig_file"
done

fake_bin="$test_dir/bin"
mkdir -p "$fake_bin"
for command_name in gh aws curl docker sleep tofu; do
  ln -s "$root_dir/tests/fixtures/release-fake-cli.sh" "$fake_bin/$command_name"
done
PATH="$fake_bin:$PATH"
export PATH

source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
image_digest=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
other_digest=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
export GITHUB_REPOSITORY=CraigDevJohnson/portfolio

reviewed_pull_json() {
  jq -nc --arg source_sha "$1" --arg base_sha "$2" '[{
    state: "closed",
    merged_at: "2026-08-30T00:00:00Z",
    merge_commit_sha: $source_sha,
    base: {ref: "main", sha: $base_sha}
  }]'
}

promotion_deployment_json() {
  rollback_version=${3:-6}
  jq -nc --arg source_sha "$1" --arg digest "$2" --arg rollback "$rollback_version" '{
    id: 42,
    ref: $source_sha,
    sha: $source_sha,
    task: "portfolio-lambda-development",
    environment: "development",
    description: ("Lambda " + $digest + " rollback-v" + $rollback),
    creator: {login: "github-actions[bot]", type: "Bot"}
  }'
}

promotion_status_json() {
  status_version=${4:-7}
  status_id=${5:-99}
  created_at=${6:-2026-08-30T00:01:00Z}
  jq -nc \
    --arg state "$1" \
    --arg source_sha "$2" \
    --arg digest "$3" \
    --arg version "$status_version" \
    --argjson id "$status_id" \
    --arg created_at "$created_at" '[{
    id: $id,
    created_at: $created_at,
    state: $state,
    environment: "development",
    environment_url: "https://dev.craigdevjohnson.com",
    description: (
      if $state == "success" then
        "Verified " + $source_sha + " " + $digest + " v" + $version
      else
        "Failed " + $source_sha + " at " + $digest
      end
    ),
    creator: {login: "github-actions[bot]", type: "Bot"}
  }]'
}

durable_deployment_pages_json() {
  rollback_version=${3:-6}
  created_at=${4:-2026-08-30T00:00:00Z}
  jq -nc \
    --arg source_sha "$1" \
    --arg digest "$2" \
    --arg rollback "$rollback_version" \
    --arg created_at "$created_at" '[[{
    id: 42,
    ref: $source_sha,
    sha: $source_sha,
    task: "portfolio-lambda-development",
    environment: "development",
    description: ("Lambda " + $digest + " rollback-v" + $rollback),
    created_at: $created_at,
    creator: {login: "github-actions[bot]", type: "Bot"}
  }]]'
}

durable_status_json() {
  status_version=${5:-7}
  status_id=${6:-99}
  created_at=${7:-2026-08-30T00:01:00Z}
  jq -nc \
    --arg state "$1" \
    --arg source_sha "$2" \
    --arg digest "$3" \
    --arg creator_login "$4" \
    --arg version "$status_version" \
    --argjson id "$status_id" \
    --arg created_at "$created_at" '[{
      id: $id,
      created_at: $created_at,
      state: $state,
      environment: "development",
      environment_url: "https://dev.craigdevjohnson.com",
      description: (
        if $state == "success" then
          "Verified " + $source_sha + " " + $digest + " v" + $version
        else
          "Deploying " + $source_sha + " at " + $digest
        end
      ),
      creator: {
        login: $creator_login,
        type: (if $creator_login == "github-actions[bot]" then "Bot" else "User" end)
      }
    }]'
}

FAKE_MAIN_SHA=$source_sha
export FAKE_MAIN_SHA
sh "$root_dir/scripts/check-current-main.sh" "$source_sha"
if sh "$root_dir/scripts/check-current-main.sh" "$other_sha" 2> /dev/null; then
  echo 'stale main was accepted' >&2
  exit 1
fi

FAKE_PULLS_JSON=$(reviewed_pull_json "$source_sha" "$other_sha")
export FAKE_PULLS_JSON
test "$(sh "$root_dir/scripts/resolve-reviewed-release-base.sh" "$source_sha")" = "$other_sha"
FAKE_PULLS_JSON='[]'
export FAKE_PULLS_JSON
if sh "$root_dir/scripts/resolve-reviewed-release-base.sh" "$source_sha" > /dev/null 2>&1; then
  echo 'unreviewed source commit was accepted' >&2
  exit 1
fi

manifest="$test_dir/production-release.json"
jq -n --arg source_sha "$source_sha" --arg image_digest "$image_digest" \
  '{source_sha:$source_sha,image_digest:$image_digest,development_deployment_id:42}' > "$manifest"
FAKE_DEPLOYMENT_JSON=$(promotion_deployment_json "$source_sha" "$image_digest")
FAKE_STATUSES_JSON=$(promotion_status_json success "$source_sha" "$image_digest")
FAKE_TAG_DIGEST=$image_digest
export FAKE_DEPLOYMENT_JSON FAKE_STATUSES_JSON FAKE_TAG_DIGEST
ECR_REPOSITORY=portfolio-lambda-releases sh "$root_dir/scripts/validate-production-release.sh" "$manifest"
FAKE_DEPLOYMENT_JSON=$(printf '%s\n' "$FAKE_DEPLOYMENT_JSON" |
  jq '.task = "deploy"')
export FAKE_DEPLOYMENT_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted a deployment from another task' >&2
  exit 1
fi
FAKE_DEPLOYMENT_JSON=$(promotion_deployment_json "$source_sha" "$image_digest" |
  jq '.creator = {login: "untrusted-user", type: "User"}')
export FAKE_DEPLOYMENT_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted an untrusted deployment creator' >&2
  exit 1
fi
FAKE_DEPLOYMENT_JSON=$(promotion_deployment_json "$source_sha" "$image_digest" |
  jq --arg other_sha "$other_sha" '.sha = $other_sha')
export FAKE_DEPLOYMENT_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted a mismatched resolved deployment SHA' >&2
  exit 1
fi
FAKE_DEPLOYMENT_JSON=$(promotion_deployment_json "$other_sha" "$image_digest")
export FAKE_DEPLOYMENT_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted an unrelated deployment' >&2
  exit 1
fi
FAKE_DEPLOYMENT_JSON=$(promotion_deployment_json "$source_sha" "$image_digest")
FAKE_STATUSES_JSON=$(promotion_status_json failure "$source_sha" "$image_digest")
export FAKE_DEPLOYMENT_JSON FAKE_STATUSES_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted a failed development deployment' >&2
  exit 1
fi
older_success_status=$(promotion_status_json \
  success "$source_sha" "$image_digest" 7 98 2026-08-30T00:00:00Z)
newer_failure_status=$(promotion_status_json \
  failure "$source_sha" "$image_digest" 7 99 2026-08-30T00:01:00Z)
FAKE_STATUSES_JSON=$(jq -nc \
  --argjson older "$older_success_status" \
  --argjson newer "$newer_failure_status" \
  '$older + $newer')
export FAKE_STATUSES_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted an older success after a newer failure' >&2
  exit 1
fi
FAKE_STATUSES_JSON=$older_success_status
FAKE_STATUS_PAGES_JSON=$(jq -nc \
  --argjson older "$older_success_status" \
  --argjson newer "$newer_failure_status" \
  '[$older, $newer]')
export FAKE_STATUSES_JSON FAKE_STATUS_PAGES_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion ignored a newer failed deployment-status page' >&2
  exit 1
fi
unset FAKE_STATUS_PAGES_JSON
FAKE_STATUSES_JSON=$(promotion_status_json success "$source_sha" "$image_digest")
FAKE_STATUSES_JSON=$(printf '%s\n' "$FAKE_STATUSES_JSON" |
  jq '.[0].environment_url = "https://attacker.example"')
export FAKE_DEPLOYMENT_JSON FAKE_STATUSES_JSON
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted the wrong verified environment URL' >&2
  exit 1
fi
FAKE_STATUSES_JSON=$(promotion_status_json success "$source_sha" "$image_digest")
FAKE_TAG_DIGEST=$other_digest
export FAKE_STATUSES_JSON FAKE_TAG_DIGEST
if ECR_REPOSITORY=portfolio-lambda-releases \
  sh "$root_dir/scripts/validate-production-release.sh" "$manifest" > /dev/null 2>&1; then
  echo 'promotion accepted a source tag bound to another digest' >&2
  exit 1
fi
FAKE_TAG_DIGEST=$image_digest
export FAKE_TAG_DIGEST

FAKE_BUCKET_VERSIONING=Enabled
export FAKE_BUCKET_VERSIONING
STATE_BUCKET=portfolio-tofu-state-180294223248 sh "$root_dir/scripts/check-ci-state-bucket.sh"
FAKE_BUCKET_VERSIONING=Suspended
export FAKE_BUCKET_VERSIONING
if STATE_BUCKET=portfolio-tofu-state-180294223248 sh "$root_dir/scripts/check-ci-state-bucket.sh" > /dev/null 2>&1; then
  echo 'suspended state-bucket versioning was accepted' >&2
  exit 1
fi

evidence_dir="$test_dir/evidence"
curl_log="$test_dir/curl.log"
FAKE_HEALTH_SHA=$source_sha
FAKE_QUALIFIED_IMAGE_URI="example.invalid/portfolio@$image_digest"
FAKE_CURL_LOG=$curl_log
SMOKE_WINDOW_SECONDS=300
SMOKE_INTERVAL_SECONDS=30
export FAKE_HEALTH_SHA FAKE_QUALIFIED_IMAGE_URI FAKE_CURL_LOG
export SMOKE_WINDOW_SECONDS SMOKE_INTERVAL_SECONDS
BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh"
grep -Fxq 'https://example.invalid/static/images/backgrounds/home-hero.jpg' "$curl_log"
grep -Fq -- '--connect-timeout 10' "$root_dir/scripts/verify-lambda-release.sh"
grep -Fq -- '--max-time 30' "$root_dir/scripts/verify-lambda-release.sh"
if SMOKE_WINDOW_SECONDS=0 \
  BASE_URL=https://example.invalid \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted a zero-second smoke window' >&2
  exit 1
fi

smoke_aws_log="$test_dir/smoke-aws.log"
smoke_sleep_log="$test_dir/smoke-sleep.log"
: > "$smoke_aws_log"
: > "$smoke_sleep_log"
FAKE_AWS_LOG=$smoke_aws_log \
  FAKE_SLEEP_LOG=$smoke_sleep_log \
  SMOKE_WINDOW_SECONDS=300 \
  BASE_URL=https://example.invalid \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh"
test "$(grep -Fc 'aws cloudwatch describe-alarms' "$smoke_aws_log")" -eq 11
test "$(grep -Fc 'sleep 30' "$smoke_sleep_log")" -eq 10

SMOKE_WINDOW_SECONDS=300 \
  SMOKE_INTERVAL_SECONDS=5 \
  BASE_URL=https://example.invalid \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh"
jq -e '.window_seconds == 300 and .interval_seconds == 5 and .observations == 61' \
  "$evidence_dir/alarm-smoke-window.json" > /dev/null || {
  echo 'release verification did not accept a five-second smoke interval' >&2
  exit 1
}
for invalid_smoke_interval in 30x 05 0 -5; do
  if SMOKE_INTERVAL_SECONDS="$invalid_smoke_interval" \
    BASE_URL=https://example.invalid \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    FUNCTION_NAME=portfolio-lambda-dev \
    EVIDENCE_DIR="$evidence_dir" \
    sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
    echo "release verification accepted invalid smoke interval: $invalid_smoke_interval" >&2
    exit 1
  fi
done

FAKE_ALARM_SCENARIO=insufficient
export FAKE_ALARM_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted unresolved alarm health' >&2
  exit 1
fi
jq -e 'any(.MetricAlarms[]; .StateValue == "INSUFFICIENT_DATA")' \
  "$evidence_dir/alarms.json" > /dev/null || {
  echo 'failed verification did not retain the final unresolved alarm evidence' >&2
  exit 1
}
unset FAKE_ALARM_SCENARIO
for health_scenario in redirect wrong-content degraded; do
  FAKE_HEALTH_SCENARIO=$health_scenario
  export FAKE_HEALTH_SCENARIO
  if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
    FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
    sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
    echo "release verification accepted a $health_scenario health response" >&2
    exit 1
  fi
done
unset FAKE_HEALTH_SCENARIO
FAKE_ROUTE_SCENARIO=redirect
export FAKE_ROUTE_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted a redirecting route' >&2
  exit 1
fi
FAKE_ROUTE_SCENARIO=wrong-content
export FAKE_ROUTE_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted the wrong route content type' >&2
  exit 1
fi
unset FAKE_ROUTE_SCENARIO
FAKE_ALARM_SCENARIO=missing
export FAKE_ALARM_SCENARIO
rm -f "$evidence_dir/alarm-smoke-window.json"
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted missing alarms' >&2
  exit 1
fi
jq -e '.MetricAlarms == []' "$evidence_dir/alarms.json" > /dev/null || {
  echo 'failed verification did not retain the malformed final alarm set' >&2
  exit 1
}
jq -e '.window_seconds == 300 and .interval_seconds == 30 and .observations == 11' \
  "$evidence_dir/alarm-smoke-window.json" > /dev/null || {
  echo 'failed verification omitted its smoke-window contract' >&2
  exit 1
}
FAKE_ALARM_SCENARIO=extra
export FAKE_ALARM_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted an extra alarm' >&2
  exit 1
fi
unset FAKE_ALARM_SCENARIO
FAKE_QUALIFIED_IMAGE_URI="example.invalid/portfolio@$other_digest"
export FAKE_QUALIFIED_IMAGE_URI
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'live alias version with a different digest was accepted' >&2
  exit 1
fi

tmp="$test_dir/repository"
mkdir -p "$tmp"
git -C "$tmp" init -q
git -C "$tmp" config user.email test@example.com
git -C "$tmp" config user.name Test
mkdir -p "$tmp/internal/app" "$tmp/docs" "$tmp/deploy" "$tmp/.github/workflows"
echo a > "$tmp/internal/app/app.go"
git -C "$tmp" add .
git -C "$tmp" commit -qm base
base=$(git -C "$tmp" rev-parse HEAD)
echo b >> "$tmp/internal/app/app.go"
git -C "$tmp" commit -qam runtime
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = development
base=$head
echo doc > "$tmp/docs/readme.md"
git -C "$tmp" add .
git -C "$tmp" commit -qm docs
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = skip
base=$head
echo '{}' > "$tmp/deploy/production-release.json"
git -C "$tmp" add .
git -C "$tmp" commit -qm promote
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = production
base=$head
echo x > "$tmp/.github/workflows/x.yml"
git -C "$tmp" add .
git -C "$tmp" commit -qm workflow
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = review
base=$head
echo runtime > "$tmp/internal/app/renamed.go"
git -C "$tmp" add .
git -C "$tmp" commit -qm rename-runtime-source
base=$(git -C "$tmp" rev-parse HEAD)
git -C "$tmp" mv internal/app/renamed.go docs/renamed.md
git -C "$tmp" commit -qm rename-runtime-to-docs
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = development
base=$head
mkdir -p "$tmp/infra"
echo infra > "$tmp/infra/renamed.tf"
git -C "$tmp" add .
git -C "$tmp" commit -qm rename-infra-source
base=$(git -C "$tmp" rev-parse HEAD)
git -C "$tmp" mv infra/renamed.tf internal/app/renamed.tf
git -C "$tmp" commit -qm rename-infra-to-internal
head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = review

durable_repo="$test_dir/durable-repository"
mkdir -p "$durable_repo"
git -C "$durable_repo" init -q
git -C "$durable_repo" config user.email test@example.com
git -C "$durable_repo" config user.name Test
echo base > "$durable_repo/README.md"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm base
mkdir -p "$durable_repo/.github/workflows"
echo release > "$durable_repo/.github/workflows/release.yml"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm release-epoch
release_epoch=$(git -C "$durable_repo" rev-parse HEAD)
mkdir -p "$durable_repo/internal/app"
echo runtime > "$durable_repo/internal/app/app.go"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm runtime
deployed_sha=$(git -C "$durable_repo" rev-parse HEAD)
mkdir -p "$durable_repo/docs"
echo docs > "$durable_repo/docs/readme.md"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm docs
durable_head=$(git -C "$durable_repo" rev-parse HEAD)

FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
export FAKE_DEPLOYMENT_PAGES_JSON
resolved_base=$(cd "$durable_repo" && sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head")
test "$resolved_base" = "$release_epoch"
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t' "$release_epoch")"
backlog_classification=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/classify-release-change.sh" "$resolved_base" "$durable_head")
test "$backlog_classification" = development

FAKE_DEPLOYMENT_PAGES_JSON=$(durable_deployment_pages_json "$deployed_sha" "$image_digest")
valid_deployment_pages=$FAKE_DEPLOYMENT_PAGES_JSON
FAKE_STATUSES_JSON=$(durable_status_json in_progress "$deployed_sha" "$image_digest" 'github-actions[bot]')
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_STATUSES_JSON
resolved_base=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head")
test "$resolved_base" = "$release_epoch"
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t6' "$release_epoch")"

FAKE_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --arg older_sha "$deployed_sha" \
  --arg newer_sha "$durable_head" \
  --arg digest "$image_digest" '[[
    {
      id: 41,
      ref: $older_sha,
      sha: $older_sha,
      task: "portfolio-lambda-development",
      environment: "development",
      description: ("Lambda " + $digest + " rollback-v6"),
      created_at: "2026-08-30T00:00:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    },
    {
      id: 42,
      ref: $newer_sha,
      sha: $newer_sha,
      task: "portfolio-lambda-development",
      environment: "development",
      description: ("Lambda " + $digest + " rollback-v7"),
      created_at: "2026-08-30T00:01:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    }
  ]]')
export FAKE_DEPLOYMENT_PAGES_JSON
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t6' "$release_epoch")" || {
  echo 'development cursor depended on API order for its bootstrap version' >&2
  exit 1
}

FAKE_DEPLOYMENT_PAGES_JSON=$valid_deployment_pages
FAKE_STATUSES_JSON=$(durable_status_json success "$deployed_sha" "$image_digest" 'github-actions[bot]')
valid_deployment_statuses=$FAKE_STATUSES_JSON
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_STATUSES_JSON
resolved_base=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head")
test "$resolved_base" = "$deployed_sha"
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t7' "$deployed_sha")"

FAKE_DEPLOYMENT_PAGES_JSON=$(printf '%s\n' "$valid_deployment_pages" | jq -c \
  --arg digest "$image_digest" \
  '.[0][0].description = ("Lambda " + $digest)')
export FAKE_DEPLOYMENT_PAGES_JSON
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t7' "$deployed_sha")" || {
  echo 'development cursor rejected a trusted legacy deployment record' >&2
  exit 1
}
FAKE_DEPLOYMENT_PAGES_JSON=$valid_deployment_pages
export FAKE_DEPLOYMENT_PAGES_JSON

older_in_progress_status=$(durable_status_json \
  in_progress "$deployed_sha" "$image_digest" 'github-actions[bot]' \
  7 98 2026-08-30T00:00:00Z)
newer_success_status=$(durable_status_json \
  success "$deployed_sha" "$image_digest" 'github-actions[bot]' \
  7 99 2026-08-30T00:01:00Z)
FAKE_STATUSES_JSON=$(jq -nc \
  --argjson older "$older_in_progress_status" \
  --argjson newer "$newer_success_status" \
  '$older + $newer')
export FAKE_STATUSES_JSON
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t7' "$deployed_sha")" || {
  echo 'development cursor depended on API order for its current status' >&2
  exit 1
}
FAKE_STATUSES_JSON=$older_in_progress_status
FAKE_STATUS_PAGES_JSON=$(jq -nc \
  --argjson older "$older_in_progress_status" \
  --argjson newer "$newer_success_status" \
  '[$older, $newer]')
export FAKE_STATUSES_JSON FAKE_STATUS_PAGES_JSON
resolved_coordinate=$(cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" coordinate)
test "$resolved_coordinate" = "$(printf '%s\t7' "$deployed_sha")" || {
  echo 'development cursor ignored a newer verified deployment-status page' >&2
  exit 1
}
unset FAKE_STATUS_PAGES_JSON

FAKE_STATUSES_JSON=$(durable_status_json success "$deployed_sha" "$image_digest" untrusted-user)
export FAKE_STATUSES_JSON
if (cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" > /dev/null 2>&1); then
  echo 'development cursor accepted an untrusted success status' >&2
  exit 1
fi

unrelated_sha=$(printf 'unrelated deployment\n' |
  git -C "$durable_repo" commit-tree "$(git -C "$durable_repo" rev-parse "$durable_head^{tree}")")
FAKE_DEPLOYMENT_PAGES_JSON=$(durable_deployment_pages_json "$unrelated_sha" "$image_digest")
FAKE_STATUSES_JSON=$(durable_status_json success "$unrelated_sha" "$image_digest" 'github-actions[bot]')
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_STATUSES_JSON
if (cd "$durable_repo" &&
  sh "$root_dir/scripts/resolve-development-release-base.sh" "$durable_head" > /dev/null 2>&1); then
  echo 'development cursor accepted a non-ancestor deployment' >&2
  exit 1
fi
FAKE_DEPLOYMENT_PAGES_JSON=$valid_deployment_pages
FAKE_STATUSES_JSON=$valid_deployment_statuses
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_STATUSES_JSON

FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
FAKE_MAIN_SHA=$durable_head
FAKE_PULLS_JSON=$(reviewed_pull_json "$durable_head" "$deployed_sha")
authorization_output="$test_dir/authorization-output"
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_MAIN_SHA FAKE_PULLS_JSON
(cd "$durable_repo" &&
  EVENT_SHA="$durable_head" GITHUB_OUTPUT="$authorization_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh")
grep -Fqx "source_sha=$durable_head" "$authorization_output"
grep -Fqx "base_sha=$deployed_sha" "$authorization_output"
grep -Fqx "development_base_sha=$release_epoch" "$authorization_output"
if grep -Fq 'development_base_version=' "$authorization_output"; then
  echo 'authorization exported a rollback version before development serialization' >&2
  exit 1
fi
grep -Fqx 'classification=development' "$authorization_output"

FAKE_DEPLOYMENT_PAGES_JSON=$valid_deployment_pages
FAKE_STATUSES_JSON=$valid_deployment_statuses
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_STATUSES_JSON
echo pending-runtime >> "$durable_repo/internal/app/app.go"
git -C "$durable_repo" commit -qam pending-runtime
pending_runtime_sha=$(git -C "$durable_repo" rev-parse HEAD)
mkdir -p "$durable_repo/deploy"
echo '{}' > "$durable_repo/deploy/production-release.json"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm promotion-with-runtime-backlog
promotion_with_backlog_head=$(git -C "$durable_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$promotion_with_backlog_head
FAKE_PULLS_JSON=$(reviewed_pull_json "$promotion_with_backlog_head" "$pending_runtime_sha")
promotion_with_backlog_output="$test_dir/promotion-with-backlog-authorization-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON
if (cd "$durable_repo" &&
  EVENT_SHA="$promotion_with_backlog_head" GITHUB_OUTPUT="$promotion_with_backlog_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" > /dev/null 2>&1); then
  echo 'production planning bypassed a pending development release' >&2
  exit 1
fi
grep -Fqx "development_base_sha=$deployed_sha" "$promotion_with_backlog_output"
grep -Fqx 'classification=review' "$promotion_with_backlog_output"

echo blocked-runtime >> "$durable_repo/internal/app/app.go"
echo blocked-workflow >> "$durable_repo/.github/workflows/release.yml"
git -C "$durable_repo" add .
git -C "$durable_repo" commit -qm blocked-mixed-change
blocked_mixed_sha=$(git -C "$durable_repo" rev-parse HEAD)
echo later-runtime >> "$durable_repo/internal/app/app.go"
git -C "$durable_repo" commit -qam later-runtime
laundering_head=$(git -C "$durable_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$laundering_head
FAKE_PULLS_JSON=$(reviewed_pull_json "$laundering_head" "$blocked_mixed_sha")
laundering_output="$test_dir/laundering-authorization-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON
if (cd "$durable_repo" &&
  EVENT_SHA="$laundering_head" GITHUB_OUTPUT="$laundering_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" > /dev/null 2>&1); then
  echo 'later runtime change laundered an earlier blocked release change' >&2
  exit 1
fi
grep -Fqx 'classification=review' "$laundering_output"

workflow_job() {
  awk -v target="  $1:" '
    /^  [[:alnum:]_-]+:$/ {
      if (selected) exit
      selected = ($0 == target)
    }
    selected { print }
  ' "$root_dir/.github/workflows/release.yml"
}

first_matching_line() {
  awk -v needle="$2" 'index($0, needle) { print NR; exit }' << EOF
$1
EOF
}

last_matching_line() {
  awk -v needle="$2" 'index($0, needle) { line = NR } END { if (line) print line }' << EOF
$1
EOF
}

count_matching_lines() {
  awk -v needle="$2" 'index($0, needle) { count++ } END { print count + 0 }' << EOF
$1
EOF
}

assert_before() {
  first=$(first_matching_line "$1" "$2")
  second=$(first_matching_line "$1" "$3")
  test -n "$first" && test -n "$second" && test "$first" -lt "$second"
}

authorize_job=$(workflow_job authorize)
build_job=$(workflow_job build)
development_job=$(workflow_job development)
production_job=$(workflow_job production-plan)
literal_dollar='$'

grep -Fxq 'permissions: {}' "$root_dir/.github/workflows/release.yml"
grep -Fq '    timeout-minutes: 10' << EOF
$authorize_job
EOF
grep -Fq '    timeout-minutes: 45' << EOF
$build_job
EOF
grep -Fq '    timeout-minutes: 30' << EOF
$development_job
EOF
grep -Fq '    timeout-minutes: 20' << EOF
$production_job
EOF
grep -Fq '    contents: read' << EOF
$authorize_job
EOF
grep -Fq '    pull-requests: read' << EOF
$authorize_job
EOF
grep -Fq '    deployments: read' << EOF
$authorize_job
EOF
if grep -Eq 'id-token: write|deployments: write' << EOF
$authorize_job
EOF
then
  echo 'authorize job can mint AWS credentials or write deployments' >&2
  exit 1
fi
grep -Fq '    id-token: write' << EOF
$build_job
EOF
grep -Fq '    id-token: write' << EOF
$development_job
EOF
grep -Fq '    deployments: write' << EOF
$development_job
EOF
grep -Fq '    id-token: write' << EOF
$production_job
EOF
grep -Fq '    deployments: read' << EOF
$production_job
EOF

grep -Fq 'task infrastructure-ci' "$root_dir/.github/workflows/ci.yml"
if grep -Eq 'tofu .*infra|task lambda-infrastructure-ci|sh tests/shared-root-config.sh' \
  "$root_dir/.github/workflows/ci.yml"; then
  echo 'CI workflow bypasses the authoritative infrastructure-ci task' >&2
  exit 1
fi
grep -Fq 'sh tests/release-automation.sh' "$root_dir/Taskfile.yaml"
grep -Fq 'infrastructure-ci:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-init:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-plan:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-apply:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-verify:' "$root_dir/Taskfile.yaml"
if grep -Fq 'task lambda-ci-roles-init' "$root_dir/infra/lambda/ci-roles/README.md"; then
  echo 'CI role runbook still implies checkout initialization is reused by the private plan task' >&2
  exit 1
fi
grep -Fq 'task lambda-ci-roles-plan' "$root_dir/infra/lambda/ci-roles/README.md"
grep -Fq 'task lambda-ci-roles-apply' "$root_dir/infra/lambda/ci-roles/README.md"
grep -Fq 'task lambda-ci-roles-verify' "$root_dir/infra/lambda/ci-roles/README.md"
if grep -Eq 'aws .*iam get-role' "$root_dir/infra/lambda/ci-roles/README.md"; then
  echo 'CI role runbook bypasses the authoritative role-verification task' >&2
  exit 1
fi
grep -Fq 'lambda-ci-roles-verify' "$root_dir/AGENTS.md"
for task_name in \
  lambda-ci-authorize-release \
  lambda-ci-check-current-main \
  lambda-ci-check-aws-identity \
  lambda-ci-build-release-image \
  lambda-ci-deploy-development \
  lambda-ci-plan-production; do
  grep -Fq "  $task_name:" "$root_dir/Taskfile.yaml"
done

if grep -Eq 'run: \||^[[:space:]]+(aws|docker|gh|jq|sh|tofu)[[:space:]]' \
  "$root_dir/.github/workflows/release.yml"; then
  echo 'release workflow bypasses its Taskfile entrypoints' >&2
  exit 1
fi
test "$(grep -Fc 'uses: go-task/setup-task@v2.2.0' "$root_dir/.github/workflows/release.yml")" -eq 4
grep -Fq 'run: task lambda-ci-authorize-release' << EOF
$authorize_job
EOF
grep -Fq "classification: ${literal_dollar}{{ steps.authorize.outputs.classification }}" << EOF
$authorize_job
EOF
grep -Fq "source_sha: ${literal_dollar}{{ steps.authorize.outputs.source_sha }}" << EOF
$authorize_job
EOF
grep -Fq "base_sha: ${literal_dollar}{{ steps.authorize.outputs.base_sha }}" << EOF
$authorize_job
EOF
if grep -Fq 'development_base_version:' << EOF
$authorize_job
EOF
then
  echo 'workflow exported a rollback version before development serialization' >&2
  exit 1
fi

assert_before "$build_job" 'run: task lambda-ci-check-current-main' 'uses: aws-actions/configure-aws-credentials@v6'
grep -Fq 'run: task lambda-ci-check-aws-identity' << EOF
$build_job
EOF
grep -Fq 'run: task lambda-ci-build-release-image' << EOF
$build_job
EOF
grep -Fq 'if-no-files-found: error' << EOF
$build_job
EOF
grep -Fq 'Immutable full-SHA tag already exists; reusing it for scan validation.' \
  "$root_dir/scripts/build-ci-lambda-release.sh"
if grep -Fq 'Immutable full-SHA tag already exists; refusing to rebuild.' \
  "$root_dir/scripts/build-ci-lambda-release.sh"; then
  echo 'release retry still refuses an existing immutable image' >&2
  exit 1
fi
awk '
  /if: always\(\)/ { always = NR }
  /uses: actions\/upload-artifact@v7/ && always && NR == always + 1 { found = 1 }
  END { exit(found ? 0 : 1) }
' << EOF
$build_job
EOF

assert_before \
  "$development_job" \
  'run: task lambda-ci-check-current-main' \
  'uses: aws-actions/configure-aws-credentials@v6'
grep -Fq 'run: task lambda-ci-check-aws-identity' << EOF
$development_job
EOF
grep -Fq 'run: task lambda-ci-deploy-development' << EOF
$development_job
EOF
grep -Fq 'fetch-depth: 0' << EOF
$development_job
EOF
if grep -Fq 'DEVELOPMENT_BASE_VERSION:' << EOF
$development_job
EOF
then
  echo 'development job consumed a stale pre-serialization rollback version' >&2
  exit 1
fi
development_script=$(cat "$root_dir/scripts/deploy-ci-lambda-development.sh")
assert_before "$development_script" "sh scripts/check-current-main.sh \"${literal_dollar}SOURCE_SHA\"" \
  'tofu -chdir=infra/lambda/environments/dev apply'
test "$(count_matching_lines "$development_script" 'sh scripts/create-ci-lambda-release-plan.sh')" -eq 1
grep -Fq 'sh scripts/record-ci-lambda-development.sh' \
  "$root_dir/scripts/deploy-ci-lambda-development.sh"
grep -Fq -- '-f task=portfolio-lambda-development' \
  "$root_dir/scripts/record-ci-lambda-development.sh"
grep -Fq 'if-no-files-found: error' << EOF
$development_job
EOF

grep -Fq 'environment: production-plan' << EOF
$production_job
EOF
assert_before \
  "$production_job" \
  'run: task lambda-ci-check-current-main' \
  'uses: aws-actions/configure-aws-credentials@v6'
grep -Fq 'run: task lambda-ci-check-aws-identity' << EOF
$production_job
EOF
grep -Fq 'run: task lambda-ci-plan-production' << EOF
$production_job
EOF
production_script=$(cat "$root_dir/scripts/plan-ci-lambda-production.sh")
assert_before "$production_script" "sh scripts/check-current-main.sh \"${literal_dollar}SOURCE_SHA\"" \
  'sh scripts/create-ci-lambda-release-plan.sh'
test "$(count_matching_lines "$production_script" 'sh scripts/create-ci-lambda-release-plan.sh')" -eq 1
for parent_script in \
  "$root_dir/scripts/deploy-ci-lambda-development.sh" \
  "$root_dir/scripts/plan-ci-lambda-production.sh"; do
  if grep -Eq 'tofu .* (init|plan|show)( |$)' "$parent_script"; then
    echo "release parent script duplicates plan construction: $parent_script" >&2
    exit 1
  fi
done
test "$(grep -Ec 'tofu .* init ' "$root_dir/scripts/create-ci-lambda-release-plan.sh")" -eq 1
test "$(grep -Ec 'tofu .* plan ' "$root_dir/scripts/create-ci-lambda-release-plan.sh")" -eq 1
test "$(grep -Ec 'tofu .* show -json ' "$root_dir/scripts/create-ci-lambda-release-plan.sh")" -eq 1
grep -Fq 'automated_release=true' "$root_dir/scripts/create-ci-lambda-release-plan.sh"
awk '
  /if: always\(\)/ { always = NR }
  /uses: actions\/upload-artifact@v7/ && always && NR == always + 1 { found = 1 }
  END { exit(found ? 0 : 1) }
' << EOF
$production_job
EOF
grep -Fq 'if-no-files-found: error' << EOF
$production_job
EOF

real_task=$(command -v task)
build_output="$test_dir/build-output"
docker_log="$test_dir/docker.log"
gh_log="$test_dir/gh.log"
aws_log="$test_dir/aws.log"
sleep_log="$test_dir/sleep.log"
push_marker="$test_dir/ecr-pushed"
scan_lookup_state="$test_dir/ecr-scan-lookup-state"

run_build_task() {
  : > "$build_output"
  : > "$docker_log"
  : > "$gh_log"
  : > "$aws_log"
  : > "$sleep_log"
  rm -f "$push_marker"
  rm -f "$scan_lookup_state"
  env \
    PATH="$fake_bin:$PATH" \
    FAKE_AWS_LOG="$aws_log" \
    FAKE_DOCKER_SIGNAL_PARENT_ON_PUSH="${FAKE_DOCKER_SIGNAL_PARENT_ON_PUSH:-false}" \
    FAKE_DOCKER_LOG="$docker_log" \
    FAKE_ECR_LOOKUP_SCENARIO="${FAKE_ECR_LOOKUP_SCENARIO:-existing}" \
    FAKE_ECR_PUSHED_MARKER="$push_marker" \
    FAKE_ECR_SCAN_LOOKUP_STATE="$scan_lookup_state" \
    FAKE_ECR_SCAN_SCENARIO="${FAKE_ECR_SCAN_SCENARIO:-complete}" \
    FAKE_GH_LOG="$gh_log" \
    FAKE_MAIN_SHA="${FAKE_MAIN_SHA:-$source_sha}" \
    FAKE_SLEEP_LOG="$sleep_log" \
    FAKE_TAG_DIGEST="$image_digest" \
    GH_TOKEN=fake-token \
    GITHUB_OUTPUT="$build_output" \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    SCAN_FILE="$test_dir/scan.json" \
    SOURCE_SHA="$source_sha" \
    "$real_task" --dir "$root_dir" lambda-ci-build-release-image
}

FAKE_ECR_LOOKUP_SCENARIO=existing FAKE_MAIN_SHA=$source_sha run_build_task
grep -Fqx "digest=$image_digest" "$build_output"
test ! -s "$docker_log"

FAKE_ECR_LOOKUP_SCENARIO=missing FAKE_MAIN_SHA=$source_sha run_build_task
grep -Fq 'docker buildx build' "$docker_log"
grep -Fq -- '--load' "$docker_log"
grep -Fq 'docker push' "$docker_log"
test "$(grep -Fc 'repos/CraigDevJohnson/portfolio/commits/main' "$gh_log")" -ge 2

FAKE_ECR_SCAN_SCENARIO=missing-once FAKE_MAIN_SHA=$source_sha run_build_task
test "$(grep -Fc 'ecr describe-image-scan-findings' "$aws_log")" -eq 3
test "$(grep -Fc 'sleep 5' "$sleep_log")" -eq 1

if FAKE_ECR_SCAN_SCENARIO=missing FAKE_MAIN_SHA=$source_sha \
  run_build_task > /dev/null 2>&1; then
  echo 'release build accepted a persistently missing ECR scan record' >&2
  exit 1
fi
test "$(grep -Fc 'ecr describe-image-scan-findings' "$aws_log")" -eq 12
test "$(grep -Fc 'sleep 5' "$sleep_log")" -eq 11
if grep -Fq 'ecr wait image-scan-complete' "$aws_log"; then
  echo 'release build reached the scan waiter without a visible scan record' >&2
  exit 1
fi

for scan_scenario in denied ambiguous; do
  if FAKE_ECR_SCAN_SCENARIO=$scan_scenario FAKE_MAIN_SHA=$source_sha \
    run_build_task > /dev/null 2>&1; then
    echo "release build accepted a $scan_scenario ECR scan lookup failure" >&2
    exit 1
  fi
  test "$(grep -Fc 'ecr describe-image-scan-findings' "$aws_log")" -eq 1
  test ! -s "$sleep_log"
  if grep -Fq 'ecr wait image-scan-complete' "$aws_log"; then
    echo "release build reached the scan waiter after a $scan_scenario scan lookup failure" >&2
    exit 1
  fi
done

if FAKE_DOCKER_SIGNAL_PARENT_ON_PUSH=true FAKE_ECR_LOOKUP_SCENARIO=missing \
  FAKE_MAIN_SHA=$source_sha run_build_task > /dev/null 2>&1; then
  echo 'release build resumed after a termination signal' >&2
  exit 1
fi
test ! -s "$build_output"

for lookup_scenario in denied ambiguous; do
  if FAKE_ECR_LOOKUP_SCENARIO=$lookup_scenario FAKE_MAIN_SHA=$source_sha \
    run_build_task > /dev/null 2>&1; then
    echo "release build accepted a $lookup_scenario ECR lookup failure" >&2
    exit 1
  fi
  test ! -s "$docker_log"
done

if FAKE_ECR_LOOKUP_SCENARIO=existing FAKE_MAIN_SHA=$other_sha \
  run_build_task > /dev/null 2>&1; then
  echo 'release build accepted a stale main commit' >&2
  exit 1
fi
test ! -s "$docker_log"

deployment_evidence_dir="$test_dir/deployment-evidence"
mkdir -p "$deployment_evidence_dir"
deployment_gh_log="$test_dir/deployment-gh.log"
: > "$deployment_gh_log"
FAKE_GH_LOG=$deployment_gh_log
export FAKE_GH_LOG
DEPLOYMENT_STATE=in_progress \
  ROLLBACK_VERSION=7 \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$deployment_evidence_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh"
jq -e '
  .development_deployment_id == 42 and
  .rollback_version == "7" and
  .status == "in_progress" and
  .status_recorded == true
' "$deployment_evidence_dir/github-deployment.json" > /dev/null

DEPLOYMENT_STATE=success \
  LAMBDA_VERSION=8 \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$deployment_evidence_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh"
jq -e '
  .development_deployment_id == 42 and
  .rollback_version == "7" and
  .status == "success" and
  .status_recorded == true
' "$deployment_evidence_dir/github-deployment.json" > /dev/null
test "$(grep -Fc \
  'gh api --method POST repos/CraigDevJohnson/portfolio/deployments -f ref=' \
  "$deployment_gh_log")" -eq 1
grep -Fq -- "-f description=Lambda $image_digest rollback-v7" \
  "$deployment_gh_log"
grep -Fq -- '-f state=in_progress' "$deployment_gh_log"
grep -Fq -- '-f state=success' "$deployment_gh_log"
grep -Fq -- "-f description=Verified $source_sha $image_digest v8" \
  "$deployment_gh_log"

if FAKE_DEPLOYMENT_STATUS_FAILURE=true \
  DEPLOYMENT_STATE=failure \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$deployment_evidence_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh" > /dev/null 2>&1; then
  echo 'development recorder accepted a failed GitHub deployment status' >&2
  exit 1
fi
jq -e '
  .development_deployment_id == 42 and
  .rollback_version == "7" and
  .status == "failure" and
  .status_recorded == false
' "$deployment_evidence_dir/github-deployment.json" > /dev/null
test -s "$deployment_evidence_dir/github-deployment-response.json"

transient_deployment_dir="$test_dir/transient-deployment-evidence"
transient_deployment_state="$test_dir/transient-deployment-state"
mkdir -p "$transient_deployment_dir"
DEPLOYMENT_STATE=in_progress \
  ROLLBACK_VERSION=7 \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$transient_deployment_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh"
FAKE_DEPLOYMENT_STATUS_FAILURE=once \
  FAKE_DEPLOYMENT_STATUS_FAILURE_STATE="$transient_deployment_state" \
  DEPLOYMENT_STATE=failure \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$transient_deployment_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh"
jq -e '.status == "failure" and .status_recorded == true' \
  "$transient_deployment_dir/github-deployment.json" > /dev/null

recovered_deployment_dir="$test_dir/recovered-deployment-evidence"
mkdir -p "$recovered_deployment_dir"
printf '{"id":42,"description":"Lambda %s rollback-v7"}\n' "$image_digest" \
  > "$recovered_deployment_dir/github-deployment-response.json"
DEPLOYMENT_STATE=failure \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  EVIDENCE_DIR="$recovered_deployment_dir" \
  sh "$root_dir/scripts/record-ci-lambda-development.sh"
jq -e '
  .development_deployment_id == 42 and
  .rollback_version == "7" and
  .status == "failure" and
  .status_recorded == true
' "$recovered_deployment_dir/github-deployment.json" > /dev/null

orchestration_dir="$test_dir/deployment-orchestration"
orchestration_workspace="$orchestration_dir/workspace"
orchestration_gh_log="$orchestration_dir/gh.log"
mkdir -p "$orchestration_dir/scripts" "$orchestration_workspace"
ln -s "$root_dir/scripts/deploy-ci-lambda-development.sh" \
  "$orchestration_dir/scripts/deploy-ci-lambda-development.sh"
ln -s "$root_dir/scripts/record-ci-lambda-development.sh" \
  "$orchestration_dir/scripts/record-ci-lambda-development.sh"
printf '#!/bin/sh\nexit 0\n' \
  > "$orchestration_dir/scripts/check-ci-state-bucket.sh"
printf '#!/bin/sh\n[ ! -e "${FAKE_MAIN_ADVANCED_MARKER:-}" ]\n' \
  > "$orchestration_dir/scripts/check-current-main.sh"
printf '#!/bin/sh\n' \
  > "$orchestration_dir/scripts/resolve-development-release-base.sh"
printf '[ -z "%s{FAKE_MAIN_ADVANCED_MARKER:-}" ] || : > "%sFAKE_MAIN_ADVANCED_MARKER"\n' \
  "$literal_dollar" "$literal_dollar" \
  >> "$orchestration_dir/scripts/resolve-development-release-base.sh"
printf 'printf "%%s\\t%%s\\n" "%s1" "%s{FAKE_DURABLE_VERSION:-}"\n' \
  "$literal_dollar" "$literal_dollar" \
  >> "$orchestration_dir/scripts/resolve-development-release-base.sh"
printf '#!/bin/sh\n: > "%sEVIDENCE_DIR/dev.tfplan"\n' "$literal_dollar" \
  > "$orchestration_dir/scripts/create-ci-lambda-release-plan.sh"
printf '#!/bin/sh\nexit 1\n' > "$orchestration_dir/scripts/verify-lambda-release.sh"
rollback_stub="$orchestration_dir/scripts/create-ci-lambda-rollback-plan.sh"
printf '#!/bin/sh\n' > "$rollback_stub"
printf 'printf "rollback\\n" > "%sEVIDENCE_DIR/rollback.tfplan"\n' \
  "$literal_dollar" >> "$rollback_stub"
printf 'printf "%%s\\n" "%sPRIOR_VERSION" > "%sEVIDENCE_DIR/rollback-prior-version.txt"\n' \
  "$literal_dollar" "$literal_dollar" >> "$rollback_stub"

stale_main_workspace="$orchestration_dir/stale-main-workspace"
stale_main_marker="$orchestration_dir/main-advanced"
stale_main_tofu_log="$orchestration_dir/stale-main-tofu.log"
mkdir -p "$stale_main_workspace"
: > "$stale_main_tofu_log"
if (
  cd "$orchestration_dir"
  FAKE_MAIN_ADVANCED_MARKER="$stale_main_marker" \
    FAKE_TOFU_LOG="$stale_main_tofu_log" \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    GITHUB_WORKSPACE="$stale_main_workspace" \
    STATE_BUCKET=portfolio-tofu-state-180294223248 \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    ECR_URL=example.invalid/portfolio \
    sh scripts/deploy-ci-lambda-development.sh
); then
  echo 'development orchestration accepted main advancing during base resolution' >&2
  exit 1
fi
if grep -Fq ' apply ' "$stale_main_tofu_log"; then
  echo 'development orchestration applied a plan after main advanced' >&2
  exit 1
fi

: > "$orchestration_gh_log"
if (
  cd "$orchestration_dir"
  FAKE_GH_LOG="$orchestration_gh_log" \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    GITHUB_WORKSPACE="$orchestration_workspace" \
    STATE_BUCKET=portfolio-tofu-state-180294223248 \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    ECR_URL=example.invalid/portfolio \
    sh scripts/deploy-ci-lambda-development.sh
); then
  echo 'development orchestration accepted failed verification' >&2
  exit 1
fi
jq -e '
  .status == "failure" and .status_recorded == true
' "$orchestration_workspace/evidence/github-deployment.json" > /dev/null
grep -Fq -- '-f state=in_progress' "$orchestration_gh_log"
grep -Fq -- '-f state=failure' "$orchestration_gh_log"
test -s "$orchestration_workspace/evidence/rollback.tfplan"

retry_workspace="$orchestration_dir/retry-workspace"
mkdir -p "$retry_workspace"
if (
  cd "$orchestration_dir"
  FAKE_DURABLE_VERSION=6 \
    FAKE_ALIAS_VERSION=7 \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    GITHUB_WORKSPACE="$retry_workspace" \
    STATE_BUCKET=portfolio-tofu-state-180294223248 \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    ECR_URL=example.invalid/portfolio \
    sh scripts/deploy-ci-lambda-development.sh
); then
  echo 'converged retry accepted failed verification' >&2
  exit 1
fi
grep -Fxq 6 "$retry_workspace/evidence/rollback-prior-version.txt" || {
  echo 'converged retry lost the durable last-known-good Lambda version' >&2
  exit 1
}

race_workspace="$orchestration_dir/race-workspace"
mkdir -p "$race_workspace"
if (
  cd "$orchestration_dir"
  DEVELOPMENT_BASE_VERSION=6 \
    FAKE_DURABLE_VERSION=7 \
    FAKE_ALIAS_VERSION=7 \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    GITHUB_WORKSPACE="$race_workspace" \
    STATE_BUCKET=portfolio-tofu-state-180294223248 \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    ECR_URL=example.invalid/portfolio \
    sh scripts/deploy-ci-lambda-development.sh
); then
  echo 'serialized release accepted failed verification' >&2
  exit 1
fi
grep -Fxq 7 "$race_workspace/evidence/rollback-prior-version.txt" || {
  echo 'serialized release used a stale pre-queue rollback version' >&2
  exit 1
}

for orchestration_failure in apply output signal; do
  failure_workspace="$orchestration_dir/$orchestration_failure-workspace"
  mkdir -p "$failure_workspace"
  if (
    cd "$orchestration_dir"
    if [ "$orchestration_failure" = apply ]; then
      export FAKE_TOFU_APPLY_FAILURE=true
    elif [ "$orchestration_failure" = output ]; then
      export FAKE_TOFU_OUTPUT_FAILURE=true
    else
      export FAKE_TOFU_APPLY_SIGNAL=true
    fi
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
      GITHUB_WORKSPACE="$failure_workspace" \
      STATE_BUCKET=portfolio-tofu-state-180294223248 \
      SOURCE_SHA=$source_sha \
      IMAGE_DIGEST=$image_digest \
      ECR_URL=example.invalid/portfolio \
      sh scripts/deploy-ci-lambda-development.sh
  ); then
    echo "development orchestration accepted a failed $orchestration_failure" >&2
    exit 1
  fi
  test -s "$failure_workspace/evidence/rollback.tfplan" || {
    echo "$orchestration_failure failure omitted rollback evidence" >&2
    exit 1
  }
done

alias_failure_workspace="$orchestration_dir/alias-failure-workspace"
alias_failure_gh_log="$orchestration_dir/alias-failure-gh.log"
mkdir -p "$alias_failure_workspace"
: > "$alias_failure_gh_log"
if (
  cd "$orchestration_dir"
  FAKE_ALIAS_FAILURE=true \
    FAKE_GH_LOG="$alias_failure_gh_log" \
    GITHUB_REPOSITORY=CraigDevJohnson/portfolio \
    GITHUB_WORKSPACE="$alias_failure_workspace" \
    STATE_BUCKET=portfolio-tofu-state-180294223248 \
    SOURCE_SHA=$source_sha \
    IMAGE_DIGEST=$image_digest \
    ECR_URL=example.invalid/portfolio \
    sh scripts/deploy-ci-lambda-development.sh
); then
  echo 'development orchestration accepted a failed prior-alias read' >&2
  exit 1
fi
test ! -e "$alias_failure_workspace/evidence/github-deployment.json"
test ! -s "$alias_failure_gh_log"

printf 'Release automation contracts passed\n'
