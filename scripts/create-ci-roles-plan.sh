#!/bin/sh
set -eu

: "${PLAN_FILE:?set PLAN_FILE to a new absolute saved-plan path}"
provenance_file="$PLAN_FILE.provenance.json"
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
cd "$repo_root"

fail() {
  printf 'CI role plan creation failed: %s\n' "$1" >&2
  exit 1
}

case "$PLAN_FILE" in
  /*) ;;
  *)
    printf 'PLAN_FILE must be absolute\n' >&2
    exit 1
    ;;
esac

test ! -e "$PLAN_FILE" || {
  printf 'Refusing existing PLAN_FILE: %s\n' "$PLAN_FILE" >&2
  exit 1
}
test ! -e "$provenance_file" || fail "refusing existing provenance file: $provenance_file"

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

umask 077
plan_data_dir=$(mktemp -d)
plan_json="$plan_data_dir/plan.json"
private_plan="$plan_data_dir/review.tfplan"
private_provenance="$plan_data_dir/review.provenance.json"
cli_config_file="$plan_data_dir/tofurc"
: > "$cli_config_file"
retain_plan=false
cleanup() {
  rm -rf "$plan_data_dir"
  if [ "$retain_plan" != true ]; then
    rm -f "$PLAN_FILE" "$provenance_file"
  fi
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$plan_data_dir" \
  tofu -chdir=infra/lambda/ci-roles init \
  -backend-config=backend.hcl -reconfigure -lockfile=readonly -input=false
workspace=$(TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$plan_data_dir" \
  tofu -chdir=infra/lambda/ci-roles workspace show)
[ "$workspace" = default ] || fail "refusing non-default OpenTofu workspace: $workspace"

backend_metadata="$plan_data_dir/terraform.tfstate"
test -f "$backend_metadata" || fail 'initialized backend metadata is missing'
read_reviewed_backend() {
  jq -ce '
  def unsetish:
    . == null or
    . == "" or
    . == false or
    . == [] or
    . == {} or
    (type == "object" and all(.[]; unsetish));
  .backend as $backend |
  select(
    $backend.type == "s3" and
    $backend.config.bucket == "portfolio-tofu-state-180294223248" and
    $backend.config.key == "portfolio-lambda-http-api/ci-roles/terraform.tfstate" and
    $backend.config.region == "us-west-2" and
    $backend.config.encrypt == true and
    $backend.config.use_lockfile == true and
    ([
      "access_key",
      "acl",
      "dynamodb_endpoint",
      "dynamodb_table",
      "kms_key_id",
      "secret_key",
      "sse_customer_key",
      "token",
      "profile",
      "shared_credentials_file",
      "shared_credentials_files",
      "shared_config_files",
      "role_arn",
      "session_name",
      "external_id",
      "assume_role",
      "assume_role_with_web_identity",
      "endpoint",
      "endpoints",
      "sts_endpoint",
      "iam_endpoint",
      "http_proxy",
      "https_proxy",
      "no_proxy",
      "custom_ca_bundle",
      "skip_credentials_validation",
      "skip_region_validation",
      "skip_requesting_account_id",
      "skip_s3_checksum",
      "use_path_style",
      "force_path_style",
      "insecure"
    ] | all(.[]; . as $key | ($backend.config[$key] | unsetish)))
  ) |
  {
    type: $backend.type,
    bucket: $backend.config.bucket,
    key: $backend.config.key,
    region: $backend.config.region,
    encrypt: $backend.config.encrypt,
    use_lockfile: $backend.config.use_lockfile
  }
' "$backend_metadata"
}
backend=$(read_reviewed_backend) ||
  fail 'initialized backend has unreviewed settings or does not match the reviewed S3 state'

TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$plan_data_dir" \
  tofu -chdir=infra/lambda/ci-roles plan \
  -lock-timeout=5m -input=false -out="$private_plan"
backend_after_plan=$(read_reviewed_backend) ||
  fail 'initialized backend metadata became unreviewed or unreadable'
[ "$backend_after_plan" = "$backend" ] || fail 'initialized backend changed while creating the plan'

TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$plan_data_dir" \
  tofu -chdir=infra/lambda/ci-roles show -json "$private_plan" > "$plan_json"
PLAN_JSON="$plan_json" sh "$script_dir/check-ci-roles-plan.sh"
plan_sha256=$(shasum -a 256 "$private_plan" | awk '{print $1}')
jq -nS \
  --arg schema 'portfolio.lambda-ci-roles-plan-provenance/v1' \
  --arg plan_sha256 "$plan_sha256" \
  --arg workspace "$workspace" \
  --argjson backend "$backend" \
  '{schema:$schema,plan_sha256:$plan_sha256,backend:$backend,workspace:$workspace}' \
  > "$private_provenance"
provenance_sha256=$(shasum -a 256 "$private_provenance" | awk '{print $1}')
TF_CLI_CONFIG_FILE="$cli_config_file" \
  TF_DATA_DIR="$plan_data_dir" \
  tofu -chdir=infra/lambda/ci-roles show -no-color "$private_plan"
cp "$private_plan" "$PLAN_FILE"
cp "$private_provenance" "$provenance_file"
published_plan_sha256=$(shasum -a 256 "$PLAN_FILE" | awk '{print $1}')
[ "$published_plan_sha256" = "$plan_sha256" ] ||
  fail 'published plan changed while creating the review artifacts'
published_provenance_sha256=$(shasum -a 256 "$provenance_file" | awk '{print $1}')
[ "$published_provenance_sha256" = "$provenance_sha256" ] ||
  fail 'published provenance changed while creating the review artifacts'
retain_plan=true
