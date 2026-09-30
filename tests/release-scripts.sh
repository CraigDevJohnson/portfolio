#!/bin/sh
# Offline checks for the Release workflow scripts. Fake gh, aws and tofu
# commands stand in for GitHub and AWS; nothing leaves the machine.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

mkdir -p "$test_dir/bin"
cat > "$test_dir/bin/gh" << 'CLI'
#!/bin/sh
# gh api repos/<repo>/commits/main --jq .sha
# gh api --paginate --slurp repos/<repo>/deployments?environment=development&...
# gh api repos/<repo>/deployments/<id>/statuses?...
args=$*
case "$args" in
  *"/commits/main"*) printf '%s\n' "$FAKE_MAIN_SHA" ;;
  *"/deployments?environment=development&"*)
    [ -z "${FAKE_DEPLOYMENTS_FAIL:-}" ] || exit 1
    cat "$FAKE_DEPLOYMENTS_DIR/deployments.json"
    ;;
  *"/statuses?"*)
    id=${args##*/deployments/}
    cat "$FAKE_DEPLOYMENTS_DIR/statuses.${id%%/*}.json"
    ;;
  *) exit 1 ;;
esac
CLI
cat > "$test_dir/bin/aws" << 'CLI'
#!/bin/sh
case "$*" in
  *"--query Arn"*) printf '%s\n' "$FAKE_CALLER_ARN" ;;
  *"--query Account"*) printf '%s\n' "$FAKE_CALLER_ACCOUNT" ;;
  *) exit 1 ;;
esac
CLI
cat > "$test_dir/bin/tofu" << 'CLI'
#!/bin/sh
printf 'tofu %s\n' "$*" >> "$FAKE_TOFU_LOG"
CLI
chmod +x "$test_dir/bin/gh" "$test_dir/bin/aws" "$test_dir/bin/tofu"
PATH="$test_dir/bin:$PATH"
export PATH GITHUB_REPOSITORY=CraigDevJohnson/portfolio FAKE_TOFU_LOG="$test_dir/tofu.log"

# --- classify-release-change.sh and authorize-ci-lambda-release.sh ---------
repo="$test_dir/repo"
git init -q "$repo"
git -C "$repo" config user.email test@example.com
git -C "$repo" config user.name test
commit() {
  for path in "$@"; do
    mkdir -p "$repo/$(dirname "$path")"
    printf '%s\n' "$path" >> "$repo/$path"
  done
  git -C "$repo" add -A
  git -C "$repo" commit -q -m change
  git -C "$repo" rev-parse HEAD
}
classify_last() {
  (cd "$repo" && sh "$root_dir/scripts/classify-release-change.sh" HEAD^ HEAD)
}
expect_class() {
  expected=$1
  shift
  commit "$@" > /dev/null
  actual=$(classify_last)
  [ "$actual" = "$expected" ] || fail "classify $* = $actual, want $expected"
}

commit README.md > /dev/null
expect_class skip docs/runbook.md README.md
expect_class skip tests/release-scripts.sh internal/app/app_test.go .claude/settings.json
expect_class release internal/app/app.go
expect_class release go.mod
expect_class review infra/lambda/ci-roles/main.tf
expect_class review .github/workflows/release.yml
expect_class review Taskfile.yaml
expect_class review scripts/check-lambda-plan.sh internal/app/app.go
expect_class review Dockerfile.lambda

authorize() {
  (cd "$repo" && EVENT_SHA=$1 GITHUB_OUTPUT="$test_dir/output" \
    sh "$root_dir/scripts/authorize-ci-lambda-release.sh" > /dev/null)
}
# Write the development deployment history. Each argument is SHA:STATE, oldest
# first. Like GitHub, the fake lists deployments and statuses newest first, and
# --paginate --slurp wraps the pages in an array.
export FAKE_DEPLOYMENTS_DIR="$test_dir/deployments"
deployments() {
  rm -rf "$FAKE_DEPLOYMENTS_DIR"
  mkdir -p "$FAKE_DEPLOYMENTS_DIR"
  n=0
  list='[]'
  for entry in "$@"; do
    n=$((n + 1))
    list=$(printf '%s\n' "$list" | jq --argjson id "$n" --arg sha "${entry%%:*}" \
      '[{id: $id, sha: $sha, created_at: "2026-09-\(10 + $id)T00:00:00Z"}] + .')
    jq -n --argjson id "$n" --arg state "${entry#*:}" '[
      {id: ($id * 10 + 1), state: $state, created_at: "2026-09-\(10 + $id)T00:02:00Z"},
      {id: ($id * 10), state: "in_progress", created_at: "2026-09-\(10 + $id)T00:01:00Z"}]' \
      > "$FAKE_DEPLOYMENTS_DIR/statuses.$n.json"
  done
  printf '%s\n' "$list" | jq '[.]' > "$FAKE_DEPLOYMENTS_DIR/deployments.json"
}
# expect_release CLASSIFICATION EVENT_SHA MESSAGE: a push of EVENT_SHA to main.
expect_release() {
  : > "$test_dir/output"
  FAKE_MAIN_SHA=$2 GITHUB_EVENT_NAME=workflow_run authorize "$2"
  grep -Fxq "classification=$1" "$test_dir/output" || fail "$3"
}

