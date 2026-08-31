#!/bin/sh
set -eu

manifest=${1:?usage: validate-production-release.sh MANIFEST}
: "${ECR_REPOSITORY:?set ECR_REPOSITORY}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"

source_sha=$(jq -er '.source_sha | select(type == "string" and test("^[0-9a-f]{40}$"))' "$manifest")
image_digest=$(jq -er '.image_digest | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))' "$manifest")
deployment_id=$(jq -er '.development_deployment_id | select(type == "number" and . > 0 and floor == .)' "$manifest")

deployment=$(gh api "repos/$GITHUB_REPOSITORY/deployments/$deployment_id")
printf '%s\n' "$deployment" | jq -e \
  --arg source_sha "$source_sha" \
  --arg image_digest "$image_digest" '
    (.description | capture(
      "^Lambda (?<digest>sha256:[0-9a-f]{64}) rollback-v(?<rollback>[1-9][0-9]*)$"
    )) as $release |
    (.id | type == "number") and
    .ref == $source_sha and
    .sha == $source_sha and
    .task == "portfolio-lambda-development" and
    .environment == "development" and
    $release.digest == $image_digest and
    .creator.login == "github-actions[bot]" and
    .creator.type == "Bot"
  ' > /dev/null

status_pages=$(gh api --paginate --slurp \
  "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses?per_page=100")
statuses=$(printf '%s\n' "$status_pages" | jq -ce '
  if type == "array" and all(.[]; type == "array")
  then add
  else error("invalid deployment status pages")
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
  else error("invalid deployment statuses")
  end
') || {
  echo 'GitHub returned malformed development deployment statuses' >&2
  exit 1
}
printf '%s\n' "$statuses" | jq -e \
  --arg source_sha "$source_sha" \
  --arg image_digest "$image_digest" '
    type == "array" and length > 0 and
    (
      (.[0].description | capture(
        "^Verified (?<sha>[0-9a-f]{40}) (?<digest>sha256:[0-9a-f]{64}) v(?<version>[1-9][0-9]*)$"
      )) as $verified |
      .[0].state == "success" and
      .[0].environment == "development" and
      .[0].environment_url == "https://dev.craigdevjohnson.com" and
      $verified.sha == $source_sha and
      $verified.digest == $image_digest and
      .[0].creator.login == "github-actions[bot]" and
      .[0].creator.type == "Bot"
    )
  ' > /dev/null

tag_digest=$(aws ecr describe-images \
  --repository-name "$ECR_REPOSITORY" \
  --image-ids "imageTag=git-$source_sha" \
  --query 'imageDetails[0].imageDigest' \
  --output text)
test "$tag_digest" = "$image_digest" || {
  echo 'the immutable source-SHA tag does not resolve to the promoted digest' >&2
  exit 1
}
