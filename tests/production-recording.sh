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
case "$*" in
  *'/commits/main'*)
    if [ -n "${CLOCK_AFTER_REMOTE_READ:-}" ]; then printf '%s' "$CLOCK_AFTER_REMOTE_READ" > "$CLOCK_FILE"; fi
    printf '%s\n' "${REMOTE_MAIN_SHA:-$SOURCE_SHA}"; exit 0 ;;
  *'/statuses?per_page=100'*)
    [ "${REMOTE_READ_FAIL:-false}" = false ] || exit 23
    if [ -f "$EVIDENCE_DIR/remote-statuses.json" ]; then
      cat "$EVIDENCE_DIR/remote-statuses.json"
    else
      printf '[[]]\n'
    fi
    exit 0 ;;
  *'deployments?environment=production'*)
    jq --arg newer "${NEWER_DEPLOYMENT:-false}" '
      if $newer == "true" then [[., (. + {id:92,created_at:"2026-09-28T00:00:00Z"})]] else [[.]] end
    ' "$EVIDENCE_DIR/remote-deployment.json"
    exit 0 ;;
  *'deployments/91') cat "$EVIDENCE_DIR/remote-deployment.json"; exit 0 ;;
esac
state=
description=
for arg do
  case "$arg" in state=*) state=${arg#state=} ;; esac
  case "$arg" in description=*) description=${arg#description=} ;; esac
done
if [ -n "$state" ]; then
  if [ "$state" = success ] && [ -n "${CLOCK_AFTER_FAILED_POST:-}" ]; then
    printf '%s' "$CLOCK_AFTER_FAILED_POST" > "$CLOCK_FILE"
    exit 23
  fi
  [ "${#description}" -le 140 ] || {
    echo 'deployment status description exceeds 140 characters' >&2
    exit 1
  }
  jq -nc --arg state "$state" --arg environment "${RESPONSE_ENVIRONMENT:-production}" \
    --arg description "$description" '{id:92,state:$state,environment:$environment,
      environment_url:"https://craigdevjohnson.com",description:$description,
      created_at:"2026-09-27T01:00:00Z",creator:{login:"github-actions[bot]",type:"Bot"}}' \
      > "$EVIDENCE_DIR/remote-status.json"
  jq '[[.]]' "$EVIDENCE_DIR/remote-status.json" > "$EVIDENCE_DIR/remote-statuses.json"
  if [ "${POST_LOST_RESPONSE:-false}" = true ] && [ ! -f "$EVIDENCE_DIR/lost-response" ]; then
    touch "$EVIDENCE_DIR/lost-response"
    if [ -n "${CLOCK_AFTER_LOST_POST:-}" ]; then printf '%s' "$CLOCK_AFTER_LOST_POST" > "$CLOCK_FILE"; fi
    exit 23
  fi
  cat "$EVIDENCE_DIR/remote-status.json"
else
  jq -nc --arg sha "$SOURCE_SHA" --arg digest "$IMAGE_DIGEST" --arg version "$PRIOR_VERSION" \
    --arg environment "${RESPONSE_ENVIRONMENT:-production}" --arg description "$description" '{
      id:91,ref:$sha,sha:$sha,environment:$environment,task:"portfolio-lambda-production",
      created_at:"2026-09-27T00:00:00Z",creator:{login:"github-actions[bot]",type:"Bot"},
      description:$description,
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
    }' > "$EVIDENCE_DIR/remote-deployment.json"
  cat "$EVIDENCE_DIR/remote-deployment.json"
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
export GITHUB_RUN_ID=456
create_verification() {
  for file in ci-origin-window.json browser-receipt.json public-receipt.json final-metrics.json; do
    printf '{"fixture":"%s"}\n' "$file" > "$EVIDENCE_DIR/$file"
  done
  python3 - "$root" "$EVIDENCE_DIR" <<'PYFIXTURE'
import importlib.util,json,os,sys,time
from pathlib import Path
spec=importlib.util.spec_from_file_location('observer',Path(sys.argv[1])/'scripts/observe-lambda-production.py')
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)
now=time.time(); start=now-2100
binding=dict(promotion_sha=os.environ['SOURCE_SHA'],source_sha=os.environ['DEVELOPMENT_SOURCE_SHA'],
             image_digest=os.environ['IMAGE_DIGEST'],production_deployment_id='91',lambda_version='8',base_url=p.APEX)
