#!/bin/sh
set -eu

fail() {
	printf 'App Runner retirement plan contract failed: %s\n' "$1" >&2
	exit 1
}

: "${PLAN_JSON:?set PLAN_JSON to the absolute saved-plan JSON path}"

case "$PLAN_JSON" in
	/*) ;;
	*) fail "PLAN_JSON must be absolute" ;;
esac
test -f "$PLAN_JSON" || fail "PLAN_JSON does not exist"

jq -se '
	length == 1 and
	(.[0].applyable == true) and
	(.[0].errored == false) and
	(.[0].resource_changes | type == "array") and
	(.[0].output_changes | type == "object")
' "$PLAN_JSON" >/dev/null || fail "plan JSON must contain exactly one document with resource_changes and output_changes"

jq -se '
	def managed_non_actionable: . == ["no-op"];
	def data_non_actionable: . == ["no-op"] or . == ["read"];
	.[0] |
	def expected_resources: [
		"aws_apprunner_service.app",
		"aws_iam_role.apprunner_ecr_access",
		"aws_iam_role_policy_attachment.apprunner_ecr_access",
		"aws_iam_role.apprunner_instance",
		"aws_iam_role_policy_attachment.google_connections_dynamodb",
		"aws_iam_role_policy_attachment.soccer_sessions_dynamodb",
		"aws_iam_policy.apprunner_runtime_secrets",
		"aws_iam_role_policy_attachment.apprunner_runtime_secrets"
	];
	def expected_outputs: [
		"app_runner_service_url",
		"app_runner_service_arn",
		"app_runner_service_id",
		"instance_role_arn"
	];
	def exact_resource_identity:
		if .address == "aws_apprunner_service.app" then
			.change.before.id == "arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb" and
			.change.before.arn == "arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb" and
			.change.before.service_name == "portfolio"
		elif .address == "aws_iam_role.apprunner_ecr_access" then
			.change.before.id == "portfolio-apprunner-ecr-access" and
			.change.before.arn == "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access" and
			.change.before.name == "portfolio-apprunner-ecr-access" and
			.change.before.unique_id == "AROAST6S7QWIFWIJU3SEX" and
			(.change.before.assume_role_policy | fromjson) == {
				Version: "2012-10-17",
				Statement: [{
					Effect: "Allow",
					Principal: {Service: "build.apprunner.amazonaws.com"},
					Action: "sts:AssumeRole"
				}]
			} and
			.change.before.inline_policy == []
		elif .address == "aws_iam_role_policy_attachment.apprunner_ecr_access" then
			.change.before.role == "portfolio-apprunner-ecr-access" and
			.change.before.policy_arn == "arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"
		elif .address == "aws_iam_role.apprunner_instance" then
			.change.before.id == "portfolio-apprunner-instance" and
			.change.before.arn == "arn:aws:iam::180294223248:role/portfolio-apprunner-instance" and
			.change.before.name == "portfolio-apprunner-instance" and
			.change.before.unique_id == "AROAST6S7QWIK7PZV2BTQ" and
			(.change.before.assume_role_policy | fromjson) == {
				Version: "2012-10-17",
				Statement: [{
					Effect: "Allow",
					Principal: {Service: "tasks.apprunner.amazonaws.com"},
					Action: "sts:AssumeRole"
				}]
			} and
			.change.before.inline_policy == []
		elif .address == "aws_iam_role_policy_attachment.google_connections_dynamodb" then
			.change.before.role == "portfolio-apprunner-instance" and
			.change.before.policy_arn == "arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"
		elif .address == "aws_iam_role_policy_attachment.soccer_sessions_dynamodb" then
			.change.before.role == "portfolio-apprunner-instance" and
			.change.before.policy_arn == "arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"
		elif .address == "aws_iam_policy.apprunner_runtime_secrets" then
			.change.before.id == "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets" and
			.change.before.arn == "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets" and
			.change.before.name == "portfolio-apprunner-runtime-secrets" and
			.change.before.policy_id == "ANPAST6S7QWIFH5H5PRHB"
		elif .address == "aws_iam_role_policy_attachment.apprunner_runtime_secrets" then
			.change.before.role == "portfolio-apprunner-instance" and
			.change.before.policy_arn == "arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"
		else false
		end;
	def exact_output_identity:
		if .key == "app_runner_service_url" then
			.value.before == "https://vafw855pvk.us-west-2.awsapprunner.com"
		elif .key == "app_runner_service_arn" then
			.value.before == "arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb"
		elif .key == "app_runner_service_id" then
			.value.before == "c5490e71b0e84aba90a9648e94d240fb"
		elif .key == "instance_role_arn" then
			.value.before == "arn:aws:iam::180294223248:role/portfolio-apprunner-instance"
		else false
		end;
	[
		.resource_changes[]
		| select(.mode == "managed" and (.change.actions | managed_non_actionable | not))
	] as $resource_deletes |
	[
		.output_changes | to_entries[]
		| select(.value.actions | managed_non_actionable | not)
	] as $output_deletes |
	([.resource_changes[].address] | length) == ([.resource_changes[].address] | unique | length) and
	all(.resource_changes[];
		(.mode == "managed" and ((.change.actions | managed_non_actionable) or .change.actions == ["delete"])) or
		(.mode == "data" and (.change.actions | data_non_actionable))) and
	([ $resource_deletes[].address ] | sort) == (expected_resources | sort) and
	all($resource_deletes[];
		.change.actions == ["delete"] and
		.change.before != null and
		.change.after == null and
		exact_resource_identity) and
	all(.output_changes | to_entries[];
		(.value.actions | managed_non_actionable) or .value.actions == ["delete"]) and
	([ $output_deletes[].key ] | sort) == (expected_outputs | sort) and
	all($output_deletes[];
		.value.actions == ["delete"] and
		.value.before != null and
		.value.after == null and
		exact_output_identity)
' "$PLAN_JSON" >/dev/null || fail "actionable changes do not match the approved retirement boundary"

printf 'App Runner retirement plan contract passed\n'
