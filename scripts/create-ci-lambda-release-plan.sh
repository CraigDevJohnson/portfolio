#!/bin/sh
set -eu

: "${RELEASE_ENVIRONMENT:?set RELEASE_ENVIRONMENT to development or production}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR to an absolute path}"
: "${ECR_URL:?set ECR_URL}"

case "$EVIDENCE_DIR" in
  /*) ;;
  *)
    echo 'EVIDENCE_DIR must be absolute' >&2
    exit 1
    ;;
esac
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
    root=infra/lambda/environments/dev
    plan_name=dev.tfplan
    environment=dev
    name_prefix=portfolio-lambda-dev
    expected_alarm_actions='[]'
    automated_release=true
    ;;
  production)
    root=infra/lambda/environments/prod
    plan_name=prod.tfplan
    environment=prod
    name_prefix=portfolio-lambda-prod
    expected_alarm_actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
    automated_release=false
    ;;
  *)
    echo 'RELEASE_ENVIRONMENT must be development or production' >&2
    exit 1
    ;;
esac

mkdir -p "$EVIDENCE_DIR"
for evidence_name in "$plan_name" plan.json plan.txt plan.sha256; do
  test ! -e "$EVIDENCE_DIR/$evidence_name" || {
    printf 'Refusing existing release evidence: %s\n' "$EVIDENCE_DIR/$evidence_name" >&2
    exit 1
  }
done

umask 077
plan_data_dir=$(mktemp -d)
plan_file="$plan_data_dir/$plan_name"
plan_json="$plan_data_dir/plan.json"
plan_text="$plan_data_dir/plan.txt"
plan_sha256="$plan_data_dir/plan.sha256"
plan_command_output="$plan_data_dir/plan-command.txt"
policy_output="$plan_data_dir/policy.txt"
cleanup() {
  rm -rf "$plan_data_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

printf '%s\n' \
  'Release plan did not pass policy validation; raw plan artifacts are withheld.' \
  > "$EVIDENCE_DIR/PLAN_NOT_APPROVED"

tofu -chdir="$root" init -backend-config=backend.hcl -reconfigure -input=false
workspace=$(tofu -chdir="$root" workspace show)
[ "$workspace" = default ] || {
  printf 'Refusing non-default OpenTofu workspace: %s\n' "$workspace" >&2
  exit 1
}

if ! TF_VAR_ecr_repository_url="$ECR_URL" \
  TF_VAR_image_digest="$IMAGE_DIGEST" \
  TF_VAR_alarm_action_arns="$expected_alarm_actions" \
  tofu -chdir="$root" plan \
  -lock-timeout=5m \
  -input=false \
  -out="$plan_file" > "$plan_command_output" 2>&1; then
  echo 'OpenTofu release planning failed; raw plan output is withheld' >&2
  exit 1
fi
tofu -chdir="$root" show -json "$plan_file" > "$plan_json"
tofu -chdir="$root" show -no-color "$plan_file" > "$plan_text"
plan_digest=$(sha256sum "$plan_file" | awk '{print $1}')
printf '%s  %s\n' "$plan_digest" "$plan_name" > "$plan_sha256"

if AUTOMATED_RELEASE="$automated_release" \
  PLAN_JSON="$plan_json" \
  ENVIRONMENT="$environment" \
  NAME_PREFIX="$name_prefix" \
  IMAGE_URI="$ECR_URL@$IMAGE_DIGEST" \
  EXPECTED_ALARM_ACTIONS_JSON="$expected_alarm_actions" \
  sh scripts/check-lambda-plan.sh > "$policy_output" 2>&1; then
  cat "$policy_output"
else
  status=$?
  printf '%s\n' \
    'Lambda release plan rejected by policy; detailed diagnostics are withheld.' \
    > "$EVIDENCE_DIR/policy.txt"
  cat "$EVIDENCE_DIR/policy.txt" >&2
  exit "$status"
fi

cp "$plan_file" "$EVIDENCE_DIR/$plan_name"
cp "$plan_json" "$EVIDENCE_DIR/plan.json"
cp "$plan_text" "$EVIDENCE_DIR/plan.txt"
cp "$plan_sha256" "$EVIDENCE_DIR/plan.sha256"
cp "$policy_output" "$EVIDENCE_DIR/policy.txt"
if ! (cd "$EVIDENCE_DIR" && sha256sum -c plan.sha256 > /dev/null); then
  echo 'Published release plan failed checksum verification' >&2
  exit 1
fi
rm -f "$EVIDENCE_DIR/PLAN_NOT_APPROVED"
