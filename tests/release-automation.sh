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
  scripts/record-ci-lambda-release-review.sh \
  scripts/resolve-development-release-base.sh \
  scripts/resolve-release-backlog-base.sh \
  scripts/resolve-reviewed-release-base.sh \
  scripts/validate-release-review-backlog.sh \
  scripts/validate-release-review-run.sh \
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
export GITHUB_RUN_ATTEMPT=1
export GITHUB_RUN_ID=9001

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

review_deployment_pages_json() {
  run_id=${3:-9001}
  run_attempt=${4:-1}
  jq -nc \
    --arg base_sha "$1" \
    --arg source_sha "$2" \
    --arg run_id "$run_id" \
    --arg run_attempt "$run_attempt" '[[{
    id: 84,
    ref: $source_sha,
    sha: $source_sha,
    task: "portfolio-lambda-release-review",
    environment: "release-review",
    description: (
      "Release review " + $base_sha + ".." + $source_sha +
      " run " + $run_id + "/" + $run_attempt
    ),
    created_at: "2026-08-30T00:02:00Z",
    creator: {login: "github-actions[bot]", type: "Bot"}
  }]]'
}

review_status_json() {
  run_id=${3:-9001}
  run_attempt=${4:-1}
  jq -nc \
    --arg base_sha "$1" \
    --arg source_sha "$2" \
    --arg run_id "$run_id" \
    --arg run_attempt "$run_attempt" '[{
    id: 184,
    created_at: "2026-08-30T00:03:00Z",
    state: "success",
    environment: "release-review",
    environment_url: "",
    description: (
      "Release reviewed " + $base_sha + ".." + $source_sha +
      " run " + $run_id + "/" + $run_attempt
    ),
    creator: {login: "github-actions[bot]", type: "Bot"}
  }]'
}

review_status_object_json() {
  status_id=${3:-184}
  created_at=${4:-2026-08-30T00:03:00Z}
  run_id=${5:-9001}
  run_attempt=${6:-1}
  jq -nc \
    --arg base_sha "$1" \
    --arg source_sha "$2" \
    --argjson status_id "$status_id" \
    --arg created_at "$created_at" \
    --arg run_id "$run_id" \
    --arg run_attempt "$run_attempt" '{
      id: $status_id,
      created_at: $created_at,
      state: "success",
      environment: "release-review",
      environment_url: "",
      description: (
        "Release reviewed " + $base_sha + ".." + $source_sha +
        " run " + $run_id + "/" + $run_attempt
      ),
      creator: {login: "github-actions[bot]", type: "Bot"}
    }'
}

review_run_json() {
  run_id=${2:-9001}
  run_attempt=${3:-1}
  run_status=${4:-completed}
  run_conclusion=${5:-success}
  jq -nc \
    --arg source_sha "$1" \
    --argjson run_id "$run_id" \
    --argjson run_attempt "$run_attempt" \
    --arg status "$run_status" \
    --arg conclusion "$run_conclusion" '{
      id: $run_id,
      run_attempt: $run_attempt,
      workflow_id: 346157322,
      head_sha: $source_sha,
      head_branch: "main",
      path: ".github/workflows/release.yml",
      event: "workflow_run",
      status: $status,
      conclusion: (if $conclusion == "null" then null else $conclusion end),
      repository: {full_name: "CraigDevJohnson/portfolio"},
      head_repository: {full_name: "CraigDevJohnson/portfolio"}
    }'
}

review_approvals_json() {
  jq -nc '[{
    state: "approved",
    environments: [{name: "release-review"}],
    user: {login: "CraigDevJohnson", id: 42454849, type: "User"}
  }]'
}

set_review_provenance() {
  FAKE_REVIEW_RUN_JSON=$(review_run_json "$@")
  FAKE_REVIEW_APPROVALS_JSON=$(review_approvals_json)
  export FAKE_REVIEW_RUN_JSON FAKE_REVIEW_APPROVALS_JSON
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

review_repo="$test_dir/review-repository"
mkdir -p "$review_repo/.github/workflows"
git -C "$review_repo" init -q
git -C "$review_repo" config user.email test@example.com
git -C "$review_repo" config user.name Test
echo release > "$review_repo/.github/workflows/release.yml"
git -C "$review_repo" add .
git -C "$review_repo" commit -qm development-base
review_development_base=$(git -C "$review_repo" rev-parse HEAD)
echo checkpoint >> "$review_repo/.github/workflows/release.yml"
git -C "$review_repo" commit -qam review-checkpoint
review_source=$(git -C "$review_repo" rev-parse HEAD)
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$review_development_base" "$review_source")
FAKE_REVIEW_STATUSES_JSON=$(review_status_json "$review_development_base" "$review_source")
FAKE_PULLS_JSON=$(reviewed_pull_json "$review_source" "$review_development_base")
set_review_provenance "$review_source"
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON FAKE_PULLS_JSON
resolved_review_base=$(cd "$review_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_source" "$review_development_base")
test "$resolved_review_base" = "$review_source" || {
  echo 'trusted release review did not advance the backlog cursor' >&2
  exit 1
}

FAKE_REVIEW_APPROVALS_JSON='[]'
export FAKE_REVIEW_APPROVALS_JSON
resolved_review_base=$(cd "$review_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_source" "$review_development_base" 2> /dev/null)
if [ "$resolved_review_base" != "$review_development_base" ]; then
  echo 'release review resolver trusted a run without Craig release-review approval' >&2
  exit 1
fi
set_review_provenance "$review_source"

set_review_provenance "$review_source" 9001 2
if sh "$root_dir/scripts/validate-release-review-run.sh" \
  9001 2 "$review_source" completed > /dev/null 2>&1; then
  echo 'release review validator accepted a later run attempt with reusable approval history' >&2
  exit 1
fi
set_review_provenance "$review_source"
if sh "$root_dir/scripts/validate-release-review-run.sh" \
  invalid-run 1 "$review_source" completed > /dev/null 2>&1; then
  echo 'release review validator accepted an invalid run ID' >&2
  exit 1
fi
trusted_review_run=$(review_run_json "$review_source")
while IFS='|' read -r invalid_run_label invalid_run_filter; do
  FAKE_REVIEW_RUN_JSON=$(printf '%s\n' "$trusted_review_run" | jq -c "$invalid_run_filter")
  export FAKE_REVIEW_RUN_JSON
  if sh "$root_dir/scripts/validate-release-review-run.sh" \
    9001 1 "$review_source" completed > /dev/null 2>&1; then
    echo "release review validator accepted a mismatched $invalid_run_label" >&2
    exit 1
  fi
done << EOF
run ID|.id = 9002
attempt|.run_attempt = 2
workflow ID|.workflow_id = 1
source SHA|.head_sha = "$other_sha"
branch|.head_branch = "feature"
workflow path|.path = ".github/workflows/other.yml"
event|.event = "push"
status|.status = "in_progress"
conclusion|.conclusion = "failure"
repository|.repository.full_name = "Other/repository"
head repository|.head_repository.full_name = "Other/repository"
EOF
set_review_provenance "$review_source"
FAKE_REVIEW_APPROVALS_JSON=$(review_approvals_json | jq -c '.[0].user.id = 1')
export FAKE_REVIEW_APPROVALS_JSON
if sh "$root_dir/scripts/validate-release-review-run.sh" \
  9001 1 "$review_source" completed > /dev/null 2>&1; then
  echo 'release review validator accepted approval from the wrong reviewer ID' >&2
  exit 1
fi
set_review_provenance "$review_source"

review_deployment=$(review_deployment_pages_json \
  "$review_development_base" "$review_source" | jq -c '.[0][0]')
review_status=$(review_status_json \
  "$review_development_base" "$review_source" | jq -c '.[0]')
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --argjson deployment "$review_deployment" '[[],[$deployment]]')
FAKE_REVIEW_STATUS_PAGES_JSON=$(jq -nc \
  --argjson status "$review_status" '[[],[$status]]')
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUS_PAGES_JSON
resolved_review_base=$(cd "$review_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_source" "$review_development_base")
test "$resolved_review_base" = "$review_source" || {
  echo 'release review resolver ignored a trusted candidate on page two' >&2
  exit 1
}

