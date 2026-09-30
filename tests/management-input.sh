#!/bin/sh
# The development management input is an identity-free portal switch: null, or
# exactly {"aws_region":"us-west-2"}. Anything else, including the retired
# nine-field Cognito shape, is refused before any backend access and without
# echoing the input.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM
mkdir -p "$test_dir/bin"
cat > "$test_dir/bin/tofu" <<'CLI'
#!/bin/sh
printf 'unexpected backend access\n' >> "$MANAGEMENT_TOFU_LOG"
exit 1
CLI
chmod +x "$test_dir/bin/tofu"
public=$(cat "$root_dir/tests/fixtures/management-public.json")
rejection='Management input rejected: expected management must be null or exactly {"aws_region":"us-west-2"} for development'

# reject_input INPUT [RELEASE_ENVIRONMENT]: both scripts refuse INPUT with the
# fixed message, print nothing else, and never reach tofu or write evidence.
reject_input() {
  input=$1
  release_environment=${2:-development}
  case "$release_environment" in
    production) environment=prod ;;
    *) environment=dev ;;
  esac
  for command in check-management-input create-ci-lambda-release-plan; do
    if env PATH="$test_dir/bin:$PATH" MANAGEMENT_TOFU_LOG="$test_dir/tofu.log" \
      ENVIRONMENT="$environment" EXPECTED_MANAGEMENT_JSON="$input" \
      RELEASE_ENVIRONMENT="$release_environment" \
      IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
      EVIDENCE_DIR="$test_dir/evidence" ECR_URL=unused PORTFOLIO_ACCOUNT_ID=111122223333 \
      sh "$root_dir/scripts/$command.sh" > "$test_dir/stdout" 2> "$test_dir/stderr"; then
      printf 'FAIL: %s accepted invalid management input for %s\n' "$command" "$release_environment" >&2
      exit 1
    fi
    test ! -s "$test_dir/stdout"
    test "$(cat "$test_dir/stderr")" = "$rejection"
    ! grep -Fq s3cr3t "$test_dir/stdout" "$test_dir/stderr"
    test ! -e "$test_dir/tofu.log"
    test ! -e "$test_dir/evidence"
  done
}

# The retired nine-field Cognito shape that MANAGEMENT_RUNTIME_JSON used to hold.
reject_input '{"cognito_domain":"https://portfolio-lambda-dev-mgmt.auth.us-west-2.amazoncognito.com",
  "cognito_issuer":"https://cognito-idp.us-west-2.amazonaws.com/us-west-2_s3cr3t",
  "cognito_client_id":"s3cr3t","redirect_uri":"https://dev.craigdevjohnson.com/callback",
  "logout_uri":"https://dev.craigdevjohnson.com/login","allowed_emails":["craigdevjohnson@gmail.com"],
  "allow_local_callback":false,"ec2_management_tag_key":"PortfolioManagement","ec2_management_tag_value":"dev"}'
# The new shape plus any identity or secret field.
for field in cognito_client_id client_secret session_key allowed_emails; do
  reject_input "$(printf '%s\n' "$public" | jq -c --arg field "$field" '.[$field] = "s3cr3t"')"
done
reject_input "$(printf '%s\n' "$public" | jq -c '.aws_region = ["s3cr3t"]')"
reject_input "$(printf '%s\n' "$public" | jq -c '.aws_region = "us-east-1"')"
reject_input '{}'
reject_input '{"aws_region":"s3cr3t",'
reject_input '"s3cr3t"'
reject_input '["s3cr3t"]'
reject_input true
reject_input "$public
{\"aws_region\":\"s3cr3t\","
reject_input "$public
$public"
# Production has no management portal.
reject_input "$public" production

for input in null "$public" '{"aws_region":"us-west-2"}'; do
  ENVIRONMENT=dev EXPECTED_MANAGEMENT_JSON="$input" \
    sh "$root_dir/scripts/check-management-input.sh" > "$test_dir/stdout" 2> "$test_dir/stderr"
  test ! -s "$test_dir/stdout"
  test ! -s "$test_dir/stderr"
done
ENVIRONMENT=prod EXPECTED_MANAGEMENT_JSON=null \
  sh "$root_dir/scripts/check-management-input.sh" > "$test_dir/stdout" 2> "$test_dir/stderr"
test ! -s "$test_dir/stdout"
test ! -s "$test_dir/stderr"

# The development release plan accepts the switch and hands exactly that value
# to OpenTofu. This fake tofu serves a pure image release and records the input.
mkdir -p "$test_dir/accept-bin"
cat > "$test_dir/accept-bin/tofu" <<'CLI'
#!/bin/sh
shift # -chdir=...
case "$1" in
  init) ;;
  workspace) printf 'default\n' ;;
  plan)
    printf '%s\n' "$TF_VAR_management" > "$MANAGEMENT_TOFU_INPUT"
    for argument in "$@"; do
      case "$argument" in -out=*) printf 'saved plan\n' > "${argument#-out=}" ;; esac
    done
    ;;
  show)
    if [ "$2" = -json ]; then
      jq -n --arg image "$ECR_URL@$IMAGE_DIGEST" '{resource_changes: [
        {address: "module.service.aws_lambda_function.app", mode: "managed",
          change: {actions: ["update"], before: {image_uri: "old"}, after: {image_uri: $image}, after_unknown: {}}}]}'
    else
      printf 'plan text\n'
    fi
    ;;
  *) exit 1 ;;
esac
CLI
chmod +x "$test_dir/accept-bin/tofu"
for input in null "$public"; do
  rm -rf "$test_dir/evidence" "$test_dir/tofu-input"
  env PATH="$test_dir/accept-bin:$PATH" MANAGEMENT_TOFU_INPUT="$test_dir/tofu-input" \
    EXPECTED_MANAGEMENT_JSON="$input" RELEASE_ENVIRONMENT=development \
    IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
    EVIDENCE_DIR="$test_dir/evidence" PORTFOLIO_ACCOUNT_ID=111122223333 \
    ECR_URL=111122223333.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases \
    sh "$root_dir/scripts/create-ci-lambda-release-plan.sh" > "$test_dir/stdout" 2> "$test_dir/stderr" || {
    cat "$test_dir/stderr" >&2
    printf 'FAIL: the development release plan refused management input %s\n' "$input" >&2
    exit 1
  }
  test "$(jq -c . "$test_dir/tofu-input")" = "$(printf '%s\n' "$input" | jq -c .)"
  test -s "$test_dir/evidence/dev.tfplan"
done
printf 'Management input confidentiality contracts passed\n'
