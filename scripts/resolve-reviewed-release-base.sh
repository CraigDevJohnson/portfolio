#!/bin/sh
set -eu

source_sha=${1:?usage: resolve-reviewed-release-base.sh SOURCE_SHA}
printf '%s\n' "$source_sha" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'source SHA must be a full lowercase commit SHA' >&2
  exit 1
}

pulls=$(gh api \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/commits/$source_sha/pulls")

base_sha=$(printf '%s\n' "$pulls" | jq -er --arg source_sha "$source_sha" '
  [
    .[]
    | select(
      .state == "closed"
      and .merged_at != null
      and .base.ref == "main"
      and .merge_commit_sha == $source_sha
    )
  ]
  | select(length == 1)
  | .[0].base.sha
  | select(test("^[0-9a-f]{40}$"))
') || {
  echo 'source commit is not the unique merge commit of a reviewed pull request into main' >&2
  exit 1
}

printf '%s\n' "$base_sha"
