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
		.change.actions == ["delete"] and .change.before != null and .change.after == null) and
	all(.output_changes | to_entries[];
		(.value.actions | managed_non_actionable) or .value.actions == ["delete"]) and
	([ $output_deletes[].key ] | sort) == (expected_outputs | sort) and
	all($output_deletes[];
		.value.actions == ["delete"] and .value.before != null and .value.after == null)
' "$PLAN_JSON" >/dev/null || fail "actionable changes do not match the approved retirement boundary"

printf 'App Runner retirement plan contract passed\n'
