#!/bin/sh
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
rejection='Management input rejected: expected management must be the exact reviewed public development settings'

reject_private_input() {
  input=$1
  for command in check-management-input create-ci-lambda-release-plan create-ci-lambda-rollback-plan; do
    if env PATH="$test_dir/bin:$PATH" MANAGEMENT_TOFU_LOG="$test_dir/tofu.log" \
      ENVIRONMENT=dev EXPECTED_MANAGEMENT_JSON="$input" RELEASE_ENVIRONMENT=development \
      IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
      EVIDENCE_DIR="$test_dir/evidence" ECR_URL=unused PRIOR_VERSION=7 \
      sh "$root_dir/scripts/$command.sh" > "$test_dir/stdout" 2> "$test_dir/stderr"; then
      printf 'FAIL: %s accepted invalid management input\n' "$command" >&2
      exit 1
    fi
    test ! -s "$test_dir/stdout"
    test "$(cat "$test_dir/stderr")" = "$rejection"
    ! grep -Fq s3cr3t "$test_dir/stdout" "$test_dir/stderr"
    test ! -e "$test_dir/tofu.log"
    test ! -e "$test_dir/evidence"
  done
}

for field in cognito_domain cognito_issuer cognito_client_id redirect_uri logout_uri \
  allowed_emails allow_local_callback ec2_management_tag_key ec2_management_tag_value; do
  input=$(printf '%s\n' "$public" | jq --arg field "$field" '.[$field] = ["s3cr3t"]')
  reject_private_input "$input"
done
reject_private_input '{"cognito_domain":"s3cr3t",'
reject_private_input '"s3cr3t"'
reject_private_input '["s3cr3t"]'
reject_private_input "$public
{\"cognito_domain\":\"s3cr3t\","
reject_private_input "$public
$public"
for input in null "$public"; do
  ENVIRONMENT=dev EXPECTED_MANAGEMENT_JSON="$input" \
    sh "$root_dir/scripts/check-management-input.sh" > "$test_dir/stdout" 2> "$test_dir/stderr"
  test ! -s "$test_dir/stdout"
  test ! -s "$test_dir/stderr"
done
printf 'Management input confidentiality contracts passed\n'
