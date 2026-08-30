#!/bin/sh
set -eu

root_dir=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

fake_bin="$test_dir/bin"
mkdir -p "$fake_bin"
for command_name in gh aws curl; do
	ln -s "$root_dir/tests/fixtures/release-fake-cli.sh" "$fake_bin/$command_name"
done
PATH="$fake_bin:$PATH"
export PATH

source_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
other_sha=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
image_digest=sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
other_digest=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
export GITHUB_REPOSITORY=CraigDevJohnson/portfolio

FAKE_MAIN_SHA=$source_sha
export FAKE_MAIN_SHA
sh "$root_dir/scripts/check-current-main.sh" "$source_sha"
if sh "$root_dir/scripts/check-current-main.sh" "$other_sha" 2>/dev/null; then
	echo 'stale main was accepted' >&2
	exit 1
fi

FAKE_PULLS_JSON=$(printf '[{"state":"closed","merged_at":"2026-08-30T00:00:00Z","merge_commit_sha":"%s","base":{"ref":"main","sha":"%s"}}]' "$source_sha" "$other_sha")
export FAKE_PULLS_JSON
test "$(sh "$root_dir/scripts/resolve-reviewed-release-base.sh" "$source_sha")" = "$other_sha"
FAKE_PULLS_JSON='[]'
export FAKE_PULLS_JSON
if sh "$root_dir/scripts/resolve-reviewed-release-base.sh" "$source_sha" >/dev/null 2>&1; then
	echo 'unreviewed source commit was accepted' >&2
	exit 1
fi

manifest="$test_dir/production-release.json"
printf '{"source_sha":"%s","image_digest":"%s","development_deployment_id":42}\n' "$source_sha" "$image_digest" >"$manifest"
FAKE_DEPLOYMENT_JSON=$(printf '{"ref":"%s","environment":"development","description":"Lambda %s"}' "$source_sha" "$image_digest")
FAKE_STATUSES_JSON=$(printf '[{"state":"success","environment":"development","description":"Verified %s at %s"}]' "$source_sha" "$image_digest")
FAKE_TAG_DIGEST=$image_digest
export FAKE_DEPLOYMENT_JSON FAKE_STATUSES_JSON FAKE_TAG_DIGEST
ECR_REPOSITORY=portfolio-lambda-releases sh "$root_dir/scripts/validate-production-release.sh" "$manifest"
FAKE_DEPLOYMENT_JSON=$(printf '{"ref":"%s","environment":"development","description":"Lambda %s"}' "$other_sha" "$image_digest")
export FAKE_DEPLOYMENT_JSON
if ECR_REPOSITORY=portfolio-lambda-releases sh "$root_dir/scripts/validate-production-release.sh" "$manifest" >/dev/null 2>&1; then
	echo 'promotion accepted an unrelated deployment' >&2
	exit 1
fi
FAKE_DEPLOYMENT_JSON=$(printf '{"ref":"%s","environment":"development","description":"Lambda %s"}' "$source_sha" "$image_digest")
FAKE_STATUSES_JSON=$(printf '[{"state":"failure","environment":"development","description":"Verified %s at %s"}]' "$source_sha" "$image_digest")
export FAKE_DEPLOYMENT_JSON FAKE_STATUSES_JSON
if ECR_REPOSITORY=portfolio-lambda-releases sh "$root_dir/scripts/validate-production-release.sh" "$manifest" >/dev/null 2>&1; then
	echo 'promotion accepted a failed development deployment' >&2
	exit 1
fi
FAKE_STATUSES_JSON=$(printf '[{"state":"success","environment":"development","description":"Verified %s at %s"}]' "$source_sha" "$image_digest")
FAKE_TAG_DIGEST=$other_digest
export FAKE_STATUSES_JSON FAKE_TAG_DIGEST
if ECR_REPOSITORY=portfolio-lambda-releases sh "$root_dir/scripts/validate-production-release.sh" "$manifest" >/dev/null 2>&1; then
	echo 'promotion accepted a source tag bound to another digest' >&2
	exit 1
fi
FAKE_TAG_DIGEST=$image_digest
export FAKE_TAG_DIGEST

