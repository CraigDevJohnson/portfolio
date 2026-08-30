#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
fake_bin="$tmp_dir/bin"
command_log="$tmp_dir/commands.log"
plan_json="$tmp_dir/retirement-plan.json"
valid_plan_json="$tmp_dir/valid-retirement-plan.json"
plan_file="$tmp_dir/retirement.tfplan"
apply_marker="$tmp_dir/apply-ran"
mkdir -p "$fake_bin"

real_stat=$(command -v stat)
if "$real_stat" -c '%u:%a:%h' "$tmp_dir" >/dev/null 2>&1; then
	real_stat_style=gnu
else
	real_stat_style=bsd
fi

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
				assume_role_policy: ({Version: "2012-10-17", Statement: [{Effect: "Allow", Principal: {Service: "build.apprunner.amazonaws.com"}, Action: "sts:AssumeRole"}]} | tojson),
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
				assume_role_policy: ({Version: "2012-10-17", Statement: [{Effect: "Allow", Principal: {Service: "tasks.apprunner.amazonaws.com"}, Action: "sts:AssumeRole"}]} | tojson),
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
			{mode: "managed", address: "aws_lambda_function.app", change: no_op({})}
		],
		output_changes: {
			app_runner_service_url: deletion("https://vafw855pvk.us-west-2.awsapprunner.com"),
			app_runner_service_arn: deletion("arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb"),
			app_runner_service_id: deletion("c5490e71b0e84aba90a9648e94d240fb"),
			instance_role_arn: deletion("arn:aws:iam::180294223248:role/portfolio-apprunner-instance"),
			lambda_function_name: no_op("portfolio")
		}
	}
' >"$plan_json"
cp "$plan_json" "$valid_plan_json"

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*"sts get-caller-identity"*"--query Arn"*)
		printf 'arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_123/test\n'
		;;
	*"sts get-caller-identity"*"--query Account"*) printf '180294223248\n' ;;
	"sts get-caller-identity --output json --no-cli-pager")
		printf '%s\n' '{"Account":"180294223248","Arn":"arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_123/test","UserId":"test"}'
		;;
	"apprunner describe-service --service-arn arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb --output json --no-cli-pager")
		printf '%s\n' '{"Service":{"ServiceArn":"arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb","ServiceName":"portfolio","Status":"RUNNING","SourceConfiguration":{"AuthenticationConfiguration":{"AccessRoleArn":"arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access"},"ImageRepository":{"ImageIdentifier":"180294223248.dkr.ecr.us-west-2.amazonaws.com/portfolio:latest","ImageRepositoryType":"ECR"}},"InstanceConfiguration":{"InstanceRoleArn":"arn:aws:iam::180294223248:role/portfolio-apprunner-instance"}}}'
		;;
	"apprunner describe-custom-domains --service-arn arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb --output json --no-cli-pager")
		if [ "${FAKE_PREFLIGHT_FAIL:-false}" = true ]; then
			printf '%s\n' '{"ServiceArn":"arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb","CustomDomains":[{"DomainName":"dev.craigdevjohnson.com","Status":"ACTIVE"}]}'
		else
			printf '%s\n' '{"ServiceArn":"arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb","CustomDomains":[]}'
		fi
		;;
	"apprunner list-tags-for-resource --resource-arn arn:aws:apprunner:us-west-2:180294223248:service/portfolio/c5490e71b0e84aba90a9648e94d240fb --output json --no-cli-pager")
		printf '%s\n' '{"Tags":[{"Key":"Environment","Value":"development"},{"Key":"ManagedBy","Value":"opentofu"},{"Key":"Name","Value":"portfolio"},{"Key":"Project","Value":"portfolio"}]}'
		;;
	"iam get-role --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager")
		printf '%s\n' '{"Role":{"Path":"/","RoleName":"portfolio-apprunner-ecr-access","RoleId":"AROAST6S7QWIFWIJU3SEX","Arn":"arn:aws:iam::180294223248:role/portfolio-apprunner-ecr-access","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"build.apprunner.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}'
		;;
	"iam get-role --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		printf '%s\n' '{"Role":{"Path":"/","RoleName":"portfolio-apprunner-instance","RoleId":"AROAST6S7QWIK7PZV2BTQ","Arn":"arn:aws:iam::180294223248:role/portfolio-apprunner-instance","AssumeRolePolicyDocument":{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"tasks.apprunner.amazonaws.com"},"Action":"sts:AssumeRole"}]}}}'
		;;
	"iam list-attached-role-policies --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager")
		printf '%s\n' '{"AttachedPolicies":[{"PolicyArn":"arn:aws:iam::aws:policy/service-role/AWSAppRunnerServicePolicyForECRAccess"}]}'
		;;
	"iam list-attached-role-policies --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		printf '%s\n' '{"AttachedPolicies":[{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-google-connections-dynamodb"},{"PolicyArn":"arn:aws:iam::180294223248:policy/portfolio-soccer-sessions-dynamodb"}]}'
		;;
	"iam list-role-policies --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager"|\
	"iam list-role-policies --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		printf '%s\n' '{"PolicyNames":[]}'
		;;
	"iam list-instance-profiles-for-role --role-name portfolio-apprunner-ecr-access --output json --no-cli-pager"|\
	"iam list-instance-profiles-for-role --role-name portfolio-apprunner-instance --output json --no-cli-pager")
		printf '%s\n' '{"InstanceProfiles":[]}'
		;;
	"iam list-policy-versions --policy-arn arn:aws:iam::180294223248:policy/portfolio-apprunner-runtime-secrets --output json --no-cli-pager")
		printf '%s\n' '{"Versions":[{"VersionId":"v1","IsDefaultVersion":true}]}'
		;;
	*) printf 'unexpected fake aws command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/tofu" <<'EOF'
