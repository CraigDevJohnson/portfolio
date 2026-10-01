#!/bin/sh
# Accept a CI release plan only when it updates the environment's Lambda image
# and live alias, and the LPS history worker's image when that worker exists.
# Both images must be the release image. The CI roles cannot change anything
# else, so any other change means infrastructure is pending: Craig applies it
# with workloads-admin first.
set -eu

: "${PLAN_JSON:?set PLAN_JSON to the saved-plan JSON path}"
: "${IMAGE_URI:?set IMAGE_URI to the digest-qualified release image}"

fail() {
  printf 'Release plan rejected: %s\n' "$1" >&2
  exit 1
}

printf '%s\n' "$IMAGE_URI" |
  grep -Eq '^[0-9]{12}\.dkr\.ecr\.us-west-2\.amazonaws\.com/portfolio-lambda-releases@sha256:[0-9a-f]{64}$' ||
  fail 'IMAGE_URI must be a digest in portfolio-lambda-releases'
test -f "$PLAN_JSON" || fail 'PLAN_JSON does not exist'

jq -e '.resource_changes | type == "array"' "$PLAN_JSON" > /dev/null ||
  fail 'plan JSON has no resource_changes array'

jq -e '
  all(.resource_changes[];
    .change.importing == null and .previous_address == null and .deposed == null)
' "$PLAN_JSON" > /dev/null || fail 'plan moves, imports or has deposed objects'

jq -e '
  all(.resource_changes[] | select(.mode == "managed");
    .change.actions == ["no-op"] or .change.actions == ["update"])
' "$PLAN_JSON" > /dev/null ||
  fail 'plan creates, replaces or deletes resources; apply infrastructure changes first'

# Attributes that change, ignoring values that are only known after apply.
jq -e --arg image "$IMAGE_URI" '
  def changed_attributes:
    (.change.before // {}) as $before |
    (.change.after // {}) as $after |
    [(.change.after_unknown // {}) | to_entries[] | select(.value == true) | .key] as $unknown |
    [($before + $after) | keys[] |
      select(. as $key | ($unknown | index($key)) == null and $before[$key] != $after[$key])];
  def allowed:
    {
      "module.service.aws_lambda_function.app": ["image_uri"],
      "module.service.aws_lambda_alias.live": ["function_version"],
      "module.service.aws_lambda_function.history_worker[0]": ["image_uri"]
    };
  [.resource_changes[] | select(.mode == "managed" and .change.actions == ["update"])] as $updates |
  [.resource_changes[] | select(.address == "module.service.aws_lambda_function.app")] as $function |
  all($updates[]; (allowed[.address] // null) as $attributes |
    $attributes != null and (changed_attributes - $attributes) == []) and
  ($function | length) == 1 and
  $function[0].change.after.image_uri == $image and
  all($updates[] | select(.address == "module.service.aws_lambda_function.history_worker[0]");
    .change.after.image_uri == $image)
' "$PLAN_JSON" > /dev/null ||
  fail 'plan must change only the Lambda images and live alias, to the release image'

printf 'Release plan accepted: only the Lambda images and live alias change\n'
