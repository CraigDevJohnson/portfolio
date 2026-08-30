#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
checker="$repo_root/scripts/check-lambda-plan.sh"
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

make_artifact_plan() {
	jq -n '{
		resource_changes: [
			{
				address: "aws_ecr_repository.lambda_releases",
				type: "aws_ecr_repository",
				name: "lambda_releases",
				change: {actions: ["create"], after: {
					name: "portfolio-lambda-releases",
					image_tag_mutability: "IMMUTABLE",
					force_delete: false,
					image_scanning_configuration: [{scan_on_push: true}],
					encryption_configuration: [{encryption_type: "AES256"}]
				}, after_sensitive: {}}
			},
			{
				address: "aws_ecr_lifecycle_policy.lambda_releases",
				type: "aws_ecr_lifecycle_policy",
				name: "lambda_releases",
				change: {actions: ["create"], after: {
					repository: "portfolio-lambda-releases",
					policy: ({rules: [{rulePriority: 1, description: "Expire untagged images after 30 days", selection: {tagStatus: "untagged", countType: "sinceImagePushed", countUnit: "days", countNumber: 30}, action: {type: "expire"}}]} | tojson)
				}, after_sensitive: {}}
			},
			{
				address: "aws_ecr_repository_policy.lambda_releases",
				type: "aws_ecr_repository_policy",
				name: "lambda_releases",
				change: {actions: ["create"], after: {
					repository: "portfolio-lambda-releases",
					policy: ({Version: "2012-10-17", Statement: [{Sid: "LambdaPull", Effect: "Allow", Principal: {Service: "lambda.amazonaws.com"}, Action: ["ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"], Condition: {StringEquals: {"aws:SourceAccount": "180294223248"}, ArnLike: {"aws:SourceArn": "arn:aws:lambda:us-west-2:180294223248:function:portfolio-lambda-*"}}}]} | tojson)
				}, after_sensitive: {}}
			}
		]
	}' >"$1"
}

