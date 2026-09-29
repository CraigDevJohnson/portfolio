#!/bin/sh
# Create one environment's saved release plan in EVIDENCE_DIR and check that it
# only releases the new image.
set -eu

: "${RELEASE_ENVIRONMENT:?set RELEASE_ENVIRONMENT to development or production}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
: "${ECR_URL:?set ECR_URL}"
: "${PORTFOLIO_ACCOUNT_ID:?set PORTFOLIO_ACCOUNT_ID}"

printf '%s\n' "$IMAGE_DIGEST" | grep -Eq '^sha256:[0-9a-f]{64}$' || {
  echo 'IMAGE_DIGEST must be a SHA-256 digest' >&2
  exit 1
}
[ -z "${TF_WORKSPACE+x}" ] || {
  echo 'TF_WORKSPACE must be unset' >&2
  exit 1
}

case "$RELEASE_ENVIRONMENT" in
  development)
    environment=dev
    ;;
  production)
    environment=prod
    # Production alarms notify the workloads us-west-2 alerts topic.
    export TF_VAR_alarm_action_arns="[\"arn:aws:sns:us-west-2:$PORTFOLIO_ACCOUNT_ID:alerts\"]"
    ;;
  *)
    echo 'RELEASE_ENVIRONMENT must be development or production' >&2
    exit 1
    ;;
esac
root=infra/lambda/environments/$environment
management=${EXPECTED_MANAGEMENT_JSON:-null}
ENVIRONMENT="$environment" EXPECTED_MANAGEMENT_JSON="$management" \
  sh "$(dirname "$0")/check-management-input.sh"

mkdir -p "$EVIDENCE_DIR"
plan_file="$EVIDENCE_DIR/$environment.tfplan"
test ! -e "$plan_file" || {
  printf 'Refusing existing plan: %s\n' "$plan_file" >&2
  exit 1
}

tofu -chdir="$root" init -backend-config=backend.hcl -reconfigure -input=false
workspace=$(tofu -chdir="$root" workspace show)
[ "$workspace" = default ] || {
  printf 'Refusing non-default OpenTofu workspace: %s\n' "$workspace" >&2
  exit 1
}

TF_VAR_management="$management" \
  TF_VAR_ecr_repository_url="$ECR_URL" \
  TF_VAR_image_digest="$IMAGE_DIGEST" \
  tofu -chdir="$root" plan -lock-timeout=5m -input=false -out="$plan_file"
tofu -chdir="$root" show -json "$plan_file" > "$EVIDENCE_DIR/$environment-plan.json"
tofu -chdir="$root" show -no-color "$plan_file" > "$EVIDENCE_DIR/$environment-plan.txt"

PLAN_JSON="$EVIDENCE_DIR/$environment-plan.json" \
  IMAGE_URI="$ECR_URL@$IMAGE_DIGEST" \
  sh "$(dirname "$0")/check-lambda-plan.sh"
