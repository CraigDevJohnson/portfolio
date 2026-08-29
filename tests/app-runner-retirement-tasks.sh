#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
fake_bin="$tmp_dir/bin"
command_log="$tmp_dir/commands.log"
plan_json="$tmp_dir/retirement-plan.json"
plan_file="$tmp_dir/retirement.tfplan"
apply_marker="$tmp_dir/apply-ran"
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

jq -n '
	def deletion($before): {actions: ["delete"], before: $before, after: null};
	def no_op($before): {actions: ["no-op"], before: $before, after: $before};
	{
		errored: false,
		resource_changes: [
			{mode: "managed", address: "aws_apprunner_service.app", change: deletion({})},
			{mode: "managed", address: "aws_iam_role.apprunner_ecr_access", change: deletion({})},
			{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_ecr_access", change: deletion({})},
			{mode: "managed", address: "aws_iam_role.apprunner_instance", change: deletion({})},
			{mode: "managed", address: "aws_iam_role_policy_attachment.google_connections_dynamodb", change: deletion({})},
			{mode: "managed", address: "aws_iam_role_policy_attachment.soccer_sessions_dynamodb", change: deletion({})},
			{mode: "managed", address: "aws_iam_policy.apprunner_runtime_secrets", change: deletion({})},
			{mode: "managed", address: "aws_iam_role_policy_attachment.apprunner_runtime_secrets", change: deletion({})},
			{mode: "managed", address: "aws_lambda_function.app", change: no_op({})}
		],
		output_changes: {
			app_runner_service_url: deletion("url"),
			app_runner_service_arn: deletion("arn"),
			app_runner_service_id: deletion("id"),
			instance_role_arn: deletion("role"),
			lambda_function_name: no_op("portfolio")
		}
	}
' >"$plan_json"

cat >"$fake_bin/aws" <<'EOF'
#!/bin/sh
set -eu
printf 'aws %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	*"sts get-caller-identity"*"--query Arn"*)
		printf 'arn:aws:sts::180294223248:assumed-role/AWSReservedSSO_PortfolioDeployer_123/test\n'
		;;
	*"sts get-caller-identity"*"--query Account"*) printf '180294223248\n' ;;
	*) printf 'unexpected fake aws command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF

cat >"$fake_bin/tofu" <<'EOF'
#!/bin/sh
set -eu
printf 'tofu %s\n' "$*" >>"$COMMAND_LOG"
case "$*" in
	"-chdir=infra init -reconfigure -input=false") ;;
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
			printf 'replaced\n' >>"$plan_file"
		fi
		;;
	*" show -no-color "*) printf 'synthetic human-readable plan\n' ;;
	*" apply "*) : >"$APPLY_MARKER" ;;
	*) printf 'unexpected fake tofu command: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$fake_bin/aws" "$fake_bin/tofu"

real_task=$(command -v task)

task_env() {
	env -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN \
		PATH="$fake_bin:$PATH" \
		COMMAND_LOG="$command_log" \
		FAKE_PLAN_JSON="$plan_json" \
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
	rm -f "$plan_file"
	run_task legacy-apprunner-retirement-plan \
		PLAN_FILE="$plan_file" \
		APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
}

run_apply() {
	mode=${1:-false}
	printf 'reviewed plan\n' >"$plan_file"
	plan_sha256=$(shasum -a 256 "$plan_file" | awk '{print $1}')
	task_env "FAKE_REPLACE_PLAN=$mode" "$real_task" --dir "$repo_root" \
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
grep -Fx "tofu -chdir=infra init -reconfigure -input=false" "$command_log" >/dev/null || fail "legacy init arguments"
if grep -Fq 'backend.hcl' "$command_log"; then fail "legacy init must not reference backend.hcl"; fi
pass "legacy init has no backend.hcl override"

for variable in TF_CLI_ARGS TF_CLI_ARGS_init TF_CLI_ARGS_workspace TF_CLI_ARGS_plan TF_CLI_ARGS_show TF_CLI_ARGS_apply TF_WORKSPACE TF_DATA_DIR; do
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
grep -Fx "tofu -chdir=infra plan -refresh=true -lock-timeout=5m -input=false -out=$plan_file" "$command_log" >/dev/null || fail "retirement plan arguments"
if grep -Eq -- '(-target|-destroy|auto-approve)' "$command_log"; then fail "retirement plan used a forbidden argument"; fi
pass "retirement plan has no forbidden arguments"

: >"$command_log"
rm -f "$apply_marker"
expect_pass "retirement apply revalidates the saved plan before applying" run_apply
test -f "$apply_marker" || fail "retirement apply did not run"
show_line=$(grep -n -F "tofu -chdir=infra show -json $plan_file" "$command_log" | head -n 1 | cut -d: -f1)
apply_line=$(grep -n -F "tofu -chdir=infra apply -input=false $plan_file" "$command_log" | head -n 1 | cut -d: -f1)
test -n "$show_line" && test -n "$apply_line" && test "$show_line" -lt "$apply_line" || fail "retirement apply did not inspect before applying"
pass "retirement apply checks before apply"

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply rejects a mismatched checksum" run_task \
	legacy-apprunner-retirement-apply \
	PLAN_FILE="$plan_file" \
	APPROVED_PLAN_SHA256=wrong \
	APPROVED_STATE_LOCK_URI=s3://portfolio-tofu-state-180294223248/portfolio/terraform.tfstate.tflock
test ! -f "$apply_marker" || fail "retirement apply ran after checksum mismatch"

: >"$command_log"
rm -f "$apply_marker"
expect_fail "retirement apply rejects a replaced saved plan" run_apply true
test ! -f "$apply_marker" || fail "retirement apply ran after plan replacement"

printf '%s tests passed\n' "$pass_count"
