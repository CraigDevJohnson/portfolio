#!/bin/sh
set -eu

fail() {
	printf 'Lambda plan contract failed: %s\n' "$1" >&2
	exit 1
}

: "${PLAN_JSON:?set PLAN_JSON to the absolute saved-plan JSON path}"
: "${ENVIRONMENT:?set ENVIRONMENT to artifacts, dev, or prod}"
: "${NAME_PREFIX:?set NAME_PREFIX to the exact root-owned name}"
: "${IMAGE_URI:?set IMAGE_URI to the digest-qualified release URI}"
: "${EXPECTED_ALARM_ACTIONS_JSON:?set EXPECTED_ALARM_ACTIONS_JSON to a JSON array}"
AUTOMATED_RELEASE=${AUTOMATED_RELEASE:-false}

case "$AUTOMATED_RELEASE" in
	true | false) ;;
	*) fail "AUTOMATED_RELEASE must be true or false" ;;
esac

case "$PLAN_JSON" in
	/*) ;;
	*) fail "PLAN_JSON must be absolute" ;;
esac
test -f "$PLAN_JSON" || fail "PLAN_JSON does not exist"

case "$ENVIRONMENT:$NAME_PREFIX" in
	artifacts:portfolio-lambda-releases | dev:portfolio-lambda-dev | prod:portfolio-lambda-prod) ;;
	*) fail "environment and name prefix do not match a replacement root" ;;
esac

case "$IMAGE_URI" in
	180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:????????????????????????????????????????????????????????????????) ;;
	*) fail "IMAGE_URI must use the replacement repository and a SHA-256 digest" ;;
esac
printf '%s\n' "$IMAGE_URI" | grep -Eq '@sha256:[0-9a-f]{64}$' ||
	fail "IMAGE_URI digest must contain 64 lowercase hexadecimal characters"

printf '%s\n' "$EXPECTED_ALARM_ACTIONS_JSON" |
	jq -e 'type == "array" and all(.[]; type == "string")' >/dev/null ||
	fail "EXPECTED_ALARM_ACTIONS_JSON must be an array of strings"
jq -e '.resource_changes | type == "array"' "$PLAN_JSON" >/dev/null ||
	fail "plan JSON has no resource_changes array"

jq -e '
	all(.resource_changes[]; (.change.actions | index("delete")) == null)
' "$PLAN_JSON" >/dev/null || fail "delete or replace action found"

jq -e '
	[
		.resource_changes[]
		| select(
			(.address | test("aws_apprunner|aws_amplify"; "i")) or
			(.change.after | tostring | test("aws_apprunner|aws_amplify"; "i"))
		)
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "legacy App Runner or Amplify resource found"

jq -e '
	[
		.resource_changes[].change
		| [.before, .after][]
		| ..
		| strings
		| select(test("(^|[:/])(latest|lambda-latest)$"; "i"))
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "mutable image tag found"

jq -e --arg environment "$ENVIRONMENT" '
	def allowed_secret_path:
		type == "string" and
		. == ("/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY");
	[
		.resource_changes[].change
		| [.before, .after][]
		| ..
		| objects
		| to_entries[]
		| select(
			(.key | test("secret|password|token"; "i")) or
			((.key | ascii_downcase) == "private_key")
		)
		| .value
		| select(. != null and . != false and . != "" and (allowed_secret_path | not))
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "secret value found in plan"

jq -e '
	def known_null_private_key($values; $unknown):
		($values |
			type == "object" and
			has("private_key") and
			.private_key == null) and
		(($unknown // {}) |
			type == "object" and
			((has("private_key") | not) or .private_key == false));
	def allowed_null_sensitive_marker($resource_type; $values; $unknown; $path):
		$resource_type == "aws_acm_certificate" and
		$path == ["private_key"] and
		known_null_private_key($values; $unknown);
	[
		.resource_changes[]?
		| .type as $resource_type
		| .change as $change
		| [
			{values: $change.before, unknown: null, sensitive: $change.before_sensitive},
			{values: $change.after, unknown: $change.after_unknown, sensitive: $change.after_sensitive}
		][]
		| . as $snapshot
		| ($snapshot.sensitive // false)
		| path(.. | select(type == "boolean" and .)) as $path
		| select(allowed_null_sensitive_marker($resource_type; $snapshot.values; $snapshot.unknown; $path) | not)
	] | length == 0
' "$PLAN_JSON" >/dev/null || fail "sensitive value found in plan"

if [ "$AUTOMATED_RELEASE" = true ]; then
	[ "$ENVIRONMENT" = dev ] || fail "automated apply is limited to development"
	jq -e --arg image "$IMAGE_URI" '
		def true_paths($unknown):
			[($unknown // {}) | paths(scalars) as $path | select(getpath($path) == true) | $path];
		def only_allowed_unknowns($unknown; $allowed):
			all(true_paths($unknown)[]; .[0] as $attribute | $allowed | index($attribute) != null);
		def without_changes($value; $unknown; $explicit):
			$value | delpaths(($explicit + true_paths($unknown)) | unique);
		def numbered_version:
			type == "string" and test("^[0-9]+$");
		def release_image:
			type == "string" and test("^180294223248\\.dkr\\.ecr\\.us-west-2\\.amazonaws\\.com/portfolio-lambda-releases@sha256:[0-9a-f]{64}$");

		[.resource_changes[] | select(.mode == "managed" and .change.actions != ["no-op"])] as $changed |
		($changed | map({address, type, actions: .change.actions}) | sort_by(.address)) == ([
			{address: "module.service.aws_lambda_alias.live", type: "aws_lambda_alias", actions: ["update"]},
			{address: "module.service.aws_lambda_function.app", type: "aws_lambda_function", actions: ["update"]}
		] | sort_by(.address)) and
		all(.resource_changes[] | select(.mode == "managed" and (.change.actions == ["no-op"] | not));
			.change.before != null and .change.after != null) and
		all(.resource_changes[] | select(.mode == "data");
			.change.actions == ["read"] or .change.actions == ["no-op"]) and
		all(.resource_changes[]; .mode == "managed" or .mode == "data") and
		(.variables.live_version_override |
			type == "object" and has("value") and .value == null) and

		(first($changed[] | select(.address == "module.service.aws_lambda_function.app"))) as $function |
		($function.change.before.function_name == "portfolio-lambda-dev") and
		($function.change.after.function_name == "portfolio-lambda-dev") and
		($function.change.before.image_uri | release_image) and
		($function.change.before.image_uri != $image) and
		($function.change.after.image_uri == $image) and
		only_allowed_unknowns($function.change.after_unknown; [
			"arn",
			"code_sha256",
			"id",
			"invoke_arn",
			"last_modified",
			"qualified_arn",
			"qualified_invoke_arn",
			"signing_job_arn",
			"signing_profile_version_arn",
			"source_code_hash",
			"source_code_size",
			"version"
		]) and
		(without_changes($function.change.before; $function.change.after_unknown; [["image_uri"]]) ==
			without_changes($function.change.after; $function.change.after_unknown; [["image_uri"]])) and

		(first($changed[] | select(.address == "module.service.aws_lambda_alias.live"))) as $alias |
		($alias.change.before.name == "live") and
		($alias.change.after.name == "live") and
		($alias.change.before.function_name == "portfolio-lambda-dev") and
		($alias.change.after.function_name == "portfolio-lambda-dev") and
		($alias.change.before.function_version | numbered_version) and
		($alias.change.after.function_version == null) and
		($alias.change.after_unknown.function_version == true) and
		only_allowed_unknowns($alias.change.after_unknown; ["arn", "function_version", "id", "invoke_arn"]) and
		(without_changes($alias.change.before; $alias.change.after_unknown; [["function_version"]]) ==
			without_changes($alias.change.after; $alias.change.after_unknown; [["function_version"]]))
	' "$PLAN_JSON" >/dev/null || fail "automated release may update only the immutable image and live alias version"
	printf 'Lambda automated release plan contract passed for %s\n' "$ENVIRONMENT"
	exit 0
fi

if [ "$ENVIRONMENT" = artifacts ]; then
	jq -e '
		def by_address($address): first(.resource_changes[] | select(.address == $address));
		([.resource_changes[].address] | sort) == ([
			"aws_ecr_lifecycle_policy.lambda_releases",
			"aws_ecr_repository.lambda_releases",
			"aws_ecr_repository_policy.lambda_releases"
		] | sort) and
		(by_address("aws_ecr_repository.lambda_releases").change.after |
			.name == "portfolio-lambda-releases" and
			.image_tag_mutability == "IMMUTABLE" and
			.force_delete == false and
			.image_scanning_configuration[0].scan_on_push == true and
			.encryption_configuration[0].encryption_type == "AES256") and
		(by_address("aws_ecr_lifecycle_policy.lambda_releases").change.after |
			.repository == "portfolio-lambda-releases" and
			(.policy | fromjson) == {
				rules: [{
					rulePriority: 1,
					description: "Expire untagged images after 30 days",
					selection: {
						tagStatus: "untagged",
						countType: "sinceImagePushed",
						countUnit: "days",
						countNumber: 30
					},
					action: {type: "expire"}
				}]
			}) and
		(by_address("aws_ecr_repository_policy.lambda_releases").change.after |
			.repository == "portfolio-lambda-releases" and
			(.policy | fromjson) as $policy |
			$policy.Version == "2012-10-17" and
			($policy.Statement | length) == 1 and
			$policy.Statement[0].Sid == "LambdaPull" and
			$policy.Statement[0].Effect == "Allow" and
			$policy.Statement[0].Principal == {Service: "lambda.amazonaws.com"} and
			($policy.Statement[0].Action | sort) == (["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"] | sort) and
			$policy.Statement[0].Condition == {
				StringEquals: {"aws:SourceAccount": "180294223248"},
				ArnLike: {"aws:SourceArn": "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"}
			})
	' "$PLAN_JSON" >/dev/null || fail "artifact root ownership or repository policy drifted"
	printf 'Lambda artifact plan contract passed\n'
	exit 0
fi

expected_boundary=arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary
expected_retention=14
expected_protection=false
expected_reserved_concurrency=-1
if [ "$ENVIRONMENT" = prod ]; then
	expected_retention=90
	expected_protection=true
	expected_reserved_concurrency=10
fi

jq -e --arg environment "$ENVIRONMENT" '
	def allowed_managed($address; $type):
		[
			["module.service.aws_apigatewayv2_api.app", "aws_apigatewayv2_api"],
			["module.service.aws_apigatewayv2_integration.lambda", "aws_apigatewayv2_integration"],
			["module.service.aws_apigatewayv2_route.default", "aws_apigatewayv2_route"],
			["module.service.aws_apigatewayv2_stage.default", "aws_apigatewayv2_stage"],
			["module.service.aws_cloudwatch_log_group.api_access", "aws_cloudwatch_log_group"],
			["module.service.aws_cloudwatch_log_group.lambda", "aws_cloudwatch_log_group"],
			["module.service.aws_cloudwatch_metric_alarm.api_5xx", "aws_cloudwatch_metric_alarm"],
			["module.service.aws_cloudwatch_metric_alarm.api_latency", "aws_cloudwatch_metric_alarm"],
			["module.service.aws_cloudwatch_metric_alarm.lambda_duration", "aws_cloudwatch_metric_alarm"],
			["module.service.aws_cloudwatch_metric_alarm.lambda_errors", "aws_cloudwatch_metric_alarm"],
			["module.service.aws_cloudwatch_metric_alarm.lambda_throttles", "aws_cloudwatch_metric_alarm"],
			["module.service.aws_dynamodb_table.google_connections", "aws_dynamodb_table"],
			["module.service.aws_dynamodb_table.soccer_sessions", "aws_dynamodb_table"],
			["module.service.aws_iam_role.lambda", "aws_iam_role"],
			["module.service.aws_iam_role_policy.lambda", "aws_iam_role_policy"],
			["module.service.aws_lambda_alias.live", "aws_lambda_alias"],
			["module.service.aws_lambda_function.app", "aws_lambda_function"],
			["module.service.aws_lambda_permission.api", "aws_lambda_permission"],
			["module.service.aws_acm_certificate.custom[0]", "aws_acm_certificate"],
			["module.service.aws_acm_certificate_validation.custom[0]", "aws_acm_certificate_validation"]
		] +
		(if $environment == "dev" then
			[
				["module.service.aws_apigatewayv2_domain_name.custom[\"dev.craigdevjohnson.com\"]", "aws_apigatewayv2_domain_name"],
				["module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]", "aws_apigatewayv2_api_mapping"]
			]
		else
			[
				["module.service.aws_apigatewayv2_domain_name.custom[\"craigdevjohnson.com\"]", "aws_apigatewayv2_domain_name"],
				["module.service.aws_apigatewayv2_domain_name.custom[\"www.craigdevjohnson.com\"]", "aws_apigatewayv2_domain_name"],
				["module.service.aws_apigatewayv2_api_mapping.custom[\"craigdevjohnson.com\"]", "aws_apigatewayv2_api_mapping"],
				["module.service.aws_apigatewayv2_api_mapping.custom[\"www.craigdevjohnson.com\"]", "aws_apigatewayv2_api_mapping"]
			]
		end) |
		any(.[]; .[0] == $address and .[1] == $type);
	all(.resource_changes[] | select(.mode != "data");
		.mode == "managed" and allowed_managed(.address; .type))
' "$PLAN_JSON" >/dev/null || fail "environment plan contains an unapproved resource address or type"

jq -e '
	all(.resource_changes[] | select(.mode == "data");
		.address == "module.service.data.aws_iam_policy_document.lambda" and
		.type == "aws_iam_policy_document" and
		.change.actions == ["read"])
' "$PLAN_JSON" >/dev/null || fail "environment plan contains an unapproved data-source read"

jq -e \
	--arg environment "$ENVIRONMENT" \
	--arg prefix "$NAME_PREFIX" '
	def exact_keys($expected): (keys | sort) == ($expected | sort);
	def by_address($address): first(.resource_changes[] | select(.address == $address));
	def service_configuration_resources($address):
		[.configuration.root_module.module_calls.service.module.resources[] | select(.address == $address)];
	def exact_data_statement:
		type == "object" and
		exact_keys(["actions", "condition", "effect", "not_actions", "not_principals", "not_resources", "principals", "resources", "sid"]) and
		(.actions | type == "array" and length > 0 and all(.[]; type == "string")) and
		.condition == [] and
		.effect == null and
		.not_actions == null and
		.not_principals == [] and
		.not_resources == null and
		.principals == [] and
		(.resources | type == "array" and length > 0 and all(.[]; . == null or type == "string")) and
		.sid == null;
	def default_unknown_data_statement($statement): {
		actions: [range(0; ($statement.actions | length)) | false],
		condition: [],
		not_principals: [],
		principals: [],
		resources: [range(0; ($statement.resources | length)) | false]
	};
	def exact_unknown_data_statement($statement):
		type == "object" and
		((keys - ["actions", "condition", "effect", "not_actions", "not_principals", "not_resources", "principals", "resources", "sid"]) | length == 0) and
		.actions == [range(0; ($statement.actions | length)) | false] and
		.condition == [] and
		((.effect // false) == false) and
		((.not_actions // false) == false) and
		.not_principals == [] and
		((.not_resources // false) == false) and
		.principals == [] and
		(.resources | type == "array" and length == ($statement.resources | length) and all(.[]; type == "boolean")) and
		((.sid // false) == false);
	def resource_slots($values; $unknowns):
		[range(0; ($values | length)) as $index | {value: $values[$index], unknown: $unknowns[$index]}] | sort_by(tojson);
	def normalized_data_statement($unknown):
		{actions: (.actions | sort), resources: resource_slots(.resources; $unknown.resources)};
	def exact_arn_or_deferred($value; $unknown; $expected):
		($value == $expected and $unknown == false) or
		($value == null and $unknown == true);
	def empty_optional_json:
		. == null or . == "";
	def empty_optional_documents:
		. == null or . == [];
	def values_array: if type == "array" then . else [.] end;
	def decoded_data_statement($statement): {
		actions: ($statement.Action | values_array),
		condition: [],
		effect: null,
		not_actions: null,
		not_principals: [],
		not_resources: null,
		principals: [],
		resources: ($statement.Resource | values_array),
		sid: null
	};
	def exact_known_policy($value; $expected):
		(try ($value | fromjson) catch null) as $document |
		($value | type) == "string" and
		($document | type) == "object" and
		($document | exact_keys(["Statement", "Version"])) and
		$document.Version == "2012-10-17" and
		($document.Statement | type == "array" and length == 5) and
		all($document.Statement[];
			type == "object" and
			exact_keys(["Action", "Effect", "Resource"]) and
			.Effect == "Allow" and
			(.Action | values_array | length > 0 and all(.[]; type == "string")) and
			(.Resource | values_array | length > 0 and all(.[]; type == "string"))) and
		([
			$document.Statement[] |
			{actions: (.Action | values_array | sort), resources: (.Resource | values_array | sort)}
		] | sort_by(tojson)) == $expected;

	try (
		by_address("module.service.aws_dynamodb_table.google_connections").change as $google_change |
		by_address("module.service.aws_dynamodb_table.soccer_sessions").change as $soccer_change |
		by_address("module.service.aws_cloudwatch_log_group.lambda").change as $lambda_log_change |
		$google_change.after.arn as $google_arn |
		($google_change.after_unknown.arn // false) as $google_arn_unknown |
		$soccer_change.after.arn as $soccer_arn |
		($soccer_change.after_unknown.arn // false) as $soccer_arn_unknown |
		$lambda_log_change.after.arn as $lambda_log_arn |
		($lambda_log_change.after_unknown.arn // false) as $lambda_log_arn_unknown |
		by_address("module.service.aws_iam_role_policy.lambda") as $runtime_policy |
		by_address("module.service.aws_lambda_function.app") as $lambda |
		[
			.resource_changes[] |
			select(
				.address == "module.service.data.aws_iam_policy_document.lambda" and
				.mode == "data" and
				.type == "aws_iam_policy_document"
			)
		] as $policy_data_changes |
		(try ($runtime_policy.change.after.policy | fromjson) catch null) as $runtime_policy_document |
		(
			if ($policy_data_changes | length) == 1 then
				$policy_data_changes[0]
			elif
				($policy_data_changes | length) == 0 and
				$runtime_policy.change.actions == ["no-op"] and
				($runtime_policy_document | type) == "object"
			then
				{
					change: {
						actions: ["read"],
						after: {
							override_json: null,
							override_policy_documents: null,
							policy_id: null,
							source_json: null,
							source_policy_documents: null,
							statement: [
								$runtime_policy_document.Statement[] |
								decoded_data_statement(.)
							],
							version: null
						},
						after_unknown: {}
					}
				}
			else
				null
			end
		) as $policy_data_change |
		service_configuration_resources("aws_iam_role_policy.lambda") as $runtime_policy_configurations |
		service_configuration_resources("data.aws_iam_policy_document.lambda") as $policy_data_configurations |
		service_configuration_resources("data.aws_kms_alias.ssm") as $kms_alias_configurations |
		$runtime_policy_configurations[0] as $runtime_policy_configuration |
		$policy_data_configurations[0] as $policy_data_configuration |
		$kms_alias_configurations[0] as $kms_alias_configuration |
		(
			$policy_data_change.change.after_unknown.statement //
			[$policy_data_change.change.after.statement[] | default_unknown_data_statement(.)]
		) as $policy_unknown_statements |
		($policy_data_change != null) and
		($runtime_policy_configurations | length) == 1 and
		($policy_data_configurations | length) == 1 and
		($kms_alias_configurations | length) == 1 and
		($policy_data_change.change.actions == ["read"]) and
		($policy_data_change.change.after.source_json | empty_optional_json) and
		($policy_data_change.change.after.override_json | empty_optional_json) and
		($policy_data_change.change.after.source_policy_documents | empty_optional_documents) and
		($policy_data_change.change.after.override_policy_documents | empty_optional_documents) and
		(($policy_data_change.change.after_unknown.source_json // false) == false) and
		(($policy_data_change.change.after_unknown.override_json // false) == false) and
		(($policy_data_change.change.after_unknown.source_policy_documents // false) == false) and
		(($policy_data_change.change.after_unknown.override_policy_documents // false) == false) and
		($policy_data_change.change.after.statement | type == "array" and length == 5) and
		($policy_unknown_statements | type == "array" and length == 5) and
		all($policy_data_change.change.after.statement[]; exact_data_statement) and
		([
			range(0; 5) as $index |
			($policy_unknown_statements[$index] | exact_unknown_data_statement($policy_data_change.change.after.statement[$index]))
		] | all) and
		[
			$policy_data_change.change.after.statement[] |
			select((.actions | sort) == ["kms:Decrypt"]) |
			.resources[]
		] as $kms_resources |
		($kms_resources | length) == 1 and
		($kms_resources[0] | test("^arn:aws:kms:us-west-2:180294223248:key/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")) and
		exact_arn_or_deferred(
			$google_arn;
			$google_arn_unknown;
			"arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-google-connections"
		) and
		exact_arn_or_deferred(
			$soccer_arn;
			$soccer_arn_unknown;
			"arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-soccer-sessions"
		) and
		exact_arn_or_deferred(
			$lambda_log_arn;
			$lambda_log_arn_unknown;
			"arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $prefix
		) and
		(if $lambda_log_arn_unknown then null else ($lambda_log_arn + ":*") end) as $lambda_policy_resource |
		([{
			actions: ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"],
			resources: [$google_arn]
		}, {
			actions: ["dynamodb:PutItem"],
			resources: [$soccer_arn]
		}, {
			actions: ["ssm:GetParameters"],
			resources: [
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"
			]
		}, {
			actions: ["kms:Decrypt"],
			resources: $kms_resources
		}, {
			actions: ["logs:CreateLogStream", "logs:PutLogEvents"],
			resources: [$lambda_policy_resource]
		}] | map(.actions |= sort | .resources |= sort) | sort_by(tojson)) as $expected_statements |
		([{
			actions: ["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"],
			resources: resource_slots([$google_arn]; [$google_arn_unknown])
		}, {
			actions: ["dynamodb:PutItem"],
			resources: resource_slots([$soccer_arn]; [$soccer_arn_unknown])
		}, {
			actions: ["ssm:GetParameters"],
			resources: resource_slots([
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"
			]; [false, false, false])
		}, {
			actions: ["kms:Decrypt"],
			resources: resource_slots($kms_resources; [false])
		}, {
			actions: ["logs:CreateLogStream", "logs:PutLogEvents"],
			resources: resource_slots([$lambda_policy_resource]; [$lambda_log_arn_unknown])
		}] | map(.actions |= sort) | sort_by(tojson)) as $expected_statement_slots |
		([
			range(0; 5) as $index |
			($policy_data_change.change.after.statement[$index] |
				normalized_data_statement($policy_unknown_statements[$index]))
		] | sort_by(tojson)) == $expected_statement_slots and
		($policy_data_configuration.mode == "data") and
		($policy_data_configuration.type == "aws_iam_policy_document") and
		($policy_data_configuration.name == "lambda") and
		($policy_data_configuration.expressions | exact_keys(["statement"])) and
		($policy_data_configuration.expressions.statement == [{
			actions: {constant_value: ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]},
			resources: {references: [
				"aws_dynamodb_table.google_connections.arn",
				"aws_dynamodb_table.google_connections"
			]}
		}, {
			actions: {constant_value: ["dynamodb:PutItem"]},
			resources: {references: [
				"aws_dynamodb_table.soccer_sessions.arn",
				"aws_dynamodb_table.soccer_sessions"
			]}
		}, {
			actions: {constant_value: ["ssm:GetParameters"]},
			resources: {references: [
				"local.ssm_paths",
				"data.aws_partition.current.partition",
				"data.aws_partition.current",
				"var.aws_region",
				"data.aws_caller_identity.current.account_id",
				"data.aws_caller_identity.current"
			]}
		}, {
			actions: {constant_value: ["kms:Decrypt"]},
			resources: {references: [
				"data.aws_kms_alias.ssm.target_key_arn",
				"data.aws_kms_alias.ssm"
			]}
		}, {
			actions: {constant_value: ["logs:CreateLogStream", "logs:PutLogEvents"]},
			resources: {references: [
				"aws_cloudwatch_log_group.lambda.arn",
				"aws_cloudwatch_log_group.lambda"
			]}
		}]) and
		($kms_alias_configuration.mode == "data") and
		($kms_alias_configuration.type == "aws_kms_alias") and
		($kms_alias_configuration.name == "ssm") and
		($kms_alias_configuration.expressions == {name: {constant_value: "alias/aws/ssm"}}) and
		($runtime_policy_configuration.mode == "managed") and
		($runtime_policy_configuration.type == "aws_iam_role_policy") and
		($runtime_policy_configuration.name == "lambda") and
		($runtime_policy_configuration.expressions.policy | exact_keys(["references"])) and
		(($runtime_policy_configuration.expressions.policy.references | sort) == ([
			"data.aws_iam_policy_document.lambda",
			"data.aws_iam_policy_document.lambda.json"
		] | sort)) and
		(
			(
				($policy_data_changes | length) == 1 and
				($runtime_policy.change.after | exact_keys(["name", "policy"])) and
				($runtime_policy.change.after.policy | type) == "string" and
				$runtime_policy.change.after_unknown == {
					id: true,
					name_prefix: true,
					role: true
				} and
				exact_known_policy($runtime_policy.change.after.policy; $expected_statements)
			) or (
				($policy_data_changes | length) == 1 and
				($runtime_policy.change.after | exact_keys(["name"])) and
				$runtime_policy.change.after_unknown == {
					id: true,
					name_prefix: true,
					policy: true,
					role: true
				}
			) or (
				($policy_data_changes | length) == 1 and
				($runtime_policy.change.after | exact_keys(["name", "role"])) and
				$runtime_policy.change.after.role == ($prefix + "-execution") and
				$runtime_policy.change.after_unknown == {
					id: true,
					name_prefix: true,
					policy: true
				}
			) or (
				($policy_data_changes | length) == 0 and
				$runtime_policy.change.actions == ["no-op"] and
				$runtime_policy.change.before == $runtime_policy.change.after and
				($runtime_policy.change.after | exact_keys(["id", "name", "name_prefix", "policy", "role"])) and
				$runtime_policy.change.after.id == (($prefix + "-execution") + ":" + ($prefix + "-runtime")) and
				$runtime_policy.change.after.name == ($prefix + "-runtime") and
				$runtime_policy.change.after.name_prefix == "" and
				$runtime_policy.change.after.role == ($prefix + "-execution") and
				$runtime_policy.change.after_unknown == {} and
				exact_known_policy($runtime_policy.change.after.policy; $expected_statements)
			)
		) and
		$lambda.change.after.environment == [{variables: {
			CLIENT_ID_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY"),
			CLIENT_SECRET_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY"),
			GOOGLE_CONNECTION_TABLE_NAME: ($prefix + "-google-connections"),
			LOG_ADD_SOURCE: "false",
			LOG_FORMAT: "json",
			LOG_LEVEL: "info",
			LPS_SESSION_KEY: ("/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"),
			SOCCER_SESSION_TABLE_NAME: ($prefix + "-soccer-sessions")
		}}]
	) catch false
' "$PLAN_JSON" >/dev/null || fail "runtime policy or Lambda environment contract drifted"

jq -e \
	--arg prefix "$NAME_PREFIX" \
	--arg image "$IMAGE_URI" \
	--arg boundary "$expected_boundary" \
	--argjson retention "$expected_retention" \
	--argjson protection "$expected_protection" \
	--argjson reserved_concurrency "$expected_reserved_concurrency" \
	--argjson expected_actions "$EXPECTED_ALARM_ACTIONS_JSON" '
	def resources($type): [.resource_changes[] | select(.type == $type)];
	def planned($type; $name): first(.resource_changes[] | select(.type == $type and .name == $name));
	def configured_alarms:
		[
			.configuration.root_module.module_calls.service.module.resources[] |
			select(.address | startswith("aws_cloudwatch_metric_alarm.")) |
			{
				address,
				mode,
				type,
				name,
				alarm_actions: .expressions.alarm_actions
			}
		] | sort_by(.address);
	def normalized_string_array:
		if . == null then []
		elif type == "array" and all(.[]; type == "string") then .
		else error("expected a string array or provider-normalized null")
		end;
	def exact_alarm($name; $metric; $threshold; $statistic):
		planned("aws_cloudwatch_metric_alarm"; $name) as $planned_alarm |
		$planned_alarm.change.after as $alarm |
		($alarm | has("alarm_actions")) and
		$alarm.alarm_name == ($prefix + "-" + ($name | gsub("_"; "-"))) and
		$alarm.metric_name == $metric and
		$alarm.period == 300 and
		$alarm.evaluation_periods == 1 and
		$alarm.threshold == $threshold and
		$alarm.treat_missing_data == "notBreaching" and
		(
			($planned_alarm.change.after_unknown | has("alarm_actions") | not) or
			$planned_alarm.change.after_unknown.alarm_actions == false
		) and
		($alarm.alarm_actions | normalized_string_array | sort) == ($expected_actions | sort) and
		(if $statistic == "p95" then $alarm.extended_statistic == "p95" else $alarm.statistic == $statistic end);

	.variables.alarm_action_arns.value == $expected_actions and
	.configuration.root_module.module_calls.service.expressions.alarm_action_arns == {
		references: ["var.alarm_action_arns"]
	} and
	configured_alarms == ([
		"api_5xx",
		"api_latency",
		"lambda_duration",
		"lambda_errors",
		"lambda_throttles"
	] | map({
		address: ("aws_cloudwatch_metric_alarm." + .),
		mode: "managed",
		type: "aws_cloudwatch_metric_alarm",
		name: .,
		alarm_actions: {references: ["var.alarm_action_arns"]}
	}) | sort_by(.address)) and
	(resources("aws_iam_role") | length) == 1 and
	all(resources("aws_iam_role")[]; .change.after.name == ($prefix + "-execution") and .change.after.permissions_boundary == $boundary) and
	(resources("aws_iam_role_policy") | length) == 1 and
	all(resources("aws_iam_role_policy")[]; .change.after.name == ($prefix + "-runtime")) and
	(resources("aws_lambda_function") | length) == 1 and
	all(resources("aws_lambda_function")[];
		.change.after.function_name == $prefix and
		.change.after.image_uri == $image and
		.change.after.reserved_concurrent_executions == $reserved_concurrency) and
	(resources("aws_apigatewayv2_api") | length) == 1 and
	all(resources("aws_apigatewayv2_api")[]; .change.after.name == ($prefix + "-http")) and
	(resources("aws_cloudwatch_log_group") | length) == 2 and
	all(resources("aws_cloudwatch_log_group")[];
		.change.after.retention_in_days == $retention and
		(.change.after.name == ("/aws/lambda/" + $prefix) or .change.after.name == ("/aws/apigateway/" + $prefix + "/access"))) and
	(resources("aws_dynamodb_table") | length) == 2 and
	all(resources("aws_dynamodb_table")[];
		.change.after.deletion_protection_enabled == $protection and
		.change.after.point_in_time_recovery[0].enabled == $protection and
		(.change.after.name == ($prefix + "-google-connections") or .change.after.name == ($prefix + "-soccer-sessions"))) and
	(resources("aws_cloudwatch_metric_alarm") | length) == 5 and
	exact_alarm("lambda_errors"; "Errors"; 1; "Sum") and
	exact_alarm("lambda_throttles"; "Throttles"; 1; "Sum") and
	exact_alarm("lambda_duration"; "Duration"; 24000; "p95") and
	exact_alarm("api_5xx"; "5xx"; 1; "Sum") and
	exact_alarm("api_latency"; "Latency"; 25000; "p95")
' "$PLAN_JSON" >/dev/null || fail "environment boundary, names, protection, retention, image, or alarm contract drifted"

printf 'Lambda %s plan contract passed\n' "$ENVIRONMENT"