binding['window_id']=p.window_id(binding['promotion_sha'],'91')
window=dict(binding,schema_version=1,ci_origin_window='passed',started_at=p.utc(start),ended_at=p.utc(start+1800),
            observations=[dict(observed_at=p.utc(start+t),elapsed_seconds=t,lambda_version='8') for t in range(0,1801,30)])
public=dict(binding,schema_version=1,operator='CraigDevJohnson',collector_sha=binding['promotion_sha'],
            operator_public_window='passed',interval_seconds=30,started_at=window['started_at'],ended_at=window['ended_at'],
            observations=[dict(item,checks=p.PUBLIC_CHECKS) for item in window['observations']],
            fresh_public_read=dict(observed_at=p.utc(now),duration_seconds=1,checks=p.PUBLIC_CHECKS,binding_sha256=p.binding_digest(binding)))
for name,value in [('ci-origin-window.json',window),('public-receipt.json',public)]:
 (Path(sys.argv[2])/name).write_text(json.dumps(value))
PYFIXTURE
  jq -n --arg window_id "$(jq -r .window_id "$EVIDENCE_DIR/public-receipt.json")" \
    --arg source "$DEVELOPMENT_SOURCE_SHA" --arg promotion "$SOURCE_SHA" --arg digest "$IMAGE_DIGEST" \
    --arg public "$(sha256sum "$EVIDENCE_DIR/public-receipt.json" | cut -d' ' -f1)" \
    --arg window "$(sha256sum "$EVIDENCE_DIR/ci-origin-window.json" | cut -d' ' -f1)" \
    --arg browser "$(sha256sum "$EVIDENCE_DIR/browser-receipt.json" | cut -d' ' -f1)" \
    --arg metrics "$(sha256sum "$EVIDENCE_DIR/final-metrics.json" | cut -d' ' -f1)" '{
      schema_version:3,status:"verified",window_id:$window_id,source_sha:$source,promotion_sha:$promotion,
      image_digest:$digest,lambda_version:"8",production_deployment_id:91,
      ci_origin_window:"passed",operator_public_window:"passed",browser_contract:"passed",metric_coverage:"passed",
      operator:"CraigDevJohnson",provenance:"protected-github-operator",acceptance_run_id:"456",
      ci_origin_window_sha256:$window,public_receipt_sha256:$public,browser_receipt_sha256:$browser,final_metrics_sha256:$metrics
    }' > "$EVIDENCE_DIR/production-verification.json"
}
record() { DEPLOYMENT_STATE="$1" sh "$root/scripts/record-ci-lambda-production.sh"; }
record in_progress
for payload_field in schema_version development_source_sha release_identity_sha256 scan_sha256 planning_run_id \
  planning_run_attempt approval_id reviewer_login; do
  grep -Fq "payload[$payload_field]=" "$CALL_LOG" || {
    echo "Recorder omitted deployment payload field: $payload_field" >&2
    exit 1
  }
done
create_verification
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
grep -Fq -- '-f description=Verified v8 ci-origin=ok operator-public=ok' "$CALL_LOG"
# Both terminal updates must address the original deployment, never create another.
grep -Fq 'deployments/91/statuses' "$CALL_LOG"
[ "$(grep -c -- '--method POST .*task=portfolio-lambda-production' "$CALL_LOG")" -eq 1 ]
export EVIDENCE_DIR="$tmp/missing-origin-www"
record in_progress
create_verification
rm "$EVIDENCE_DIR/browser-receipt.json"
before=$(wc -l < "$CALL_LOG")
if record success >/dev/null 2>&1; then
  echo 'Recorder accepted success without browser evidence' >&2
  exit 1
