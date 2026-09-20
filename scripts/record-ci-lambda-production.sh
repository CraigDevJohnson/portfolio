#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${DEVELOPMENT_SOURCE_SHA:?set DEVELOPMENT_SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${DEVELOPMENT_DEPLOYMENT_ID:?set DEVELOPMENT_DEPLOYMENT_ID}"
: "${PLAN_SHA256:?set PLAN_SHA256}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
: "${DEPLOYMENT_STATE:?set DEPLOYMENT_STATE to in_progress, success, or failure}"

printf '%s\n' "$SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'release SHAs must be full lowercase commit SHAs' >&2
  exit 1
}
printf '%s\n' "$DEVELOPMENT_SOURCE_SHA" | grep -Eq '^[0-9a-f]{40}$' || {
  echo 'release SHAs must be full lowercase commit SHAs' >&2
  exit 1
}
printf '%s\n' "$IMAGE_DIGEST" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
  echo 'IMAGE_DIGEST must be a lowercase SHA-256 digest' >&2
  exit 1
}
printf '%s\n' "$DEVELOPMENT_DEPLOYMENT_ID" | grep -Eq '^[1-9][0-9]*$' || {
  echo 'DEVELOPMENT_DEPLOYMENT_ID must be a positive GitHub deployment ID' >&2
  exit 1
}
printf '%s\n' "$PLAN_SHA256" | grep -Eq '^[0-9a-f]{64}$' || {
  echo 'PLAN_SHA256 must be a lowercase SHA-256 checksum' >&2
  exit 1
}

mkdir -p "$EVIDENCE_DIR"
response_file="$EVIDENCE_DIR/github-production-deployment-response.json"
evidence_file="$EVIDENCE_DIR/github-production-deployment.json"
final_version=null

case "$DEPLOYMENT_STATE" in
  in_progress)
    : "${PRIOR_VERSION:?set PRIOR_VERSION for an in-progress deployment}"
    printf '%s\n' "$PRIOR_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
      echo 'PRIOR_VERSION must be a positive Lambda version' >&2
      exit 1
    }
    test ! -e "$evidence_file" || {
      echo 'production deployment evidence already exists' >&2
      exit 1
    }
    gh api --method POST "repos/$GITHUB_REPOSITORY/deployments" \
      -f ref="$SOURCE_SHA" \
      -f task=portfolio-lambda-production \
      -f environment=production \
      -F auto_merge=false \
      -f description="Lambda $IMAGE_DIGEST rollback-v$PRIOR_VERSION" \
      > "$response_file"
    deployment_id=$(jq -er '.id | select(type == "number" and . > 0 and floor == .)' "$response_file")
    jq -e --arg sha "$SOURCE_SHA" \
      --arg description "Lambda $IMAGE_DIGEST rollback-v$PRIOR_VERSION" '
      .ref == $sha and .sha == $sha and .environment == "production" and
      .task == "portfolio-lambda-production" and .description == $description
    ' "$response_file" > /dev/null || {
      echo 'GitHub returned inconsistent production deployment coordinates' >&2
      exit 1
    }
    description="Deploying $DEVELOPMENT_SOURCE_SHA at $IMAGE_DIGEST"
    ;;
  success | failure)
    fields=$(jq -er \
      --arg source_sha "$SOURCE_SHA" \
      --arg development_source_sha "$DEVELOPMENT_SOURCE_SHA" \
      --arg image_digest "$IMAGE_DIGEST" \
      --arg plan_sha256 "$PLAN_SHA256" \
      --argjson development_deployment_id "$DEVELOPMENT_DEPLOYMENT_ID" '
      select(
        .source_sha == $source_sha and
        .development_source_sha == $development_source_sha and
        .image_digest == $image_digest and
        .plan_sha256 == $plan_sha256 and
        .development_deployment_id == $development_deployment_id and
        (.production_deployment_id | type == "number" and . > 0 and floor == .) and
        (.prior_version | type == "string" and test("^[1-9][0-9]*$"))
      ) |
      [.production_deployment_id, .prior_version] | @tsv
    ' "$evidence_file") || {
      echo 'production deployment evidence is missing or inconsistent' >&2
      exit 1
    }
    deployment_id=$(printf '%s\n' "$fields" | cut -f1)
    PRIOR_VERSION=$(printf '%s\n' "$fields" | cut -f2)
    if [ "$DEPLOYMENT_STATE" = success ]; then
      : "${LAMBDA_VERSION:?set LAMBDA_VERSION for a successful deployment}"
      printf '%s\n' "$LAMBDA_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
        echo 'LAMBDA_VERSION must be a positive Lambda version' >&2
        exit 1
      }
      verification_file="$EVIDENCE_DIR/production-verification.json"
      jq -e \
        --arg source_sha "$DEVELOPMENT_SOURCE_SHA" \
        --arg image_digest "$IMAGE_DIGEST" \
        --arg version "$LAMBDA_VERSION" '
        .source_sha == $source_sha and .image_digest == $image_digest and
        .lambda_version == $version and .origin == "verified" and
        .public_apex == "verified" and .public_www == "verified"
      ' "$verification_file" > /dev/null || {
        echo 'production verification evidence is missing or inconsistent' >&2
        exit 1
      }
      for route in origin-apex public-apex public-www; do
        test -s "$EVIDENCE_DIR/$route/verification.json" || {
          echo "production verification omitted $route evidence" >&2
          exit 1
        }
        test -s "$EVIDENCE_DIR/$route/alarms.json" || {
          echo "production verification omitted $route alarm evidence" >&2
          exit 1
        }
      done
      description="Verified $DEVELOPMENT_SOURCE_SHA $IMAGE_DIGEST v$LAMBDA_VERSION public-apex=ok public-www=ok"
      final_version=$LAMBDA_VERSION
    else
      description="Failed $DEVELOPMENT_SOURCE_SHA at $IMAGE_DIGEST"
    fi
    ;;
  *)
    echo 'DEPLOYMENT_STATE must be in_progress, success, or failure' >&2
    exit 1
    ;;
