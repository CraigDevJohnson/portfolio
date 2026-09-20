#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
cat > "$tmp/bin/gh" <<'CLI'
#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALL_LOG"
state=
description=
for arg do
  case "$arg" in state=*) state=${arg#state=} ;; esac
  case "$arg" in description=*) description=${arg#description=} ;; esac
done
if [ -n "$state" ]; then
  [ "${#description}" -le 140 ] || {
    echo 'deployment status description exceeds 140 characters' >&2
    exit 1
  }
  jq -nc --arg state "$state" --arg environment "${RESPONSE_ENVIRONMENT:-production}" \
    '{id:92,state:$state,environment:$environment}'
else
  jq -nc --arg sha "$SOURCE_SHA" --arg digest "$IMAGE_DIGEST" --arg version "$PRIOR_VERSION" \
    --arg environment "${RESPONSE_ENVIRONMENT:-production}" '{
      id:91,ref:$sha,sha:$sha,environment:$environment,task:"portfolio-lambda-production",
      description:("Lambda " + $digest + " rollback-v" + $version),
      payload: {
        schema_version: 1,
        development_source_sha: env.DEVELOPMENT_SOURCE_SHA,
        release_identity_sha256: env.RELEASE_IDENTITY_SHA256,
        scan_sha256: env.SCAN_SHA256,
        planning_run_id: env.PLANNING_RUN_ID,
        planning_run_attempt: env.PLANNING_RUN_ATTEMPT,
        approval_id: env.APPROVAL_ID,
        reviewer_login: env.REVIEWER_LOGIN
      }
    }'
fi
CLI
chmod +x "$tmp/bin/gh"
export PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/calls"
export SOURCE_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
export DEVELOPMENT_SOURCE_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
export IMAGE_DIGEST=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
export PLAN_SHA256=dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
export RELEASE_IDENTITY_SHA256=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
export SCAN_SHA256=ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
export PLANNING_RUN_ID=123456 PLANNING_RUN_ATTEMPT=2
export APPROVAL_ID=approval-789 REVIEWER_LOGIN=CraigDevJohnson
export DEVELOPMENT_DEPLOYMENT_ID=90 PRIOR_VERSION=7 LAMBDA_VERSION=8
export GITHUB_REPOSITORY=CraigDevJohnson/portfolio EVIDENCE_DIR="$tmp/success"
record() { DEPLOYMENT_STATE="$1" sh "$root/scripts/record-ci-lambda-production.sh"; }
record in_progress
for payload_field in schema_version development_source_sha release_identity_sha256 scan_sha256 planning_run_id \
  planning_run_attempt approval_id reviewer_login; do
  grep -Fq "payload[$payload_field]=" "$CALL_LOG" || {
    echo "Recorder omitted deployment payload field: $payload_field" >&2
    exit 1
  }
done
for route in origin-apex origin-www public-apex public-www; do
  mkdir -p "$EVIDENCE_DIR/$route"
  printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/$route/verification.json"
  printf '{"MetricAlarms":[]}\n' > "$EVIDENCE_DIR/$route/alarms.json"
done
jq -n --arg source "$DEVELOPMENT_SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '{
  source_sha:$source,image_digest:$digest,lambda_version:"8",origin:"verified",
  public_apex:"verified",public_www:"verified",oauth_redirect:"verified",
  oauth_callback:"verified",oauth_logout:"verified",secure_cookies:"verified"
}' > "$EVIDENCE_DIR/production-verification.json"
record success
jq -e '
  (keys | sort) == (["approval_id", "development_deployment_id", "development_source_sha",
    "final_version", "image_digest", "plan_sha256", "planning_run_attempt", "planning_run_id",
    "prior_version", "production_deployment_id", "release_identity_sha256", "reviewer_login",
    "scan_sha256", "schema_version", "source_sha", "status", "status_recorded"] | sort) and
  .schema_version == 1 and .production_deployment_id == 91 and .status == "success" and .status_recorded and
  .final_version == "8" and .release_identity_sha256 == env.RELEASE_IDENTITY_SHA256 and
  .scan_sha256 == env.SCAN_SHA256 and .planning_run_id == env.PLANNING_RUN_ID and
  .planning_run_attempt == env.PLANNING_RUN_ATTEMPT and .approval_id == env.APPROVAL_ID and
  .reviewer_login == env.REVIEWER_LOGIN
' \
  "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
grep -Fq -- '-f description=Verified v8 public-apex=ok public-www=ok' "$CALL_LOG"
# Both terminal updates must address the original deployment, never create another.
grep -Fq 'deployments/91/statuses' "$CALL_LOG"
[ "$(grep -c 'task=portfolio-lambda-production' "$CALL_LOG")" -eq 1 ]
export EVIDENCE_DIR="$tmp/missing-origin-www"
record in_progress
for route in origin-apex public-apex public-www; do # Intentionally omit origin-www.
  mkdir -p "$EVIDENCE_DIR/$route"
  printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/$route/verification.json"
  printf '{"MetricAlarms":[]}\n' > "$EVIDENCE_DIR/$route/alarms.json"
