#!/bin/sh
# Authorize one Release run for the current main commit and classify it.
# A manual (workflow_dispatch) run re-releases current main and always waits
# for release review.
set -eu

: "${EVENT_SHA:?set EVENT_SHA}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)

fail() {
  printf 'Lambda release authorization failed: %s\n' "$1" >&2
  exit 1
}

printf '%s\n' "$EVENT_SHA" | grep -Eq '^[0-9a-f]{40}$' ||
  fail 'EVENT_SHA must be a full lowercase commit SHA'
[ "$(git rev-parse HEAD)" = "$EVENT_SHA" ] || fail 'checked-out commit does not match EVENT_SHA'
sh "$script_dir/check-current-main.sh" "$EVENT_SHA"

if [ "${GITHUB_EVENT_NAME:-}" = workflow_dispatch ]; then
  classification=review
else
  # The first parent is the previous main commit, so this is the pushed change.
  classification=$(sh "$script_dir/classify-release-change.sh" "$EVENT_SHA^1" "$EVENT_SHA")
fi

{
  printf 'source_sha=%s\n' "$EVENT_SHA"
  printf 'classification=%s\n' "$classification"
} >> "$GITHUB_OUTPUT"
printf 'Release %s is classified as %s\n' "$EVENT_SHA" "$classification"