newer_failed_review_status=$(printf '%s\n' "$review_status" | jq -c '
  .id = 185 |
  .created_at = "2026-08-30T00:04:00Z" |
  .state = "failure" |
  .description = "Release review failed"
')
FAKE_REVIEW_STATUS_PAGES_JSON=$(jq -nc \
  --argjson success "$review_status" \
  --argjson failure "$newer_failed_review_status" '[[$success],[$failure]]')
export FAKE_REVIEW_STATUS_PAGES_JSON
resolved_review_base=$(cd "$review_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_source" "$review_development_base")
test "$resolved_review_base" = "$review_development_base" || {
  echo 'release review resolver let an older success override the newest failure' >&2
  exit 1
}

FAKE_REVIEW_STATUS_PAGES_JSON=$(jq -nc --argjson status "$review_status" '[[$status]]')
FAKE_REVIEW_RUN_JSON=$(review_run_json "$review_source" 9001 1 completed failure)
export FAKE_REVIEW_STATUS_PAGES_JSON FAKE_REVIEW_RUN_JSON
resolved_review_base=$(cd "$review_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_source" "$review_development_base" 2> /dev/null)
test "$resolved_review_base" = "$review_development_base" || {
  echo 'failed release run poisoned review backlog recovery' >&2
  exit 1
}
set_review_provenance "$review_source"

assert_review_pages_fail() {
  malformed_page_label=$1
  if (cd "$review_repo" &&
    sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
      "$review_source" "$review_development_base" > /dev/null 2>&1); then
    echo "release review resolver accepted $malformed_page_label" >&2
    exit 1
  fi
}

FAKE_REVIEW_DEPLOYMENT_PAGES_JSON='[{}]'
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON
assert_review_pages_fail 'a malformed deployment page shape'
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --argjson deployment "$review_deployment" '[[$deployment],[$deployment]]')
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON
assert_review_pages_fail 'duplicate deployment IDs across pages'
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --argjson deployment "$review_deployment" '[[$deployment]]')
FAKE_REVIEW_STATUS_PAGES_JSON='[{}]'
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUS_PAGES_JSON
assert_review_pages_fail 'a malformed status page shape'
FAKE_REVIEW_STATUS_PAGES_JSON=$(jq -nc \
  --argjson status "$review_status" '[[$status],[$status]]')
export FAKE_REVIEW_STATUS_PAGES_JSON
assert_review_pages_fail 'duplicate status IDs across pages'
unset FAKE_REVIEW_STATUS_PAGES_JSON

laundering_repo="$test_dir/review-laundering-repository"
mkdir -p "$laundering_repo/.github/workflows" "$laundering_repo/internal/app"
git -C "$laundering_repo" init -q
git -C "$laundering_repo" config user.email test@example.com
git -C "$laundering_repo" config user.name Test
echo release > "$laundering_repo/.github/workflows/release.yml"
echo deployed > "$laundering_repo/internal/app/app.go"
git -C "$laundering_repo" add .
git -C "$laundering_repo" commit -qm development-base
laundering_development_base=$(git -C "$laundering_repo" rev-parse HEAD)
echo pending-runtime >> "$laundering_repo/internal/app/app.go"
git -C "$laundering_repo" commit -qam pending-runtime
laundering_pull_base=$(git -C "$laundering_repo" rev-parse HEAD)
echo checkpoint >> "$laundering_repo/.github/workflows/release.yml"
git -C "$laundering_repo" commit -qam review-checkpoint
laundering_source=$(git -C "$laundering_repo" rev-parse HEAD)
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$laundering_development_base" "$laundering_source")
FAKE_REVIEW_STATUSES_JSON=$(review_status_json \
  "$laundering_development_base" "$laundering_source")
FAKE_PULLS_JSON=$(reviewed_pull_json "$laundering_source" "$laundering_pull_base")
set_review_provenance "$laundering_source"
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON FAKE_PULLS_JSON
if (cd "$laundering_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$laundering_source" "$laundering_development_base" > /dev/null 2>&1); then
  echo 'release review checkpoint laundered an undeployed runtime change' >&2
  exit 1
fi
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON

review_authorization_repo="$test_dir/review-authorization-repository"
mkdir -p "$review_authorization_repo/.github/workflows"
git -C "$review_authorization_repo" init -q
git -C "$review_authorization_repo" config user.email test@example.com
git -C "$review_authorization_repo" config user.name Test
echo release > "$review_authorization_repo/.github/workflows/release.yml"
git -C "$review_authorization_repo" add .
git -C "$review_authorization_repo" commit -qm release-epoch
review_authorization_base=$(git -C "$review_authorization_repo" rev-parse HEAD)
echo checkpoint >> "$review_authorization_repo/.github/workflows/release.yml"
git -C "$review_authorization_repo" commit -qam review-checkpoint
review_authorization_source=$(git -C "$review_authorization_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$review_authorization_source
FAKE_PULLS_JSON=$(reviewed_pull_json \
  "$review_authorization_source" "$review_authorization_base")
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
review_authorization_output="$test_dir/review-authorization-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON FAKE_DEPLOYMENT_PAGES_JSON
(cd "$review_authorization_repo" &&
  EVENT_SHA="$review_authorization_source" GITHUB_OUTPUT="$review_authorization_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh")
grep -Fqx "base_sha=$review_authorization_base" "$review_authorization_output"
grep -Fqx "development_base_sha=$review_authorization_base" "$review_authorization_output"
grep -Fqx 'classification=review' "$review_authorization_output"

recovery_authorization_repo="$test_dir/recovery-authorization-repository"
mkdir -p \
  "$recovery_authorization_repo/.github/workflows" \
  "$recovery_authorization_repo/internal/app" \
  "$recovery_authorization_repo/scripts"
git -C "$recovery_authorization_repo" init -q
git -C "$recovery_authorization_repo" config user.email test@example.com
git -C "$recovery_authorization_repo" config user.name Test
echo release > "$recovery_authorization_repo/.github/workflows/release.yml"
echo deployed > "$recovery_authorization_repo/internal/app/app.go"
git -C "$recovery_authorization_repo" add .
git -C "$recovery_authorization_repo" commit -qm release-epoch
recovery_development_base=$(git -C "$recovery_authorization_repo" rev-parse HEAD)
echo pending-runtime >> "$recovery_authorization_repo/internal/app/app.go"
git -C "$recovery_authorization_repo" commit -qam pending-runtime
recovery_pull_base=$(git -C "$recovery_authorization_repo" rev-parse HEAD)
echo verifier-fix > "$recovery_authorization_repo/scripts/verify-release.sh"
git -C "$recovery_authorization_repo" add .
git -C "$recovery_authorization_repo" commit -qm reviewed-verifier-fix
recovery_source=$(git -C "$recovery_authorization_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$recovery_source
FAKE_PULLS_JSON=$(reviewed_pull_json "$recovery_source" "$recovery_pull_base")
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON='[[]]'
recovery_authorization_output="$test_dir/recovery-authorization-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON FAKE_DEPLOYMENT_PAGES_JSON
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON
(cd "$recovery_authorization_repo" &&
  EVENT_SHA="$recovery_source" GITHUB_OUTPUT="$recovery_authorization_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh")
grep -Fqx "base_sha=$recovery_pull_base" "$recovery_authorization_output"
grep -Fqx "development_base_sha=$recovery_development_base" "$recovery_authorization_output"
grep -Fqx 'classification=development-reviewed' "$recovery_authorization_output"

review_record_log="$test_dir/release-review-record.log"
: > "$review_record_log"
FAKE_GH_LOG=$review_record_log
FAKE_MAIN_SHA=$review_authorization_source
FAKE_PULLS_JSON=$(reviewed_pull_json \
  "$review_authorization_source" "$review_authorization_base")
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
set_review_provenance "$review_authorization_source" 9001 1 in_progress null
export FAKE_GH_LOG FAKE_MAIN_SHA FAKE_PULLS_JSON FAKE_DEPLOYMENT_PAGES_JSON
(cd "$review_authorization_repo" &&
  SOURCE_SHA="$review_authorization_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh")
grep -Fq -- '-f task=portfolio-lambda-release-review' "$review_record_log"
grep -Fq -- '-f environment=release-review' "$review_record_log"
grep -Fq -- "-f ref=$review_authorization_source" "$review_record_log"
grep -Fq -- '-f state=success' "$review_record_log"
grep -Fq -- '/actions/runs/9001/attempts/1' "$review_record_log"
grep -Fq -- '/actions/runs/9001/approvals' "$review_record_log"
review_run_check_line=$(awk '/\/actions\/runs\/9001\/attempts\/1/ {print NR; exit}' \
  "$review_record_log")
review_deployment_post_line=$(awk '/--method POST .*\/deployments / {print NR; exit}' \
  "$review_record_log")
test "$review_run_check_line" -lt "$review_deployment_post_line"
if grep -Eq '(^| )aws( |$)|id-token|AWS_' "$review_record_log"; then
  echo 'release review recorder requested AWS authority' >&2
  exit 1
fi
unset FAKE_GH_LOG

FAKE_REVIEW_DEPLOYMENT_CREATED_AT=not-a-github-timestamp
export FAKE_REVIEW_DEPLOYMENT_CREATED_AT
if (cd "$review_authorization_repo" &&
  SOURCE_SHA="$review_authorization_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh" > /dev/null 2>&1); then
  echo 'release review recorder accepted a deployment response without a strict created_at' >&2
  exit 1
fi
unset FAKE_REVIEW_DEPLOYMENT_CREATED_AT

FAKE_REVIEW_STATUS_CREATED_AT=not-a-github-timestamp
export FAKE_REVIEW_STATUS_CREATED_AT
if (cd "$review_authorization_repo" &&
  SOURCE_SHA="$review_authorization_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh" > /dev/null 2>&1); then
  echo 'release review recorder accepted a status response without a strict created_at' >&2
  exit 1
fi
unset FAKE_REVIEW_STATUS_CREATED_AT

stale_review_log="$test_dir/stale-release-review.log"
stale_review_marker="$test_dir/stale-release-review-main-advanced"
: > "$stale_review_log"
rm -f "$stale_review_marker"
FAKE_GH_LOG=$stale_review_log
FAKE_MAIN_SHA=$review_authorization_source
FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER=$stale_review_marker
FAKE_MAIN_SHA_AFTER_REVIEW_DEPLOYMENT=$review_authorization_base
export FAKE_GH_LOG FAKE_MAIN_SHA FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER
export FAKE_MAIN_SHA_AFTER_REVIEW_DEPLOYMENT
if (cd "$review_authorization_repo" &&
  SOURCE_SHA="$review_authorization_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh" > /dev/null 2>&1); then
  echo 'release review recorder succeeded after main advanced' >&2
  exit 1
fi
grep -Fq -- '-f task=portfolio-lambda-release-review' "$stale_review_log"
if grep -Fq -- '-f state=success' "$stale_review_log"; then
  echo 'stale release review recorded a success status' >&2
  exit 1
fi
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$review_authorization_base" "$review_authorization_source")
FAKE_REVIEW_STATUSES_JSON='[]'
FAKE_PULLS_JSON=$(reviewed_pull_json \
  "$review_authorization_source" "$review_authorization_base")
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON FAKE_PULLS_JSON
resolved_review_base=$(cd "$review_authorization_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$review_authorization_source" "$review_authorization_base")
test "$resolved_review_base" = "$review_authorization_base" || {
  echo 'release review resolver trusted a deployment without success' >&2
  exit 1
}
unset FAKE_GH_LOG FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER
unset FAKE_MAIN_SHA_AFTER_REVIEW_DEPLOYMENT
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON

mixed_promotion_repo="$test_dir/mixed-promotion-review-repository"
mkdir -p "$mixed_promotion_repo/.github/workflows" "$mixed_promotion_repo/deploy"
git -C "$mixed_promotion_repo" init -q
git -C "$mixed_promotion_repo" config user.email test@example.com
git -C "$mixed_promotion_repo" config user.name Test
echo release > "$mixed_promotion_repo/.github/workflows/release.yml"
echo '{"schema_version":1}' > "$mixed_promotion_repo/deploy/production-release.json"
git -C "$mixed_promotion_repo" add .
git -C "$mixed_promotion_repo" commit -qm release-epoch
mixed_promotion_base=$(git -C "$mixed_promotion_repo" rev-parse HEAD)
echo checkpoint >> "$mixed_promotion_repo/.github/workflows/release.yml"
echo '{"schema_version":1,"reviewed":true}' \
  > "$mixed_promotion_repo/deploy/production-release.json"
git -C "$mixed_promotion_repo" commit -qam mixed-promotion-review
mixed_promotion_source=$(git -C "$mixed_promotion_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$mixed_promotion_source
FAKE_PULLS_JSON=$(reviewed_pull_json "$mixed_promotion_source" "$mixed_promotion_base")
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
mixed_promotion_output="$test_dir/mixed-promotion-review-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON FAKE_DEPLOYMENT_PAGES_JSON
if (cd "$mixed_promotion_repo" &&
  EVENT_SHA="$mixed_promotion_source" GITHUB_OUTPUT="$mixed_promotion_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" > /dev/null 2>&1); then
  echo 'release review authorization checkpointed a current mixed promotion' >&2
  exit 1
fi
mixed_promotion_log="$test_dir/mixed-promotion-review.log"
: > "$mixed_promotion_log"
FAKE_GH_LOG=$mixed_promotion_log
export FAKE_GH_LOG
if (cd "$mixed_promotion_repo" &&
  SOURCE_SHA="$mixed_promotion_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh" > /dev/null 2>&1); then
  echo 'release review recorder checkpointed a current mixed promotion' >&2
  exit 1
fi
if grep -Fq -- '--method POST' "$mixed_promotion_log"; then
  echo 'release review recorder wrote a deployment for a mixed promotion' >&2
  exit 1
fi
unset FAKE_GH_LOG

echo later-review >> "$mixed_promotion_repo/.github/workflows/release.yml"
git -C "$mixed_promotion_repo" commit -qam later-clean-review
delayed_promotion_source=$(git -C "$mixed_promotion_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$delayed_promotion_source
FAKE_PULLS_JSON=$(reviewed_pull_json "$delayed_promotion_source" "$mixed_promotion_source")
delayed_promotion_output="$test_dir/delayed-promotion-review-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON
if (cd "$mixed_promotion_repo" &&
  EVENT_SHA="$delayed_promotion_source" GITHUB_OUTPUT="$delayed_promotion_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" > /dev/null 2>&1); then
  echo 'later release review checkpoint laundered a prior mixed promotion' >&2
  exit 1
fi
delayed_promotion_log="$test_dir/delayed-promotion-review.log"
: > "$delayed_promotion_log"
FAKE_GH_LOG=$delayed_promotion_log
export FAKE_GH_LOG
if (cd "$mixed_promotion_repo" &&
  SOURCE_SHA="$delayed_promotion_source" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh" > /dev/null 2>&1); then
  echo 'later release review recorder laundered a prior mixed promotion' >&2
  exit 1
fi
if grep -Fq -- '--method POST' "$delayed_promotion_log"; then
  echo 'later release review recorder wrote past a prior mixed promotion' >&2
  exit 1
fi
unset FAKE_GH_LOG

pending_promotion_repo="$test_dir/pending-promotion-review-repository"
mkdir -p "$pending_promotion_repo/.github/workflows" "$pending_promotion_repo/deploy"
git -C "$pending_promotion_repo" init -q
git -C "$pending_promotion_repo" config user.email test@example.com
git -C "$pending_promotion_repo" config user.name Test
echo release > "$pending_promotion_repo/.github/workflows/release.yml"
echo '{"schema_version":1}' > "$pending_promotion_repo/deploy/production-release.json"
git -C "$pending_promotion_repo" add .
git -C "$pending_promotion_repo" commit -qm development-base
pending_promotion_development_base=$(git -C "$pending_promotion_repo" rev-parse HEAD)
echo '{"schema_version":1,"source_sha":"verified"}' \
  > "$pending_promotion_repo/deploy/production-release.json"
git -C "$pending_promotion_repo" commit -qam standalone-promotion
pending_promotion_merge=$(git -C "$pending_promotion_repo" rev-parse HEAD)
echo checkpoint >> "$pending_promotion_repo/.github/workflows/release.yml"
git -C "$pending_promotion_repo" commit -qam review-after-promotion
pending_promotion_first_review=$(git -C "$pending_promotion_repo" rev-parse HEAD)
echo follow-up >> "$pending_promotion_repo/.github/workflows/release.yml"
git -C "$pending_promotion_repo" commit -qam review-checkpoint-fix
pending_promotion_review=$(git -C "$pending_promotion_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$pending_promotion_review
FAKE_PULLS_BY_COMMIT_JSON=$(jq -nc \
  --arg review "$pending_promotion_review" \
  --arg first_review "$pending_promotion_first_review" \
  --arg promotion "$pending_promotion_merge" \
  --argjson review_pull "$(reviewed_pull_json \
    "$pending_promotion_review" "$pending_promotion_first_review")" \
  --argjson first_review_pull "$(reviewed_pull_json \
    "$pending_promotion_first_review" "$pending_promotion_merge")" \
  --argjson promotion_pull "$(reviewed_pull_json \
    "$pending_promotion_merge" "$pending_promotion_development_base")" \
  '{
    ($review):$review_pull,
    ($first_review):$first_review_pull,
    ($promotion):$promotion_pull
  }')
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON='[[]]'
pending_promotion_output="$test_dir/pending-promotion-review-output"
export FAKE_MAIN_SHA FAKE_PULLS_BY_COMMIT_JSON FAKE_DEPLOYMENT_PAGES_JSON
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON
(cd "$pending_promotion_repo" &&
  EVENT_SHA="$pending_promotion_review" GITHUB_OUTPUT="$pending_promotion_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh")
grep -Fqx "base_sha=$pending_promotion_first_review" "$pending_promotion_output"
grep -Fqx \
  "development_base_sha=$pending_promotion_development_base" \
  "$pending_promotion_output"
grep -Fqx 'classification=review' "$pending_promotion_output"

set_review_provenance "$pending_promotion_review" 9001 1 in_progress null
(cd "$pending_promotion_repo" &&
  SOURCE_SHA="$pending_promotion_review" \
    sh "$root_dir/scripts/record-ci-lambda-release-review.sh")

FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$pending_promotion_development_base" "$pending_promotion_review")
FAKE_REVIEW_STATUSES_JSON=$(review_status_json \
  "$pending_promotion_development_base" "$pending_promotion_review")
set_review_provenance "$pending_promotion_review"
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON
resolved_pending_promotion_base=$(cd "$pending_promotion_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$pending_promotion_review" "$pending_promotion_development_base")
test "$resolved_pending_promotion_base" = "$pending_promotion_review" || {
  echo 'standalone pending promotion blocked a later release review checkpoint' >&2
  exit 1
}
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON='[[]]'
unset FAKE_REVIEW_STATUSES_JSON
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON

git -C "$pending_promotion_repo" checkout -qb multiple-promotions \
  "$pending_promotion_merge"
echo '{"schema_version":1,"source_sha":"verified-again"}' \
  > "$pending_promotion_repo/deploy/production-release.json"
git -C "$pending_promotion_repo" commit -qam second-standalone-promotion
second_pending_promotion=$(git -C "$pending_promotion_repo" rev-parse HEAD)
echo second-checkpoint >> "$pending_promotion_repo/.github/workflows/release.yml"
git -C "$pending_promotion_repo" commit -qam review-after-second-promotion
multiple_promotion_review=$(git -C "$pending_promotion_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$multiple_promotion_review
FAKE_PULLS_BY_COMMIT_JSON=$(jq -nc \
  --arg review "$multiple_promotion_review" \
  --arg second "$second_pending_promotion" \
  --arg first "$pending_promotion_merge" \
  --argjson review_pull "$(reviewed_pull_json \
    "$multiple_promotion_review" "$second_pending_promotion")" \
  --argjson second_pull "$(reviewed_pull_json \
    "$second_pending_promotion" "$pending_promotion_merge")" \
  --argjson first_pull "$(reviewed_pull_json \
    "$pending_promotion_merge" "$pending_promotion_development_base")" \
  '{($review):$review_pull,($second):$second_pull,($first):$first_pull}')
multiple_promotion_output="$test_dir/multiple-promotion-review-output"
multiple_promotion_error="$test_dir/multiple-promotion-review-error"
export FAKE_MAIN_SHA FAKE_PULLS_BY_COMMIT_JSON
if (cd "$pending_promotion_repo" &&
  EVENT_SHA="$multiple_promotion_review" GITHUB_OUTPUT="$multiple_promotion_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" \
      > /dev/null 2> "$multiple_promotion_error"); then
  echo 'release review checkpoint accepted multiple pending promotions' >&2
  exit 1
fi
grep -Fq 'checkpoint recovery contains multiple production promotions' \
  "$multiple_promotion_error" || {
  echo 'multiple-promotion test did not reach the exact checkpoint guard' >&2
  exit 1
}
unset FAKE_PULLS_BY_COMMIT_JSON FAKE_REVIEW_DEPLOYMENT_PAGES_JSON
unset FAKE_REVIEW_STATUSES_JSON

assert_pending_promotion_prefix_rejected() {
  blocked_label=$1
  blocked_path=$2
  blocked_repo="$test_dir/pending-promotion-$blocked_label-repository"
  mkdir -p "$blocked_repo/.github/workflows" "$blocked_repo/deploy"
  git -C "$blocked_repo" init -q
  git -C "$blocked_repo" config user.email test@example.com
  git -C "$blocked_repo" config user.name Test
  echo release > "$blocked_repo/.github/workflows/release.yml"
  echo '{"schema_version":1}' > "$blocked_repo/deploy/production-release.json"
  git -C "$blocked_repo" add .
  git -C "$blocked_repo" commit -qm development-base
  blocked_development_base=$(git -C "$blocked_repo" rev-parse HEAD)
  mkdir -p "$(dirname "$blocked_repo/$blocked_path")"
  echo blocked > "$blocked_repo/$blocked_path"
  git -C "$blocked_repo" add .
  git -C "$blocked_repo" commit -qm "pending-$blocked_label"
  blocked_prefix=$(git -C "$blocked_repo" rev-parse HEAD)
  echo '{"schema_version":1,"source_sha":"verified"}' \
    > "$blocked_repo/deploy/production-release.json"
  git -C "$blocked_repo" commit -qam standalone-promotion
  blocked_promotion=$(git -C "$blocked_repo" rev-parse HEAD)
  echo checkpoint >> "$blocked_repo/.github/workflows/release.yml"
  git -C "$blocked_repo" commit -qam review-after-promotion
  blocked_review=$(git -C "$blocked_repo" rev-parse HEAD)
  FAKE_MAIN_SHA=$blocked_review
  FAKE_PULLS_BY_COMMIT_JSON=$(jq -nc \
    --arg review "$blocked_review" \
    --arg promotion "$blocked_promotion" \
    --arg prefix "$blocked_prefix" \
    --argjson review_pull "$(reviewed_pull_json \
      "$blocked_review" "$blocked_promotion")" \
    --argjson promotion_pull "$(reviewed_pull_json \
      "$blocked_promotion" "$blocked_prefix")" \
    --argjson prefix_pull "$(reviewed_pull_json \
      "$blocked_prefix" "$blocked_development_base")" \
    '{
      ($review):$review_pull,
      ($promotion):$promotion_pull,
      ($prefix):$prefix_pull
    }')
  blocked_output="$test_dir/pending-promotion-$blocked_label-output"
  blocked_error="$test_dir/pending-promotion-$blocked_label-error"
  export FAKE_MAIN_SHA FAKE_PULLS_BY_COMMIT_JSON
  if (cd "$blocked_repo" &&
    EVENT_SHA="$blocked_review" GITHUB_OUTPUT="$blocked_output" \
      sh "$root_dir/scripts/authorize-ci-lambda-release.sh" \
        > /dev/null 2> "$blocked_error"); then
    echo "release review checkpoint laundered pending $blocked_label" >&2
    exit 1
  fi
  grep -Fq 'checkpoint recovery contains a runtime, mixed, or unknown pull request' \
    "$blocked_error" || {
    echo "pending-$blocked_label test did not reach the exact checkpoint guard" >&2
    exit 1
  }
}

assert_pending_promotion_prefix_rejected runtime internal/app/app.go
assert_pending_promotion_prefix_rejected unknown chrome-extension/manifest.json
unset FAKE_PULLS_BY_COMMIT_JSON

reviewed_runtime_repo="$test_dir/reviewed-runtime-repository"
mkdir -p "$reviewed_runtime_repo/.github/workflows" "$reviewed_runtime_repo/internal/app"
git -C "$reviewed_runtime_repo" init -q
git -C "$reviewed_runtime_repo" config user.email test@example.com
git -C "$reviewed_runtime_repo" config user.name Test
echo release > "$reviewed_runtime_repo/.github/workflows/release.yml"
git -C "$reviewed_runtime_repo" add .
git -C "$reviewed_runtime_repo" commit -qm release-epoch
reviewed_runtime_development_base=$(git -C "$reviewed_runtime_repo" rev-parse HEAD)
echo checkpoint >> "$reviewed_runtime_repo/.github/workflows/release.yml"
git -C "$reviewed_runtime_repo" commit -qam review-checkpoint
reviewed_runtime_checkpoint=$(git -C "$reviewed_runtime_repo" rev-parse HEAD)
echo runtime > "$reviewed_runtime_repo/internal/app/app.go"
git -C "$reviewed_runtime_repo" add .
git -C "$reviewed_runtime_repo" commit -qm runtime
reviewed_runtime_source=$(git -C "$reviewed_runtime_repo" rev-parse HEAD)
FAKE_MAIN_SHA=$reviewed_runtime_source
FAKE_PULLS_JSON=$(reviewed_pull_json \
  "$reviewed_runtime_source" "$reviewed_runtime_checkpoint")
FAKE_REVIEW_SOURCE_SHA=$reviewed_runtime_checkpoint
FAKE_REVIEW_PULLS_JSON=$(reviewed_pull_json \
  "$reviewed_runtime_checkpoint" "$reviewed_runtime_development_base")
FAKE_DEPLOYMENT_PAGES_JSON='[[]]'
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$reviewed_runtime_development_base" "$reviewed_runtime_checkpoint")
FAKE_REVIEW_STATUSES_JSON=$(review_status_json \
  "$reviewed_runtime_development_base" "$reviewed_runtime_checkpoint")
set_review_provenance "$reviewed_runtime_checkpoint"
reviewed_runtime_output="$test_dir/reviewed-runtime-authorization-output"
export FAKE_MAIN_SHA FAKE_PULLS_JSON FAKE_REVIEW_SOURCE_SHA FAKE_REVIEW_PULLS_JSON
export FAKE_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON
(cd "$reviewed_runtime_repo" &&
  EVENT_SHA="$reviewed_runtime_source" GITHUB_OUTPUT="$reviewed_runtime_output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh")
grep -Fqx "base_sha=$reviewed_runtime_checkpoint" "$reviewed_runtime_output"
grep -Fqx "development_base_sha=$reviewed_runtime_development_base" "$reviewed_runtime_output"
grep -Fqx 'classification=development' "$reviewed_runtime_output"
unset FAKE_REVIEW_SOURCE_SHA FAKE_REVIEW_PULLS_JSON
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON

ancestry_repo="$test_dir/review-ancestry-repository"
mkdir -p "$ancestry_repo/.github/workflows"
git -C "$ancestry_repo" init -q
git -C "$ancestry_repo" config user.email test@example.com
git -C "$ancestry_repo" config user.name Test
echo release > "$ancestry_repo/.github/workflows/release.yml"
git -C "$ancestry_repo" add .
git -C "$ancestry_repo" commit -qm development-base
ancestry_development_base=$(git -C "$ancestry_repo" rev-parse HEAD)
echo first >> "$ancestry_repo/.github/workflows/release.yml"
git -C "$ancestry_repo" commit -qam first-review
ancestry_first_review=$(git -C "$ancestry_repo" rev-parse HEAD)
echo second >> "$ancestry_repo/.github/workflows/release.yml"
git -C "$ancestry_repo" commit -qam second-review
ancestry_second_review=$(git -C "$ancestry_repo" rev-parse HEAD)
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --arg base "$ancestry_development_base" \
  --arg first "$ancestry_first_review" \
  --arg second "$ancestry_second_review" '[[
    {
      id: 84, ref: $first, sha: $first,
      task: "portfolio-lambda-release-review", environment: "release-review",
      description: ("Release review " + $base + ".." + $first + " run 9001/1"),
      created_at: "2026-08-30T00:05:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    },
    {
      id: 85, ref: $second, sha: $second,
      task: "portfolio-lambda-release-review", environment: "release-review",
      description: ("Release review " + $base + ".." + $second + " run 9002/1"),
      created_at: "2026-08-30T00:04:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    }
  ]]')
ancestry_first_status=$(review_status_object_json \
  "$ancestry_development_base" "$ancestry_first_review" 184 2026-08-30T00:06:00Z 9001 1)
ancestry_second_status=$(review_status_object_json \
  "$ancestry_development_base" "$ancestry_second_review" 185 2026-08-30T00:05:00Z 9002 1)
FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON=$(jq -nc \
  --argjson first "$ancestry_first_status" \
  --argjson second "$ancestry_second_status" \
  '{"84":[$first],"85":[$second]}')
FAKE_PULLS_BY_COMMIT_JSON=$(jq -nc \
  --arg first "$ancestry_first_review" \
  --arg first_base "$ancestry_development_base" \
  --arg second "$ancestry_second_review" \
  --arg second_base "$ancestry_first_review" \
  --argjson first_pull "$(reviewed_pull_json \
    "$ancestry_first_review" "$ancestry_development_base")" \
  --argjson second_pull "$(reviewed_pull_json \
    "$ancestry_second_review" "$ancestry_first_review")" \
  '{($first):$first_pull,($second):$second_pull}')
FAKE_REVIEW_RUNS_BY_ID_JSON=$(jq -nc \
  --argjson first "$(review_run_json "$ancestry_first_review" 9001)" \
  --argjson second "$(review_run_json "$ancestry_second_review" 9002)" \
  '{"9001":$first,"9002":$second}')
FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON=$(jq -nc \
  --argjson approval "$(review_approvals_json)" \
  '{"9001":$approval,"9002":$approval}')
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON
export FAKE_PULLS_BY_COMMIT_JSON FAKE_REVIEW_RUNS_BY_ID_JSON
export FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON
resolved_review_base=$(cd "$ancestry_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$ancestry_second_review" "$ancestry_development_base")
test "$resolved_review_base" = "$ancestry_second_review" || {
  echo 'release review cursor was selected by API timestamp instead of Git ancestry' >&2
  exit 1
}
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON
unset FAKE_PULLS_BY_COMMIT_JSON FAKE_REVIEW_RUNS_BY_ID_JSON
unset FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON

divergent_repo="$test_dir/divergent-review-repository"
mkdir -p "$divergent_repo/.github/workflows"
git -C "$divergent_repo" init -q
git -C "$divergent_repo" config user.email test@example.com
git -C "$divergent_repo" config user.name Test
echo release > "$divergent_repo/.github/workflows/release.yml"
git -C "$divergent_repo" add .
git -C "$divergent_repo" commit -qm development-base
divergent_development_base=$(git -C "$divergent_repo" rev-parse HEAD)
divergent_default_branch=$(git -C "$divergent_repo" branch --show-current)
git -C "$divergent_repo" branch second-review
echo first > "$divergent_repo/.github/workflows/first.yml"
git -C "$divergent_repo" add .
git -C "$divergent_repo" commit -qm first-review
divergent_first_review=$(git -C "$divergent_repo" rev-parse HEAD)
git -C "$divergent_repo" checkout -q second-review
echo second > "$divergent_repo/.github/workflows/second.yml"
git -C "$divergent_repo" add .
git -C "$divergent_repo" commit -qm second-review
divergent_second_review=$(git -C "$divergent_repo" rev-parse HEAD)
git -C "$divergent_repo" checkout -q "$divergent_default_branch"
git -C "$divergent_repo" merge -q --no-ff second-review -m combined-source
divergent_source=$(git -C "$divergent_repo" rev-parse HEAD)
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(jq -nc \
  --arg base "$divergent_development_base" \
  --arg first "$divergent_first_review" \
  --arg second "$divergent_second_review" '[[
    {
      id: 84, ref: $first, sha: $first,
      task: "portfolio-lambda-release-review", environment: "release-review",
      description: ("Release review " + $base + ".." + $first + " run 9001/1"),
      created_at: "2026-08-30T00:04:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    },
    {
      id: 85, ref: $second, sha: $second,
      task: "portfolio-lambda-release-review", environment: "release-review",
      description: ("Release review " + $base + ".." + $second + " run 9002/1"),
      created_at: "2026-08-30T00:05:00Z",
      creator: {login: "github-actions[bot]", type: "Bot"}
    }
  ]]')
divergent_first_status=$(review_status_object_json \
  "$divergent_development_base" "$divergent_first_review" 184 \
  2026-08-30T00:03:00Z 9001 1)
divergent_second_status=$(review_status_object_json \
  "$divergent_development_base" "$divergent_second_review" 185 \
  2026-08-30T00:03:00Z 9002 1)
FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON=$(jq -nc \
  --argjson first "$divergent_first_status" \
  --argjson second "$divergent_second_status" \
  '{"84":[$first],"85":[$second]}')
FAKE_PULLS_BY_COMMIT_JSON=$(jq -nc \
  --arg first "$divergent_first_review" \
  --arg second "$divergent_second_review" \
  --argjson first_pull "$(reviewed_pull_json \
    "$divergent_first_review" "$divergent_development_base")" \
  --argjson second_pull "$(reviewed_pull_json \
    "$divergent_second_review" "$divergent_development_base")" \
  '{($first):$first_pull,($second):$second_pull}')
FAKE_REVIEW_RUNS_BY_ID_JSON=$(jq -nc \
  --argjson first "$(review_run_json "$divergent_first_review" 9001)" \
  --argjson second "$(review_run_json "$divergent_second_review" 9002)" \
  '{"9001":$first,"9002":$second}')
FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON=$(jq -nc \
  --argjson approval "$(review_approvals_json)" \
  '{"9001":$approval,"9002":$approval}')
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON
export FAKE_PULLS_BY_COMMIT_JSON FAKE_REVIEW_RUNS_BY_ID_JSON
export FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON
if (cd "$divergent_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$divergent_source" "$divergent_development_base" > /dev/null 2>&1); then
  echo 'release review resolver accepted divergent trusted checkpoints' >&2
  exit 1
fi
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON
unset FAKE_PULLS_BY_COMMIT_JSON FAKE_REVIEW_RUNS_BY_ID_JSON
unset FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON

obsolete_repo="$test_dir/obsolete-review-repository"
mkdir -p "$obsolete_repo/.github/workflows" "$obsolete_repo/internal/app"
git -C "$obsolete_repo" init -q
git -C "$obsolete_repo" config user.email test@example.com
git -C "$obsolete_repo" config user.name Test
echo release > "$obsolete_repo/.github/workflows/release.yml"
git -C "$obsolete_repo" add .
git -C "$obsolete_repo" commit -qm development-base
obsolete_original_base=$(git -C "$obsolete_repo" rev-parse HEAD)
echo review >> "$obsolete_repo/.github/workflows/release.yml"
git -C "$obsolete_repo" commit -qam review-checkpoint
obsolete_review=$(git -C "$obsolete_repo" rev-parse HEAD)
echo deployed > "$obsolete_repo/internal/app/app.go"
git -C "$obsolete_repo" add .
git -C "$obsolete_repo" commit -qm newer-development
obsolete_development_base=$(git -C "$obsolete_repo" rev-parse HEAD)
FAKE_REVIEW_DEPLOYMENT_PAGES_JSON=$(review_deployment_pages_json \
  "$obsolete_original_base" "$obsolete_review")
FAKE_REVIEW_STATUSES_JSON='not-valid-status-json'
FAKE_PULLS_JSON=$(reviewed_pull_json "$obsolete_review" "$obsolete_original_base")
FAKE_REVIEW_RUN_JSON='not-valid-run-json'
FAKE_REVIEW_APPROVALS_JSON='not-valid-approval-json'
export FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON FAKE_PULLS_JSON
export FAKE_REVIEW_RUN_JSON FAKE_REVIEW_APPROVALS_JSON
resolved_review_base=$(cd "$obsolete_repo" &&
  sh "$root_dir/scripts/resolve-release-backlog-base.sh" \
    "$obsolete_development_base" "$obsolete_development_base")
test "$resolved_review_base" = "$obsolete_development_base" || {
  echo 'obsolete release review checkpoint rewound the development cursor' >&2
  exit 1
}
unset FAKE_REVIEW_DEPLOYMENT_PAGES_JSON FAKE_REVIEW_STATUSES_JSON
set_review_provenance "$review_source"

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
curl_argument_log="$test_dir/curl-arguments.log"
FAKE_HEALTH_SHA=$source_sha
FAKE_QUALIFIED_IMAGE_URI="example.invalid/portfolio@$image_digest"
FAKE_CURL_LOG=$curl_log
FAKE_CURL_ARGUMENT_LOG=$curl_argument_log
SMOKE_WINDOW_SECONDS=300
SMOKE_INTERVAL_SECONDS=30
export FAKE_HEALTH_SHA FAKE_QUALIFIED_IMAGE_URI FAKE_CURL_LOG FAKE_CURL_ARGUMENT_LOG
export SMOKE_WINDOW_SECONDS SMOKE_INTERVAL_SECONDS
ORIGIN_HOST=origin.example.invalid \
  BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh"
grep -Fxq 'https://example.invalid/static/images/backgrounds/home-hero.jpg' "$curl_log"
grep -Fq -- '--connect-to example.invalid:443:origin.example.invalid:443' "$curl_argument_log"
jq -e '
  .base_url == "https://example.invalid" and
  .origin_host == "origin.example.invalid"
' "$evidence_dir/route-probe-target.json" > /dev/null
grep -Fq -- '--connect-timeout 10' "$root_dir/scripts/verify-lambda-release.sh"
grep -Fq -- '--max-time 30' "$root_dir/scripts/verify-lambda-release.sh"
if ORIGIN_HOST='origin.example.invalid --resolve attacker.example:443:127.0.0.1' \
  BASE_URL=https://example.invalid \
  SOURCE_SHA=$source_sha \
  IMAGE_DIGEST=$image_digest \
  FUNCTION_NAME=portfolio-lambda-dev \
  EVIDENCE_DIR="$evidence_dir" \
  sh "$root_dir/scripts/verify-lambda-release.sh" > /dev/null 2>&1; then
  echo 'release verification accepted an invalid origin hostname' >&2
  exit 1
fi
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
expected_alarm_command='aws cloudwatch describe-alarms --alarm-names'
expected_alarm_command="$expected_alarm_command portfolio-lambda-dev-api-5xx"
expected_alarm_command="$expected_alarm_command portfolio-lambda-dev-api-latency"
expected_alarm_command="$expected_alarm_command portfolio-lambda-dev-lambda-duration"
expected_alarm_command="$expected_alarm_command portfolio-lambda-dev-lambda-errors"
expected_alarm_command="$expected_alarm_command portfolio-lambda-dev-lambda-throttles"
expected_alarm_command="$expected_alarm_command --no-paginate --output json"
test "$(grep -Fxc "$expected_alarm_command" "$smoke_aws_log")" -eq 11
if grep -Fq 'aws cloudwatch describe-alarms --alarm-name-prefix' "$smoke_aws_log"; then
  echo 'release verification enumerated CloudWatch alarms beyond the exact allowlist' >&2
  exit 1
fi
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

assert_runtime_change() {
  runtime_base=$1
  runtime_head=$2
  runtime_label=$3
  for runtime_scope in current development-backlog release-review release-review-current; do
    runtime_classification=$(cd "$tmp" &&
      sh "$root_dir/scripts/classify-release-change.sh" \
        "$runtime_base" "$runtime_head" "$runtime_scope")
    [ "$runtime_classification" = development ] || {
      echo "$runtime_label was not runtime in $runtime_scope scope" >&2
      exit 1
    }
  done
}

base=$head
echo 'go 1.25.0' > "$tmp/go.work"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-workspace
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" go.work
base=$head
echo checksum > "$tmp/go.work.sum"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-workspace-sum
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" go.work.sum
base=$head
mkdir -p "$tmp/vendor/example.invalid/module"
echo vendored > "$tmp/vendor/example.invalid/module/module.go"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-vendor-input
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" vendor
base=$head
mkdir -p "$tmp/cmd/web/static"
echo embedded > "$tmp/cmd/web/static/runtime.md"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-embedded-markdown
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" cmd-web-static-markdown
base=$head
echo embedded > "$tmp/cmd/web/static/runtime asset.txt"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-runtime-path-with-space
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" runtime-path-with-space
base=$head
mkdir -p "$tmp/pkg"
echo runtime > "$tmp/pkg/foo.go"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-unknown-go-build-input
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" pkg-go-input
base=$head
mkdir -p "$tmp/pkg/embed"
echo embedded > "$tmp/pkg/embed/data.bin"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-unknown-embedded-build-input
head=$(git -C "$tmp" rev-parse HEAD)
assert_runtime_change "$base" "$head" pkg-embedded-input
base=$head
echo review > "$tmp/.github/workflows/review-only.yml"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-known-review-input
head=$(git -C "$tmp" rev-parse HEAD)
for review_scope in current development-backlog release-review release-review-current; do
  review_classification=$(cd "$tmp" &&
    sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head" "$review_scope")
  [ "$review_classification" = review ] || {
    echo "known workflow change was not review-only in $review_scope scope" >&2
    exit 1
  }
done
base=$head
mkdir -p "$tmp/chrome-extension"
echo excluded > "$tmp/chrome-extension/manifest.json"
git -C "$tmp" add .
git -C "$tmp" commit -qm add-unknown-excluded-path
head=$(git -C "$tmp" rev-parse HEAD)
for ordinary_scope in current development-backlog; do
  ordinary_classification=$(cd "$tmp" &&
    sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head" "$ordinary_scope")
  [ "$ordinary_classification" = review ] || {
    echo "unknown excluded path did not fail closed in $ordinary_scope scope" >&2
    exit 1
  }
done
for checkpoint_scope in release-review release-review-current; do
  checkpoint_classification=$(cd "$tmp" &&
    sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head" "$checkpoint_scope")
  [ "$checkpoint_classification" = development ] || {
    echo "unknown path was checkpointable in $checkpoint_scope scope" >&2
    exit 1
  }
done

weird_filename_repo="$test_dir/weird-filename-repository"
mkdir -p "$weird_filename_repo/cmd"
git -C "$weird_filename_repo" init -q
git -C "$weird_filename_repo" config user.email test@example.com
git -C "$weird_filename_repo" config user.name Test
echo base > "$weird_filename_repo/README.md"
git -C "$weird_filename_repo" add .
git -C "$weird_filename_repo" commit -qm base
weird_filename_base=$(git -C "$weird_filename_repo" rev-parse HEAD)
newline_runtime_path=$(printf 'cmd/runtime\nname.go')
tab_runtime_path=$(printf 'cmd/runtime\tname.go')
backslash_runtime_path='cmd/runtime\name.go'
printf 'runtime\n' > "$weird_filename_repo/$newline_runtime_path"
printf 'runtime\n' > "$weird_filename_repo/$tab_runtime_path"
printf 'runtime\n' > "$weird_filename_repo/$backslash_runtime_path"
git -C "$weird_filename_repo" add .
git -C "$weird_filename_repo" commit -qm weird-runtime-paths
weird_filename_head=$(git -C "$weird_filename_repo" rev-parse HEAD)
for classification_scope in current development-backlog release-review release-review-current; do
  if classification=$(cd "$weird_filename_repo" &&
    sh "$root_dir/scripts/classify-release-change.sh" \
      "$weird_filename_base" "$weird_filename_head" "$classification_scope" 2> /dev/null); then
    [ "$classification" != review ] || {
      echo "C-quoted runtime paths were reviewable in $classification_scope scope" >&2
      exit 1
    }
  fi
done

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
release_review_job=$(workflow_job release-review)
development_review_job=$(workflow_job development-review)
build_job=$(workflow_job build)
development_job=$(workflow_job development)
production_job=$(workflow_job production-plan)
literal_dollar='$'

grep -Fxq 'permissions: {}' "$root_dir/.github/workflows/release.yml"
grep -Fq '    timeout-minutes: 10' << EOF
$authorize_job
EOF
grep -Fq '    timeout-minutes: 10' << EOF
$release_review_job
EOF
grep -Fq '    timeout-minutes: 10' << EOF
$development_review_job
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
grep -Fq '    actions: read' << EOF
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
if grep -Fq '    actions: read' << EOF
$build_job
$development_job
$production_job
EOF
then
  echo 'AWS release jobs received unnecessary Actions read authority' >&2
  exit 1
fi
grep -Fq '    environment: release-review' << EOF
$release_review_job
EOF
grep -Fq '    deployments: write' << EOF
$release_review_job
EOF
grep -Fq '    actions: read' << EOF
$release_review_job
EOF
grep -Fq '    pull-requests: read' << EOF
$release_review_job
EOF
if grep -Eq 'id-token: write|aws-actions/configure-aws-credentials|AWS_[A-Z_]+_ROLE_ARN' << EOF
$release_review_job
EOF
then
  echo 'release review job can request AWS authority' >&2
  exit 1
fi
grep -Fq '    environment: release-review' << EOF
$development_review_job
EOF
grep -Fq "needs.authorize.outputs.classification == 'development-reviewed'" << EOF
$development_review_job
EOF
grep -Fq 'run: task lambda-ci-check-current-main' << EOF
$development_review_job
EOF
if grep -Eq \
  'id-token: write|deployments: write|aws-actions/configure-aws-credentials|AWS_[A-Z_]+_ROLE_ARN' << EOF
$development_review_job
EOF
then
  echo 'protected development review can request AWS or deployment-write authority' >&2
  exit 1
fi
grep -Fq 'needs: [authorize, development-review]' << EOF
$build_job
EOF
grep -Fq '!cancelled()' << EOF
$build_job
EOF
grep -Fq "needs.authorize.result == 'success'" << EOF
$build_job
EOF
if grep -Fq '      always() &&' << EOF
$build_job
EOF
then
  echo 'privileged release build remains runnable after cancellation' >&2
  exit 1
fi
grep -Fq "needs.development-review.result == 'skipped'" << EOF
$build_job
EOF
grep -Fq "needs.development-review.result == 'success'" << EOF
$build_job
EOF
grep -Fq "needs.authorize.outputs.classification == 'development-reviewed'" << EOF
$development_job
EOF
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
grep -Fq 'protected `release-review` GitHub Environment' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'receives no AWS credentials' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'backlog cursor only' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'exact first attempt of the Release workflow' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'environment-review history' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'Failed or cancelled run markers are ignored' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq '`deploy/production-release.json` change' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq 'five exact alarm names' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
grep -Fq '`alias-before.json` separately retains' \
  "$root_dir/docs/deployment/aws-lambda-api-gateway.md"
for task_name in \
  lambda-ci-authorize-release \
  lambda-ci-record-release-review \
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
test "$(grep -Fc 'uses: go-task/setup-task@v2.2.0' "$root_dir/.github/workflows/release.yml")" -eq 6
grep -Fq 'run: task lambda-ci-authorize-release' << EOF
$authorize_job
EOF
grep -Fq 'run: task lambda-ci-record-release-review' << EOF
$release_review_job
EOF
grep -Fq "GITHUB_RUN_ATTEMPT: '{{default \"\" .GITHUB_RUN_ATTEMPT}}'" \
  "$root_dir/Taskfile.yaml"
grep -Fq "GITHUB_RUN_ID: '{{default \"\" .GITHUB_RUN_ID}}'" \
  "$root_dir/Taskfile.yaml"
test -x "$root_dir/scripts/record-ci-lambda-release-review.sh"
test -x "$root_dir/scripts/resolve-release-backlog-base.sh"
test -x "$root_dir/scripts/validate-release-review-backlog.sh"
test -x "$root_dir/scripts/validate-release-review-run.sh"
grep -Fq "GITHUB_RUN_ATTEMPT: ${literal_dollar}{{ github.run_attempt }}" << EOF
$release_review_job
EOF
grep -Fq "GITHUB_RUN_ID: ${literal_dollar}{{ github.run_id }}" << EOF
$release_review_job
EOF
grep -Fq 'fetch-depth: 0' << EOF
$release_review_job
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
printf '#!/bin/sh\nprintf "%%s\\n" "%s{ORIGIN_HOST:-}" > "%sEVIDENCE_DIR/verification-origin-host.txt"\nexit 1\n' \
  "$literal_dollar" "$literal_dollar" \
  > "$orchestration_dir/scripts/verify-lambda-release.sh"
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
grep -Fxq origin.example.invalid \
  "$orchestration_workspace/evidence/verification-origin-host.txt" || {
  echo 'development orchestration did not bind verification to the API Gateway origin' >&2
  exit 1
}

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
jq -e '.rollback_version == "6"' \
  "$retry_workspace/evidence/github-deployment.json" > /dev/null || {
  echo 'converged retry advertised a rollback version other than its retained plan target' >&2
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