FAKE_BUCKET_VERSIONING=Enabled
export FAKE_BUCKET_VERSIONING
STATE_BUCKET=portfolio-tofu-state-180294223248 sh "$root_dir/scripts/check-ci-state-bucket.sh"
FAKE_BUCKET_VERSIONING=Suspended
export FAKE_BUCKET_VERSIONING
if STATE_BUCKET=portfolio-tofu-state-180294223248 sh "$root_dir/scripts/check-ci-state-bucket.sh" >/dev/null 2>&1; then
	echo 'suspended state-bucket versioning was accepted' >&2
	exit 1
fi

evidence_dir="$test_dir/evidence"
FAKE_HEALTH_SHA=$source_sha
FAKE_QUALIFIED_IMAGE_URI="example.invalid/portfolio@$image_digest"
export FAKE_HEALTH_SHA FAKE_QUALIFIED_IMAGE_URI
BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
	FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
	sh "$root_dir/scripts/verify-lambda-release.sh"
FAKE_ALARM_SCENARIO=missing
export FAKE_ALARM_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
	FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
	sh "$root_dir/scripts/verify-lambda-release.sh" >/dev/null 2>&1; then
	echo 'release verification accepted missing alarms' >&2
	exit 1
fi
FAKE_ALARM_SCENARIO=extra
export FAKE_ALARM_SCENARIO
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
	FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
	sh "$root_dir/scripts/verify-lambda-release.sh" >/dev/null 2>&1; then
	echo 'release verification accepted an extra alarm' >&2
	exit 1
fi
unset FAKE_ALARM_SCENARIO
FAKE_QUALIFIED_IMAGE_URI="example.invalid/portfolio@$other_digest"
export FAKE_QUALIFIED_IMAGE_URI
if BASE_URL=https://example.invalid SOURCE_SHA=$source_sha IMAGE_DIGEST=$image_digest \
	FUNCTION_NAME=portfolio-lambda-dev EVIDENCE_DIR="$evidence_dir" \
	sh "$root_dir/scripts/verify-lambda-release.sh" >/dev/null 2>&1; then
	echo 'live alias version with a different digest was accepted' >&2
	exit 1
fi

tmp="$test_dir/repository"
mkdir -p "$tmp"
git -C "$tmp" init -q
git -C "$tmp" config user.email test@example.com
git -C "$tmp" config user.name Test
mkdir -p "$tmp/internal/app" "$tmp/docs" "$tmp/deploy" "$tmp/.github/workflows"
echo a >"$tmp/internal/app/app.go"; git -C "$tmp" add .; git -C "$tmp" commit -qm base; base=$(git -C "$tmp" rev-parse HEAD)
echo b >>"$tmp/internal/app/app.go"; git -C "$tmp" commit -qam runtime; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = development
base=$head; echo doc >"$tmp/docs/readme.md"; git -C "$tmp" add .; git -C "$tmp" commit -qm docs; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = skip
base=$head; echo '{}' >"$tmp/deploy/production-release.json"; git -C "$tmp" add .; git -C "$tmp" commit -qm promote; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = production
base=$head; echo x >"$tmp/.github/workflows/x.yml"; git -C "$tmp" add .; git -C "$tmp" commit -qm workflow; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = review
base=$head; echo runtime >"$tmp/internal/app/renamed.go"; git -C "$tmp" add .; git -C "$tmp" commit -qm rename-runtime-source; base=$(git -C "$tmp" rev-parse HEAD)
git -C "$tmp" mv internal/app/renamed.go docs/renamed.md; git -C "$tmp" commit -qm rename-runtime-to-docs; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = development
base=$head; mkdir -p "$tmp/infra"; echo infra >"$tmp/infra/renamed.tf"; git -C "$tmp" add .; git -C "$tmp" commit -qm rename-infra-source; base=$(git -C "$tmp" rev-parse HEAD)
git -C "$tmp" mv infra/renamed.tf internal/app/renamed.tf; git -C "$tmp" commit -qm rename-infra-to-internal; head=$(git -C "$tmp" rev-parse HEAD)
test "$(cd "$tmp" && sh "$root_dir/scripts/classify-release-change.sh" "$base" "$head")" = review

workflow_job() {
	awk -v target="  $1:" '
		/^  [[:alnum:]_-]+:$/ {
			if (selected) exit
			selected = ($0 == target)
		}
		selected { print }
	' "$root_dir/.github/workflows/release.yml"
}

