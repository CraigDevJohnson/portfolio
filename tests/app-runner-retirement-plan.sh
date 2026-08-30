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
			applyable: true,
			errored: false,
			resource_changes: [
				{mode: "managed", address: "aws_apprunner_service.app", change: deletion({
					id: "arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb",
					arn: "arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb",
					service_name: "portfolio"
				})},
				{mode: "managed", address: "aws_iam_role.apprunner_ecr_access", change: deletion({
					id: "portfolio-apprunner-ecr-access",
					arn: "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access",
					name: "portfolio-apprunner-ecr-access",
					unique_id: "AROAST6S7QWIFWIJU3SEX",
					assume_role_policy: ({
						Version: "2012-10-17",
						Statement: [{
							Effect: "Allow",
							Principal: {Service: "build.apprunner.amazonaws.com"},
							Action: "sts:AssumeRole"
						}]
					} | tojson),
					inline_policy: []
				})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_ecr_access", change: deletion({
					role: "portfolio-apprunner-ecr-access",
					policy_arn: "arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"
				})},
				{mode: "managed", address: "aws_iam_role.apprunner_instance", change: deletion({
					id: "portfolio-apprunner-instance",
					arn: "arn:aws:iam::180294223248:role/portfolio-apprunner-instance",
					name: "portfolio-apprunner-instance",
					unique_id: "AROAST6S7QWIK7PZV2BTQ",
					assume_role_policy: ({
						Version: "2012-10-17",
						Statement: [{
							Effect: "Allow",
							Principal: {Service: "tasks.apprunner.amazonaws.com"},
							Action: "sts:AssumeRole"
						}]
					} | tojson),
					inline_policy: []
				})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.google_connections_dynamodb", change: deletion({
					role: "portfolio-apprunner-instance",
					policy_arn: "arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"
				})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.soccer_sessions_dynamodb", change: deletion({
					role: "portfolio-apprunner-instance",
					policy_arn: "arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"
				})},
				{mode: "managed", address: "aws_iam_policy.apprunner_runtime_secrets", change: deletion({
					id: "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
					arn: "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets",
					name: "portfolio-apprunner-runtime-secrets"
				})},
				{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_runtime_secrets", change: deletion({
					role: "portfolio-apprunner-instance",
					policy_arn: "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"
				})},
				{mode: "managed", address: "aws_lambda_function.app", change: no_op({function_name: "portfolio"})},
				{mode: "data", address: "data.aws_caller_identity.current", change: {actions: ["read"], before: null, after: {account_id: "180294223248"}}}
			],
			output_changes: {
				app_runner_service_url: deletion("https://vafw855pvk.us-west-2.awsapprunner.com"),
				app_runner_service_arn: deletion("arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb"),
				app_runner_service_id: deletion("c5490e71b0e84aba90a9648e94d240fb"),
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
mutate_and_reject "App Runner service ARN drift" '(.resource_changes[] | select(.address == "aws_apprunner_service.app").change.before.arn) = "arn:aws:apprunner:us-west-2:180294223248:service/not-portfolio/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
mutate_and_reject "App Runner service name drift" '(.resource_changes[] | select(.address == "aws_apprunner_service.app").change.before.service_name) = "not-portfolio"'
mutate_and_reject "ECR access role name drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_ecr_access").change.before.name) = "not-the-ecr-role"'
mutate_and_reject "ECR access role ID drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_ecr_access").change.before.unique_id) = "AROAST6S7QWI000000000"'
mutate_and_reject "ECR access role trust drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_ecr_access").change.before.assume_role_policy) = ({Version: "2012-10-17", Statement: [{Effect: "Allow", Principal: {Service: ["build.apprunner.amazonaws.com", "lambda.amazonaws.com"]}, Action: "sts:AssumeRole"}]} | tojson)'
mutate_and_reject "ECR access role inline policy" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_ecr_access").change.before.inline_policy) = [{name: "unexpected"}]'
mutate_and_reject "instance role name drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_instance").change.before.name) = "not-the-instance-role"'
mutate_and_reject "instance role ID drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_instance").change.before.unique_id) = "AROAST6S7QWI000000001"'
mutate_and_reject "instance role trust drift" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_instance").change.before.assume_role_policy) = ({Version: "2012-10-17", Statement: [{Effect: "Allow", Principal: {Service: ["tasks.apprunner.amazonaws.com", "lambda.amazonaws.com"]}, Action: "sts:AssumeRole"}]} | tojson)'
mutate_and_reject "instance role inline policy" '(.resource_changes[] | select(.address == "aws_iam_role.apprunner_instance").change.before.inline_policy) = [{name: "unexpected"}]'
mutate_and_reject "ECR attachment role drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.apprunner_ecr_access").change.before.role) = "portfolio-apprunner-instance"'
mutate_and_reject "ECR attachment policy drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.apprunner_ecr_access").change.before.policy_arn) = "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"'
mutate_and_reject "Google attachment role drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.google_connections_dynamodb").change.before.role) = "portfolio-apprunner-ecr-access"'
mutate_and_reject "Google attachment policy drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.google_connections_dynamodb").change.before.policy_arn) = "arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"'
mutate_and_reject "soccer attachment role drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.soccer_sessions_dynamodb").change.before.role) = "portfolio-apprunner-ecr-access"'
mutate_and_reject "soccer attachment policy drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.soccer_sessions_dynamodb").change.before.policy_arn) = "arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"'
mutate_and_reject "runtime attachment role drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.apprunner_runtime_secrets").change.before.role) = "portfolio-apprunner-ecr-access"'
mutate_and_reject "runtime attachment policy drift" '(.resource_changes[] | select(.address == "aws_iam_role_policy_attachment.apprunner_runtime_secrets").change.before.policy_arn) = "arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"'
mutate_and_reject "runtime policy ARN drift" '(.resource_changes[] | select(.address == "aws_iam_policy.apprunner_runtime_secrets").change.before.arn) = "arn:aws:iam::180294223248:policy/not-the-runtime-policy"'
mutate_and_reject "managed resource read action" '(.resource_changes[] | select(.address == "aws_lambda_function.app").change.actions) = ["read"]'
mutate_and_reject "data source delete action" '(.resource_changes[] | select(.mode == "data").change.actions) = ["delete"]'
mutate_and_reject "unknown resource mode" '(.resource_changes[] | select(.address == "aws_lambda_function.app").mode) = "unknown"'
mutate_and_reject "duplicate managed resource address" '.resource_changes += [(.resource_changes[] | select(.address == "aws_apprunner_service.app") | .change.actions = ["no-op"])]'
mutate_and_reject "unexpected output delete" '.output_changes.unapproved = {actions: ["delete"], before: "value", after: null}'
mutate_and_reject "missing expected output delete" 'del(.output_changes.app_runner_service_url)'
mutate_and_reject "output read action" '.output_changes.lambda_function_name.actions = ["read"]'
mutate_and_reject "output delete with a non-null after value" '.output_changes.app_runner_service_url.after = "https://example.awsapprunner.com"'
mutate_and_reject "App Runner URL output drift" '.output_changes.app_runner_service_url.before = "https://wrong.us-west-2.awsapprunner.com"'
mutate_and_reject "App Runner ARN output drift" '.output_changes.app_runner_service_arn.before = "arn:aws:apprunner:us-west-2:180294223248:service/not-portfolio/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
mutate_and_reject "App Runner ID output drift" '.output_changes.app_runner_service_id.before = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"'
mutate_and_reject "instance role output drift" '.output_changes.instance_role_arn.before = "arn:aws:iam::180294223248:role/not-the-instance-role"'
mutate_and_reject "errored plan" '.errored = true'
mutate_and_reject "non-applyable plan" '.applyable = false'
mutate_and_reject "missing applyable marker" 'del(.applyable)'

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
