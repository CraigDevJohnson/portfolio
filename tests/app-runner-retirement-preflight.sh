#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
fake_bin="$tmp_dir/bin"
command_log="$tmp_dir/commands.log"
mkdir -p "$fake_bin"

pass_count=0

pass() {
	pass_count=$((pass_count + 1))
	printf 'PASS: %s\n' "$1"
}

fail() {
	printf 'FAIL: %s\n' "$1" >&2
	exit 1
}

expect_pass() {
	name=$1
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		pass "$name"
	else
		cat "$tmp_dir/output" >&2
		fail "$name"
	fi
}

expect_fail() {
	name=$1
	mode=$2
	want=$3
	if run_preflight "$mode" >"$tmp_dir/output" 2>&1; then
		fail "$name unexpectedly passed"
	fi
	grep -F "$want" "$tmp_dir/output" >/dev/null || {
		cat "$tmp_dir/output" >&2
		fail "$name did not report $want"
	}
	pass "$name"
}

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"

service_arn=arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb
case "$*" in
	"sts get-caller-identity --output json --no-cli-pager")
		if [ "$FAKE_CASE" = wrong-identity ]; then
			printf '%s\n' '{"Account":"000000000000","Arn":"arn:aws:iam::000000000000:root","UserId":"root"}'
		else
			printf '%s\n' '{"Account":"180294223248","Arn":"arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_123/test","UserId":"test"}'
		fi
		;;
	"apprunner describe-service --service-arn $service_arn --output json --no-cli-pager")
		status=RUNNING
		instance_role=arn:aws:iam::180294223248:role/portfolio-apprunner-instance
		if [ "$FAKE_CASE" = bad-service-status ]; then status=PAUSED; fi
		if [ "$FAKE_CASE" = wrong-instance-role ]; then instance_role=arn:aws:iam::180294223248:role/unexpected; fi
		jq -n \
			--arg arn "$service_arn" \
			--arg status "$status" \
			--arg instance_role "$instance_role" '
			{
				Service: {
					ServiceArn: $arn,
					ServiceName: "portfolio",
					Status: $status,
					SourceConfiguration: {
						AuthenticationConfiguration: {
							AccessRoleArn: "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access"
						},
						ImageRepository: {
							ImageIdentifier: "180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio:latest",
							ImageRepositoryType: "ECR"
						}
					},
					InstanceConfiguration: {InstanceRoleArn: $instance_role}
				}
			}'
		;;
	"apprunner describe-custom-domains --service-arn $service_arn --output json --no-cli-pager")
		if [ "$FAKE_CASE" = custom-domain ]; then
			jq -n --arg arn "$service_arn" '{ServiceArn: $arn, DNSTarget: "example.awsapprunner.com", CustomDomains: [{DomainName: "dev.craigdevjohnson.com", Status: "ACTIVE"}]}'
		else
			jq -n --arg arn "$service_arn" '{ServiceArn: $arn, DNSTarget: "example.awsapprunner.com", CustomDomains: []}'
		fi
		;;
	"apprunner list-tags-for-resource --resource-arn $service_arn --output json --no-cli-pager")
		if [ "$FAKE_CASE" = wrong-tags ]; then
			printf '%s\n' '{"Tags":[{"Key":"Name","Value":"portfolio"}]}'
		else
			printf '%s\n' '{"Tags":[{"Key":"Environment","Value":"development"},{"Key":"ManagedBy","Value":"opentofu"},{"Key":"Name","Value":"portfolio"},{"Key":"Project","Value":"portfolio"}]}'
		fi
		;;
	"iam get-role --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager")
		role_id=AROAST6S7QWIFWIJU3SEX
		if [ "$FAKE_CASE" = wrong-access-role-id ]; then role_id=AROAST6S7QWI000000000; fi
		if [ "$FAKE_CASE" = broadened-access-trust ]; then
			service_principal='["build.apprunner.amazonaws.com","lambda.amazonaws.com"]'
		else
			service_principal='"build.apprunner.amazonaws.com"'
		fi
		jq -n \
			--arg role_id "$role_id" \
			--argjson service_principal "$service_principal" '
			{
				Role: {
					Path: "/",
					RoleName: "portfolio-apprunner-ecr-access",
					RoleId: $role_id,
					Arn: "arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access",
					AssumeRolePolicyDocument: {
						Version: "2012-10-17",
						Statement: [{
							Effect: "Allow",
							Principal: {Service: $service_principal},
							Action: "sts:AssumeRole"
						}]
					}
				}
			}'
		;;
	"iam get-role --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		role_id=AROAST6S7QWIK7PZV2BTQ
		if [ "$FAKE_CASE" = wrong-instance-role-id ]; then role_id=AROAST6S7QWI000000001; fi
		if [ "$FAKE_CASE" = broadened-instance-trust ]; then
			service_principal='["tasks.apprunner.amazonaws.com","lambda.amazonaws.com"]'
		else
			service_principal='"tasks.apprunner.amazonaws.com"'
		fi
		jq -n \
			--arg role_id "$role_id" \
			--argjson service_principal "$service_principal" '
			{
				Role: {
					Path: "/",
					RoleName: "portfolio-apprunner-instance",
					RoleId: $role_id,
					Arn: "arn:aws:iam::180294223248:role/portfolio-apprunner-instance",
					AssumeRolePolicyDocument: {
						Version: "2012-10-17",
						Statement: [{
							Effect: "Allow",
							Principal: {Service: $service_principal},
							Action: "sts:AssumeRole"
						}]
					}
				}
			}'
		;;
	"iam list-attached-role-policies --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager")
		printf '%s\n' '{"AttachedPolicies":[{"PolicyName":"AWSAppRunnerServicePolicyForECRAccess","PolicyArn":"arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"}]}'
		;;
	"iam list-attached-role-policies --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		if [ "$FAKE_CASE" = extra-attachment ]; then
			printf '%s\n' '{"AttachedPolicies":[{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"},{"PolicyArn":"arn:aws:iam::180294223248:policy/unexpected"}]}'
		else
			printf '%s\n' '{"AttachedPolicies":[{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"}]}'
		fi
		;;
	"iam list-role-policies --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager"|\
	"iam list-role-policies --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		if [ "$FAKE_CASE" = inline-policy ]; then
			printf '%s\n' '{"PolicyNames":["unexpected"]}'
		else
			printf '%s\n' '{"PolicyNames":[]}'
		fi
		;;
	"iam list-instance-profiles-for-role --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager"|\
	"iam list-instance-profiles-for-role --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		if [ "$FAKE_CASE" = instance-profile ]; then
			printf '%s\n' '{"InstanceProfiles":[{"InstanceProfileName":"unexpected"}]}'
		else
			printf '%s\n' '{"InstanceProfiles":[]}'
		fi
		;;
	"iam list-entities-for-policy --policy-arn arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets --policy-usage-filter PermissionsPolicy --output json --no-cli-pager")
		case "$FAKE_CASE" in
			external-policy-role)
				printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[],"PolicyRoles":[{"RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWIK7PZV2BTQ"},{"RoleName":"unexpected","RoleId":"AROAST6S7QWI000000002"}]}'
				;;
			external-policy-user)
				printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[{"UserName":"unexpected","UserId":"AIDA00000000000000000"}],"PolicyRoles":[{"RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWIK7PZV2BTQ"}]}'
				;;
			external-policy-group)
				printf '%s\n' '{"PolicyGroups":[{"GroupName":"unexpected","GroupId":"AGPA00000000000000000"}],"PolicyUsers":[],"PolicyRoles":[{"RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWIK7PZV2BTQ"}]}'
				;;
			wrong-policy-role-id)
				printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[],"PolicyRoles":[{"RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWI000000001"}]}'
				;;
			*)
				printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[],"PolicyRoles":[{"RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWIK7PZV2BTQ"}]}'
				;;
		esac
		;;
	"iam list-entities-for-policy --policy-arn arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets --policy-usage-filter PermissionsBoundary --output json --no-cli-pager")
		if [ "$FAKE_CASE" = boundary-policy-entity ]; then
			printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[],"PolicyRoles":[{"RoleName":"unexpected","RoleId":"AROAST6S7QWI000000003"}]}'
		else
			printf '%s\n' '{"PolicyGroups":[],"PolicyUsers":[],"PolicyRoles":[]}'
		fi
		;;
	"iam list-policy-versions --policy-arn arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets --output json --no-cli-pager")
		if [ "$FAKE_CASE" = extra-policy-version ]; then
			printf '%s\n' '{"Versions":[{"VersionId":"v2","IsDefaultVersion":true},{"VersionId":"v1","IsDefaultVersion":false}]}'
		else
			printf '%s\n' '{"Versions":[{"VersionId":"v1","IsDefaultVersion":true}]}'
		fi
		;;
	*)
		printf 'unexpected fake aws command: %s\n' "$*" >&2
		exit 1
		;;