released_sha=$(git -C "$repo" rev-parse HEAD)
head_sha=$(commit internal/app/server.go)
deployments "$released_sha:success"
expect_release release "$head_sha" 'push of app code must release'
grep -Fxq "source_sha=$head_sha" "$test_dir/output" || fail 'authorization must emit the source SHA'

deployments "$released_sha:success" "$head_sha:failure"
expect_release release "$head_sha" 'a failed deployment must not move the release base'

deployments
expect_release review "$head_sha" 'a push with no verified development release must wait for review'

deployments "$(printf '%040d' 0 | tr 0 a):success"
expect_release review "$head_sha" 'a release base outside this history must wait for review'

deployments "$released_sha:success"
export FAKE_DEPLOYMENTS_FAIL=1
expect_release review "$head_sha" 'an unreadable deployment history must wait for review'
unset FAKE_DEPLOYMENTS_FAIL

: > "$test_dir/output"
FAKE_MAIN_SHA=$head_sha GITHUB_EVENT_NAME=workflow_dispatch authorize "$head_sha"
grep -Fxq "classification=review" "$test_dir/output" || fail 'manual runs must wait for release review'

if FAKE_MAIN_SHA=0000000000000000000000000000000000000000 GITHUB_EVENT_NAME=workflow_run \
  authorize "$head_sha" 2> /dev/null; then
  fail 'a stale main commit must not be authorized'
fi

# Backlog: a tooling change whose release never reached development must still
# be reviewed when a later application change ships it. A multi-commit push is
# the same case.
tooling_sha=$(commit scripts/check-lambda-plan.sh)
app_sha=$(commit internal/app/app.go)
deployments "$released_sha:inactive" "$head_sha:success" "$tooling_sha:failure"
expect_release review "$app_sha" 'an unreviewed tooling change must not ship with a later app change'
deployments "$released_sha:inactive" "$head_sha:success"
expect_release review "$app_sha" 'a multi-commit push with a tooling change must wait for review'
deployments "$released_sha:inactive" "$head_sha:inactive" "$tooling_sha:success"
expect_release release "$app_sha" 'a verified tooling release must not hold back later app changes'

# --- check-ci-aws-identity.sh ---------------------------------------------
identity() {
  PORTFOLIO_ACCOUNT_ID=111122223333 sh "$root_dir/scripts/check-ci-aws-identity.sh" "$1" 2> /dev/null
}
export FAKE_CALLER_ACCOUNT=111122223333
export FAKE_CALLER_ARN=arn:aws:sts::111122223333:assumed-role/portfolio-release-builder-ci/GitHubActions
identity arn:aws:iam::111122223333:role/portfolio-release-builder-ci || fail 'expected CI role rejected'
if identity arn:aws:iam::111122223333:role/portfolio-development-deployer-ci; then
  fail 'a different CI role must be rejected'
fi
if identity arn:aws:iam::999999999999:role/portfolio-release-builder-ci; then
  fail 'a role in another account must be rejected'
fi
FAKE_CALLER_ACCOUNT=999999999999
if identity arn:aws:iam::111122223333:role/portfolio-release-builder-ci; then
  fail 'credentials from another account must be rejected'
fi

# --- check-lambda-plan.sh -------------------------------------------------
image=111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
old_image=111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
release_plan=$(jq -n --arg image "$image" --arg old "$old_image" '{
  resource_changes: [
    {address: "module.service.aws_lambda_function.app", mode: "managed",
      change: {actions: ["update"],
        before: {image_uri: $old, memory_size: 512, version: "3"},
        after: {image_uri: $image, memory_size: 512},
        after_unknown: {version: true}}},
    {address: "module.service.aws_lambda_alias.live", mode: "managed",
      change: {actions: ["update"], before: {function_version: "3", name: "live"},
        after: {name: "live"}, after_unknown: {function_version: true}}},
    {address: "module.service.aws_dynamodb_table.google_connections", mode: "managed",
      change: {actions: ["no-op"], before: {name: "t"}, after: {name: "t"}, after_unknown: {}}},
    {address: "module.service.data.aws_caller_identity.current", mode: "data",
      change: {actions: ["read"], before: null, after: {}, after_unknown: {}}}
  ]}')
