#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
script="$repo_root/scripts/update-portfolio-deployer-retirement-policy.sh"
development_policy="$repo_root/infra/lambda/bootstrap/portfolio-deployer-development-bootstrap-policy.json"
retirement_policy="$repo_root/infra/lambda/bootstrap/portfolio-deployer-app-runner-retirement-policy.json"
development_sha=$(shasum -a 256 "$development_policy" | awk '{print $1}')
retirement_sha=$(shasum -a 256 "$retirement_policy" | awk '{print $1}')

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
fake_bin="$tmp_dir/bin"
policy_state="$tmp_dir/inline-policy.json"
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
	shift
	if "$@" >"$tmp_dir/output" 2>&1; then
		fail "$name unexpectedly passed"
	fi
	pass "$name"
}

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"

case "$*" in
	"sts get-caller-identity --output json --no-cli-pager")
		if [ "$FAKE_CASE" = wrong-identity ]; then
			printf '%s\n' '{"Account":"000000000000","Arn":"arn:aws:iam::000000000000:root"}'
		else
			printf '%s\n' '{"Account":"180294223248","Arn":"arn:aws:iam::180294223248:root"}'
		fi
		;;
	"sso-admin list-instances --output json --no-cli-pager")
		printf '%s\n' '{"Instances":[{"InstanceArn":"arn:aws:sso:::instance/ssoins-7907da8f49133a24","Status":"ACTIVE"}]}'
		;;
	"sso-admin list-permission-sets --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --output json --no-cli-pager")
		printf '%s\n' '{"PermissionSets":["arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd"]}'
		;;
	"sso-admin describe-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --query PermissionSet.Name --output text --no-cli-pager")
		printf '%s\n' 'PortfolioDeployer'
		;;
	"sso-admin list-accounts-for-provisioned-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --output json --no-cli-pager")
		if [ "$FAKE_CASE" = multiple-accounts ]; then
			printf '%s\n' '{"AccountIds":["180294223248","999999999999"]}'
		else
			printf '%s\n' '{"AccountIds":["180294223248"]}'
		fi
		;;
	"sso-admin list-managed-policies-in-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --output json --no-cli-pager")
		printf '%s\n' '{"AttachedManagedPolicies":[]}'
		;;
	"sso-admin list-customer-managed-policy-references-in-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --output json --no-cli-pager")
		printf '%s\n' '{"CustomerManagedPolicyReferences":[]}'
		;;
	"sso-admin get-permissions-boundary-for-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --output json --no-cli-pager")
		if [ "$FAKE_CASE" = boundary ]; then
			printf '%s\n' '{"PermissionsBoundary":{"ManagedPolicyArn":"arn:aws:iam::aws:policy/ReadOnlyAccess"}}'
		else
			printf '%s\n' 'ResourceNotFoundException: PermissionsBoundary not present' >&2
			exit 254
		fi
		;;
	"sso-admin get-inline-policy-for-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --query InlinePolicy --output text --no-cli-pager")
		if [ "$FAKE_CASE" = source-drift ]; then
			printf '%s\n' '{"Version":"2012-10-17","Statement":[]}'
		else
			jq -c . "$POLICY_STATE"
		fi
		;;
	"accessanalyzer validate-policy --policy-document file://"*" --policy-type IDENTITY_POLICY --output json --no-cli-pager")
		if [ "$FAKE_CASE" = analyzer-finding ]; then
			printf '%s\n' '{"findings":[{"findingType":"ERROR","issueCode":"TEST"}]}'
		else
			printf '%s\n' '{"findings":[]}'
		fi
		;;
	"sso-admin put-inline-policy-to-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --inline-policy file://"*)
		for argument in "$@"; do
			case "$argument" in file://*) cp "${argument#file://}" "$POLICY_STATE" ;; esac
		done
		printf '%s\n' '{}'
		;;
	"sso-admin provision-permission-set --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --permission-set-arn arn:aws:sso:::permissionSet/ssoins-7907da8f49133a24/ps-7907d3bd319f58fd --target-type AWS_ACCOUNT --target-id 180294223248 --query PermissionSetProvisioningStatus.RequestId --output text --no-cli-pager")
		if jq -e '.Statement[]? | select(.Sid == "LegacyAppRunnerReadDelete")' "$POLICY_STATE" >/dev/null; then
			printf '%s\n' 'request-retirement'
		else
			printf '%s\n' 'request-development'
		fi
		;;
	"sso-admin describe-permission-set-provisioning-status --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --provision-permission-set-request-id request-retirement --query PermissionSetProvisioningStatus.Status --output text --no-cli-pager")
		if [ "$FAKE_CASE" = install-provision-fail ]; then printf '%s\n' FAILED; else printf '%s\n' SUCCEEDED; fi
		;;
	"sso-admin describe-permission-set-provisioning-status --instance-arn arn:aws:sso:::instance/ssoins-7907da8f49133a24 --provision-permission-set-request-id request-development --query PermissionSetProvisioningStatus.Status --output text --no-cli-pager")
		printf '%s\n' SUCCEEDED
		;;
	*)
		printf 'unexpected fake aws command: %s\n' "$*" >&2
		exit 1
		;;
