#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA to the promotion commit}"
: "${DEVELOPMENT_SOURCE_SHA:?set DEVELOPMENT_SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${DEVELOPMENT_DEPLOYMENT_ID:?set DEVELOPMENT_DEPLOYMENT_ID}"
: "${PLAN_SHA256:?set PLAN_SHA256}"
: "${RELEASE_IDENTITY_SHA256:?set RELEASE_IDENTITY_SHA256}"
: "${SCAN_SHA256:?set SCAN_SHA256}"
: "${PLANNING_RUN_ID:?set PLANNING_RUN_ID}"
: "${PLANNING_RUN_ATTEMPT:?set PLANNING_RUN_ATTEMPT}"
: "${APPROVAL_ID:?set APPROVAL_ID}"
: "${REVIEWER_LOGIN:?set REVIEWER_LOGIN}"
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
for binding in RELEASE_IDENTITY_SHA256 SCAN_SHA256; do
  case "$binding" in
    RELEASE_IDENTITY_SHA256) value=$RELEASE_IDENTITY_SHA256 ;;
    SCAN_SHA256) value=$SCAN_SHA256 ;;
  esac
  printf '%s\n' "$value" | grep -Eq '^[0-9a-f]{64}$' || {
    echo "$binding must be a lowercase SHA-256 checksum" >&2
    exit 1
  }
done
for identifier in PLANNING_RUN_ID PLANNING_RUN_ATTEMPT; do
  case "$identifier" in
    PLANNING_RUN_ID) value=$PLANNING_RUN_ID ;;
    PLANNING_RUN_ATTEMPT) value=$PLANNING_RUN_ATTEMPT ;;
  esac
  printf '%s\n' "$value" | grep -Eq '^[1-9][0-9]*$' || {
    echo "$identifier must be a positive integer" >&2
    exit 1
  }
done
printf '%s\n' "$APPROVAL_ID" | grep -Eq '^[A-Za-z0-9._:-]+$' || {
  echo 'APPROVAL_ID contains unsupported characters' >&2
  exit 1
}
printf '%s\n' "$REVIEWER_LOGIN" | grep -Eq '^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$' || {
  echo 'REVIEWER_LOGIN must be a valid GitHub login' >&2
  exit 1
}

mkdir -p "$EVIDENCE_DIR"
response_file="$EVIDENCE_DIR/github-production-deployment-response.json"
evidence_file="$EVIDENCE_DIR/github-production-deployment.json"
final_version=null