make_environment_plan() {
	output=$1
	environment=$2
	prefix="portfolio-lambda-$environment"
	image_uri="180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if [ "$environment" = prod ]; then
		protection=true
		retention=90
		reserved_concurrency=10
		alarm_actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
	else
		protection=false
		retention=14
		reserved_concurrency=-1
		alarm_actions='[]'
	fi
	jq -n \
		--arg environment "$environment" \
		--arg prefix "$prefix" \
		--arg image "$image_uri" \
		--arg boundary "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" \
		--argjson protection "$protection" \
		--argjson retention "$retention" \
		--argjson reserved_concurrency "$reserved_concurrency" \
		--argjson alarm_actions "$alarm_actions" '
		def change($after): {actions: ["create"], before: null, after: $after, after_unknown: {}, before_sensitive: false, after_sensitive: {}};
		def resource($address; $type; $name; $after): {mode: "managed", address: $address, type: $type, name: $name, change: change($after)};
		def policy_statement($actions; $resources): {
			actions: $actions,
			condition: [],
			effect: null,
			not_actions: null,
			not_principals: [],
			not_resources: null,
			principals: [],
			resources: $resources,
			sid: null
		};
		def policy_statements: [
			policy_statement(["dynamodb:DeleteItem", "dynamodb:GetItem", "dynamodb:PutItem"]; ["arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-google-connections"]),
			policy_statement(["dynamodb:PutItem"]; ["arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-soccer-sessions"]),
			policy_statement(["ssm:GetParameters"]; [
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY",
				"arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"
			]),
			policy_statement(["kms:Decrypt"]; ["arn:aws:kms:us-west-2:180294223248:key/00000000-0000-0000-0000-000000000000"]),
			policy_statement(["logs:CreateLogStream", "logs:PutLogEvents"]; ["arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $prefix + ":*"])
		];
		def policy_statement_expression($actions; $references): {
			actions: {constant_value: $actions},
			resources: {references: $references}
		};
		def policy_statement_expressions: [
			policy_statement_expression(
				["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"];
				["aws_dynamodb_table.google_connections.arn", "aws_dynamodb_table.google_connections"]
			),
			policy_statement_expression(
				["dynamodb:PutItem"];
				["aws_dynamodb_table.soccer_sessions.arn", "aws_dynamodb_table.soccer_sessions"]
			),
			policy_statement_expression(
				["ssm:GetParameters"];
				["local.ssm_paths", "data.aws_partition.current.partition", "data.aws_partition.current", "var.aws_region", "data.aws_caller_identity.current.account_id", "data.aws_caller_identity.current"]
			),
			policy_statement_expression(
				["kms:Decrypt"];
				["data.aws_kms_alias.ssm.target_key_arn", "data.aws_kms_alias.ssm"]
			),
			policy_statement_expression(
				["logs:CreateLogStream", "logs:PutLogEvents"];
				["aws_cloudwatch_log_group.lambda.arn", "aws_cloudwatch_log_group.lambda"]
			)
		];
		def lambda_variables: {
			CLIENT_ID_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_ID_KEY"),
			CLIENT_SECRET_KEY: ("/portfolio/lambda/" + $environment + "/CLIENT_SECRET_KEY"),
			GOOGLE_CONNECTION_TABLE_NAME: ($prefix + "-google-connections"),
			LOG_ADD_SOURCE: "false",
			LOG_FORMAT: "json",
			LOG_LEVEL: "info",
			LPS_SESSION_KEY: ("/portfolio/lambda/" + $environment + "/LPS_SESSION_KEY"),
			SOCCER_SESSION_TABLE_NAME: ($prefix + "-soccer-sessions")
		};
		def alarm($short; $metric; $threshold; $statistic):
			resource("module.service.aws_cloudwatch_metric_alarm." + $short; "aws_cloudwatch_metric_alarm"; $short; {
				alarm_name: ($prefix + "-" + ($short | gsub("_"; "-"))),
				metric_name: $metric,
				period: 300,
				evaluation_periods: 1,
				threshold: $threshold,
				treat_missing_data: "notBreaching",
				alarm_actions: $alarm_actions
			} + (if $statistic == "p95" then {extended_statistic: "p95"} else {statistic: $statistic} end));
		def alarm_configuration($short): {
			address: ("aws_cloudwatch_metric_alarm." + $short),
			mode: "managed",
			type: "aws_cloudwatch_metric_alarm",
			name: $short,
			expressions: {alarm_actions: {references: ["var.alarm_action_arns"]}}
		};
		{
			variables: {alarm_action_arns: {value: $alarm_actions}},
			resource_changes: [
				resource("module.service.aws_iam_role.lambda"; "aws_iam_role"; "lambda"; {name: ($prefix + "-execution"), permissions_boundary: $boundary}),
				(resource("module.service.aws_iam_role_policy.lambda"; "aws_iam_role_policy"; "lambda"; {name: ($prefix + "-runtime")}) | .change.after_unknown = {id: true, name_prefix: true, policy: true, role: true}),
				resource("module.service.aws_lambda_function.app"; "aws_lambda_function"; "app"; {function_name: $prefix, image_uri: $image, reserved_concurrent_executions: $reserved_concurrency, environment: [{variables: lambda_variables}]}),
				resource("module.service.aws_apigatewayv2_api.app"; "aws_apigatewayv2_api"; "app"; {name: ($prefix + "-http")}),
				resource("module.service.aws_cloudwatch_log_group.lambda"; "aws_cloudwatch_log_group"; "lambda"; {name: ("/aws/lambda/" + $prefix), arn: ("arn:aws:logs:us-west-2:180294223248:log-group:/aws/lambda/" + $prefix), retention_in_days: $retention}),
				resource("module.service.aws_cloudwatch_log_group.api_access"; "aws_cloudwatch_log_group"; "api_access"; {name: ("/aws/apigateway/" + $prefix + "/access"), retention_in_days: $retention}),
				resource("module.service.aws_dynamodb_table.google_connections"; "aws_dynamodb_table"; "google_connections"; {name: ($prefix + "-google-connections"), arn: ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-google-connections"), deletion_protection_enabled: $protection, point_in_time_recovery: [{enabled: $protection}]}),
				resource("module.service.aws_dynamodb_table.soccer_sessions"; "aws_dynamodb_table"; "soccer_sessions"; {name: ($prefix + "-soccer-sessions"), arn: ("arn:aws:dynamodb:us-west-2:180294223248:table/" + $prefix + "-soccer-sessions"), deletion_protection_enabled: $protection, point_in_time_recovery: [{enabled: $protection}]}),
				alarm("lambda_errors"; "Errors"; 1; "Sum"),
				alarm("lambda_throttles"; "Throttles"; 1; "Sum"),
				alarm("lambda_duration"; "Duration"; 24000; "p95"),
				alarm("api_5xx"; "5xx"; 1; "Sum"),
				alarm("api_latency"; "Latency"; 25000; "p95"),
				{
					mode: "data",
					address: "module.service.data.aws_iam_policy_document.lambda",
					type: "aws_iam_policy_document",
					name: "lambda",
					change: {
						actions: ["read"],
						before: null,
						after: {
							override_json: null,
							override_policy_documents: null,
							policy_id: null,
							source_json: null,
							source_policy_documents: null,
							statement: policy_statements,
							version: null
						},
						after_unknown: {id: true, json: true, minified_json: true},
						before_sensitive: false,
						after_sensitive: {}
					}
				}
			],
			configuration: {
				root_module: {
					module_calls: {
						service: {
							expressions: {alarm_action_arns: {references: ["var.alarm_action_arns"]}},
							module: {
								resources: [{
									address: "aws_iam_role_policy.lambda",
									mode: "managed",
									type: "aws_iam_role_policy",
									name: "lambda",
									expressions: {
										policy: {references: ["data.aws_iam_policy_document.lambda.json", "data.aws_iam_policy_document.lambda"]}
									}
								}, {
									address: "data.aws_iam_policy_document.lambda",
									mode: "data",
									type: "aws_iam_policy_document",
									name: "lambda",
									expressions: {statement: policy_statement_expressions}
								}, {
									address: "data.aws_kms_alias.ssm",
									mode: "data",
									type: "aws_kms_alias",
									name: "ssm",
									expressions: {name: {constant_value: "alias/aws/ssm"}}
								},
								alarm_configuration("lambda_errors"),
								alarm_configuration("lambda_throttles"),
								alarm_configuration("lambda_duration"),
								alarm_configuration("api_5xx"),
								alarm_configuration("api_latency")
								]
							}
						}
					}
				}
			}
		}' >"$output"
}

run_check() {
	plan=$1
	environment=$2
	if [ "$environment" = artifacts ]; then
		prefix=portfolio-lambda-releases
		actions='[]'
	else
		prefix="portfolio-lambda-$environment"
		if [ "$environment" = prod ]; then
			actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
		else
			actions='[]'
		fi
	fi
	PLAN_JSON="$plan" \
		ENVIRONMENT="$environment" \
		NAME_PREFIX="$prefix" \
		IMAGE_URI="180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
		EXPECTED_ALARM_ACTIONS_JSON="$actions" \
		sh "$checker"
}

artifact_plan="$tmp_dir/artifact.json"
dev_plan="$tmp_dir/dev.json"
prod_plan="$tmp_dir/prod.json"
make_artifact_plan "$artifact_plan"
make_environment_plan "$dev_plan" dev
make_environment_plan "$prod_plan" prod

dev_known_policy_plan="$tmp_dir/dev-known-policy.json"
jq '
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.statement) as $statements |
	({
		Version: "2012-10-17",
		Statement: [$statements[] | {
			Effect: "Allow",
			Action: (if (.actions | length) == 1 then .actions[0] else .actions end),
			Resource: (if (.resources | length) == 1 then .resources[0] else .resources end)
		}]
	} | tojson) as $policy |
	(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after.policy) = $policy |
	del(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after_unknown.policy)
' "$dev_plan" >"$dev_known_policy_plan"

dev_converged_runtime_policy_plan="$tmp_dir/dev-converged-runtime-policy.json"
jq '
	del(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda")) |
	(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= (
		.after.policy as $policy |
		.actions = ["no-op"] |
		.after = {
			id: "portfolio-lambda-dev-execution:portfolio-lambda-dev-runtime",
			name: "portfolio-lambda-dev-runtime",
			name_prefix: "",
			policy: $policy,
			role: "portfolio-lambda-dev-execution"
		} |
		.before = .after |
		.after_unknown = {}
	)
' "$dev_known_policy_plan" >"$dev_converged_runtime_policy_plan"

dev_empty_policy_composition_plan="$tmp_dir/dev-empty-policy-composition.json"
jq '
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after) |= (
		.source_json = "" |
		.override_json = "" |
		.source_policy_documents = [] |
		.override_policy_documents = []
	)
' "$dev_plan" >"$dev_empty_policy_composition_plan"

dev_deferred_runtime_policy_plan="$tmp_dir/dev-deferred-runtime-policy.json"
jq '
	(.resource_changes[] | select(
		.address == "module.service.aws_dynamodb_table.google_connections" or
		.address == "module.service.aws_dynamodb_table.soccer_sessions" or
		.address == "module.service.aws_cloudwatch_log_group.lambda"
	) | .change) |= (
		.after.arn = null |
		.after_unknown.arn = true
	) |
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change) |= (
		.after.statement[0].resources = [null] |
		.after.statement[1].resources = [null] |
		.after.statement[4].resources = [null] |
		.after_unknown.statement = [{
			actions: [false, false, false],
			condition: [],
			not_principals: [],
			principals: [],
			resources: [true]
		}, {
			actions: [false],
			condition: [],
			not_principals: [],
			principals: [],
			resources: [true]
		}, {
			actions: [false],
			condition: [],
			not_principals: [],
			principals: [],
			resources: [false, false, false]
		}, {
			actions: [false],
			condition: [],
			not_principals: [],
			principals: [],
			resources: [false]
		}, {
			actions: [false, false],
			condition: [],
			not_principals: [],
			principals: [],
			resources: [true]
		}]
	) |
	(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after_unknown) = {
		id: true,
		name_prefix: true,
		policy: true,
		role: true
	}
' "$dev_plan" >"$dev_deferred_runtime_policy_plan"

dev_partial_state_runtime_policy_plan="$tmp_dir/dev-partial-state-runtime-policy.json"
jq '
	(.resource_changes[] | select(.address == "module.service.aws_cloudwatch_log_group.lambda") | .change) |= (
		.after.arn = null |
		.after_unknown.arn = true
	) |
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change) |= (
		.after.statement[4].resources = [null] |
		.after_unknown.statement = [
			{actions: [false, false, false], condition: [], not_principals: [], principals: [], resources: [false]},
			{actions: [false], condition: [], not_principals: [], principals: [], resources: [false]},
			{actions: [false], condition: [], not_principals: [], principals: [], resources: [false, false, false]},
			{actions: [false], condition: [], not_principals: [], principals: [], resources: [false]},
			{actions: [false, false], condition: [], not_principals: [], principals: [], resources: [true]}
		]
	) |
	(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= (
		.after.role = "portfolio-lambda-dev-execution" |
		.after_unknown = {id: true, name_prefix: true, policy: true}
	)
' "$dev_plan" >"$dev_partial_state_runtime_policy_plan"

dev_provider_empty_alarm_actions_plan="$tmp_dir/dev-provider-empty-alarm-actions.json"
jq '
	(.resource_changes[] | select(.type == "aws_cloudwatch_metric_alarm") | .change) |= (
		.after.alarm_actions = null |
		del(.after_unknown.alarm_actions)
	)
' "$dev_plan" >"$dev_provider_empty_alarm_actions_plan"

dev_null_acm_private_key_plan="$tmp_dir/dev-null-acm-private-key.json"
jq '.resource_changes += [{
	mode: "managed",
	address: "module.service.aws_acm_certificate.custom[0]",
	type: "aws_acm_certificate",
	name: "custom",
	change: {
		actions: ["create"],
		before: null,
		after: {
			domain_name: "dev.craigdevjohnson.com",
			private_key: null
		},
		after_unknown: {},
		before_sensitive: false,
		after_sensitive: {private_key: true}
	}
}]' "$dev_plan" >"$dev_null_acm_private_key_plan"

expect_pass "artifact repository, lifecycle, and pull-policy plan" run_check "$artifact_plan" artifacts
expect_pass "development replacement plan" run_check "$dev_plan" dev
expect_pass "production replacement plan" run_check "$prod_plan" prod
expect_pass "development plan with decoded runtime policy" run_check "$dev_known_policy_plan" dev
expect_pass "development plan with converged runtime policy" run_check "$dev_converged_runtime_policy_plan" dev
expect_pass "development plan with empty policy composition inputs" run_check "$dev_empty_policy_composition_plan" dev
expect_pass "development plan with provider-deferred runtime resources" run_check "$dev_deferred_runtime_policy_plan" dev
expect_pass "development plan after partial role and table creation" run_check "$dev_partial_state_runtime_policy_plan" dev
expect_pass "development plan with provider-normalized empty alarm actions" run_check "$dev_provider_empty_alarm_actions_plan" dev
expect_pass "ACM plan with null provider-sensitive private key" run_check "$dev_null_acm_private_key_plan" dev

mutate_and_reject() {
	name=$1
	source=$2
	filter=$3
	mutated="$tmp_dir/mutated.json"
	jq "$filter" "$source" >"$mutated"
	environment=dev
	case "$source" in
		*artifact*) environment=artifacts ;;
		*prod*) environment=prod ;;
	esac
	expect_fail "$name" run_check "$mutated" "$environment"
}

