#!/bin/sh
set -eu

: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${PRIOR_VERSION:?set PRIOR_VERSION}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"
: "${ECR_URL:?set ECR_URL}"

printf '%s\n' "$PRIOR_VERSION" | grep -Eq '^[0-9]+$'
TF_VAR_ecr_repository_url="$ECR_URL" \
  TF_VAR_image_digest="$IMAGE_DIGEST" \
  TF_VAR_live_version_override="$PRIOR_VERSION" \
  tofu -chdir=infra/lambda/environments/dev plan \
  -lock-timeout=5m \
  -input=false \
  -out="$EVIDENCE_DIR/rollback.tfplan"
sha256sum "$EVIDENCE_DIR/rollback.tfplan" > "$EVIDENCE_DIR/rollback.sha256"
echo 'Rollback is saved but requires operator review and checked apply.' \
  > "$EVIDENCE_DIR/ROLLBACK_REQUIRES_APPROVAL"
