#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
for command in aws tofu gh; do
  printf '#!/bin/sh\nprintf "external command invoked\\n" >> "$CALL_LOG"\nexit 99\n' > "$tmp/bin/$command"
  chmod +x "$tmp/bin/$command"
done
for entrypoint in apply-ci-lambda-production.sh deploy-ci-lambda-production.sh; do
  if PATH="$tmp/bin:$PATH" CALL_LOG="$tmp/calls" \
    PRODUCTION_APPLY_ENABLED=true PRODUCTION_ACTIVATION_PREPARATION=true \
    sh "$root/scripts/$entrypoint" > "$tmp/output" 2>&1; then
    echo "Production entrypoint unexpectedly enabled: $entrypoint" >&2
    exit 1
  fi
  grep -Fq 'Production apply is disabled pending readiness and activation review' "$tmp/output"
done
test ! -e "$tmp/calls"
for guard in APPROVED_IDENTITY_SHA256 APPROVED_BACKEND_SHA256 APPROVED_APPROVAL_SHA256 \
  approval.json reviewer_login approval_id apply_authorized \
  manifest_sha256 scan_sha256 plan_json_sha256 plan_text_sha256 policy_sha256 \
  prior_verified_version production_deployment_id sanitize_opentofu_environment \
  validate-production-release.sh resolve-production-rollback-coordinate.sh; do
  grep -Fq "$guard" "$root/scripts/apply-ci-lambda-production.sh" || {
    echo "Production apply contract omits $guard" >&2; exit 1;
  }
done
echo 'Production activation remains disabled before any external command'
