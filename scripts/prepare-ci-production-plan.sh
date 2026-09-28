#!/bin/sh
set -eu
python3 scripts/check-foundation-alarm-route.py
if [ -n "${BOOTSTRAP_EVIDENCE_JSON:-}" ]; then
  : "${RUNNER_TEMP:?set RUNNER_TEMP}"
  umask 077
  printf '%s' "$BOOTSTRAP_EVIDENCE_JSON" > "$RUNNER_TEMP/bootstrap.json"
  export BOOTSTRAP_EVIDENCE_FILE="$RUNNER_TEMP/bootstrap.json"
fi
sh scripts/plan-ci-lambda-production.sh
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  printf 'plan_sha256=%s\n' "$(sha256sum evidence/prod.tfplan | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"
  printf 'identity_sha256=%s\n' "$(sha256sum evidence/release-identity.json | cut -d' ' -f1)" >> "$GITHUB_OUTPUT"
fi