check_plan() {
  printf '%s\n' "$1" > "$test_dir/plan.json"
  PLAN_JSON="$test_dir/plan.json" IMAGE_URI="$image" \
    sh "$root_dir/scripts/check-lambda-plan.sh" > /dev/null 2>&1
}
reject_plan() {
  if check_plan "$(printf '%s\n' "$release_plan" | jq "$1")"; then
    fail "plan checker accepted: $2"
  fi
}
check_plan "$release_plan" || fail 'a pure image release must be accepted'
check_plan "$(printf '%s\n' "$release_plan" | jq --arg image "$image" '
  .resource_changes[0].change.actions = ["no-op"] |
  .resource_changes[0].change.before.image_uri = $image |
  .resource_changes[1].change.actions = ["no-op"]')" || fail 'a converged no-op release must be accepted'
reject_plan '.resource_changes[0].change.after.image_uri = "other"' 'a different image'
reject_plan '.resource_changes[0].change.after.memory_size = 1024' 'another function attribute'
reject_plan '.resource_changes[2].change.actions = ["update"] | .resource_changes[2].change.after.name = "u"' \
  'an update to another resource'
reject_plan '.resource_changes[2].change.actions = ["delete", "create"]' 'a replacement'
reject_plan '.resource_changes[2].change.actions = ["delete"]' 'a delete'
reject_plan '.resource_changes += [{address: "module.service.aws_iam_role.x", mode: "managed",
  change: {actions: ["create"], before: null, after: {}, after_unknown: {}}}]' 'a create'
reject_plan '.resource_changes[2].change.importing = {id: "t"}' 'an import'
reject_plan '.resource_changes[2].previous_address = "module.service.aws_dynamodb_table.old"' 'a move'
reject_plan 'del(.resource_changes[0])' 'a plan without the function'
# Once the #80 history worker exists, its image follows every release (LPS
# history readiness packet 6.1 item 5): only that attribute, only to the
# release image.
worker_update=$(jq -n --arg image "$image" --arg old "$old_image" '{
  address: "module.service.aws_lambda_function.history_worker[0]", mode: "managed",
  change: {actions: ["update"],
    before: {image_uri: $old, memory_size: 512, timeout: 300, version: "2"},
    after: {image_uri: $image, memory_size: 512, timeout: 300},
    after_unknown: {version: true}}}')
worker_plan=$(printf '%s\n' "$release_plan" | jq --argjson worker "$worker_update" '.resource_changes += [$worker]')
check_plan "$worker_plan" || fail 'a release that also moves the history worker to the release image must be accepted'
if check_plan "$(printf '%s\n' "$worker_plan" | jq --arg old "$old_image" '.resource_changes[-1].change.after.image_uri = $old | .resource_changes[-1].change.before.image_uri = "other"')"; then
  fail 'plan checker accepted: a history worker image other than the release image'
fi
if check_plan "$(printf '%s\n' "$worker_plan" | jq '.resource_changes[-1].change.after.timeout = 900')"; then
  fail 'plan checker accepted: another history worker attribute'
fi

# --- apply-ci-lambda-production.sh ----------------------------------------
mkdir -p "$test_dir/workspace/evidence"
printf 'saved plan\n' > "$test_dir/workspace/evidence/prod.tfplan"
: > "$FAKE_TOFU_LOG"
if (cd "$root_dir" && FAKE_MAIN_SHA=$head_sha SOURCE_SHA=$head_sha \
  IMAGE_DIGEST=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  GITHUB_WORKSPACE="$test_dir/workspace" PLANNED_PLAN_SHA256=0000 \
  sh scripts/apply-ci-lambda-production.sh > /dev/null 2>&1); then
  fail 'production apply must refuse a plan that differs from the approved one'
fi
[ ! -s "$FAKE_TOFU_LOG" ] || fail 'production apply ran tofu before checking the plan checksum'

# --- release.yml job routing -----------------------------------------------
# release-review is skipped for app-only releases. Every job after build needs
# an explicit !cancelled() guard, or the implicit success() check skips it.
for job in development production-plan production; do
  guard=$(awk -v job="  $job:" '
    $0 == job { in_job = 1; next }
    in_job && /^  [a-z-]+:$/ { exit }
    in_job && /!cancelled\(\)/ { print "yes"; exit }
  ' "$root_dir/.github/workflows/release.yml")
  [ "$guard" = yes ] || fail "release.yml job $job lacks an explicit !cancelled() guard"
done

printf 'Release script contracts passed\n'