fi
[ "$(wc -l < "$CALL_LOG")" -eq "$before" ]
export EVIDENCE_DIR="$tmp/missing-oauth"
record in_progress
create_verification
jq '.browser_contract="pending"' "$EVIDENCE_DIR/production-verification.json" > "$tmp/pending.json"
mv "$tmp/pending.json" "$EVIDENCE_DIR/production-verification.json"
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
create_verification
if (RESPONSE_ENVIRONMENT=development record success) >/dev/null 2>&1; then
  echo 'Recorder accepted a status for the wrong environment' >&2
  exit 1
fi
jq -e '.status_recorded == false' "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
echo 'Production deployment recording contracts passed'

# A first deployment records a real null predecessor, never a bootstrap rollback target.
export EVIDENCE_DIR="$tmp/first" PRIOR_VERSION=null
mkdir -p "$EVIDENCE_DIR"
jq -n --arg source "$SOURCE_SHA" --arg digest "$IMAGE_DIGEST" '{
  promotion_sha:$source,image_digest:$digest,production_deployment_id:null,prior_verified_version:null,
  bootstrap:{function_name:"portfolio-lambda-prod",alias_name:"live",alias_version:"1",
    alias_revision_id:"revision-1",
    image_uri:("180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@" + $digest)}
}' > "$EVIDENCE_DIR/release-identity.json"
export RELEASE_IDENTITY_SHA256="$(sha256sum "$EVIDENCE_DIR/release-identity.json" | awk '{print $1}')"
record in_progress
jq -e '.prior_version == null and .status == "in_progress"' "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
grep -Fq 'first-deployment' "$EVIDENCE_DIR/github-production-deployment-response.json"
create_verification
record success
jq -e '.prior_version == null and .final_version == "8" and .status == "success"' \
  "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
export EVIDENCE_DIR="$tmp/first-failure"
mkdir -p "$EVIDENCE_DIR"
cp "$tmp/first/release-identity.json" "$EVIDENCE_DIR/release-identity.json"
record in_progress
rm "$EVIDENCE_DIR/github-production-deployment.json"
record failure
jq -e '.prior_version == null and .status == "failure" and .production_deployment_id == 91' \
  "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
export EVIDENCE_DIR="$tmp/unbound-first"
if record in_progress >/dev/null 2>&1; then
  echo 'Accepted a first deployment without bound bootstrap evidence' >&2; exit 1
fi
echo 'First production recording success, failure and missing bootstrap contracts passed'

# Remote history, not a possibly stale local artifact, controls terminal writes.
export PRIOR_VERSION=7 RELEASE_IDENTITY_SHA256=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
prepare_terminal_case() {
  export EVIDENCE_DIR="$tmp/$1"
  record in_progress
  create_verification
}
post_count() { grep -c -- '--method POST .*deployments/91/statuses' "$CALL_LOG"; }
prepare_terminal_case terminal-success
record success
before=$(post_count)
record success
[ "$(post_count)" -eq "$before" ]
if record failure > "$tmp/late-failure.out" 2>&1; then
  echo 'Late failure overwrote terminal success' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]
jq -e '.status == "success" and .status_recorded' "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
jq -e '.[0][0].state == "success"' "$EVIDENCE_DIR/remote-statuses.json" >/dev/null
prepare_terminal_case terminal-failure
record failure
before=$(post_count)
record failure
[ "$(post_count)" -eq "$before" ]
if record success > "$tmp/late-success.out" 2>&1; then
  echo 'Late success overwrote terminal failure' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]

prepare_terminal_case stale-success
before=$(post_count)
if (NEWER_DEPLOYMENT=true record success) > "$tmp/stale-success.out" 2>&1; then
  echo 'Older attempt succeeded after a newer production attempt existed' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]
