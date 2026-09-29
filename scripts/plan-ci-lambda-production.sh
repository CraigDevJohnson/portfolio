#!/bin/sh
# Create the saved production plan for the image that development just
# verified. The production job applies exactly this plan after Craig approves
# the production environment.
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${GITHUB_WORKSPACE:?set GITHUB_WORKSPACE}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
evidence_dir="$GITHUB_WORKSPACE/evidence"

sh scripts/check-current-main.sh "$SOURCE_SHA"
sh scripts/check-ci-state-bucket.sh
RELEASE_ENVIRONMENT=production EVIDENCE_DIR="$evidence_dir" \
  sh scripts/create-ci-lambda-release-plan.sh

plan_sha256=$(shasum -a 256 "$evidence_dir/prod.tfplan" | awk '{print $1}')
printf 'plan_sha256=%s\n' "$plan_sha256" >> "$GITHUB_OUTPUT"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    printf '### Production plan for %s\n\n' "$SOURCE_SHA"
    # The backticks are Markdown, not shell syntax.
    # shellcheck disable=SC2016
    printf 'Image `%s`, saved plan SHA-256 `%s`.\n\n' "$IMAGE_DIGEST" "$plan_sha256"
    printf '```text\n'
    cat "$evidence_dir/prod-plan.txt"
    printf '```\n'
  } >> "$GITHUB_STEP_SUMMARY"
fi