#!/bin/sh
set -eu
printf 'tofu %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	"-chdir=infra init -reconfigure -lockfile=readonly -input=false") ;;
	"-chdir=infra workspace show") printf '%s\n' "${FAKE_WORKSPACE:-default}" ;;
	*" plan "*)
		for argument in "$@"; do
			case "$argument" in -out=*) printf 'saved plan\n' >"${argument#-out=}" ;; esac
		done
		;;
	*" show -json "*)
		cat "$FAKE_PLAN_JSON"
		if [ "${FAKE_REPLACE_PLAN:-false}" = true ]; then
			for argument in "$@"; do plan_file=$argument; done
			chmod 600 "$plan_file"
			printf 'replaced\n' >>"$plan_file"
		fi
		;;
	*" show -no-color "*) printf 'synthetic human-readable plan\n' ;;
	*" apply "*) : >"$APPLY_MARKER" ;;
	*) printf 'unexpected fake tofu command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/stat" <<'EOF'
#!/bin/sh
set -eu
printf 'stat %s\n' "$*" >>"$COMMAND_LOG"

real_metadata() {
	metadata_target=$1
	case "$REAL_STAT_STYLE" in
		gnu) metadata_output=$("$REAL_STAT" -c '%u:%a:%h' "$metadata_target") ;;
		bsd) metadata_output=$("$REAL_STAT" -f '%u:%Mp%Lp:%l' "$metadata_target") ;;
		*) exit 1 ;;
	esac
	metadata_owner=${metadata_output%%:*}
	metadata_remainder=${metadata_output#*:}
	metadata_mode=${metadata_remainder%%:*}
	metadata_links=${metadata_remainder#*:}
	metadata_mode=${FAKE_STAT_MODE:-$metadata_mode}
	if [ "${FAKE_STAT_STYLE:-gnu}" = bsd ]; then
		case "$metadata_mode" in
			[0-7][0-7][0-7]) metadata_mode=0$metadata_mode ;;
		esac
	fi
	printf '%s:%s:%s\n' \
		"${FAKE_STAT_OWNER:-$metadata_owner}" \
		"$metadata_mode" \
		"${FAKE_STAT_LINKS:-$metadata_links}"
}