mutate_and_reject "delete action" "$dev_plan" '.resource_changes[0].change.actions = ["delete"]'
mutate_and_reject "replace action" "$dev_plan" '.resource_changes[0].change.actions = ["delete", "create"]'
mutate_and_reject "legacy App Runner address" "$dev_plan" '.resource_changes[0].address = "module.service.aws_apprunner_service.legacy"'
mutate_and_reject "legacy resource name" "$dev_plan" '.resource_changes[2].change.after.function_name = "portfolio"'
mutate_and_reject "mutable image tag" "$dev_plan" '.resource_changes[2].change.after.image_uri = "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases:latest"'
mutate_and_reject "development reserved concurrency drift" "$dev_plan" '.resource_changes[2].change.after.reserved_concurrent_executions = 5'
mutate_and_reject "production reserved concurrency drift" "$prod_plan" '.resource_changes[2].change.after.reserved_concurrent_executions = -1'
mutate_and_reject "secret plan value" "$dev_plan" '.resource_changes[2].change.after.oauth_token = "do-not-store-this"'
mutate_and_reject "secret prior-state value" "$dev_plan" '.resource_changes[2].change.before = {oauth_token: "do-not-store-this"}'
mutate_and_reject "ACM plan with non-null sensitive private key" "$dev_null_acm_private_key_plan" '(.resource_changes[] | select(.type == "aws_acm_certificate") | .change.after.private_key) = "do-not-store-this"'
mutate_and_reject "ACM plan with unmarked non-null private key" "$dev_null_acm_private_key_plan" '(.resource_changes[] | select(.type == "aws_acm_certificate") | .change) |= (.after.private_key = "do-not-store-this" | del(.after_sensitive.private_key))'
mutate_and_reject "ACM plan with missing sensitive private key" "$dev_null_acm_private_key_plan" '(.resource_changes[] | select(.type == "aws_acm_certificate") | .change) |= del(.after.private_key)'
mutate_and_reject "ACM plan with deferred sensitive private key" "$dev_null_acm_private_key_plan" '(.resource_changes[] | select(.type == "aws_acm_certificate") | .change.after_unknown.private_key) = true'
mutate_and_reject "ACM plan with unexpected null sensitive field" "$dev_null_acm_private_key_plan" '(.resource_changes[] | select(.type == "aws_acm_certificate") | .change) |= (.after.unreviewed = null | .after_sensitive.unreviewed = true)'
mutate_and_reject "root sensitive marker" "$dev_plan" '.resource_changes[0].change.after_sensitive = true'
mutate_and_reject "missing execution boundary" "$dev_plan" '.resource_changes[0].change.after.permissions_boundary = null'
mutate_and_reject "wrong deterministic API name" "$dev_plan" '.resource_changes[3].change.after.name = "portfolio-http"'
mutate_and_reject "development table protection drift" "$dev_plan" '.resource_changes[6].change.after.deletion_protection_enabled = true'
mutate_and_reject "production table protection drift" "$prod_plan" '.resource_changes[6].change.after.deletion_protection_enabled = false'
mutate_and_reject "log retention drift" "$dev_plan" '.resource_changes[4].change.after.retention_in_days = 7'
mutate_and_reject "alarm action mismatch" "$prod_plan" '.resource_changes[8].change.after.alarm_actions = []'
mutate_and_reject "unknown empty alarm actions" "$dev_provider_empty_alarm_actions_plan" '.resource_changes[8].change.after_unknown.alarm_actions = true'
mutate_and_reject "malformed empty alarm actions" "$dev_provider_empty_alarm_actions_plan" '.resource_changes[8].change.after.alarm_actions = false'
mutate_and_reject "missing empty alarm actions" "$dev_provider_empty_alarm_actions_plan" 'del(.resource_changes[8].change.after.alarm_actions)'
mutate_and_reject "drifted root alarm action value" "$dev_provider_empty_alarm_actions_plan" '.variables.alarm_action_arns.value = null'
mutate_and_reject "altered root alarm action reference" "$dev_provider_empty_alarm_actions_plan" '.configuration.root_module.module_calls.service.expressions.alarm_action_arns.references = ["var.unreviewed_alarm_actions"]'
mutate_and_reject "altered resource alarm action reference" "$dev_provider_empty_alarm_actions_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "aws_cloudwatch_metric_alarm.lambda_errors") | .expressions.alarm_actions.references) = ["var.unreviewed_alarm_actions"]'
mutate_and_reject "alarm threshold drift" "$dev_plan" '.resource_changes[10].change.after.threshold = 29000'
mutate_and_reject "extra artifact resource" "$artifact_plan" '.resource_changes += [{address: "aws_iam_role.legacy", type: "aws_iam_role", name: "legacy", change: {actions: ["create"], after: {name: "legacy"}, after_sensitive: {}}}]'
mutate_and_reject "artifact lifecycle drift" "$artifact_plan" '.resource_changes[1].change.after.policy = ({rules: []} | tojson)'
mutate_and_reject "artifact repository-policy drift" "$artifact_plan" '.resource_changes[2].change.after.policy = ({Version: "2012-10-17", Statement: []} | tojson)'
mutate_and_reject "unapproved IAM user resource" "$dev_plan" '.resource_changes += [{mode: "managed", address: "module.service.aws_iam_user.unapproved", type: "aws_iam_user", name: "unapproved", change: {actions: ["create"], after: {name: "unapproved"}, after_sensitive: {}}}]'
mutate_and_reject "unapproved SSM parameter resource" "$dev_plan" '.resource_changes += [{mode: "managed", address: "module.service.aws_ssm_parameter.unapproved", type: "aws_ssm_parameter", name: "unapproved", change: {actions: ["create"], after: {name: "/portfolio/lambda/dev/unapproved"}, after_sensitive: {}}}]'

