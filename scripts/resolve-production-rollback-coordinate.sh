#!/bin/sh
set -eu

: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"

fail() {
  printf 'Production rollback coordinate resolution failed: %s\n' "$1" >&2
  exit 1
}

pages=$(gh api --paginate --slurp \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/deployments?environment=production&task=portfolio-lambda-production&per_page=100")
deployments=$(printf '%s\n' "$pages" | jq -ce '
  if type == "array" and all(.[]; type == "array") then add else error("invalid pages") end |
  if all(.[];
    (.id | type == "number" and . > 0 and floor == .) and
    (.created_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")))
  then sort_by([.created_at, .id]) | reverse else error("invalid deployments") end
') || fail 'GitHub returned malformed production deployments'

for deployment_id in $(printf '%s\n' "$deployments" | jq -r '.[].id'); do
  deployment=$(printf '%s\n' "$deployments" | jq -ce --argjson id "$deployment_id" '
    [.[] | select(.id == $id)] | select(length == 1) | .[0]
  ') || fail 'production deployment identifiers are not unique'
  fields=$(printf '%s\n' "$deployment" | jq -er '
    select(
      .ref == .sha and (.sha | test("^[0-9a-f]{40}$")) and
      .task == "portfolio-lambda-production" and .environment == "production" and
      .creator.login == "github-actions[bot]" and .creator.type == "Bot"
    ) |
    (.description | capture(
      "^Lambda (?<digest>sha256:[0-9a-f]{64}) rollback-v(?<prior>[1-9][0-9]*)$"
    )) as $r |
    [.sha, $r.digest] | @tsv
  ') || fail "deployment $deployment_id does not match the trusted production schema"
  source_sha=$(printf '%s\n' "$fields" | cut -f1)
  image_digest=$(printf '%s\n' "$fields" | cut -f2)

  status_pages=$(gh api --paginate --slurp \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses?per_page=100")
  statuses=$(printf '%s\n' "$status_pages" | jq -ce '
    if type == "array" and all(.[]; type == "array") then add else error("invalid pages") end |
    if all(.[];
      (.id | type == "number" and . > 0 and floor == .) and
      (.created_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T"))) and
      ((map(.id) | unique | length) == length)
    then sort_by([.created_at, .id]) | reverse else error("invalid statuses") end
  ') || fail "deployment $deployment_id returned malformed statuses"
  [ "$(printf '%s\n' "$statuses" | jq -r '.[0].state // empty')" = success ] || continue

  version=$(printf '%s\n' "$statuses" | jq -er \
    --arg sha "$source_sha" --arg digest "$image_digest" '
    .[0] |
    (.description | capture(
      "^Verified (?<sha>[0-9a-f]{40}) (?<digest>sha256:[0-9a-f]{64}) " +
      "v(?<version>[1-9][0-9]*) public-apex=ok public-www=ok$"
    )) as $v |
    select(
      .state == "success" and .environment == "production" and
      .environment_url == "https://craigdevjohnson.com" and
      .creator.login == "github-actions[bot]" and .creator.type == "Bot" and
      $v.sha == $sha and $v.digest == $digest
    ) | $v.version
  ') || fail "deployment $deployment_id has an untrusted success status"
  printf '%s\t%s\t%s\t%s\n' "$deployment_id" "$source_sha" "$image_digest" "$version"
  exit 0
done

printf '%s\n' \
  'Production rollback coordinate resolution failed: no durable verified deployment.' \
  'Explicit bootstrap evidence is required.' >&2
exit 2
