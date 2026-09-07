#!/bin/sh
set -eu
fail() {
  printf 'Management input rejected: %s\n' "$1" >&2
  exit 1
}
EXPECTED_MANAGEMENT_JSON=${EXPECTED_MANAGEMENT_JSON:-null}
printf '%s\n' "$EXPECTED_MANAGEMENT_JSON" | jq -e --arg env "$ENVIRONMENT" '
  . == null or ($env == "dev" and type == "object" and
    (keys | sort) == (["cognito_domain", "cognito_issuer", "cognito_client_id", "redirect_uri", "logout_uri",
      "allowed_emails", "allow_local_callback", "ec2_management_tag_key", "ec2_management_tag_value"] | sort) and
    (.cognito_domain | test("^https://[a-z0-9-]+\\.auth\\.us-west-2\\.amazoncognito\\.com$")) and
    (.cognito_issuer | test("^https://cognito-idp\\.us-west-2\\.amazonaws\\.com/us-west-2_[A-Za-z0-9]+$")) and
    (.cognito_client_id | test("^[a-z0-9]{1,128}$")) and
    .redirect_uri == "https://dev.craigdevjohnson.com/callback" and
    .logout_uri == "https://dev.craigdevjohnson.com/login" and
    .allowed_emails == ["craigdevjohnson@gmail.com"] and
    (.allow_local_callback | type == "boolean") and
    .ec2_management_tag_key == "PortfolioManagement" and .ec2_management_tag_value == "dev")
' >/dev/null || fail "expected management must be the exact reviewed public development settings"