mutate_and_reject "development plan cannot use production runtime paths" "$dev_plan" '
	(.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change.after.environment[0].variables) |= with_entries(
		if (.key == "CLIENT_ID_KEY" or .key == "CLIENT_SECRET_KEY" or .key == "LPS_SESSION_KEY") then .value |= sub("/dev/"; "/prod/") else . end
	) |
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.statement[] | select(.actions == ["ssm:GetParameters"]) | .resources[]) |= sub("/dev/"; "/prod/")'
mutate_and_reject "runtime policy rejects wildcard SSM resources" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.statement[] | select(.actions == ["ssm:GetParameters"]) | .resources) = ["arn:aws:ssm:us-west-2:180294223248:parameter/portfolio/lambda/dev/*"]'
mutate_and_reject "runtime policy rejects altered actions" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.statement[] | select(.actions == ["ssm:GetParameters"]) | .actions) = ["ssm:GetParameter", "ssm:GetParameters"]'
mutate_and_reject "runtime policy rejects an unbound deferred value" "$dev_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "aws_iam_role_policy.lambda") | .expressions.policy.references) = ["var.unreviewed_policy"]'
mutate_and_reject "Lambda environment rejects an extra variable" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.aws_lambda_function.app") | .change.after.environment[0].variables.UNREVIEWED) = "value"'
mutate_and_reject "decoded runtime policy rejects altered actions" "$dev_known_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after.policy) |= (fromjson | (.Statement[] | select(.Action == "ssm:GetParameters") | .Action) = ["ssm:GetParameter", "ssm:GetParameters"] | tojson)'
mutate_and_reject "converged runtime policy rejects altered actions" "$dev_converged_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= ((.after.policy |= (fromjson | (.Statement[] | select(.Action == "ssm:GetParameters") | .Action) = ["ssm:GetParameter", "ssm:GetParameters"] | tojson)) | .before.policy = .after.policy)'
mutate_and_reject "converged runtime policy rejects mismatched prior state" "$dev_converged_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.before.role) = "unreviewed-role"'
mutate_and_reject "converged runtime policy rejects partial unknown fields without its data read" "$dev_converged_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change) |= (.before.role = "unreviewed-role" | .after = {name: .after.name, policy: .after.policy} | .after_unknown = {id: true, name_prefix: true, role: true})'
mutate_and_reject "runtime policy rejects injected source policy documents" "$dev_plan" '
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.source_policy_documents) = [({Version: "2012-10-17", Statement: [{Effect: "Allow", Action: "kms:Decrypt", Resource: "*"}]} | tojson)] |
	(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.source_policy_documents) = {constant_value: [({Version: "2012-10-17", Statement: [{Effect: "Allow", Action: "kms:Decrypt", Resource: "*"}]} | tojson)]}'