esac
EOF
chmod +x "$fake_bin/aws"

run_preflight() {
	mode=${1:-passing}
	env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
		PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		FAKE_CASE="$mode" \
		AWS_PROFILE=portfolio-deployer \
		AWS_REGION=us-west-2 \
		sh "$repo_root/scripts/check-app-runner-retirement-preflight.sh"
}

: >"$command_log"
expect_pass "exact App Runner retirement preflight" run_preflight passing

expect_fail "preflight rejects the wrong identity" wrong-identity "unexpected AWS account"
expect_fail "preflight rejects a non-running service" bad-service-status "service contract failed"
expect_fail "preflight rejects the wrong instance role" wrong-instance-role "service contract failed"
expect_fail "preflight rejects an App Runner custom domain" custom-domain "custom-domain association remains"
expect_fail "preflight rejects missing service tags" wrong-tags "service tag contract failed"
expect_fail "preflight rejects the wrong ECR access role ID" wrong-access-role-id "ECR access role identity contract failed"
expect_fail "preflight rejects broadened ECR access trust" broadened-access-trust "ECR access role identity contract failed"
expect_fail "preflight rejects the wrong instance role ID" wrong-instance-role-id "instance role identity contract failed"
expect_fail "preflight rejects broadened instance trust" broadened-instance-trust "instance role identity contract failed"
expect_fail "preflight rejects an extra role attachment" extra-attachment "instance role attachment contract failed"
expect_fail "preflight rejects inline role policies" inline-policy "inline role policies remain"
expect_fail "preflight rejects instance profiles" instance-profile "instance profiles remain"
expect_fail "preflight rejects an external runtime-policy role" external-policy-role "runtime policy attachment contract failed"
expect_fail "preflight rejects an external runtime-policy user" external-policy-user "runtime policy attachment contract failed"
expect_fail "preflight rejects an external runtime-policy group" external-policy-group "runtime policy attachment contract failed"
expect_fail "preflight rejects a recreated runtime-policy role" wrong-policy-role-id "runtime policy attachment contract failed"
expect_fail "preflight rejects runtime-policy boundary use" boundary-policy-entity "runtime policy boundary contract failed"
expect_fail "preflight rejects extra managed-policy versions" extra-policy-version "runtime policy version contract failed"

if grep -E ' (delete|detach|disassociate|create|update|put|apply)(-| |$)' "$command_log" >/dev/null; then
	fail "preflight invoked a mutating command"
fi
pass "preflight invokes only read operations"

printf '%s tests passed\n' "$pass_count"
