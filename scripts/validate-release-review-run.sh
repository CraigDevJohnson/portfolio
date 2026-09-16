#!/bin/sh
set -eu

run_id=${1:?usage: validate-release-review-run.sh RUN_ID RUN_ATTEMPT SOURCE_SHA STATE}
run_attempt=${2:?usage: validate-release-review-run.sh RUN_ID RUN_ATTEMPT SOURCE_SHA STATE}
source_sha=${3:?usage: validate-release-review-run.sh RUN_ID RUN_ATTEMPT SOURCE_SHA STATE}
expected_state=${4:?usage: validate-release-review-run.sh RUN_ID RUN_ATTEMPT SOURCE_SHA STATE}

fail() {
  printf 'Release review run validation failed: %s\n' "$1" >&2
  exit 1
}

: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
printf '%s\n' "$run_id" | grep -Eq '^[1-9][0-9]*$' ||
  fail 'run ID must be a positive decimal integer'
[ "$run_attempt" = 1 ] ||
  fail 'release-review checkpoints only trust the first run attempt'
printf '%s\n' "$source_sha" | grep -Eq '^[0-9a-f]{40}$' ||
  fail 'source must be a full lowercase commit SHA'
case "$expected_state" in
  recording | completed) ;;
  *) fail 'expected state must be recording or completed' ;;
esac

run=$(gh api \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/attempts/$run_attempt")
printf '%s\n' "$run" | jq -e \
  --arg run_id "$run_id" \
  --arg run_attempt "$run_attempt" \
  --arg source_sha "$source_sha" \
  --arg repository "$GITHUB_REPOSITORY" \
  --arg expected_state "$expected_state" '
    (.id | type == "number" and tostring == $run_id) and
    (.run_attempt | type == "number" and tostring == $run_attempt) and
    .workflow_id == 346157322 and
    .head_sha == $source_sha and
    .head_branch == "main" and
    .path == ".github/workflows/release.yml" and
    .event == "workflow_run" and
    .repository.full_name == $repository and
    .head_repository.full_name == $repository and
    if $expected_state == "recording" then
      .status == "in_progress" and .conclusion == null
    else
      .status == "completed" and .conclusion == "success"
    end
  ' > /dev/null || fail 'the referenced run attempt is not the trusted Release workflow run'

approvals=$(gh api \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/approvals")
printf '%s\n' "$approvals" | jq -e '
  type == "array" and any(.[];
    .state == "approved" and
    .user.login == "CraigDevJohnson" and
    .user.id == 42454849 and
    .user.type == "User" and
    (.environments | type == "array") and
    any(.environments[]; .name == "release-review")
  )
' > /dev/null || fail 'Craig has not approved the release-review environment for this run'