first_matching_line() {
	awk -v needle="$2" 'index($0, needle) { print NR; exit }' <<EOF
$1
EOF
}

last_matching_line() {
	awk -v needle="$2" 'index($0, needle) { line = NR } END { if (line) print line }' <<EOF
$1
EOF
}

count_matching_lines() {
	awk -v needle="$2" 'index($0, needle) { count++ } END { print count + 0 }' <<EOF
$1
EOF
}

assert_before() {
	first=$(first_matching_line "$1" "$2")
	second=$(first_matching_line "$1" "$3")
	test -n "$first" && test -n "$second" && test "$first" -lt "$second"
}

authorize_job=$(workflow_job authorize)
build_job=$(workflow_job build)
development_job=$(workflow_job development)
production_job=$(workflow_job production-plan)

grep -Fq 'task lambda-infrastructure-ci' "$root_dir/.github/workflows/ci.yml"
grep -Fq 'sh tests/release-automation.sh' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-init:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-plan:' "$root_dir/Taskfile.yaml"
grep -Fq 'lambda-ci-roles-apply:' "$root_dir/Taskfile.yaml"
grep -Fq 'task lambda-ci-roles-init' "$root_dir/infra/lambda/ci-roles/README.md"
grep -Fq 'task lambda-ci-roles-plan' "$root_dir/infra/lambda/ci-roles/README.md"
grep -Fq 'task lambda-ci-roles-apply' "$root_dir/infra/lambda/ci-roles/README.md"
grep -Fq '  pull-requests: read' "$root_dir/.github/workflows/release.yml"

test "$(count_matching_lines "$authorize_job" 'sh scripts/check-current-main.sh')" -eq 1
assert_before "$authorize_job" 'sh scripts/check-current-main.sh' 'sh scripts/resolve-reviewed-release-base.sh'
assert_before "$authorize_job" 'sh scripts/resolve-reviewed-release-base.sh' "sh scripts/classify-release-change.sh \"\$BASE_SHA\" \"\$SOURCE_SHA\""

test "$(count_matching_lines "$build_job" 'sh scripts/check-current-main.sh')" -eq 1
assert_before "$build_job" 'sh scripts/check-current-main.sh' 'uses: aws-actions/configure-aws-credentials@v6'
grep -Fq 'if-no-files-found: error' <<EOF
$build_job
EOF
grep -Fq 'Immutable full-SHA tag already exists; reusing it for scan validation.' <<EOF
$build_job
EOF
if grep -Fq 'Immutable full-SHA tag already exists; refusing to rebuild.' <<EOF
$build_job
EOF
then
	echo 'release retry still refuses an existing immutable image' >&2
	exit 1
fi
awk '
	/if: always\(\)/ { always = NR }
	/uses: actions\/upload-artifact@v7/ && always && NR == always + 1 { found = 1 }
	END { exit(found ? 0 : 1) }
' <<EOF
$build_job
EOF

test "$(count_matching_lines "$development_job" 'sh scripts/check-current-main.sh')" -eq 2
assert_before "$development_job" 'sh scripts/check-current-main.sh' 'uses: aws-actions/configure-aws-credentials@v6'
grep -Fq 'AUTOMATED_RELEASE: "true"' <<EOF
$development_job
EOF
development_last_guard=$(last_matching_line "$development_job" 'sh scripts/check-current-main.sh')
development_apply=$(first_matching_line "$development_job" 'tofu -chdir=infra/lambda/environments/dev apply')
test "$development_last_guard" -lt "$development_apply"

grep -Fq 'environment: production-plan' <<EOF
$production_job
EOF
test "$(count_matching_lines "$production_job" 'sh scripts/check-current-main.sh')" -eq 2
assert_before "$production_job" 'sh scripts/check-current-main.sh' 'uses: aws-actions/configure-aws-credentials@v6'
production_last_guard=$(last_matching_line "$production_job" 'sh scripts/check-current-main.sh')
production_init=$(first_matching_line "$production_job" 'tofu -chdir=infra/lambda/environments/prod init')
test "$production_last_guard" -lt "$production_init"
printf 'Release automation contracts passed\n'