case "${FAKE_STAT_STYLE:-gnu}" in
	gnu)
		if [ "${1:-}" = -c ] && [ "${2:-}" = '%u:%a:%h' ] && [ "$#" -eq 3 ]; then
			real_metadata "$3"
		elif [ "${1:-}" = -f ]; then
			# GNU stat accepts -f with filesystem-report semantics. Exiting zero
			# here proves callers do not mistake that output for BSD metadata.
			printf '%s\n' 'gnu-stat-filesystem-output'
		else
			exit 1
		fi
		;;
	bsd)
		if [ "${1:-}" = -c ]; then
			exit 1
		elif [ "${1:-}" = -f ] && [ "${2:-}" = '%u:%Mp%Lp:%l' ] && [ "$#" -eq 3 ]; then
			real_metadata "$3"
		else
			exit 1
		fi
		;;
	malformed)
		if [ "${1:-}" = -c ] && [ "${2:-}" = '%u:%a:%h' ] && [ "$#" -eq 3 ]; then
			printf '%s\n' 'not-metadata'
		else
			exit 1
		fi
		;;
	unavailable) exit 1 ;;
	*) exit 1 ;;
esac
EOF
chmod +x "$fake_bin/aws" "$fake_bin/tofu" "$fake_bin/stat"

real_task=$(command -v task)

task_env() {
	env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
		PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		FAKE_PLAN_JSON="$plan_json" \
		FAKE_STAT_STYLE="${FAKE_STAT_STYLE:-gnu}" \
		REAL_STAT="$real_stat" \
		REAL_STAT_STYLE="$real_stat_style" \
		APPLY_MARKER="$apply_marker" \
		AWS_PROFILE=portfolio-deployer \
		AWS_REGION=us-west-2 \
		"$@"
}

run_task() {
	task_env "$real_task" --dir "$repo_root" "$@"
}

run_task_with_env() {
	variable=$1
	value=$2
	shift 2
	task_env "$variable=$value" "$real_task" --dir "$repo_root" "$@"
}

run_plan() {
	stat_style=${1:-gnu}
	rm -f "$plan_file"
	task_env "FAKE_STAT_STYLE=$stat_style" "$real_task" --dir "$repo_root" \
		legacy-apprunner-retirement-plan \
		PLAN_FILE="$plan_file" \
		APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
}

run_apply() {
	mode=${1:-false}
	preflight_fail=${2:-false}
	stat_style=${3:-gnu}
	initial_mode=${4:-400}
	rm -f "$plan_file"
	printf 'reviewed plan\n' >"$plan_file"
	chmod "$initial_mode" "$plan_file"
	plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
	task_env "FAKE_REPLACE_PLAN=$mode" "FAKE_PREFLIGHT_FAIL=$preflight_fail" \
		"FAKE_STAT_STYLE=$stat_style" "$real_task" --dir "$repo_root" \
		legacy-apprunner-retirement-apply \
		PLAN_FILE="$plan_file" \
		APPROVED_PLAN_SHA256="$plan_sha256" \
		APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
}

run_workspace_rejection() {
	task_name=$1
	case "$task_name" in
		legacy-apprunner-retirement-init)
			run_task_with_env FAKE_WORKSPACE retirement "$task_name"
			;;
		legacy-apprunner-retirement-plan)
			run_task_with_env FAKE_WORKSPACE retirement "$task_name" \
				PLAN_FILE="$plan_file" \
				APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
			;;
		legacy-apprunner-retirement-apply)
			printf 'reviewed plan\n' >"$plan_file"
			plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
			run_task_with_env FAKE_WORKSPACE retirement "$task_name" \
				PLAN_FILE="$plan_file" \
				APPROVED_PLAN_SHA256="$plan_sha256" \
				APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
			;;
		esac
}

