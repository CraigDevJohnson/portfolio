#!/bin/sh
# The operator plan and apply tasks must give OpenTofu the same inputs. When it
# applies a saved plan, OpenTofu re-reads *.auto.tfvars and -var flags and
# refuses any input whose raw value differs from the plan's, and a -var value
# never matches the same value from a file. For each environment this runs the
# exact tofu commands `task --dry` prints against a local root with the
# environment's real variables.tf and auto.tfvars. No backend, provider or AWS.
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

# run_task ENV ACTION TASK_ARGS...: run the tofu ACTION command that task
# lambda-ENV-ACTION prints, with the environment root swapped for the fixture.
run_task() {
  env=$1 action=$2
  shift 2
  real="tofu -chdir=\"infra/lambda/environments/$env\" $action "
  # --color=false: under CI (no TTY) Task colours its lines, and the escape
  # codes would hide the "task: [...]" prefix from sed.
  command=$(task --color=false --dir "$root_dir" --dry --verbose "lambda-$env-$action" "$@" 2>&1 |
    sed -n "s/^task: \[[^]]*\] //; /^tofu -chdir=\"[^\"]*\" $action /p")
  case "$command" in
    *"
"*) fail "lambda-$env-$action $* printed more than one $action command" ;;
    "$real"*) ;;
    *) fail "lambda-$env-$action $* printed no $action command for the $env root" ;;
  esac
  command="tofu -chdir=\"$test_dir/$env\" $action ${command#"$real"}"
  (cd "$test_dir" && eval "$command") > "$test_dir/tofu.log" 2>&1 || {
    cat "$test_dir/tofu.log" >&2
    fail "lambda-$env-$action $* failed against the fixture"
  }
}

# expect_release ENV EXPECTED_ACTIVATE [ACTIVATE_CUSTOM_DOMAIN=...]: plan and
# apply one release the way DEPLOY-INSTRUCTIONS.md shows.
expect_release() {
  env=$1 expected=$2
  shift 2
  plan_file="$test_dir/$env.tfplan"
  rm -f "$plan_file"
  run_task "$env" plan IMAGE_DIGEST="$digest" PLAN_FILE="$plan_file" "$@"
  run_task "$env" apply PLAN_FILE="$plan_file" "$@"
  applied=$(tofu -chdir="$test_dir/$env" show -json |
    jq -c '.values.root_module.resources[0].values.input.activate_custom_domain')
  [ "$applied" = "$expected" ] ||
    fail "lambda-$env $* applied activate_custom_domain=$applied, want $expected"
}

for env in dev prod; do
  fixture="$test_dir/$env"
  mkdir -p "$fixture"
  cp "$root_dir/infra/lambda/environments/$env/variables.tf" \
    "$root_dir/infra/lambda/environments/$env/$env.auto.tfvars" "$fixture/"
  grep -Eq '^activate_custom_domain +=' "$fixture/$env.auto.tfvars" ||
    fail "$env.auto.tfvars no longer sets activate_custom_domain; revisit this test"
  cat > "$fixture/main.tf" << 'HCL'
resource "terraform_data" "release" {
  input = {
    image                  = "${var.ecr_repository_url}@${var.image_digest}"
    activate_custom_domain = var.activate_custom_domain
    alarm_action_arns      = var.alarm_action_arns
  }
}
HCL
  tofu -chdir="$fixture" init -input=false > /dev/null

  # The hostname-free step first, then the domain step, then an explicit true.
  expect_release "$env" false ACTIVATE_CUSTOM_DOMAIN=false
  expect_release "$env" true
  expect_release "$env" true ACTIVATE_CUSTOM_DOMAIN=true
done

printf 'Operator plan and apply contracts passed\n'
