#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
: "${DEPLOYMENT_STATE:?set DEPLOYMENT_STATE to in_progress, success, or failure}"

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
deployment_evidence_file="$EVIDENCE_DIR/github-deployment.json"
rollback_version=

case "$DEPLOYMENT_STATE" in
  in_progress)
    test ! -e "$deployment_evidence_file" || {
      echo 'development deployment evidence already exists' >&2
      exit 1
    }
    : "${ROLLBACK_VERSION:?set ROLLBACK_VERSION for an in-progress deployment}"
    printf '%s\n' "$ROLLBACK_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
      echo 'ROLLBACK_VERSION must be a positive decimal Lambda version' >&2
      exit 1
    }
    rollback_version=$ROLLBACK_VERSION
    gh api --method POST "repos/$GITHUB_REPOSITORY/deployments" \
      -f ref="$SOURCE_SHA" \
      -f task=portfolio-lambda-development \
      -f environment=development \
      -F auto_merge=false \
      -f description="Lambda $IMAGE_DIGEST rollback-v$rollback_version" \
      > "$deployment_response_file"
    deployment_id=$(jq -er \
      '.id | select(type == "number" and . > 0 and floor == .)' \
      "$deployment_response_file")
    jq -e \
      --arg description "Lambda $IMAGE_DIGEST rollback-v$rollback_version" \
      '.description == $description' "$deployment_response_file" > /dev/null
    description="Deploying $SOURCE_SHA at $IMAGE_DIGEST"
    ;;
  success | failure)
    if [ -f "$deployment_evidence_file" ]; then
      deployment_fields=$(jq -er \
        --arg source_sha "$SOURCE_SHA" \
        --arg image_digest "$IMAGE_DIGEST" '
          select(
            .development_deployment_id > 0 and
            (.development_deployment_id | floor) == .development_deployment_id and
            .source_sha == $source_sha and
            .image_digest == $image_digest and
            (.rollback_version | type == "string" and test("^[1-9][0-9]*$"))
          ) |
          [.development_deployment_id, .rollback_version] | @tsv
        ' "$deployment_evidence_file")
      deployment_id=$(printf '%s\n' "$deployment_fields" | cut -f1)
      rollback_version=$(printf '%s\n' "$deployment_fields" | cut -f2)
    elif [ "$DEPLOYMENT_STATE" = failure ] && [ -f "$deployment_response_file" ]; then
      deployment_fields=$(jq -er --arg image_digest "$IMAGE_DIGEST" '
        (.description | capture(
          "^Lambda (?<digest>sha256:[0-9a-f]{64}) rollback-v(?<rollback>[1-9][0-9]*)$"
        )) as $deployment |
        select(
          (.id | type == "number" and . > 0 and floor == .) and
          $deployment.digest == $image_digest
        ) |
        [.id, $deployment.rollback] | @tsv
      ' "$deployment_response_file")
      deployment_id=$(printf '%s\n' "$deployment_fields" | cut -f1)
      rollback_version=$(printf '%s\n' "$deployment_fields" | cut -f2)
    else
      echo 'development deployment evidence does not exist' >&2
      exit 1
    fi
    if [ "$DEPLOYMENT_STATE" = success ]; then
      : "${LAMBDA_VERSION:?set LAMBDA_VERSION for a successful deployment}"
      printf '%s\n' "$LAMBDA_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
        echo 'LAMBDA_VERSION must be a positive decimal Lambda version' >&2
        exit 1
      }
      description="Verified $SOURCE_SHA $IMAGE_DIGEST v$LAMBDA_VERSION"
    else
      description="Failed $SOURCE_SHA at $IMAGE_DIGEST"
    fi
    ;;
  *)
    echo 'DEPLOYMENT_STATE must be in_progress, success, or failure' >&2
    exit 1
    ;;
esac

deployment_evidence_tmp=$(mktemp "$EVIDENCE_DIR/.github-deployment.XXXXXX")
jq -n \
  --argjson development_deployment_id "$deployment_id" \
  --arg source_sha "$SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg rollback_version "$rollback_version" \
  --arg status "$DEPLOYMENT_STATE" \
  '{
    development_deployment_id:$development_deployment_id,
    source_sha:$source_sha,
    image_digest:$image_digest,
    rollback_version:$rollback_version,
    status:$status,
    status_recorded:false
  }' > "$deployment_evidence_tmp"
mv "$deployment_evidence_tmp" "$deployment_evidence_file"

status_response_file="$EVIDENCE_DIR/github-deployment-status-$DEPLOYMENT_STATE-response.json"
status_attempt=1
while :; do
  if gh api --method POST "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses" \
    -f state="$DEPLOYMENT_STATE" \
    -f environment=development \
    -f environment_url=https://dev.craigdevjohnson.com \
    -f description="$description" > "$status_response_file"; then
    break
  fi
  [ "$status_attempt" -lt 3 ] || {
    echo 'GitHub deployment status failed after three attempts' >&2
    exit 1
  }
  sleep "$status_attempt"
  status_attempt=$((status_attempt + 1))
done
jq -e --arg state "$DEPLOYMENT_STATE" '
  (.id | type == "number") and .state == $state
' "$status_response_file" > /dev/null
deployment_evidence_tmp=$(mktemp "$EVIDENCE_DIR/.github-deployment.XXXXXX")
jq '.status_recorded = true' \
  "$deployment_evidence_file" > "$deployment_evidence_tmp"
mv "$deployment_evidence_tmp" "$deployment_evidence_file"
