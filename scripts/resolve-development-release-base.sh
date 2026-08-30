#!/bin/sh
set -eu

source_sha=${1:?usage: resolve-development-release-base.sh SOURCE_SHA}
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"

fail() {
  printf 'Development release base resolution failed: %s\n' "$1" >&2
  exit 1
}

printf '%s\n' "$source_sha" | grep -Eq '^[0-9a-f]{40}$' ||
  fail 'source SHA must be a full lowercase commit SHA'
git cat-file -e "$source_sha^{commit}" 2> /dev/null || fail 'source SHA is not available as a commit'

deployment_pages=$(gh api --paginate --slurp \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/deployments?environment=development&task=portfolio-lambda-development&per_page=100")
deployments=$(printf '%s\n' "$deployment_pages" | jq -ce '
  if type == "array" and all(.[]; type == "array") then add else error("invalid deployment pages") end
') || fail 'GitHub returned malformed development deployment pages'

for deployment_id in $(printf '%s\n' "$deployments" | jq -r '.[].id'); do
  deployment=$(printf '%s\n' "$deployments" | jq -ce --argjson id "$deployment_id" '
    [.[] | select(.id == $id)] | select(length == 1) | .[0]
  ') || fail 'development deployment identifiers must be unique integers'

  deployment_fields=$(printf '%s\n' "$deployment" | jq -er '
    select(
      (.id | type == "number") and
      (.ref | type == "string" and test("^[0-9a-f]{40}$")) and
      (.sha | type == "string" and test("^[0-9a-f]{40}$")) and
      .ref == .sha and
      .task == "portfolio-lambda-development" and
      .environment == "development" and
      (.description | type == "string" and test("^Lambda sha256:[0-9a-f]{64}$")) and
      .creator.login == "github-actions[bot]" and
      .creator.type == "Bot"
    ) |
    [.sha, (.description | sub("^Lambda "; ""))] | @tsv
  ') || fail "deployment $deployment_id does not match the trusted development schema"
  deployment_sha=$(printf '%s\n' "$deployment_fields" | cut -f1)
  deployment_digest=$(printf '%s\n' "$deployment_fields" | cut -f2)

  statuses=$(gh api \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses?per_page=100")
  printf '%s\n' "$statuses" | jq -e 'type == "array"' > /dev/null ||
    fail "deployment $deployment_id returned malformed statuses"
  status_state=$(printf '%s\n' "$statuses" | jq -r '.[0].state // empty')
  [ "$status_state" = success ] || continue

  printf '%s\n' "$statuses" | jq -e \
    --arg sha "$deployment_sha" \
    --arg digest "$deployment_digest" '
      .[0] |
      .state == "success" and
      .environment == "development" and
      .environment_url == "https://dev.craigdevjohnson.com" and
      .description == ("Verified " + $sha + " at " + $digest) and
      .creator.login == "github-actions[bot]" and
      .creator.type == "Bot"
    ' > /dev/null || fail "deployment $deployment_id has an untrusted success status"

  git cat-file -e "$deployment_sha^{commit}" 2> /dev/null ||
    fail "deployment $deployment_id refers to an unavailable commit"
  git merge-base --is-ancestor "$deployment_sha" "$source_sha" ||
    fail "deployment $deployment_id is not an ancestor of the source commit"
  printf '%s\n' "$deployment_sha"
  exit 0
done

release_epochs=$(git log --first-parent --diff-filter=A --format=%H \
  "$source_sha" -- .github/workflows/release.yml)
epoch_count=$(printf '%s\n' "$release_epochs" | awk 'NF { count++ } END { print count + 0 }')
[ "$epoch_count" -eq 1 ] || fail 'expected exactly one release-workflow bootstrap epoch'
release_epoch=$(printf '%s\n' "$release_epochs" | awk 'NF { print; exit }')
git merge-base --is-ancestor "$release_epoch" "$source_sha" ||
  fail 'release-workflow bootstrap epoch is not an ancestor of the source commit'
printf '%s\n' "$release_epoch"