mutate_and_reject "runtime policy rejects injected source JSON" "$dev_plan" '
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.source_json) = ({Version: "2012-10-17", Statement: []} | tojson) |
	(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.source_json) = {constant_value: "injected"}'
mutate_and_reject "runtime policy rejects injected override JSON" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.override_json) = ({Version: "2012-10-17", Statement: []} | tojson)'
mutate_and_reject "runtime policy rejects injected override policy documents" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.override_policy_documents) = [({Version: "2012-10-17", Statement: []} | tojson)]'
mutate_and_reject "runtime policy rejects unknown source or override composition" "$dev_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after_unknown) += {source_json: true, override_json: true, source_policy_documents: true, override_policy_documents: true}'
mutate_and_reject "runtime policy rejects configured override composition" "$dev_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.override_policy_documents) = {references: ["var.unreviewed_policy"]}'
mutate_and_reject "runtime policy rejects duplicate KMS alias configuration" "$dev_plan" '.configuration.root_module.module_calls.service.module.resources += [{address: "data.aws_kms_alias.ssm", mode: "data", type: "aws_kms_alias", name: "ssm", expressions: {name: {constant_value: "alias/customer-managed"}}}]'
mutate_and_reject "runtime policy rejects changed KMS alias and resource" "$dev_plan" '
	(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after.statement[] | select(.actions == ["kms:Decrypt"]) | .resources) = ["arn:aws:kms:us-west-2:180294223248:key/11111111-1111-1111-1111-111111111111"] |
	(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_kms_alias.ssm") | .expressions.name.constant_value) = "alias/customer-managed"'
mutate_and_reject "runtime policy rejects changed structured KMS reference" "$dev_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.statement[] | select(.actions.constant_value == ["kms:Decrypt"]) | .resources.references) = ["var.unreviewed_kms_key"]'
mutate_and_reject "runtime policy rejects changed structured action" "$dev_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.statement[] | select(.actions.constant_value == ["ssm:GetParameters"]) | .actions.constant_value) = ["ssm:GetParameter", "ssm:GetParameters"]'
mutate_and_reject "runtime policy rejects a deferred ARN without its provider unknown marker" "$dev_deferred_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_dynamodb_table.google_connections") | .change.after_unknown.arn) = false'
mutate_and_reject "runtime policy rejects a deferred policy resource without its paired unknown marker" "$dev_deferred_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after_unknown.statement[0].resources) = [false]'
mutate_and_reject "runtime policy rejects a deferred statement effect" "$dev_deferred_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.data.aws_iam_policy_document.lambda") | .change.after_unknown.statement[0].effect) = true'
mutate_and_reject "runtime policy rejects extra deferred inline-policy fields" "$dev_deferred_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after_unknown.unreviewed) = true'
mutate_and_reject "runtime policy rejects an unbound deferred table ARN" "$dev_deferred_runtime_policy_plan" '(.configuration.root_module.module_calls.service.module.resources[] | select(.address == "data.aws_iam_policy_document.lambda") | .expressions.statement[] | select(.actions.constant_value == ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]) | .resources.references) = ["var.unreviewed_table_arn"]'
mutate_and_reject "partial runtime policy rejects the wrong known role" "$dev_partial_state_runtime_policy_plan" '(.resource_changes[] | select(.address == "module.service.aws_iam_role_policy.lambda") | .change.after.role) = "unreviewed-role"'

dev_domain_plan="$tmp_dir/dev-domain.json"
jq '.resource_changes += [
	{mode: "managed", address: "module.service.aws_acm_certificate.custom[0]", type: "aws_acm_certificate", name: "custom", change: {actions: ["create"], after: {domain_name: "dev.craigdevjohnson.com"}, after_sensitive: {}}},
	{mode: "managed", address: "module.service.aws_acm_certificate_validation.custom[0]", type: "aws_acm_certificate_validation", name: "custom", change: {actions: ["create"], after: {}, after_sensitive: {}}},
	{mode: "managed", address: "module.service.aws_apigatewayv2_domain_name.custom[\"dev.craigdevjohnson.com\"]", type: "aws_apigatewayv2_domain_name", name: "custom", change: {actions: ["create"], after: {domain_name: "dev.craigdevjohnson.com"}, after_sensitive: {}}},
	{mode: "managed", address: "module.service.aws_apigatewayv2_api_mapping.custom[\"dev.craigdevjohnson.com\"]", type: "aws_apigatewayv2_api_mapping", name: "custom", change: {actions: ["create"], after: {}, after_sensitive: {}}}
]' "$dev_plan" >"$dev_domain_plan"
expect_pass "approved conditional development domain resources" run_check "$dev_domain_plan" dev
mutate_and_reject "unapproved conditional domain address" "$dev_domain_plan" '.resource_changes[-1].address = "module.service.aws_apigatewayv2_api_mapping.custom[\"attacker.example\"]"'

expect_pass "exact reviewed IAM policy-document data read" run_check "$dev_plan" dev
mutate_and_reject "unapproved data-source address" "$dev_plan" '(.resource_changes[] | select(.mode == "data") | .address) = "module.service.data.aws_iam_policy_document.unapproved"'
mutate_and_reject "unapproved data-source type" "$dev_plan" '(.resource_changes[] | select(.mode == "data") | .type) = "aws_caller_identity"'
mutate_and_reject "data source presented as a managed resource" "$dev_plan" '(.resource_changes[] | select(.mode == "data") | .mode) = "managed"'
mutate_and_reject "data source with a non-read action" "$dev_plan" '(.resource_changes[] | select(.mode == "data") | .change.actions) = ["create"]'