done
jq -n --arg source "$DEVELOPMENT_SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '{
  source_sha:$source,image_digest:$digest,lambda_version:"8",origin:"verified",
  public_apex:"verified",public_www:"verified",oauth_redirect:"verified",
  oauth_callback:"verified",oauth_logout:"verified",secure_cookies:"verified"
}' > "$EVIDENCE_DIR/production-verification.json"
before=$(wc -l < "$CALL_LOG")
if record success >/dev/null 2>&1; then
  echo 'Recorder accepted success without direct www origin evidence' >&2
  exit 1
fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
export EVIDENCE_DIR="$tmp/missing-oauth"
record in_progress
for route in origin-apex origin-www public-apex public-www; do
  mkdir -p "$EVIDENCE_DIR/$route"
  printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/$route/verification.json"
  printf '{"MetricAlarms":[]}\n' > "$EVIDENCE_DIR/$route/alarms.json"
done
jq -n --arg source "$DEVELOPMENT_SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '{
  source_sha:$source,image_digest:$digest,lambda_version:"8",origin:"verified",
  public_apex:"verified",public_www:"verified"
}' > "$EVIDENCE_DIR/production-verification.json"
before=$(wc -l < "$CALL_LOG")
if record success >/dev/null 2>&1; then
  echo 'Recorder accepted success without OAuth and cookie verification' >&2
  exit 1
fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
export EVIDENCE_DIR="$tmp/failure"
record in_progress
record failure
jq -e '.production_deployment_id == 91 and .status == "failure" and .status_recorded' \
  "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
# Recover a created deployment when interruption occurs before normalized evidence.
export EVIDENCE_DIR="$tmp/response-recovery"
record in_progress
rm "$EVIDENCE_DIR/github-production-deployment.json"
record failure
jq -e '.production_deployment_id == 91 and .prior_version == "7" and
  .status == "failure" and .status_recorded' \
  "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
# Do not recover a response whose protected provenance differs from this run.
export EVIDENCE_DIR="$tmp/response-mismatch"
record in_progress
rm "$EVIDENCE_DIR/github-production-deployment.json"
jq '.payload.approval_id = "approval-substituted"' \
  "$EVIDENCE_DIR/github-production-deployment-response.json" \
  > "$EVIDENCE_DIR/changed-response.json"
mv "$EVIDENCE_DIR/changed-response.json" \
  "$EVIDENCE_DIR/github-production-deployment-response.json"
before=$(wc -l < "$CALL_LOG")
if record failure >/dev/null 2>&1; then
  echo 'Recorder recovered a deployment from substituted response provenance' >&2
  exit 1
fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
before=$(wc -l < "$CALL_LOG")
if (IMAGE_DIGEST=invalid record success) >/dev/null 2>&1; then exit 1; fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
# Terminal recording must reject provenance that differs from the deployment evidence.
export EVIDENCE_DIR="$tmp/provenance-mismatch"
record in_progress
before=$(wc -l < "$CALL_LOG")
if (APPROVAL_ID=approval-substituted record failure) >/dev/null 2>&1; then
  echo 'Recorder accepted substituted approval provenance' >&2
  exit 1
fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
export EVIDENCE_DIR="$tmp/wrong-environment" RESPONSE_ENVIRONMENT=development
if record in_progress >/dev/null 2>&1; then
  echo 'Recorder accepted a deployment for the wrong environment' >&2
  exit 1
fi
unset RESPONSE_ENVIRONMENT
export EVIDENCE_DIR="$tmp/wrong-status"
record in_progress
for route in origin-apex origin-www public-apex public-www; do
  mkdir -p "$EVIDENCE_DIR/$route"
  printf '{"lambda_version":"8"}\n' > "$EVIDENCE_DIR/$route/verification.json"
  printf '{"MetricAlarms":[]}\n' > "$EVIDENCE_DIR/$route/alarms.json"
done
jq -n --arg source "$DEVELOPMENT_SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '{
  source_sha:$source,image_digest:$digest,lambda_version:"8",origin:"verified",
  public_apex:"verified",public_www:"verified",oauth_redirect:"verified",
  oauth_callback:"verified",oauth_logout:"verified",secure_cookies:"verified"
}' > "$EVIDENCE_DIR/production-verification.json"
if (RESPONSE_ENVIRONMENT=development record success) >/dev/null 2>&1; then
  echo 'Recorder accepted a status for the wrong environment' >&2
  exit 1
fi
jq -e '.status_recorded == false' "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
echo 'Production deployment recording contracts passed'
