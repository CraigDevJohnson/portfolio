#!/bin/sh
set -eu
fail() {
  printf 'Management input rejected: %s\n' "$1" >&2
  exit 1
}
EXPECTED_MANAGEMENT_JSON=${EXPECTED_MANAGEMENT_JSON:-null}
printf '%s\n' "$EXPECTED_MANAGEMENT_JSON" | jq -e -s --arg env "$ENVIRONMENT" '
  def matches_string($pattern):
    if type == "string" then test($pattern) else false end;
  if length != 1 then false
  else .[0] |
    if . == null then true
    elif type != "object" then false
    else ($env == "dev" and
    (keys | sort) == (["cognito_domain", "cognito_issuer", "cognito_client_id", "redirect_uri", "logout_uri",
      "allowed_emails", "allow_local_callback", "ec2_management_tag_key", "ec2_management_tag_value"] | sort) and
    (.cognito_domain | matches_string("^https://[a-z0-9-]+\\.auth\\.us-west-2\\.amazoncognito\\.com$")) and
    (.cognito_issuer | matches_string("^https://cognito-idp\\.us-west-2\\.amazonaws\\.com/us-west-2_[A-Za-z0-9]+$")) and
    (.cognito_client_id | matches_string("^[a-z0-9]{1,128}$")) and
    .redirect_uri == "https://dev.craigdevjohnson.com/callback" and
    .logout_uri == "https://dev.craigdevjohnson.com/login" and
    .allowed_emails == ["craigdevjohnson@gmail.com"] and
    (.allow_local_callback | type == "boolean") and
    .ec2_management_tag_key == "PortfolioManagement" and .ec2_management_tag_value == "dev")
    end
  end
' >/dev/null 2>&1 || fail "expected management must be the exact reviewed public development settings"