documented_versioning_mutation_is_guarded() {
	document=$1
	perl -0ne '
		my $found = 0;
		while (/```bash\n(.*?)```/sg) {
			my $block = $1;
			next unless $block =~ /s3api put-bucket-versioning/;
			exit 1 unless $block =~ /task lambda-artifacts-init.*s3api put-bucket-versioning/s;
			$found = 1;
		}
		exit($found ? 0 : 1);
	' "$document"
}

expect_pass "deployment guide guards bucket versioning immediately before mutation" documented_versioning_mutation_is_guarded "$repo_root/DEPLOY-INSTRUCTIONS.md"
expect_pass "authoritative plan guards bucket versioning immediately before mutation" documented_versioning_mutation_is_guarded "$repo_root/docs/superpowers/plans/2026-08-21-development-lambda-cutover.md"

documented_parameter_streams_are_fail_closed() {
	document=$1
	perl -0ne '
		my ($task) = /(### Task 10:.*?)(?=\n---\n\n### Task 11:)/s;
		exit 1 unless defined $task;
		exit 1 if $task =~ /--cli-input-json file:\/\/\/dev\/stdin/;

		my @blocks = grep { /ssm put-parameter/ } ($task =~ /```bash\n(.*?)```/sg);
		exit 1 unless @blocks == 2;

		for my $block (@blocks) {
			exit 1 unless $block =~ /^set -euo pipefail$/m;
			exit 1 unless $block =~ /^set \+x$/m;
			exit 1 unless $block =~ /aws --profile "\$AWS_PROFILE" configure get cli_history/;
			exit 1 unless $block =~ /\nassert_aws_cli_history_disabled\n.*(?:openssl rand|--with-decryption)/s;
			exit 1 unless (() = $block =~ /ssm put-parameter/g) == 1;
			exit 1 unless $block =~ /--no-overwrite/;
			exit 1 unless $block =~ /--value file:\/\/\/dev\/stdin/;
		}

		my ($session) = grep { /openssl rand/ } @blocks;
		my ($oauth) = grep { /--with-decryption/ } @blocks;
		exit 1 unless defined $session && defined $oauth;
		exit 1 unless $session =~ /--name \/portfolio\/lambda\/dev\/LPS_SESSION_KEY/;
		exit 1 unless $oauth =~ /for name in CLIENT_ID_KEY CLIENT_SECRET_KEY; do/;
		exit 1 unless $oauth =~ /--name "\/portfolio\/lambda\/dev\/\$name"/;
		exit 1 unless $oauth =~ /--name "\/portfolio\/\$name"/;
		exit 0;
	' "$document"
}

expect_pass "parameter streams are non-overwriting and fail closed on CLI history" documented_parameter_streams_are_fail_closed "$repo_root/docs/superpowers/plans/2026-08-21-development-lambda-cutover.md"

fake_bin="$tmp_dir/fake-bin"
mkdir "$fake_bin"
command_log="$tmp_dir/commands.log"
: >"$command_log"

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*"sts get-caller-identity"*"--query Arn"*)
		printf '%s\n' "${FAKE_ARN:-arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_abc/craig}"
		;;
	*"sts get-caller-identity"*"--query Account"*)
		printf '%s\n' "${FAKE_ACCOUNT:-180294223248}"
		;;
	*"ecr get-login-password"*)
		printf 'fake-password\n'
		;;
	*"ecr describe-repositories"*)
		printf '%s\n' "${FAKE_REPOSITORY_MUTABILITY:-IMMUTABLE}"
		;;
	*"ecr wait image-scan-complete"*)
		case "${FAKE_WAITER_MODE:-complete}" in
			complete) ;;
			denied)
				printf 'An error occurred (AccessDeniedException) while waiting for image scan completion: denied\n' >&2
				exit 254
				;;
			failed)
				printf 'Waiter ImageScanComplete failed: terminal scan status FAILED\n' >&2
				exit 255
				;;
			*)
				printf 'invalid FAKE_WAITER_MODE\n' >&2
				exit 1
				;;
		esac
		;;
	*"ecr describe-image-scan-findings"*)
		case "${FAKE_SCAN_MODE:-complete}" in
			complete)
				printf '%s\n' '{"ScanStatus":"COMPLETE","FindingSeverityCounts":{"CRITICAL":0,"HIGH":0}}'
				;;
			missing-once)
				if [ ! -f "$FAKE_SCAN_LOOKUP_STATE" ]; then
					: >"$FAKE_SCAN_LOOKUP_STATE"
					printf 'An error occurred (ScanNotFoundException) when calling the DescribeImageScanFindings operation: scan does not exist yet\n' >&2
					exit 254
				fi
				printf '%s\n' '{"ScanStatus":"COMPLETE","FindingSeverityCounts":{}}'
				;;
			missing)
				printf 'An error occurred (ScanNotFoundException) when calling the DescribeImageScanFindings operation: scan does not exist yet\n' >&2
				exit 254
				;;
			ambiguous)
				printf 'AccessDeniedException included the words ScanNotFoundException\n' >&2
				exit 254
				;;
			denied)
				printf 'An error occurred (AccessDeniedException) when calling the DescribeImageScanFindings operation: denied\n' >&2
				exit 254
				;;
			failed)
				printf '%s\n' '{"ScanStatus":"FAILED","FindingSeverityCounts":{}}'
				;;
			*)
				printf 'invalid FAKE_SCAN_MODE\n' >&2
				exit 1
				;;
		esac
		;;
	*"ecr describe-images"*"{Digest:imageDigest"*)
		printf '%s\n' '{"Digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","PushedAt":"2026-08-22T00:00:00Z","ScanStatus":"COMPLETE"}'
		;;
	*"ecr describe-images"*"imageDigest"*)
		if [ ! -f "$FAKE_LOOKUP_STATE" ]; then
			: >"$FAKE_LOOKUP_STATE"
			case "${FAKE_LOOKUP_MODE:-absent}" in
				absent)
					printf 'An error occurred (ImageNotFoundException) when calling the DescribeImages operation: image does not exist\n' >&2
					exit 254
					;;
				denied)
					printf 'An error occurred (AccessDeniedException) when calling the DescribeImages operation: denied\n' >&2
					exit 254
					;;
				ambiguous)
					printf 'AccessDeniedException included the words ImageNotFoundException\n' >&2
					exit 254
					;;
				existing) printf '%s\n' "${FAKE_EXISTING_DIGEST:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" ;;
				*) printf 'invalid FAKE_LOOKUP_MODE\n' >&2; exit 1 ;;
			esac
		else
			printf '%s\n' "${FAKE_PUSHED_DIGEST:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
		fi
		;;
	*"ecr describe-images"*)
		printf '%s\n' '{"Digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","PushedAt":"2026-08-22T00:00:00Z","ScanStatus":"COMPLETE"}'
		;;
	*)
		printf 'unexpected fake aws command: %s\n' "$*" >&2
		exit 1
		;;