: >"$command_log"
expect_pass "legacy init uses the root backend configuration" run_task legacy-apprunner-retirement-init
grep -Fx "tofu -chdir=infra init -reconfigure -lockfile=readonly -input=false" "$command_log" >/dev/null || fail "legacy init arguments"
if grep -Fq 'backend.hcl' "$command_log"; then fail "legacy init must not reference backend.hcl"; fi
pass "legacy init has no backend.hcl override"

for variable in TF_CLI_ARGS TF_CLI_ARGS_init TF_CLI_ARGS_workspace TF_CLI_ARGS_plan TF_CLI_ARGS_show TF_CLI_ARGS_apply TF_WORKSPACE TF_DATA_DIR TF_VAR_aws_region TF_VAR_app_name TF_VAR_environment TF_VAR_unexpected; do
	for task_name in legacy-apprunner-retirement-init legacy-apprunner-retirement-plan legacy-apprunner-retirement-apply; do
		: >"$command_log"
		expect_fail "$task_name rejects $variable" run_task_with_env "$variable" unexpected "$task_name"
		if grep -Fq 'tofu ' "$command_log"; then fail "$task_name ran tofu with $variable set"; fi
	done
done

for task_name in legacy-apprunner-retirement-init legacy-apprunner-retirement-plan legacy-apprunner-retirement-apply; do
	: >"$command_log"
	expect_fail "$task_name rejects a non-default workspace" run_workspace_rejection "$task_name"
	grep -Fx 'tofu -chdir=infra workspace show' "$command_log" >/dev/null || fail "$task_name did not check its workspace"
done

: >"$command_log"
expect_pass "retirement plan has locked full-refresh saved-plan arguments" run_plan
grep -Fx "tofu -chdir=infra init -reconfigure -lockfile=readonly -input=false" "$command_log" >/dev/null || fail "retirement plan did not initialize"
grep -Fx "tofu -chdir=infra plan -refresh=true -lock-timeout=5m -input=false -out=$plan_file" "$command_log" >/dev/null || fail "retirement plan arguments"
grep -F "stat -c %u:%a:%h $tmp_dir" "$command_log" >/dev/null || fail "retirement plan did not use GNU stat metadata"
if grep -Fq 'stat -f ' "$command_log"; then fail "retirement plan used BSD stat after GNU stat succeeded"; fi
pass "retirement plan uses unambiguous GNU stat metadata"
init_line=$(grep -n -F 'tofu -chdir=infra init -reconfigure -lockfile=readonly -input=false' "$command_log" | head -n 1 | cut -d: -f1)
preflight_line=$(grep -n -F 'aws apprunner describe-service ' "$command_log" | head -n 1 | cut -d: -f1)
plan_line=$(grep -n -F "tofu -chdir=infra plan -refresh=true -lock-timeout=5m -input=false -out=$plan_file" "$command_log" | head -n 1 | cut -d: -f1)
test -n "$init_line" && test -n "$preflight_line" && test -n "$plan_line" && \
	test "$init_line" -lt "$preflight_line" && test "$preflight_line" -lt "$plan_line" || \
	fail "retirement plan did not initialize and preflight before planning"
pass "retirement plan preflights immediately before planning"
if grep -Eq -- '(-target|-destroy|auto-approve)' "$command_log"; then fail "retirement plan used a forbidden argument"; fi
pass "retirement plan has no forbidden arguments"

: >"$command_log"
expect_pass "retirement plan supports BSD stat metadata" run_plan bsd
grep -F "stat -c %u:%a:%h $tmp_dir" "$command_log" >/dev/null || fail "retirement plan did not probe GNU stat first"
grep -F "stat -f %u:%Mp%Lp:%l $tmp_dir" "$command_log" >/dev/null || fail "retirement plan did not fall back to BSD stat"
pass "retirement plan uses the BSD stat fallback"