esac
EOF
chmod +x "$fake_bin/aws"

run_update() {
	action=$1
	mode=${2:-passing}
	case "$action" in
		install)
			approved_name=APPROVED_POLICY_SHA256
			approved_value=$retirement_sha
			;;
		restore)
			approved_name=APPROVED_RESTORE_POLICY_SHA256
			approved_value=$development_sha
			;;
		*) fail "unknown test action" ;;
	esac
	env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
		PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		POLICY_STATE="$policy_state" \
		FAKE_CASE="$mode" \
		AWS_PROFILE=root \
		AWS_REGION=us-west-2 \
		"$approved_name=$approved_value" \
		sh "$script" "$action"
}

cp "$development_policy" "$policy_state"
: >"$command_log"
expect_pass "installs and provisions the reviewed retirement policy" run_update install
test "$(jq -S -c . "$policy_state")" = "$(jq -S -c . "$retirement_policy")" || fail "install state"
test "$(grep -c 'put-inline-policy-to-permission-set' "$command_log")" = 1 || fail "install put count"

cp "$retirement_policy" "$policy_state"
: >"$command_log"
expect_pass "restores and provisions the reviewed development policy" run_update restore
test "$(jq -S -c . "$policy_state")" = "$(jq -S -c . "$development_policy")" || fail "restore state"

cp "$development_policy" "$policy_state"
: >"$command_log"
expect_fail "failed install automatically restores the development policy" run_update install install-provision-fail
test "$(jq -S -c . "$policy_state")" = "$(jq -S -c . "$development_policy")" || fail "failed install did not restore"
test "$(grep -c 'put-inline-policy-to-permission-set' "$command_log")" = 2 || fail "failed install restore put count"
grep -F 'request-development' "$command_log" >/dev/null || fail "failed install did not reprovision development"

for mode in wrong-identity multiple-accounts boundary source-drift analyzer-finding; do
	cp "$development_policy" "$policy_state"
	: >"$command_log"
	expect_fail "rejects $mode before policy replacement" run_update install "$mode"
	if grep -Fq 'put-inline-policy-to-permission-set' "$command_log"; then
		fail "$mode reached policy replacement"
	fi
done

cp "$development_policy" "$policy_state"
: >"$command_log"
expect_fail "rejects an unapproved candidate checksum" env \
	-u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
	PATH="$fake_bin:$PATH" \
	COMMAND_LOG="$command_log" \
	POLICY_STATE="$policy_state" \
	FAKE_CASE=passing \
	AWS_PROFILE=root \
	AWS_REGION=us-west-2 \
	APPROVED_POLICY_SHA256=0000000000000000000000000000000000000000000000000000000000000000 \
	sh "$script" install
if grep -Fq 'put-inline-policy-to-permission-set' "$command_log"; then
	fail "checksum rejection reached policy replacement"
fi

printf '%s tests passed\n' "$pass_count"