esac
EOF

cat >"$fake_bin/tofu" <<'EOF'
#!/bin/sh
set -eu
printf 'tofu %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*" init "*) ;;
	*" workspace show"*) printf 'default\n' ;;
	*" plan "*)
		for argument in "$@"; do
			case "$argument" in
				-out=*) : >"${argument#-out=}" ;;
			esac
		done
		;;
	*" show -json "*) cat "$FAKE_PLAN_JSON" ;;
	*" show -no-color "*) printf 'synthetic human-readable plan\n' ;;
	*" apply "*) ;;
	*" output -raw ecr_repository_url"*) printf '180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases\n' ;;
	*)
		printf 'unexpected fake tofu command: %s\n' "$*" >&2
		exit 1
		;;
esac
EOF

cat >"$fake_bin/git" <<'EOF'
#!/bin/sh
set -eu
printf 'git %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	"status --porcelain") ;;
	"rev-parse HEAD") printf '0123456789abcdef0123456789abcdef01234567\n' ;;
	*) /usr/bin/git "$@" ;;
esac
EOF

cat >"$fake_bin/docker" <<'EOF'
#!/bin/sh
set -eu
printf 'docker %s\n' "$*" >>"$COMMAND_LOG"
case "$1" in
	login) cat >/dev/null ;;
	tag) ;;
	push) [ "${FAKE_PUSH_FAIL:-false}" != true ] ;;
	*) printf 'unexpected fake docker command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/task" <<'EOF'
