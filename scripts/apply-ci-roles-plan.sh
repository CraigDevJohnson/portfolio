#!/bin/sh
set -eu

: "${PLAN_FILE:?set PLAN_FILE to the absolute reviewed saved-plan path}"
: "${PROVENANCE_FILE:?set PROVENANCE_FILE to the absolute reviewed provenance path}"
: "${APPROVED_PLAN_SHA256:?set APPROVED_PLAN_SHA256}"
: "${APPROVED_PROVENANCE_SHA256:?set APPROVED_PROVENANCE_SHA256}"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
cd "$repo_root"

fail() {
  printf 'CI role saved-plan apply failed: %s\n' "$1" >&2
  exit 1
}

case "$PLAN_FILE" in
  /*) ;;
  *) fail 'PLAN_FILE must be absolute' ;;
esac
case "$PROVENANCE_FILE" in
  /*) ;;
  *) fail 'PROVENANCE_FILE must be absolute' ;;
esac
test -f "$PLAN_FILE" || fail "PLAN_FILE does not exist: $PLAN_FILE"
test -f "$PROVENANCE_FILE" || fail "PROVENANCE_FILE does not exist: $PROVENANCE_FILE"

for variable_name in \
  TF_DATA_DIR \
  TF_WORKSPACE \
  TF_CLI_ARGS \
  TF_CLI_ARGS_init \
  TF_CLI_ARGS_plan \
  TF_CLI_ARGS_show \
  TF_CLI_ARGS_apply \
  TF_CLI_CONFIG_FILE \
  TOFU_CLI_CONFIG_FILE \
  TF_ENCRYPTION \
  TF_REATTACH_PROVIDERS \
  TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE \
  AWS_ACCESS_KEY_ID \
  AWS_SECRET_ACCESS_KEY \
  AWS_SESSION_TOKEN \
  AWS_DEFAULT_PROFILE \
  AWS_CONFIG_FILE \
  AWS_SHARED_CREDENTIALS_FILE \
  AWS_ROLE_ARN \
  AWS_ROLE_SESSION_NAME \
  AWS_WEB_IDENTITY_TOKEN_FILE \
  AWS_CONTAINER_CREDENTIALS_FULL_URI \
  AWS_CONTAINER_CREDENTIALS_RELATIVE_URI \
  AWS_EC2_METADATA_SERVICE_ENDPOINT \
  AWS_ENDPOINT_URL \
  AWS_ENDPOINT_URL_IAM \
  AWS_ENDPOINT_URL_DYNAMODB \
  AWS_ENDPOINT_URL_S3 \
  AWS_ENDPOINT_URL_STS \
  AWS_IAM_ENDPOINT \
  AWS_DYNAMODB_ENDPOINT \
  AWS_S3_ENDPOINT \
  AWS_STS_ENDPOINT \
  AWS_SSE_CUSTOMER_KEY; do
  eval "variable_is_set=\${$variable_name+x}"
  [ -z "$variable_is_set" ] || fail "$variable_name must be unset"
done
[ "${AWS_PROFILE:-}" = portfolio-ci-roles-administrator ] ||
  fail 'AWS_PROFILE must be portfolio-ci-roles-administrator'
[ "${AWS_REGION:-}" = us-west-2 ] || fail 'AWS_REGION must be us-west-2'

printf '%s\n' "$APPROVED_PLAN_SHA256" | grep -Eq '^[0-9a-f]{64}$' ||
  fail 'APPROVED_PLAN_SHA256 must be the exact reviewed SHA-256 checksum'
printf '%s\n' "$APPROVED_PROVENANCE_SHA256" | grep -Eq '^[0-9a-f]{64}$' ||
  fail 'APPROVED_PROVENANCE_SHA256 must be the exact reviewed SHA-256 checksum'

umask 077
apply_data_dir=$(mktemp -d)
cleanup() {
  rm -rf "$apply_data_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
snapshot_plan="$apply_data_dir/approved.tfplan"
snapshot_provenance="$apply_data_dir/approved.provenance.json"
plan_json="$apply_data_dir/approved-plan.json"
cli_config_file="$apply_data_dir/tofurc"
: > "$cli_config_file"
cp "$PLAN_FILE" "$snapshot_plan"
cp "$PROVENANCE_FILE" "$snapshot_provenance"

actual_plan_sha256=$(shasum -a 256 "$snapshot_plan" | awk '{print $1}')
[ "$actual_plan_sha256" = "$APPROVED_PLAN_SHA256" ] ||
  fail 'PLAN_FILE checksum does not match APPROVED_PLAN_SHA256'
actual_provenance_sha256=$(shasum -a 256 "$snapshot_provenance" | awk '{print $1}')
[ "$actual_provenance_sha256" = "$APPROVED_PROVENANCE_SHA256" ] ||
  fail 'PROVENANCE_FILE checksum does not match APPROVED_PROVENANCE_SHA256'

jq -e --arg plan_sha256 "$actual_plan_sha256" '
  type == "object" and
  (keys | sort) == (["backend", "plan_sha256", "schema", "workspace"] | sort) and
  .schema == "portfolio.lambda-ci-roles-plan-provenance/v1" and
  .plan_sha256 == $plan_sha256 and
  .workspace == "default" and
  (.backend | type == "object") and
  (.backend | keys | sort) == (["bucket", "encrypt", "key", "region", "type", "use_lockfile"] | sort) and
  .backend.type == "s3" and
  .backend.bucket == "portfolio-tofu-state-180294223248" and
  .backend.key == "portfolio-lambda-http-api/ci-roles/terraform.tfstate" and
  .backend.region == "us-west-2" and
  .backend.encrypt == true and
  .backend.use_lockfile == true
' "$snapshot_provenance" > /dev/null ||
  fail 'provenance does not bind the plan to the reviewed backend and workspace'

TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$apply_data_dir" \
  tofu -chdir=infra/lambda/ci-roles init \
  -backend=false -lockfile=readonly -input=false
TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$apply_data_dir" \
  tofu -chdir=infra/lambda/ci-roles show -json "$snapshot_plan" > "$plan_json"
PLAN_JSON="$plan_json" sh "$script_dir/check-ci-roles-plan.sh"
TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$apply_data_dir" \
  tofu -chdir=infra/lambda/ci-roles apply \
  -lock-timeout=5m -input=false "$snapshot_plan"
