#!/bin/sh
set -eu

source_sha=${1:?usage: resolve-release-backlog-base.sh SOURCE_SHA DEVELOPMENT_BASE_SHA}
development_base_sha=${2:?usage: resolve-release-backlog-base.sh SOURCE_SHA DEVELOPMENT_BASE_SHA}
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)

fail() {
  printf 'Release backlog base resolution failed: %s\n' "$1" >&2
  exit 1
}

for candidate_sha in "$source_sha" "$development_base_sha"; do
  printf '%s\n' "$candidate_sha" | grep -Eq '^[0-9a-f]{40}$' ||
    fail 'source and development base must be full lowercase commit SHAs'
  git cat-file -e "$candidate_sha^{commit}" 2> /dev/null ||
    fail "commit $candidate_sha is unavailable"
done
git merge-base --is-ancestor "$development_base_sha" "$source_sha" ||
  fail 'development base is not an ancestor of the source commit'

deployment_pages=$(gh api --paginate --slurp \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  "repos/$GITHUB_REPOSITORY/deployments?environment=release-review&task=portfolio-lambda-release-review&per_page=100")
deployments=$(printf '%s\n' "$deployment_pages" | jq -ce '
  if type == "array" and all(.[]; type == "array") then
    add |
    if all(.[];
      (.id | type == "number" and . > 0 and floor == .) and
      (.created_at | type == "string" and test(
        "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
      ))) and
      ((map(.id) | unique | length) == length)
    then sort_by([.created_at, .id]) | reverse
    else error("invalid release review deployment ordering fields")
    end
  else error("invalid release review deployment pages")
  end
') || fail 'GitHub returned malformed release review deployment pages'

resolved_base=$development_base_sha
for deployment_id in $(printf '%s\n' "$deployments" | jq -r '.[].id'); do
  deployment=$(printf '%s\n' "$deployments" | jq -ce --argjson id "$deployment_id" '
    [.[] | select(.id == $id)] | select(length == 1) | .[0]
  ') || fail 'release review deployment identifiers must be unique integers'
  deployment_fields=$(printf '%s\n' "$deployment" | jq -er '
    select(
      (.ref | type == "string" and test("^[0-9a-f]{40}$")) and
      (.sha | type == "string" and test("^[0-9a-f]{40}$")) and
      .ref == .sha and
      .task == "portfolio-lambda-release-review" and
      .environment == "release-review" and
      (.description | type == "string") and
      .creator.login == "github-actions[bot]" and
      .creator.type == "Bot"
    ) |
    (.description | capture(
      "^Release review (?<base>[0-9a-f]{40})\\.\\.(?<source>[0-9a-f]{40}) " +
      "run (?<run>[1-9][0-9]*)/(?<attempt>1)$"
    )) as $review |
    select($review.source == .sha) |
    [$review.base, $review.source, $review.run, $review.attempt] | @tsv
  ') || fail "deployment $deployment_id does not match the trusted release review schema"
  reviewed_development_base=$(printf '%s\n' "$deployment_fields" | cut -f1)
  reviewed_source_sha=$(printf '%s\n' "$deployment_fields" | cut -f2)
  reviewed_run_id=$(printf '%s\n' "$deployment_fields" | cut -f3)
  reviewed_run_attempt=$(printf '%s\n' "$deployment_fields" | cut -f4)

  git cat-file -e "$reviewed_source_sha^{commit}" 2> /dev/null ||
    fail "deployment $deployment_id refers to an unavailable commit"
  git merge-base --is-ancestor "$reviewed_source_sha" "$source_sha" ||
    fail "deployment $deployment_id is not an ancestor of the source commit"
  if git merge-base --is-ancestor "$reviewed_source_sha" "$development_base_sha"; then
    continue
  fi

  status_pages=$(gh api --paginate --slurp \
    -H 'Accept: application/vnd.github+json' \
    -H 'X-GitHub-Api-Version: 2022-11-28' \
    "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses?per_page=100")
  statuses=$(printf '%s\n' "$status_pages" | jq -ce '
    if type == "array" and all(.[]; type == "array")
    then add
    else error("invalid release review status pages")
    end |
    . as $statuses |
    if
      ($statuses | type) == "array" and
      all($statuses[];
        (.id | type == "number" and . > 0 and floor == .) and
        (.created_at | type == "string" and test(
          "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"
        ))) and
      (($statuses | map(.id) | unique | length) == ($statuses | length))
    then $statuses | sort_by([.created_at, .id]) | reverse
    else error("invalid release review statuses")
    end
  ') || fail "deployment $deployment_id returned malformed statuses"
  status_state=$(printf '%s\n' "$statuses" | jq -r '.[0].state // empty')
  [ "$status_state" = success ] || continue
  printf '%s\n' "$statuses" | jq -e \
    --arg base_sha "$reviewed_development_base" \
    --arg source_sha "$reviewed_source_sha" \
    --arg run_id "$reviewed_run_id" \
    --arg run_attempt "$reviewed_run_attempt" '
      .[0] |
      .state == "success" and
      .environment == "release-review" and
      ((.environment_url // "") == "") and
      .description == (
        "Release reviewed " + $base_sha + ".." + $source_sha +
        " run " + $run_id + "/" + $run_attempt
      ) and
      .creator.login == "github-actions[bot]" and
      .creator.type == "Bot"
    ' > /dev/null || fail "deployment $deployment_id has an untrusted success status"

  if ! sh "$script_dir/validate-release-review-run.sh" \
    "$reviewed_run_id" "$reviewed_run_attempt" "$reviewed_source_sha" completed; then
    continue
  fi

  git merge-base --is-ancestor "$development_base_sha" "$reviewed_source_sha" ||
    fail "deployment $deployment_id diverges from the trusted development cursor"
  [ "$reviewed_development_base" = "$development_base_sha" ] ||
    fail "deployment $deployment_id is not based on the trusted development cursor"
  reviewed_pull_base=$(sh "$script_dir/resolve-reviewed-release-base.sh" "$reviewed_source_sha")
  git cat-file -e "$reviewed_pull_base^{commit}" 2> /dev/null ||
    fail "deployment $deployment_id refers to an unavailable pull-request base"
  git merge-base --is-ancestor "$reviewed_pull_base" "$reviewed_source_sha" ||
    fail "deployment $deployment_id pull-request base is not an ancestor"
  current_classification=$(sh "$script_dir/classify-release-change.sh" \
    "$reviewed_pull_base" "$reviewed_source_sha")
  [ "$current_classification" = review ] ||
    fail "deployment $deployment_id source is not a review-class pull request"
  if ! sh "$script_dir/validate-release-review-backlog.sh" \
    "$development_base_sha" "$reviewed_pull_base" "$reviewed_source_sha"; then
    fail "deployment $deployment_id does not cover a review-class backlog"
  fi
  if [ "$resolved_base" = "$reviewed_source_sha" ]; then
    continue
  fi
  if git merge-base --is-ancestor "$resolved_base" "$reviewed_source_sha"; then
    resolved_base=$reviewed_source_sha
  elif ! git merge-base --is-ancestor "$reviewed_source_sha" "$resolved_base"; then
    fail 'trusted release review checkpoints diverge in Git history'
  fi
done

printf '%s\n' "$resolved_base"
