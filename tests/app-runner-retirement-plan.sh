#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
checker="$repo_root/scripts/check-app-runner-retirement-plan.sh"
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM

pass_count=0

pass() {
	pass_count=$((pass_count + 1))
	printf 'PASS: %s\n' "$1"
}

expect_pass() {
	name=$1
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		pass "$name"
	else
		printf 'FAIL: %s\n' "$name" >&2
		cat "$tmp_dir/output" >&2
		exit 1
	fi
}

expect_fail() {
	name=$1
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		printf 'FAIL: %s unexpectedly passed\n' "$name" >&2
		exit 1
	fi
	pass "$name"
}

run_check() {
	PLAN_JSON=$1 sh "$checker"
}

make_plan() {
	jq -n '
		def deletion($before): {actions: ["delete"], before: $before, after: null};
		def no_op($before): {actions: ["no-op"], before: $before, after: $before};
		{
			resource_changes: [
				{mode: "managed", address: "aws_apprunner_service.app", change: deletion({service_name: "portfolio"})},
				{mode: "managed", address: "aws_iam_role.apprunner_ecr_access", change: deletion({name: "portfolio-apprunner-ecr-access"})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_ecr_access", change: deletion({role: "portfolio-apprunner-ecr-access"})},
				{mode: "managed", address: "aws_iam_role.apprunner_instance", change: deletion({name: "portfolio-apprunner-instance"})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.google_connections_dynamodb", change: deletion({role: "portfolio-apprunner-instance"})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.soccer_sessions_dynamodb", change: deletion({role: "portfolio-apprunner-instance"})},
				{mode: "managed", address: "aws_iam_policy.apprunner_runtime_secrets", change: deletion({name: "portfolio-apprunner-runtime-secrets"})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_runtime_secrets", change: deletion({role: "portfolio-apprunner-instance"})},
				{mode: "managed", address: "aws_lambda_function.app", change: no_op({function_name: "portfolio"})},
				{mode: "data", address: "data.aws_caller_identity.current", change: {actions: ["read"], before: null, after: {account_id: "180294223248"}}}
			],
			output_changes: {
				app_runner_service_url: deletion("https://example.awsapprunner.com"),
				app_runner_service_arn: deletion("arn:aws:apprunner:us-west-2:180294223248:service/portfolio"),
				app_runner_service_id: deletion("service-id"),
				instance_role_arn: deletion("arn:aws:iam::180294223248:role/portfolio-apprunner-instance"),
				lambda_function_name: no_op("portfolio")
			}
		}
	' >"$1"
}

mutate_and_reject() {
	name=$1
	filter=$2
	jq "$filter" "$plan" >"$tmp_dir/mutated.json"
	expect_fail "$name" run_check "$tmp_dir/mutated.json"
}

plan="$tmp_dir/valid.json"
make_plan "$plan"

expect_pass "exact App Runner retirement deletes" run_check "$plan"
mutate_and_reject "missing expected delete" 'del(.resource_changes[] | select(.address == "aws_apprunner_service.app"))'
mutate_and_reject "unexpected delete" '.resource_changes += [{mode: "managed", address: "aws_iam_role.unapproved", change: {actions: ["delete"], before: {name: "unapproved"}, after: null}}]'
mutate_and_reject "create action" '(.resource_changes[] | select(.address == "aws_lambda_function.app").change.actions) = ["create"]'
mutate_and_reject "update action" '(.resource_changes[] | select(.address == "aws_lambda_function.app").change.actions) = ["update"]'
mutate_and_reject "replacement action" '(.resource_changes[] | select(.address == "aws_lambda_function.app").change.actions) = ["delete", "create"]'
mutate_and_reject "expected address with a non-delete action" '(.resource_changes[] | select(.address == "aws_apprunner_service.app").change.actions) = ["no-op"]'
mutate_and_reject "delete with a non-null after value" '(.resource_changes[] | select(.address == "aws_apprunner_service.app").change.after) = {service_name: "portfolio"}'
mutate_and_reject "delete with a null before value" '(.resource_changes[] | select(.address == "aws_apprunner_service.app").change.before) = null'
mutate_and_reject "managed resource read action" '(.resource_changes[] | select(.address == "aws_lambda_function.app").change.actions) = ["read"]'
mutate_and_reject "data source delete action" '(.resource_changes[] | select(.mode == "data").change.actions) = ["delete"]'
mutate_and_reject "unknown resource mode" '(.resource_changes[] | select(.address == "aws_lambda_function.app").mode) = "unknown"'
mutate_and_reject "duplicate managed resource address" '.resource_changes += [(.resource_changes[] | select(.address == "aws_apprunner_service.app") | .change.actions = ["no-op"])]'
mutate_and_reject "unexpected output delete" '.output_changes.unapproved = {actions: ["delete"], before: "value", after: null}'
mutate_and_reject "missing expected output delete" 'del(.output_changes.app_runner_service_url)'
mutate_and_reject "output read action" '.output_changes.lambda_function_name.actions = ["read"]'
mutate_and_reject "output delete with a non-null after value" '.output_changes.app_runner_service_url.after = "https://example.awsapprunner.com"'

{
	jq '.resource_changes[0].address = "aws_iam_role.unapproved"' "$plan"
	jq '.' "$plan"
} >"$tmp_dir/multiple-documents.json"
expect_fail "multiple top-level JSON documents" run_check "$tmp_dir/multiple-documents.json"

expect_fail "relative PLAN_JSON path" env PLAN_JSON=tests/app-runner-retirement-plan.sh sh "$checker"
expect_fail "missing PLAN_JSON file" env PLAN_JSON="$tmp_dir/missing.json" sh "$checker"

jq 'del(.resource_changes)' "$plan" >"$tmp_dir/missing-resources.json"
expect_fail "missing resource_changes" run_check "$tmp_dir/missing-resources.json"
jq 'del(.output_changes)' "$plan" >"$tmp_dir/missing-outputs.json"
expect_fail "missing output_changes" run_check "$tmp_dir/missing-outputs.json"

printf '%s tests passed\n' "$pass_count"
