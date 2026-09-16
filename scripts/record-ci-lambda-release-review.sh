#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
: "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)

fail() {
  printf 'Lambda release review recorder failed: %s\n' "$1" >&2
  exit 1
}

printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' ||
  fail 'SOURCE_SHA must be a full lowercase commit SHA'
[ "$(git rev-parse HEAD)" = "$SOURCE_SHA" ] ||
  fail 'checked-out commit does not match SOURCE_SHA'
sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"

reviewed_base_sha=$(sh "$script_dir/resolve-reviewed-release-base.sh" "$SOURCE_SHA")
git cat-file -e "$reviewed_base_sha^{commit}" 2> /dev/null ||
  fail 'reviewed pull-request base is unavailable'
git merge-base --is-ancestor "$reviewed_base_sha" "$SOURCE_SHA" ||
  fail 'reviewed pull-request base is not an ancestor of SOURCE_SHA'
current_classification=$(sh "$script_dir/classify-release-change.sh" \
  "$reviewed_base_sha" "$SOURCE_SHA")
[ "$current_classification" = review ] ||
  fail 'SOURCE_SHA is not a review-class pull request'
current_checkpoint_classification=$(sh "$script_dir/classify-release-change.sh" \
  "$reviewed_base_sha" "$SOURCE_SHA" release-review-current)
[ "$current_checkpoint_classification" = review ] ||
  fail 'release review cannot checkpoint a current runtime or promotion change'

development_base_sha=$(sh "$script_dir/resolve-development-release-base.sh" "$SOURCE_SHA")
if ! sh "$script_dir/validate-release-review-backlog.sh" \
  "$development_base_sha" "$reviewed_base_sha" "$SOURCE_SHA"; then
  fail 'release review cannot checkpoint this development backlog'
fi

sh "$script_dir/validate-release-review-run.sh" \
  "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "$SOURCE_SHA" recording
sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
deployment_description="Release review $development_base_sha..$SOURCE_SHA"
deployment_description="$deployment_description run $GITHUB_RUN_ID/$GITHUB_RUN_ATTEMPT"
status_description="Release reviewed $development_base_sha..$SOURCE_SHA"
status_description="$status_description run $GITHUB_RUN_ID/$GITHUB_RUN_ATTEMPT"
deployment_response=$(gh api --method POST "repos/$GITHUB_REPOSITORY/deployments" \
  -f ref="$SOURCE_SHA" \
  -f task=portfolio-lambda-release-review \
  -f environment=release-review \
  -F auto_merge=false \
  -f description="$deployment_description")
deployment_id=$(printf '%s\n' "$deployment_response" | jq -er \
  --arg source_sha "$SOURCE_SHA" \
  --arg description "$deployment_description" '
    select(
      (.id | type == "number" and . > 0 and floor == .) and
      (.created_at | type == "string" and test(
        "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
      )) and
      .ref == $source_sha and
      .sha == $source_sha and
      .task == "portfolio-lambda-release-review" and
      .environment == "release-review" and
      .description == $description and
      .creator.login == "github-actions[bot]" and
      .creator.type == "Bot"
    ) |
    .id
  ') || fail 'GitHub returned an untrusted release review deployment'

sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
status_response=$(gh api --method POST \
  "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses" \
  -f state=success \
  -f environment=release-review \
  -f description="$status_description")
printf '%s\n' "$status_response" | jq -e \
  --arg description "$status_description" '
    (.id | type == "number" and . > 0 and floor == .) and
    (.created_at | type == "string" and test(
      "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
    )) and
    .state == "success" and
    .environment == "release-review" and
    ((.environment_url // "") == "") and
    .description == $description and
    .creator.login == "github-actions[bot]" and
    .creator.type == "Bot"
  ' > /dev/null || fail 'GitHub returned an untrusted release review status'
