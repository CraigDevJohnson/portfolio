#!/bin/sh
set -eu

: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${PRIOR_VERSION:?set PRIOR_VERSION}"
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
printf '%s\n' "$PRIOR_VERSION" | grep -Eq '^[1-9][0-9]*$' || {
  echo 'PRIOR_VERSION must be a positive Lambda version' >&2
  exit 1
}
[ -z "${TF_WORKSPACE+x}" ] || {
  echo 'TF_WORKSPACE must be unset' >&2
  exit 1
}

root=infra/lambda/environments/dev
plan_name=rollback.tfplan
mkdir -p "$EVIDENCE_DIR"
for evidence_name in \
  "$plan_name" rollback.json rollback.txt rollback.sha256 rollback-policy.txt; do
  test ! -e "$EVIDENCE_DIR/$evidence_name" || {
    printf 'Refusing existing rollback evidence: %s\n' \
      "$EVIDENCE_DIR/$evidence_name" >&2
    exit 1
  }
done

umask 077
plan_data_dir=$(mktemp -d)
plan_file="$plan_data_dir/$plan_name"
plan_json="$plan_data_dir/rollback.json"
plan_text="$plan_data_dir/rollback.txt"
plan_sha256="$plan_data_dir/rollback.sha256"
plan_command_output="$plan_data_dir/plan-command.txt"
policy_output="$plan_data_dir/rollback-policy.txt"
cleanup() {
  rm -rf "$plan_data_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

printf '%s\n' \
  'Rollback plan did not pass policy validation; raw plan artifacts are withheld.' \
  > "$EVIDENCE_DIR/ROLLBACK_NOT_APPROVED"

tofu -chdir="$root" init -backend-config=backend.hcl -reconfigure -input=false
workspace=$(tofu -chdir="$root" workspace show)
[ "$workspace" = default ] || {
  printf 'Refusing non-default OpenTofu workspace: %s\n' "$workspace" >&2
  exit 1
}

if ! TF_VAR_ecr_repository_url="$ECR_URL" \
  TF_VAR_image_digest="$IMAGE_DIGEST" \
  TF_VAR_live_version_override="$PRIOR_VERSION" \
  tofu -chdir="$root" plan \
  -lock-timeout=5m \
  -input=false \
  -out="$plan_file" > "$plan_command_output" 2>&1; then
  echo 'OpenTofu rollback planning failed; raw plan output is withheld' >&2
  exit 1
fi
tofu -chdir="$root" show -json "$plan_file" > "$plan_json"
tofu -chdir="$root" show -no-color "$plan_file" > "$plan_text"
plan_digest=$(sha256sum "$plan_file" | awk '{print $1}')
printf '%s  %s\n' "$plan_digest" "$plan_name" > "$plan_sha256"

if AUTOMATED_RELEASE=rollback \
  PRIOR_VERSION="$PRIOR_VERSION" \
  PLAN_JSON="$plan_json" \
  ENVIRONMENT=dev \
  NAME_PREFIX=portfolio-lambda-dev \
  IMAGE_URI="$ECR_URL@$IMAGE_DIGEST" \
  EXPECTED_ALARM_ACTIONS_JSON='[]' \
  sh scripts/check-lambda-plan.sh > "$policy_output" 2>&1; then
  cat "$policy_output"
else
  status=$?
  printf '%s\n' \
    'Lambda rollback plan rejected by policy; detailed diagnostics are withheld.' \
    > "$EVIDENCE_DIR/rollback-policy.txt"
  cat "$EVIDENCE_DIR/rollback-policy.txt" >&2
  exit "$status"
fi

cp "$plan_file" "$EVIDENCE_DIR/$plan_name"
cp "$plan_json" "$EVIDENCE_DIR/rollback.json"
cp "$plan_text" "$EVIDENCE_DIR/rollback.txt"
cp "$plan_sha256" "$EVIDENCE_DIR/rollback.sha256"
cp "$policy_output" "$EVIDENCE_DIR/rollback-policy.txt"
if ! (cd "$EVIDENCE_DIR" && sha256sum -c rollback.sha256 > /dev/null); then
  echo 'Published rollback plan failed checksum verification' >&2
  exit 1
fi
rm -f "$EVIDENCE_DIR/ROLLBACK_NOT_APPROVED"
echo 'Rollback is saved but requires operator review and checked apply.' \
  > "$EVIDENCE_DIR/ROLLBACK_REQUIRES_APPROVAL"
