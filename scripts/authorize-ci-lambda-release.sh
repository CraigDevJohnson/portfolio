#!/bin/sh
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

reviewed_base_sha=$(sh "$script_dir/resolve-reviewed-release-base.sh" "$EVENT_SHA")
development_base_sha=$(sh "$script_dir/resolve-development-release-base.sh" "$EVENT_SHA")
classification=$(sh "$script_dir/classify-release-change.sh" "$reviewed_base_sha" "$EVENT_SHA")

if [ "$classification" != review ]; then
  backlog_classification=$(sh "$script_dir/classify-release-change.sh" \
    "$development_base_sha" "$EVENT_SHA" development-backlog)
  case "$backlog_classification:$classification" in
    review:*) classification=review ;;
    development:skip | development:development) classification=development ;;
    development:production) classification=review ;;
    skip:skip | skip:development | skip:production) ;;
    *) fail "unexpected development backlog classification: $backlog_classification" ;;
  esac
fi

{
  printf 'source_sha=%s\n' "$EVENT_SHA"
  printf 'base_sha=%s\n' "$reviewed_base_sha"
  printf 'development_base_sha=%s\n' "$development_base_sha"
  printf 'classification=%s\n' "$classification"
} >> "$GITHUB_OUTPUT"

[ "$classification" != review ] ||
  fail 'infrastructure, workflow, deployment-authority, mixed, or unknown changes require a reviewed plan'
