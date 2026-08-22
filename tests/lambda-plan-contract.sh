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
		alarm_actions='["arn:aws:sns:us-west-2:180294223248:portfolio-lambda-prod-alerts"]'
	else
		protection=false
		retention=14
		alarm_actions='[]'
	fi
	jq -n \
		--arg environment "$environment" \
		--arg prefix "$prefix" \
		--arg image "$image_uri" \
		--arg boundary "arn:aws:iam::180294223248:policy/portfolio/boundaries/PortfolioLambdaExecutionBoundary" \
		--argjson protection "$protection" \
		--argjson retention "$retention" \
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
		{
			resource_changes: [
				resource("module.service.aws_iam_role.lambda"; "aws_iam_role"; "lambda"; {name: ($prefix + "-execution"), permissions_boundary: $boundary}),
				(resource("module.service.aws_iam_role_policy.lambda"; "aws_iam_role_policy"; "lambda"; {name: ($prefix + "-runtime")}) | .change.after_unknown = {policy: true}),
				resource("module.service.aws_lambda_function.app"; "aws_lambda_function"; "app"; {function_name: $prefix, image_uri: $image, environment: [{variables: lambda_variables}]}),
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
						after: {statement: policy_statements},
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
							module: {
								resources: [{
									address: "aws_iam_role_policy.lambda",
									mode: "managed",
									type: "aws_iam_role_policy",
									name: "lambda",
									expressions: {
										policy: {references: ["data.aws_iam_policy_document.lambda.json", "data.aws_iam_policy_document.lambda"]}
									}
								}]
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

expect_pass "artifact repository, lifecycle, and pull-policy plan" run_check "$artifact_plan" artifacts
expect_pass "development replacement plan" run_check "$dev_plan" dev
expect_pass "production replacement plan" run_check "$prod_plan" prod
expect_pass "development plan with decoded runtime policy" run_check "$dev_known_policy_plan" dev

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
mutate_and_reject "secret plan value" "$dev_plan" '.resource_changes[2].change.after.oauth_token = "do-not-store-this"'
mutate_and_reject "secret prior-state value" "$dev_plan" '.resource_changes[2].change.before = {oauth_token: "do-not-store-this"}'
mutate_and_reject "missing execution boundary" "$dev_plan" '.resource_changes[0].change.after.permissions_boundary = null'
mutate_and_reject "wrong deterministic API name" "$dev_plan" '.resource_changes[3].change.after.name = "portfolio-http"'
mutate_and_reject "development table protection drift" "$dev_plan" '.resource_changes[6].change.after.deletion_protection_enabled = true'
mutate_and_reject "production table protection drift" "$prod_plan" '.resource_changes[6].change.after.deletion_protection_enabled = false'
mutate_and_reject "log retention drift" "$dev_plan" '.resource_changes[4].change.after.retention_in_days = 7'
mutate_and_reject "alarm action mismatch" "$prod_plan" '.resource_changes[8].change.after.alarm_actions = []'
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
chmod +x "$fake_bin"/*

real_task=$(command -v task)
lookup_state="$tmp_dir/ecr-lookup-state"
run_task() {
	rm -f "$lookup_state"
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

if rg -n -- '--auto-approve|-target=|:latest|lambda-latest' "$command_log" >/dev/null; then
	printf 'FAIL: replacement command execution used an unsafe plan or mutable tag\n' >&2
	exit 1
fi
pass "replacement command executions used no auto-approve, target, or mutable tag"

printf 'PASS: %s Lambda plan contracts\n' "$pass_count"