#!/bin/sh
set -eu
printf 'task %s\n' "$*" >>"$COMMAND_LOG"
test "$1" = build-lambda-image
EOF
cat >"$fake_bin/sleep" <<'EOF'
#!/bin/sh
set -eu
printf 'sleep %s\n' "$*" >>"$COMMAND_LOG"
EOF
chmod +x "$fake_bin"/*

real_task=$(command -v task)
lookup_state="$tmp_dir/ecr-lookup-state"
scan_lookup_state="$tmp_dir/ecr-scan-lookup-state"
run_task() {
	rm -f "$lookup_state"
	rm -f "$scan_lookup_state"
	env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
		PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		FAKE_PLAN_JSON="${TASK7_PLAN_JSON:-$dev_plan}" \
		FAKE_EXISTING_DIGEST="${FAKE_EXISTING_DIGEST:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" \
		FAKE_LOOKUP_MODE="${FAKE_LOOKUP_MODE:-absent}" \
		FAKE_LOOKUP_STATE="$lookup_state" \
		FAKE_PUSHED_DIGEST="${FAKE_PUSHED_DIGEST:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" \
		FAKE_PUSH_FAIL="${FAKE_PUSH_FAIL:-false}" \
		FAKE_REPOSITORY_MUTABILITY="${FAKE_REPOSITORY_MUTABILITY:-IMMUTABLE}" \
		FAKE_SCAN_MODE="${FAKE_SCAN_MODE:-complete}" \
		FAKE_SCAN_LOOKUP_STATE="$scan_lookup_state" \
		FAKE_WAITER_MODE="${FAKE_WAITER_MODE:-complete}" \
		AWS_PROFILE=portfolio-deployer \
		AWS_REGION=us-west-2 \
		"$real_task" --dir "$repo_root" "$@"
}

expect_pass "exact SSO identity guard" run_task lambda-dev-init
expect_fail "identity guard rejects wrong profile" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" AWS_PROFILE=default AWS_REGION=us-west-2 "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects wrong region" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" AWS_PROFILE=portfolio-deployer AWS_REGION=us-east-1 "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects ambient static credentials" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 AWS_ACCESS_KEY_ID=AKIASTATIC "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects root ARN" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" FAKE_ARN=arn:aws:iam::180294223248:root AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 "$real_task" --dir "$repo_root" lambda-dev-init
expect_fail "identity guard rejects wrong SSO role" env PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" FAKE_ARN=arn:aws:sts::180294223248:assumed-role/OtherRole/craig AWS_PROFILE=portfolio-deployer AWS_REGION=us-west-2 "$real_task" --dir "$repo_root" lambda-dev-init

expect_pass "development backend init verifies default workspace" run_task lambda-dev-init
plan_file="$tmp_dir/dev.tfplan"
expect_fail "plan requires an absolute path" run_task lambda-dev-plan PLAN_FILE=relative.tfplan IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
expect_fail "plan rejects the wrong lock acknowledgement" run_task lambda-dev-plan PLAN_FILE="$plan_file" IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/wrong.tflock
expect_pass "development plan accepts only its exact lock acknowledgement" run_task lambda-dev-plan PLAN_FILE="$plan_file" IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
expect_fail "plan refuses an existing saved-plan path" run_task lambda-dev-plan PLAN_FILE="$plan_file" IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
expect_fail "apply rejects the wrong lock acknowledgement" run_task lambda-dev-apply PLAN_FILE="$plan_file" APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/wrong.tflock
approved_plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
expect_pass "apply consumes only the checksum-bound saved plan" run_task lambda-dev-apply PLAN_FILE="$plan_file" APPROVED_PLAN_SHA256="$approved_plan_sha256" APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock
printf 'replacement plan bytes\n' >"$plan_file"
expect_fail "apply rejects a plan replaced at the approved path" run_task lambda-dev-apply PLAN_FILE="$plan_file" APPROVED_PLAN_SHA256="$approved_plan_sha256" APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/dev/terraform.tfstate.tflock

artifact_plan_file="$tmp_dir/artifacts.tfplan"
TASK7_PLAN_JSON="$artifact_plan" expect_pass "artifact plan accepts only its exact lock acknowledgement" run_task lambda-artifacts-plan PLAN_FILE="$artifact_plan_file" APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/artifacts/terraform.tfstate.tflock
prod_plan_file="$tmp_dir/prod.tfplan"
TASK7_PLAN_JSON="$prod_plan" expect_pass "production plan accepts only its exact lock acknowledgement" run_task lambda-prod-plan PLAN_FILE="$prod_plan_file" IMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ALARM_ACTION_ARNS_JSON='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]' APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio-lambda-http-api/prod/terraform.tfstate.tflock

expect_pass "immutable full-SHA release push" run_task lambda-release-push
grep -F 'ecr wait image-scan-complete --repository-name portfolio-lambda-releases --image-id imageTag=git-0123456789abcdef0123456789abcdef01234567' "$command_log" >/dev/null || {
	printf 'FAIL: release did not wait for the current ECR scan to complete\n' >&2
	exit 1
}
pass "release waits for the current ECR scan"
grep -F 'ecr describe-image-scan-findings --repository-name portfolio-lambda-releases --image-id imageTag=git-0123456789abcdef0123456789abcdef01234567' "$command_log" >/dev/null || {
	printf 'FAIL: release did not retrieve findings through the current ECR scan API\n' >&2
	exit 1
}
pass "release uses the current ECR scan findings API"
scan_metadata_calls=$(grep ' ecr describe-image-scan-findings ' "$command_log")
if [ -z "$scan_metadata_calls" ] || printf '%s\n' "$scan_metadata_calls" | grep -v -- ' --no-paginate ' >/dev/null; then
	printf 'FAIL: release allowed pagination while reading ECR scan metadata\n' >&2
	exit 1
fi
pass "release disables pagination for ECR scan metadata reads"
sleeps_before=$(grep -c '^sleep 5$' "$command_log" || true)
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=missing-once expect_pass "release tolerates one initial missing scan record" run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 1 || {
	printf 'FAIL: release did not retry a one-time missing scan exactly once\n' >&2
	exit 1
}
pass "release retries a one-time missing scan exactly once"
sleeps_before=$sleeps_after
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=missing expect_fail "release bounds a persistently missing scan record" run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 11 || {
	printf 'FAIL: release did not use the exact bounded scan-discovery wait\n' >&2
	exit 1
}
pass "release uses exactly eleven bounded scan-discovery sleeps"
FAKE_WAITER_MODE=denied FAKE_SCAN_MODE=complete expect_fail "release fails closed when the scan waiter is denied" run_task lambda-release-push
FAKE_WAITER_MODE=failed FAKE_SCAN_MODE=complete expect_fail "release fails closed when the scan waiter reports failure" run_task lambda-release-push
sleeps_before=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_before=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=denied expect_fail "release fails closed when scan findings are unreadable" run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_after=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 0 && test "$((scan_lookups_after - scan_lookups_before))" -eq 1 || {
	printf 'FAIL: release did not fail immediately on a denied scan lookup\n' >&2
	exit 1
}
pass "release performs one lookup and no sleep after scan denial"
sleeps_before=$sleeps_after
scan_lookups_before=$scan_lookups_after
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=ambiguous expect_fail "release rejects an ambiguous scan lookup error" run_task lambda-release-push
sleeps_after=$(grep -c '^sleep 5$' "$command_log" || true)
scan_lookups_after=$(grep -c ' ecr describe-image-scan-findings ' "$command_log" || true)
test "$((sleeps_after - sleeps_before))" -eq 0 && test "$((scan_lookups_after - scan_lookups_before))" -eq 1 || {
	printf 'FAIL: release retried an ambiguous scan lookup error\n' >&2
	exit 1
}
pass "release performs one lookup and no sleep for ambiguous scan errors"
FAKE_WAITER_MODE=complete FAKE_SCAN_MODE=failed expect_fail "release fails closed when the findings status is not complete" run_task lambda-release-push
pushes_before=$(grep -c '^docker push ' "$command_log" || true)
FAKE_LOOKUP_MODE=denied expect_fail "release push fails closed on tag lookup denial" run_task lambda-release-push
pushes_after=$(grep -c '^docker push ' "$command_log" || true)
test "$pushes_after" = "$pushes_before" || {
	printf 'FAIL: release pushed after a denied tag lookup\n' >&2
	exit 1
}
pass "release performs no push after a denied tag lookup"
FAKE_LOOKUP_MODE=ambiguous expect_fail "release accepts only the exact tag-absence error" run_task lambda-release-push
FAKE_REPOSITORY_MUTABILITY=MUTABLE expect_fail "release push requires an immutable repository" run_task lambda-release-push
pushes_before=$(grep -c '^docker push ' "$command_log" || true)
FAKE_LOOKUP_MODE=existing expect_fail "release stops when the immutable tag already exists" run_task lambda-release-push
pushes_after=$(grep -c '^docker push ' "$command_log" || true)
test "$pushes_after" = "$pushes_before" || {
	printf 'FAIL: release pushed an existing immutable tag\n' >&2
	exit 1
}
pass "release performs no push for an existing immutable tag"
FAKE_PUSH_FAIL=true expect_fail "release push propagates registry conflicts" run_task lambda-release-push
grep -F 'task build-lambda-image BUILD_REVISION=0123456789abcdef0123456789abcdef01234567 IMAGE_TAG=180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases:git-0123456789abcdef0123456789abcdef01234567' "$command_log" >/dev/null || {
	printf 'FAIL: release build did not receive the immutable full-SHA tag\n' >&2
	exit 1
}
grep -F 'docker push 180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio-lambda-releases:git-0123456789abcdef0123456789abcdef01234567' "$command_log" >/dev/null || {
	printf 'FAIL: release did not push the immutable full-SHA tag\n' >&2
	exit 1
}
pass "release command used the immutable full-SHA tag"

if grep -En -- '--auto-approve|-target=|:latest|lambda-latest' "$command_log" >/dev/null; then
	printf 'FAIL: replacement command execution used an unsafe plan or mutable tag\n' >&2
	exit 1
fi
pass "replacement command executions used no auto-approve, target, or mutable tag"

printf 'PASS: %s Lambda plan contracts\n' "$pass_count"
