#!/bin/sh
# Authorize one Release run for the current main commit and classify it.
# A manual (workflow_dispatch) run re-releases current main and always waits
# for release review.
set -eu

: "${EVENT_SHA:?set EVENT_SHA}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)

fail() {
  printf 'Lambda release authorization failed: %s\n' "$1" >&2
  exit 1
}

# Print the commit of the newest development deployment whose latest status is
# success, or nothing. GitHub records a deployment for every run of the
# development job. Its commit is the released source, because that job refuses
# to run once main has moved on.
last_development_sha() {
  gh api --paginate --slurp \
    "repos/$GITHUB_REPOSITORY/deployments?environment=development&per_page=100" |
    jq -r 'add // [] | sort_by(.created_at, .id) | reverse | .[] | "\(.id) \(.sha)"' |
    while read -r id sha; do
      state=$(gh api "repos/$GITHUB_REPOSITORY/deployments/$id/statuses?per_page=100" |
        jq -r 'sort_by(.created_at, .id) | last | .state // empty')
      if [ "$state" = success ]; then
        printf '%s\n' "$sha"
        break
      fi
    done
}

printf '%s\n' "$EVENT_SHA" | grep -Eq '^[0-9a-f]{40}$' ||
  fail 'EVENT_SHA must be a full lowercase commit SHA'
[ "$(git rev-parse HEAD)" = "$EVENT_SHA" ] || fail 'checked-out commit does not match EVENT_SHA'
sh "$script_dir/check-current-main.sh" "$EVENT_SHA"

if [ "${GITHUB_EVENT_NAME:-}" = workflow_dispatch ]; then
  classification=review
else
  # Classify everything since the last release that development verified, not
  # only the pushed commit. Otherwise a tooling or infrastructure change that
  # was never approved in release-review would ship with the next application
  # change. Without such a release on this commit's history, ask for review.
  base_sha=$(last_development_sha) || base_sha=
  if printf '%s\n' "$base_sha" | grep -Eq '^[0-9a-f]{40}$' &&
    git merge-base --is-ancestor "$base_sha" "$EVENT_SHA" 2> /dev/null; then
    classification=$(sh "$script_dir/classify-release-change.sh" "$base_sha" "$EVENT_SHA")
  else
    printf 'No verified development release on this history; asking for release review\n'
    classification=review
  fi
fi

{
  printf 'source_sha=%s\n' "$EVENT_SHA"
  printf 'classification=%s\n' "$classification"
} >> "$GITHUB_OUTPUT"
printf 'Release %s is classified as %s\n' "$EVENT_SHA" "$classification"