case "$DEPLOYMENT_STATE" in
  in_progress)
    : "${PRIOR_VERSION:?set PRIOR_VERSION for an in-progress deployment}"
    if [ "$PRIOR_VERSION" = null ]; then
      identity_file="$EVIDENCE_DIR/release-identity.json"
      [ "$(sha256sum "$identity_file" | awk '{print $1}')" = "$RELEASE_IDENTITY_SHA256" ] &&
        jq -e --arg source "$SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '
          .promotion_sha == $source and .image_digest == $digest and
          .production_deployment_id == null and .prior_verified_version == null and
          (.bootstrap | type == "object" and .function_name == "portfolio-lambda-prod" and
            .alias_name == "live" and (.alias_version | type == "string" and test("^[1-9][0-9]*$")) and
            (.alias_revision_id | type == "string" and length > 0) and
            (.image_uri | type == "string" and
              startswith("180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@") and
              test("@sha256:[0-9a-f]{64}$")))
        ' "$identity_file" >/dev/null || {
        echo 'First production deployment requires bound bootstrap evidence' >&2; exit 1;
      }
      release_description="Lambda $IMAGE_DIGEST first-deployment"
    else
      printf '%s\n' "$PRIOR_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
        echo 'PRIOR_VERSION must be a positive Lambda version or null for bootstrap' >&2; exit 1;
      }
      release_description="Lambda $IMAGE_DIGEST rollback-v$PRIOR_VERSION"
    fi
    test ! -e "$evidence_file" || {
      echo 'production deployment evidence already exists' >&2
      exit 1
    }
    gh api --method POST "repos/$GITHUB_REPOSITORY/deployments" \
      -f ref="$SOURCE_SHA" \
      -f task=portfolio-lambda-production \
      -f environment=production \
      -F auto_merge=false \
      -f description="$release_description" \
      -F 'payload[schema_version]=1' \
      -f "payload[development_source_sha]=$DEVELOPMENT_SOURCE_SHA" \
      -f "payload[release_identity_sha256]=$RELEASE_IDENTITY_SHA256" \
      -f "payload[scan_sha256]=$SCAN_SHA256" \
      -f "payload[planning_run_id]=$PLANNING_RUN_ID" \
      -f "payload[planning_run_attempt]=$PLANNING_RUN_ATTEMPT" \
      -f "payload[approval_id]=$APPROVAL_ID" \
      -f "payload[reviewer_login]=$REVIEWER_LOGIN" \
      > "$response_file"
    deployment_id=$(jq -er '.id | select(type == "number" and . > 0 and floor == .)' "$response_file")
    jq -e --arg sha "$SOURCE_SHA" \
      --arg description "$release_description" '
      .ref == $sha and .sha == $sha and .environment == "production" and
      .task == "portfolio-lambda-production" and .description == $description and
      (.payload | keys | sort) == (["approval_id", "development_source_sha", "planning_run_attempt",
        "planning_run_id", "release_identity_sha256", "reviewer_login", "scan_sha256",
        "schema_version"] | sort) and
      .payload.schema_version == 1 and
      .payload.development_source_sha == env.DEVELOPMENT_SOURCE_SHA and
      .payload.release_identity_sha256 == env.RELEASE_IDENTITY_SHA256 and
      .payload.scan_sha256 == env.SCAN_SHA256 and
      .payload.planning_run_id == env.PLANNING_RUN_ID and
      .payload.planning_run_attempt == env.PLANNING_RUN_ATTEMPT and
      .payload.approval_id == env.APPROVAL_ID and
      .payload.reviewer_login == env.REVIEWER_LOGIN
    ' "$response_file" > /dev/null || {
      echo 'GitHub returned inconsistent production deployment coordinates' >&2
      exit 1
    }
    description="Deploying $DEVELOPMENT_SOURCE_SHA at $IMAGE_DIGEST"
    ;;
  success | failure)
    if [ -f "$evidence_file" ]; then
      fields=$(jq -er \
        --arg source_sha "$SOURCE_SHA" \
        --arg development_source_sha "$DEVELOPMENT_SOURCE_SHA" \
        --arg image_digest "$IMAGE_DIGEST" \
        --arg plan_sha256 "$PLAN_SHA256" \
        --argjson development_deployment_id "$DEVELOPMENT_DEPLOYMENT_ID" '
        select(
          (keys | sort) == (["approval_id", "development_deployment_id", "development_source_sha",
            "final_version", "image_digest", "plan_sha256", "planning_run_attempt", "planning_run_id",
            "prior_version", "production_deployment_id", "release_identity_sha256", "reviewer_login",
            "scan_sha256", "schema_version", "source_sha", "status", "status_recorded"] | sort) and
          .schema_version == 1 and
          .source_sha == $source_sha and
          .development_source_sha == $development_source_sha and
          .image_digest == $image_digest and
          .plan_sha256 == $plan_sha256 and
          .release_identity_sha256 == env.RELEASE_IDENTITY_SHA256 and
          .scan_sha256 == env.SCAN_SHA256 and
          .planning_run_id == env.PLANNING_RUN_ID and
          .planning_run_attempt == env.PLANNING_RUN_ATTEMPT and
          .approval_id == env.APPROVAL_ID and
          .reviewer_login == env.REVIEWER_LOGIN and
          .development_deployment_id == $development_deployment_id and
          (.production_deployment_id | type == "number" and . > 0 and floor == .) and
          (.prior_version == null or (.prior_version | type == "string" and test("^[1-9][0-9]*$")))
        ) |
        [.production_deployment_id, (.prior_version // "null")] | @tsv
      ' "$evidence_file") || {
        echo 'production deployment evidence is missing or inconsistent' >&2
        exit 1
      }
    elif [ "$DEPLOYMENT_STATE" = failure ] && [ -f "$response_file" ]; then
      fields=$(jq -er \
        --arg source_sha "$SOURCE_SHA" \
        --arg image_digest "$IMAGE_DIGEST" '
        (.description | capture(
          "^Lambda (?<digest>sha256:[0-9a-f]{64}) (?:rollback-v(?<prior>[1-9][0-9]*)|first-deployment)$"
        )) as $release |
        select(
          (.id | type == "number" and . > 0 and floor == .) and
          .ref == $source_sha and .sha == $source_sha and
          .environment == "production" and .task == "portfolio-lambda-production" and
          $release.digest == $image_digest and
          (.payload | keys | sort) == (["approval_id", "development_source_sha", "planning_run_attempt",
            "planning_run_id", "release_identity_sha256", "reviewer_login", "scan_sha256",
            "schema_version"] | sort) and
          .payload.schema_version == 1 and
          .payload.development_source_sha == env.DEVELOPMENT_SOURCE_SHA and
          .payload.release_identity_sha256 == env.RELEASE_IDENTITY_SHA256 and
          .payload.scan_sha256 == env.SCAN_SHA256 and
          .payload.planning_run_id == env.PLANNING_RUN_ID and
          .payload.planning_run_attempt == env.PLANNING_RUN_ATTEMPT and
          .payload.approval_id == env.APPROVAL_ID and
          .payload.reviewer_login == env.REVIEWER_LOGIN
        ) |
        [.id, ($release.prior // "null")] | @tsv
      ' "$response_file") || {
        echo 'production deployment response is missing or inconsistent' >&2
        exit 1
      }
    else
      echo 'production deployment evidence does not exist' >&2
      exit 1
    fi
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
        --arg promotion_sha "$SOURCE_SHA" \
        --argjson deployment_id "$deployment_id" \
        --arg acceptance_run "${GITHUB_RUN_ID:-}" \
        --arg image_digest "$IMAGE_DIGEST" \
        --arg version "$LAMBDA_VERSION" '
        .schema_version == 3 and .status == "verified" and
        .source_sha == $source_sha and .image_digest == $image_digest and
        .promotion_sha == $promotion_sha and .production_deployment_id == $deployment_id and
        .lambda_version == $version and .ci_origin_window == "passed" and
        .operator_public_window == "passed" and
        .browser_contract == "passed" and .metric_coverage == "passed" and
        .operator == "CraigDevJohnson" and .provenance == "protected-github-operator" and
        .acceptance_run_id == $acceptance_run and ($acceptance_run | test("^[1-9][0-9]*$"))
      ' "$verification_file" > /dev/null || {
        echo 'production verification evidence is missing or inconsistent' >&2
        exit 1
      }
      for binding in ci-origin-window.json:ci_origin_window_sha256 \
        browser-receipt.json:browser_receipt_sha256 public-receipt.json:public_receipt_sha256 \
        final-metrics.json:final_metrics_sha256; do
        file=${binding%:*}; field=${binding#*:}
        test -s "$EVIDENCE_DIR/$file" &&
          test "$(sha256sum "$EVIDENCE_DIR/$file" | awk '{print $1}')" = \
          "$(jq -er ".$field" "$verification_file")" || {
          echo "production verification omitted or changed $file" >&2; exit 1;
        }
      done
      description="Verified v$LAMBDA_VERSION ci-origin=ok operator-public=ok"
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

# The shared production workflow concurrency group must cover apply and
# finalization. GitHub's status API has no conditional-write operation; these
# authoritative checks are repeated before every POST, including retries.
status_file="$EVIDENCE_DIR/github-production-deployment-status-$DEPLOYMENT_STATE-response.json"
check_remote_transition() {
  remote_same=false
  remote_deployment=$(gh api "repos/$GITHUB_REPOSITORY/deployments/$deployment_id")
  if [ "$PRIOR_VERSION" = null ]; then
    expected_description="Lambda $IMAGE_DIGEST first-deployment"
  else
    expected_description="Lambda $IMAGE_DIGEST rollback-v$PRIOR_VERSION"
  fi
  printf '%s\n' "$remote_deployment" | jq -e \
    --argjson id "$deployment_id" --arg source "$SOURCE_SHA" --arg description "$expected_description" '
    .id == $id and .sha == $source and .ref == $source and
    .environment == "production" and .task == "portfolio-lambda-production" and
    .description == $description and
    (.created_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")) and
    .creator.login == "github-actions[bot]" and .creator.type == "Bot" and
    .payload == {
      schema_version:1, development_source_sha:env.DEVELOPMENT_SOURCE_SHA,
      release_identity_sha256:env.RELEASE_IDENTITY_SHA256, scan_sha256:env.SCAN_SHA256,
      planning_run_id:env.PLANNING_RUN_ID, planning_run_attempt:env.PLANNING_RUN_ATTEMPT,
      approval_id:env.APPROVAL_ID, reviewer_login:env.REVIEWER_LOGIN
    }
  ' >/dev/null || {
    echo 'Remote production deployment identity differs from this release' >&2; return 1;
  }
  pages=$(gh api --paginate --slurp \
    "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses?per_page=100")
  statuses=$(printf '%s\n' "$pages" | jq -ce '
    if type == "array" and length > 0 and all(.[]; type == "array") then add else error("invalid pages") end |
    if all(.[]; (.id | type == "number" and . > 0 and floor == .) and
      (.created_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T"))) and
      ((map(.id) | unique | length) == length)
    then sort_by([.created_at,.id]) | reverse else error("invalid statuses") end
  ') || { echo 'Remote production statuses are malformed' >&2; return 1; }
  latest=$(printf '%s\n' "$statuses" | jq -c '.[0] // null')
  if [ "$latest" != null ]; then
    printf '%s\n' "$latest" | jq -e '
      .environment == "production" and .environment_url == "https://craigdevjohnson.com" and
      .creator.login == "github-actions[bot]" and .creator.type == "Bot"
    ' >/dev/null || { echo 'Remote production status is not trusted' >&2; return 1; }
    if printf '%s\n' "$latest" | jq -e --arg state "$DEPLOYMENT_STATE" --arg description "$description" \
      '.state == $state and .description == $description' >/dev/null; then
      printf '%s\n' "$latest" > "$status_file"
      remote_same=true
      return 0
    fi
    printf '%s\n' "$latest" | jq -e '.state == "in_progress" or .state == "pending" or .state == "queued"' >/dev/null || {
      echo 'Refusing to overwrite a terminal production result' >&2; return 1;
    }
  elif [ "$DEPLOYMENT_STATE" = success ]; then
    echo 'Production success requires an existing in-progress remote status' >&2; return 1
  fi
  if [ "$DEPLOYMENT_STATE" = success ]; then
    deployments=$(gh api --paginate --slurp \
      "repos/$GITHUB_REPOSITORY/deployments?environment=production&task=portfolio-lambda-production&per_page=100")
    printf '%s\n' "$deployments" | jq -e --argjson current "$remote_deployment" '
      if type == "array" and length > 0 and all(.[]; type == "array") then add else error("invalid pages") end |
      all(.[]; (.id | type == "number" and . > 0 and floor == .) and
        (.created_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T"))) and
      ([.[] | select(.id == $current.id)] | length) == 1 and
      all(.[]; [.created_at,.id] <= [$current.created_at,$current.id])
    ' >/dev/null || {
      echo 'A newer production deployment exists or deployment history is inconsistent' >&2; return 1;
    }
    script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
    sh "$script_dir/check-current-main.sh" "$SOURCE_SHA"
  fi
}
check_remote_transition

tmp=$(mktemp "$EVIDENCE_DIR/.github-production-deployment.XXXXXX")
jq -n \
  --argjson production_deployment_id "$deployment_id" \
  --argjson development_deployment_id "$DEVELOPMENT_DEPLOYMENT_ID" \
  --arg source_sha "$SOURCE_SHA" \
  --arg development_source_sha "$DEVELOPMENT_SOURCE_SHA" \
  --arg image_digest "$IMAGE_DIGEST" \
  --arg prior_version "$PRIOR_VERSION" \
  --arg plan_sha256 "$PLAN_SHA256" \
  --arg release_identity_sha256 "$RELEASE_IDENTITY_SHA256" \
  --arg scan_sha256 "$SCAN_SHA256" \
  --arg planning_run_id "$PLANNING_RUN_ID" \
  --arg planning_run_attempt "$PLANNING_RUN_ATTEMPT" \
  --arg approval_id "$APPROVAL_ID" \
  --arg reviewer_login "$REVIEWER_LOGIN" \
  --argjson final_version \
    "$(if [ "$final_version" = null ]; then printf null; else printf '"%s"' "$final_version"; fi)" \
  --arg status "$DEPLOYMENT_STATE" '{
    schema_version: 1,
    production_deployment_id: $production_deployment_id,
    development_deployment_id: $development_deployment_id,
    source_sha: $source_sha,
    development_source_sha: $development_source_sha,
    image_digest: $image_digest,
    prior_version: (if $prior_version == "null" then null else $prior_version end),
    plan_sha256: $plan_sha256,
    release_identity_sha256: $release_identity_sha256,
    scan_sha256: $scan_sha256,
    planning_run_id: $planning_run_id,
    planning_run_attempt: $planning_run_attempt,
    approval_id: $approval_id,
    reviewer_login: $reviewer_login,
    final_version: $final_version,
    status: $status,
    status_recorded: false
  }' > "$tmp"
mv "$tmp" "$evidence_file"

attempt=1
while :; do
  check_remote_transition
  [ "$remote_same" = false ] || break
  if gh api --method POST "repos/$GITHUB_REPOSITORY/deployments/$deployment_id/statuses" \
    -f state="$DEPLOYMENT_STATE" \
    -f environment=production \
    -f environment_url=https://craigdevjohnson.com \
    -F auto_inactive=false \
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