if (REMOTE_MAIN_SHA=ffffffffffffffffffffffffffffffffffffffff record success) > "$tmp/stale-main.out" 2>&1; then
  echo 'Production success accepted stale main' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]

prepare_terminal_case contradictory-coordinate
jq '.payload.approval_id = "different-review"' "$EVIDENCE_DIR/remote-deployment.json" > "$tmp/changed-remote"
mv "$tmp/changed-remote" "$EVIDENCE_DIR/remote-deployment.json"
before=$(post_count)
if record success > "$tmp/contradictory-coordinate.out" 2>&1; then
  echo 'Production success accepted contradictory remote coordinates' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]

prepare_terminal_case untrusted-status
jq '.[0][0].creator = {login:"someone",type:"User"}' "$EVIDENCE_DIR/remote-statuses.json" > "$tmp/changed-status"
mv "$tmp/changed-status" "$EVIDENCE_DIR/remote-statuses.json"
before=$(post_count)
if record failure > "$tmp/untrusted-status.out" 2>&1; then
  echo 'Terminal update accepted an untrusted latest status' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]

prepare_terminal_case remote-error
before=$(post_count)
if (REMOTE_READ_FAIL=true record failure) > "$tmp/remote-error.out" 2>&1; then
  echo 'Terminal update ignored unreadable remote status' >&2; exit 1
fi
[ "$(post_count)" -eq "$before" ]

prepare_terminal_case lost-response
before=$(post_count)
POST_LOST_RESPONSE=true record success
[ "$(post_count)" -eq $((before + 1)) ]
jq -e '.status == "success" and .status_recorded' "$EVIDENCE_DIR/github-production-deployment.json" >/dev/null
grep -Fq -- '-F auto_inactive=false' "$CALL_LOG"
echo 'Remote terminal transitions reject stale or conflicting writes and retry idempotently'

# A fake clock advances only in the mocked GitHub API, proving the last local
# check occurs after remote reads and again after failed POST retry delays.
mkdir "$tmp/clock-module"
cat > "$tmp/clock-module/sitecustomize.py" <<'PYTIME'
import os,time
clock_file=os.environ.get('CLOCK_FILE')
if clock_file:
    def clock():
        with open(clock_file) as stream:
            return float(stream.read())
    time.time=clock
PYTIME
export CLOCK_FILE="$tmp/clock" PYTHONPATH="$tmp/clock-module"
printf '1800003000' > "$CLOCK_FILE"
prepare_terminal_case fresh-at-entry-stale-after-read
before=$(post_count)
if (CLOCK_AFTER_REMOTE_READ=1800003301 record success) > "$tmp/stale-read.out" 2>&1; then
  echo 'Recorder posted success after remote reads exhausted freshness' >&2; exit 1
fi
grep -q 'stale or future' "$tmp/stale-read.out"
[ "$(post_count)" -eq "$before" ]
printf '1800003000' > "$CLOCK_FILE"
prepare_terminal_case fresh-first-post-stale-retry
before=$(post_count)
if (CLOCK_AFTER_FAILED_POST=1800003301 record success) > "$tmp/stale-retry.out" 2>&1; then
  echo 'Recorder posted a retry after freshness expired' >&2; exit 1
fi
grep -q 'stale or future' "$tmp/stale-retry.out"
[ "$(post_count)" -eq "$((before + 1))" ]
printf '1800003000' > "$CLOCK_FILE"
prepare_terminal_case successful-post-lost-response-expired
before=$(post_count)
POST_LOST_RESPONSE=true CLOCK_AFTER_LOST_POST=1800003301 record success
[ "$(post_count)" -eq "$((before + 1))" ]
record success
[ "$(post_count)" -eq "$((before + 1))" ]
unset CLOCK_FILE PYTHONPATH
echo 'Freshness is required before each new success POST; existing success remains idempotent'
