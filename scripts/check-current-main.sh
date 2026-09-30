#!/bin/sh
# Refuse to act on a source commit that is no longer the tip of main.
set -eu

expected_sha=${1:?usage: check-current-main.sh EXPECTED_SHA}
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
printf '%s\n' "$expected_sha" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'expected source SHA must be a full lowercase commit SHA' >&2
  exit 1
}

current_sha=$(gh api "repos/$GITHUB_REPOSITORY/commits/main" --jq .sha)
test "$current_sha" = "$expected_sha" || {
  echo 'A newer main commit exists; refusing stale release authority.' >&2
  exit 1
}
