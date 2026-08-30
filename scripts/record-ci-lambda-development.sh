#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"

printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'SOURCE_SHA must be a full lowercase commit SHA' >&2
  exit 1
}
printf '%s\n' "$IMAGE_DIGEST" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
  echo 'IMAGE_DIGEST must be a lowercase SHA-256 digest' >&2
  exit 1
}
mkdir -p "$EVIDENCE_DIR"

deployment_response_file="$EVIDENCE_DIR/github-deployment-response.json"
gh api --method POST "repos/$GITHUB_REPOSITORY/deployments" \
  -f ref="$SOURCE_SHA" \
  -f task=portfolio-lambda-development \
  -f environment=development \
  -F auto_merge=false \
  -f description="Lambda $IMAGE_DIGEST" > "$deployment_response_file"
deployment_id=$(jq -er \
  '.id | select(type == "number" and . > 0 and floor == .)' \
  "$deployment_response_file")
jq -n \
  --argjson development_deployment_id "$deployment_id" \
  --arg source_sha "$SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  '{
    development_deployment_id:$development_deployment_id,
    source_sha:$source_sha,
    image_digest:$image_digest,
    status_recorded:false
  }' > "$EVIDENCE_DIR/github-deployment.json"

gh api --method POST "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses" \
  -f state=success \
  -f environment=development \
  -f environment_url=https://dev.craigdevjohnson.com \
  -f description="Verified $SOURCE_SHA at $IMAGE_DIGEST" \
  > "$EVIDENCE_DIR/github-deployment-status-response.json"
deployment_evidence_tmp=$(mktemp "$EVIDENCE_DIR/.github-deployment.XXXXXX")
jq '.status_recorded = true' \
  "$EVIDENCE_DIR/github-deployment.json" > "$deployment_evidence_tmp"
mv "$deployment_evidence_tmp" "$EVIDENCE_DIR/github-deployment.json"
