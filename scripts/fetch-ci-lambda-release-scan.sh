#!/bin/sh
set -eu

: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${DEVELOPMENT_SOURCE_SHA:?set DEVELOPMENT_SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${SCAN_FILE:?set SCAN_FILE to the destination path}"

printf '%s\n' "$DEVELOPMENT_SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'DEVELOPMENT_SOURCE_SHA must be a full lowercase commit SHA' >&2
  exit 1
}
printf '%s\n' "$IMAGE_DIGEST" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
  echo 'IMAGE_DIGEST must be a lowercase SHA-256 digest' >&2
  exit 1
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

runs_file="$tmp/runs.json"
gh api --method GET \
  "repos/$GITHUB_REPOSITORY/actions/workflows/release.yml/runs" \
  -f "head_sha=$DEVELOPMENT_SOURCE_SHA" \
  -f status=success \
  -F per_page=100 > "$runs_file"
run_id=$(jq -er --arg sha "$DEVELOPMENT_SOURCE_SHA" '
  select((.total_count | type == "number") and .total_count <= 100) |
  [.workflow_runs[] | select(
    .head_sha == $sha and .event == "workflow_run" and
    .status == "completed" and .conclusion == "success" and
    (.id | type == "number" and . > 0 and floor == .) and
    (.run_attempt | type == "number" and . > 0 and floor == .) and
    (.created_at | type == "string")
  )] |
  sort_by([.created_at, .run_attempt, .id]) | reverse |
  select(length > 0) | .[0].id
' "$runs_file") || {
  echo 'Could not resolve a trusted successful release run for scan evidence' >&2
  exit 1
}

artifacts_file="$tmp/artifacts.json"
gh api --method GET \
  "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/artifacts" \
  -F per_page=100 > "$artifacts_file"
artifact_id=$(jq -er --arg name "release-$DEVELOPMENT_SOURCE_SHA" '
  select((.total_count | type == "number") and .total_count <= 100) |
  [.artifacts[] | select(
    .name == $name and .expired == false and
    (.id | type == "number" and . > 0 and floor == .)
  )] | select(length == 1) | .[0].id
' "$artifacts_file") || {
  echo 'Release run does not contain one exact non-expired scan artifact' >&2
  exit 1
}

archive="$tmp/release-scan.zip"
gh api "repos/$GITHUB_REPOSITORY/actions/artifacts/$artifact_id/zip" > "$archive"
entries=$(unzip -Z1 "$archive") || {
  echo 'Release scan artifact is not a valid zip archive' >&2
  exit 1
}
[ "$entries" = scan.json ] || {
  echo 'Release scan artifact must contain only top-level scan.json' >&2
  exit 1
}
unzip -p "$archive" scan.json > "$tmp/scan.json"
jq -e --arg digest "$IMAGE_DIGEST" '
  .repositoryName == "portfolio-lambda-releases" and
  .imageId.imageDigest == $digest and
  .imageScanStatus.status == "COMPLETE" and
  (.imageScanFindings.findingSeverityCounts | type == "object") and
  ((.imageScanFindings.findingSeverityCounts.CRITICAL // 0) == 0)
' "$tmp/scan.json" > /dev/null || {
  echo 'Release artifact has no acceptable digest-bound ECR scan evidence' >&2
  exit 1
}
mv "$tmp/scan.json" "$SCAN_FILE"