: >"$command_log"
chmod 1700 "$tmp_dir"
expect_fail "retirement plan BSD metadata rejects a special-bit parent mode" run_plan bsd
chmod 700 "$tmp_dir"
if grep -Fq ' plan ' "$command_log"; then fail "retirement plan used a special-bit parent"; fi

: >"$command_log"
rm -f "$apply_marker"
expect_pass "retirement apply revalidates the saved plan before applying" run_apply
test -f "$apply_marker" || fail "retirement apply did not run"
init_line=$(grep -n -F 'tofu -chdir=infra init -reconfigure -lockfile=readonly -input=false' "$command_log" | head -n 1 | cut -d: -f1)
show_line=$(grep -n -F "tofu -chdir=infra show -json $plan_file" "$command_log" | head -n 1 | cut -d: -f1)
preflight_line=$(grep -n -F 'aws apprunner describe-service ' "$command_log" | tail -n 1 | cut -d: -f1)
apply_line=$(grep -n -F "tofu -chdir=infra apply -lock-timeout=5m -input=false $plan_file" "$command_log" | head -n 1 | cut -d: -f1)
test -n "$init_line" && test -n "$show_line" && test -n "$preflight_line" && test -n "$apply_line" && \
	test "$init_line" -lt "$show_line" && test "$show_line" -lt "$preflight_line" && test "$preflight_line" -lt "$apply_line" || \
	fail "retirement apply did not inspect and preflight before applying"
pass "retirement apply checks before apply"

: >"$command_log"
rm -f "$apply_marker"
expect_pass "retirement apply supports BSD stat metadata" run_apply false false bsd
test -f "$apply_marker" || fail "retirement apply did not run with BSD stat"
grep -F "stat -f %u:%Mp%Lp:%l $plan_file" "$command_log" >/dev/null || fail "retirement apply did not use BSD stat"

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply BSD metadata rejects a special-bit plan mode" run_apply false false bsd 1400
grep -F 'PLAN_FILE must have mode 400' "$tmp_dir/output" >/dev/null || fail "retirement apply did not reject a BSD special-bit plan mode"
test ! -f "$apply_marker" || fail "retirement apply ran with a BSD special-bit plan mode"

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply rejects a writable reviewed plan" run_apply false false gnu 600
grep -F 'PLAN_FILE must have mode 400' "$tmp_dir/output" >/dev/null || fail "retirement apply did not reject writable plan mode"
if grep -Fq ' show ' "$command_log" || grep -Fq ' apply ' "$command_log"; then
	fail "retirement apply inspected or applied a writable plan"
fi

: >"$command_log"
rm -f "$plan_file" "$tmp_dir/retirement-hardlink.tfplan" "$apply_marker"
printf 'reviewed plan\n' >"$plan_file"
chmod 400 "$plan_file"
ln "$plan_file" "$tmp_dir/retirement-hardlink.tfplan"
hardlinked_plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
expect_fail "retirement apply rejects a hard-linked reviewed plan" run_task \
	legacy-apprunner-retirement-apply \
	PLAN_FILE="$plan_file" \
	APPROVED_PLAN_SHA256="$hardlinked_plan_sha256" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
grep -F 'PLAN_FILE must have exactly one hard link' "$tmp_dir/output" >/dev/null || fail "retirement apply did not reject hard-linked plan"
test ! -f "$apply_marker" || fail "retirement apply ran with a hard-linked plan"
rm -f "$tmp_dir/retirement-hardlink.tfplan"

: >"$command_log"
rm -f "$plan_file"
expect_fail "retirement plan rejects malformed stat metadata" task_env \
	FAKE_STAT_STYLE=malformed "$real_task" --dir "$repo_root" \
	legacy-apprunner-retirement-plan \
	PLAN_FILE="$plan_file" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
grep -F 'PLAN_FILE parent metadata is malformed' "$tmp_dir/output" >/dev/null || fail "retirement plan did not report malformed stat metadata"
if grep -Fq ' plan ' "$command_log"; then fail "retirement plan ran with malformed stat metadata"; fi

