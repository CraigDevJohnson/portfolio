#!/bin/sh
set -eu

: "${SOURCE_SHA:?set SOURCE_SHA}"
: "${IMAGE_DIGEST:?set IMAGE_DIGEST}"
: "${PROMOTION_SHA:?set PROMOTION_SHA}"
: "${PRODUCTION_DEPLOYMENT_ID:?set PRODUCTION_DEPLOYMENT_ID}"
: "${API_ID:?set API_ID}"
: "${APEX_ORIGIN_HOST:?set APEX_ORIGIN_HOST}"
: "${EVIDENCE_DIR:?set EVIDENCE_DIR}"

# This driver never exercises the deferred management portal. Browser acceptance
# is handed off separately alongside the CI origin/AWS observation window.
script_dir=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
exec python3 "$script_dir/observe-lambda-production.py" observe