esac

tmp=$(mktemp "$EVIDENCE_DIR/.github-production-deployment.XXXXXX")
jq -n \
  --argjson production_deployment_id "$deployment_id" \
  --argjson development_deployment_id "$DEVELOPMENT_DEPLOYMENT_ID" \
  --arg source_sha "$SOURCE_SHA" \
  --arg development_source_sha "$DEVELOPMENT_SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg prior_version "$PRIOR_VERSION" \
  --arg plan_sha256 "$PLAN_SHA256" \
  --argjson final_version \
    "$(if [ "$final_version" = null ]; then printf null; else printf '"%s"' "$final_version"; fi)" \
  --arg status "$DEPLOYMENT_STATE" '{
    production_deployment_id: $production_deployment_id,
    development_deployment_id: $development_deployment_id,
    source_sha: $source_sha,
    development_source_sha: $development_source_sha,
    image_digest: $image_digest,
    prior_version: $prior_version,
    plan_sha256: $plan_sha256,
    final_version: $final_version,
    status: $status,
    status_recorded: false
  }' > "$tmp"
mv "$tmp" "$evidence_file"

status_file="$EVIDENCE_DIR/github-production-deployment-status-$DEPLOYMENT_STATE-response.json"
attempt=1
while :; do
  if gh api --method POST "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses" \
    -f state="$DEPLOYMENT_STATE" \
    -f environment=production \
    -f environment_url=https://craigdevjohnson.com \
    -f description="$description" > "$status_file"; then
    break
  fi
  [ "$attempt" -lt 3 ] || {
    echo 'GitHub production deployment status failed after three attempts' >&2
    exit 1
  }
  sleep "$attempt"
  attempt=$((attempt + 1))
done
jq -e --arg state "$DEPLOYMENT_STATE" '
  (.id | type == "number" and . > 0 and floor == .) and
  .state == $state and .environment == "production"
' "$status_file" > /dev/null
tmp=$(mktemp "$EVIDENCE_DIR/.github-production-deployment.XXXXXX")
jq '.status_recorded = true' "$evidence_file" > "$tmp"
mv "$tmp" "$evidence_file"