: >"$command_log"
rm -f "$plan_file"
expect_fail "retirement plan fails closed without supported stat metadata" task_env \
	FAKE_STAT_STYLE=unavailable "$real_task" --dir "$repo_root" \
	legacy-apprunner-retirement-plan \
	PLAN_FILE="$plan_file" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
grep -F 'Unable to read file metadata with GNU or BSD stat' "$tmp_dir/output" >/dev/null || fail "retirement plan did not report unavailable stat metadata"
if grep -Fq ' plan ' "$command_log"; then fail "retirement plan ran without supported stat metadata"; fi

: >"$command_log"
dangling_plan="$tmp_dir/dangling.tfplan"
ln -s "$tmp_dir/missing-plan" "$dangling_plan"
expect_fail "retirement plan rejects a dangling symlink" run_task \
	legacy-apprunner-retirement-plan \
	PLAN_FILE="$dangling_plan" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
if grep -Fq ' plan ' "$command_log"; then fail "retirement plan followed a dangling symlink"; fi

: >"$command_log"
symlink_target="$tmp_dir/symlink-target.tfplan"
printf 'reviewed plan\n' >"$symlink_target"
chmod 600 "$symlink_target"
symlink_plan="$tmp_dir/symlink.tfplan"
ln -s "$symlink_target" "$symlink_plan"
symlink_sha256=$(shasum -a 256 "$symlink_target" | awk '{print $1}')
rm -f "$apply_marker"
expect_fail "retirement apply rejects a plan symlink" run_task \
	legacy-apprunner-retirement-apply \
	PLAN_FILE="$symlink_plan" \
	APPROVED_PLAN_SHA256="$symlink_sha256" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
test ! -f "$apply_marker" || fail "retirement apply followed a plan symlink"

public_dir="$tmp_dir/public"
mkdir "$public_dir"
chmod 755 "$public_dir"
: >"$command_log"
expect_fail "retirement plan rejects a non-private parent directory" run_task \
	legacy-apprunner-retirement-plan \
	PLAN_FILE="$public_dir/retirement.tfplan" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
if grep -Fq ' plan ' "$command_log"; then fail "retirement plan used a non-private parent"; fi

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply preflight failure blocks apply" run_apply false true
grep -F 'custom-domain association remains' "$tmp_dir/output" >/dev/null || fail "retirement apply did not report preflight failure"
test ! -f "$apply_marker" || fail "retirement apply ran after preflight failure"

: >"$command_log"
rm -f "$apply_marker"
jq '.errored = true' "$valid_plan_json" >"$plan_json"
expect_fail "retirement apply checker rejection blocks apply" run_apply
grep -F 'App Runner retirement plan contract failed:' "$tmp_dir/output" >/dev/null || fail "retirement apply did not run the checker"
test ! -f "$apply_marker" || fail "retirement apply ran after checker rejection"
cp "$valid_plan_json" "$plan_json"

: >"$command_log"
rm -f "$apply_marker"
actual_plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
case "$actual_plan_sha256" in
	0000000000000000000000000000000000000000000000000000000000000000)
		mismatched_plan_sha256=1111111111111111111111111111111111111111111111111111111111111111
		;;
	*) mismatched_plan_sha256=0000000000000000000000000000000000000000000000000000000000000000 ;;
esac
expect_fail "retirement apply rejects a valid mismatched checksum" run_task \
	legacy-apprunner-retirement-apply \
	PLAN_FILE="$plan_file" \
	APPROVED_PLAN_SHA256="$mismatched_plan_sha256" \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
grep -F 'PLAN_FILE checksum does not match APPROVED_PLAN_SHA256' "$tmp_dir/output" >/dev/null || fail "retirement apply did not reject the checksum mismatch"
test ! -f "$apply_marker" || fail "retirement apply ran after checksum mismatch"

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply rejects a replaced saved plan" run_apply true
test ! -f "$apply_marker" || fail "retirement apply ran after plan replacement"

printf '%s tests passed\n' "$pass_count"
