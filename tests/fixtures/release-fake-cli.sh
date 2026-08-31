#!/bin/sh
set -eu

command_name=${0##*/}

case "$command_name" in
  gh)
    [ -z "${FAKE_GH_LOG:-}" ] || printf 'gh %s\n' "$*" >> "$FAKE_GH_LOG"
    endpoint=
    for argument in "$@"; do
      case "$argument" in
        repos/*) endpoint=$argument ;;
      esac
    done
    case "$endpoint" in
      */actions/runs/*/attempts/*)
        review_run_id=${endpoint%/attempts/*}
        review_run_id=${review_run_id##*/}
        if [ -n "${FAKE_REVIEW_RUNS_BY_ID_JSON:-}" ]; then
          printf '%s\n' "$FAKE_REVIEW_RUNS_BY_ID_JSON" |
            jq -cer --arg id "$review_run_id" '.[$id]'
        else
          printf '%s\n' "${FAKE_REVIEW_RUN_JSON:?set FAKE_REVIEW_RUN_JSON}"
        fi
        ;;
      */actions/runs/*/approvals)
        review_run_id=${endpoint%/approvals}
        review_run_id=${review_run_id##*/}
        if [ -n "${FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON:-}" ]; then
          printf '%s\n' "$FAKE_REVIEW_APPROVALS_BY_RUN_ID_JSON" |
            jq -cer --arg id "$review_run_id" '.[$id]'
        else
          printf '%s\n' "${FAKE_REVIEW_APPROVALS_JSON:?set FAKE_REVIEW_APPROVALS_JSON}"
        fi
        ;;
      */commits/main)
        if [ -n "${FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER:-}" ] &&
          [ -e "$FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER" ]; then
          printf '%s\n' "${FAKE_MAIN_SHA_AFTER_REVIEW_DEPLOYMENT:?set advanced main SHA}"
        else
          printf '%s\n' "${FAKE_MAIN_SHA:?set FAKE_MAIN_SHA}"
        fi
        ;;
      */commits/*/pulls)
        requested_commit=${endpoint%/pulls}
        requested_commit=${requested_commit##*/}
        if [ -n "${FAKE_PULLS_BY_COMMIT_JSON:-}" ]; then
          printf '%s\n' "$FAKE_PULLS_BY_COMMIT_JSON" |
            jq -cer --arg commit "$requested_commit" '.[$commit]'
        elif [ -n "${FAKE_REVIEW_SOURCE_SHA:-}" ] &&
          [ "$requested_commit" = "$FAKE_REVIEW_SOURCE_SHA" ]; then
          printf '%s\n' "${FAKE_REVIEW_PULLS_JSON:?set FAKE_REVIEW_PULLS_JSON}"
        else
          printf '%s\n' "${FAKE_PULLS_JSON:?set FAKE_PULLS_JSON}"
        fi
        ;;
      */deployments\?*)
        case "$endpoint" in
          *environment=release-review*)
            printf '%s\n' "${FAKE_REVIEW_DEPLOYMENT_PAGES_JSON:-[[]]}"
            ;;
          *) printf '%s\n' "${FAKE_DEPLOYMENT_PAGES_JSON:?set FAKE_DEPLOYMENT_PAGES_JSON}" ;;
        esac
        ;;
      */deployments/*/statuses\?*)
        if printf '%s\n' "$*" | grep -Fq -- '--slurp'; then
          case "$endpoint" in
            */deployments/*/statuses\?*)
              review_deployment_id=${endpoint%/statuses\?*}
              review_deployment_id=${review_deployment_id##*/}
              if [ -n "${FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON:-}" ]; then
                printf '%s\n' "$FAKE_REVIEW_STATUSES_BY_DEPLOYMENT_JSON" |
                  jq -cer --arg id "$review_deployment_id" '[.[$id]]'
              elif [ "$review_deployment_id" = 84 ] &&
                [ -n "${FAKE_REVIEW_STATUS_PAGES_JSON:-}" ]; then
                printf '%s\n' "$FAKE_REVIEW_STATUS_PAGES_JSON"
              elif [ "$review_deployment_id" = 84 ] &&
                [ -n "${FAKE_REVIEW_STATUSES_JSON:-}" ]; then
                printf '[\n%s\n]\n' \
                  "$FAKE_REVIEW_STATUSES_JSON"
              elif [ -n "${FAKE_STATUS_PAGES_JSON:-}" ]; then
                printf '%s\n' "$FAKE_STATUS_PAGES_JSON"
              else
                printf '[\n%s\n]\n' "${FAKE_STATUSES_JSON:?set FAKE_STATUSES_JSON}"
              fi
              ;;
          esac
        else
          if [ -n "${FAKE_REVIEW_STATUSES_JSON:-}" ] &&
            printf '%s\n' "$endpoint" | grep -Fq '/deployments/84/'; then
            printf '%s\n' "$FAKE_REVIEW_STATUSES_JSON"
          else
            printf '%s\n' "${FAKE_STATUSES_JSON:?set FAKE_STATUSES_JSON}"
          fi
        fi
        ;;
      */deployments/*/statuses)
        if printf '%s\n' "$*" | grep -Fq -- '--method POST'; then
          case "${FAKE_DEPLOYMENT_STATUS_FAILURE:-false}" in
            false) ;;
            true)
              printf 'simulated deployment status failure\n' >&2
              exit 1
              ;;
            once)
              failure_state=${FAKE_DEPLOYMENT_STATUS_FAILURE_STATE:?set failure state}
              if [ ! -e "$failure_state" ]; then
                : > "$failure_state"
                printf 'simulated transient deployment status failure\n' >&2
                exit 1
              fi
              ;;
            *)
              echo 'unexpected fake deployment status failure mode' >&2
              exit 2
              ;;
          esac
          requested_state=$(printf '%s\n' "$*" | sed -n 's/.*-f state=\([^ ]*\).*/\1/p')
          if printf '%s\n' "$*" | grep -Fq -- '-f environment=release-review'; then
            requested_description=$(printf '%s\n' "$*" |
              sed -n 's/.*-f description=\(.*\)$/\1/p')
            jq -nc \
              --arg state "$requested_state" \
              --arg description "$requested_description" \
              --arg created_at "${FAKE_REVIEW_STATUS_CREATED_AT:-2026-08-30T00:03:00Z}" '{
                id: 184,
                created_at: $created_at,
                state: $state,
                environment: "release-review",
                environment_url: "",
                description: $description,
                creator: {login: "github-actions[bot]", type: "Bot"}
              }'
          else
            printf '{"id":99,"state":"%s"}\n' "$requested_state"
          fi
        else
          printf '%s\n' "${FAKE_STATUSES_JSON:?set FAKE_STATUSES_JSON}"
        fi
        ;;
      */deployments)
        requested_description=$(printf '%s\n' "$*" |
          sed -n 's/.*-f description=\(.*\)$/\1/p')
        if printf '%s\n' "$*" | grep -Fq -- '-f task=portfolio-lambda-release-review'; then
          if [ -n "${FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER:-}" ]; then
            : > "$FAKE_MAIN_ADVANCE_AFTER_REVIEW_DEPLOYMENT_MARKER"
          fi
          requested_ref=$(printf '%s\n' "$*" | sed -n 's/.*-f ref=\([^ ]*\).*/\1/p')
          jq -nc \
            --arg ref "$requested_ref" \
            --arg description "$requested_description" \
            --arg created_at "${FAKE_REVIEW_DEPLOYMENT_CREATED_AT:-2026-08-30T00:02:00Z}" '{
              id: 84,
              created_at: $created_at,
              ref: $ref,
              sha: $ref,
              task: "portfolio-lambda-release-review",
              environment: "release-review",
              description: $description,
              creator: {login: "github-actions[bot]", type: "Bot"}
            }'
        else
          printf '{"id":42,"description":"%s"}\n' "$requested_description"
        fi
        ;;
      */deployments/*)
        printf '%s\n' "${FAKE_DEPLOYMENT_JSON:?set FAKE_DEPLOYMENT_JSON}"
        ;;
      *)
        echo "unexpected fake gh endpoint: $endpoint" >&2
        exit 2
        ;;
    esac
    ;;
  aws)
    [ -z "${FAKE_AWS_LOG:-}" ] || printf 'aws %s\n' "$*" >> "$FAKE_AWS_LOG"
    service=${1:?missing fake aws service}
    operation=${2:?missing fake aws operation}
    case "$service/$operation" in
      s3api/get-bucket-versioning)
        printf '%s\n' "${FAKE_BUCKET_VERSIONING:-None}"
        ;;
      ecr/describe-repositories)
        printf '%s\n' "${FAKE_REPOSITORY_MUTABILITY:-IMMUTABLE}"
        ;;
      ecr/describe-images)
        if [ -n "${FAKE_ECR_PUSHED_MARKER:-}" ] && [ -f "$FAKE_ECR_PUSHED_MARKER" ]; then
          printf '%s\n' "${FAKE_TAG_DIGEST:?set FAKE_TAG_DIGEST}"
          exit 0
        fi
        case "${FAKE_ECR_LOOKUP_SCENARIO:-existing}" in
          existing) printf '%s\n' "${FAKE_TAG_DIGEST:?set FAKE_TAG_DIGEST}" ;;
          missing)
            printf 'An error occurred (ImageNotFoundException) when calling the ' >&2
            printf 'DescribeImages operation: image does not exist\n' >&2
            exit 254
            ;;
          denied)
            printf 'An error occurred (AccessDeniedException) when calling the ' >&2
            printf 'DescribeImages operation: denied\n' >&2
            exit 254
            ;;
          ambiguous)
            printf 'AccessDeniedException included the words ImageNotFoundException\n' >&2
            exit 254
            ;;
          *)
            echo "unexpected ECR lookup scenario: $FAKE_ECR_LOOKUP_SCENARIO" >&2
            exit 2
            ;;
        esac
        ;;
      ecr/get-login-password)
        printf 'fake-password\n'
        ;;
      ecr/wait)
        ;;
      ecr/describe-image-scan-findings)
        case "${FAKE_ECR_SCAN_SCENARIO:-complete}" in
          complete)
            ;;
          missing-once)
            if [ ! -f "${FAKE_ECR_SCAN_LOOKUP_STATE:?set FAKE_ECR_SCAN_LOOKUP_STATE}" ]; then
              : > "$FAKE_ECR_SCAN_LOOKUP_STATE"
              printf 'An error occurred (ScanNotFoundException) when calling the ' >&2
              printf 'DescribeImageScanFindings operation: scan does not exist yet\n' >&2
              exit 254
            fi
            ;;
          missing)
            printf 'An error occurred (ScanNotFoundException) when calling the ' >&2
            printf 'DescribeImageScanFindings operation: scan does not exist yet\n' >&2
            exit 254
            ;;
          denied)
            printf 'An error occurred (AccessDeniedException) when calling the ' >&2
            printf 'DescribeImageScanFindings operation: denied\n' >&2
            exit 254
            ;;
          ambiguous)
            printf 'AccessDeniedException included the words ScanNotFoundException\n' >&2
            exit 254
            ;;
          *)
            echo "unexpected ECR scan scenario: $FAKE_ECR_SCAN_SCENARIO" >&2
            exit 2
            ;;
        esac
        printf '%s\n' \
          '{"imageScanStatus":{"status":"COMPLETE"},' \
          '"imageScanFindings":{"findingSeverityCounts":{"CRITICAL":0}}}'
        ;;
      lambda/get-alias)
        if [ "${FAKE_ALIAS_FAILURE:-false}" = true ]; then
          echo 'simulated get-alias access failure' >&2
          exit 1
        fi
        printf '{"FunctionVersion":"%s"}\n' "${FAKE_ALIAS_VERSION:-7}"
        ;;
      lambda/get-function)
        qualified=false
        for argument in "$@"; do
          if [ "$argument" = "--qualifier" ]; then
            qualified=true
          fi
        done
        [ "$qualified" = true ] || {
          echo 'Lambda verification must request an immutable published version' >&2
          exit 2
        }
        printf '{"Code":{"ImageUri":"%s"}}\n' "${FAKE_QUALIFIED_IMAGE_URI:?set FAKE_QUALIFIED_IMAGE_URI}"
        ;;
      cloudwatch/describe-alarms)
        case "${FAKE_ALARM_SCENARIO:-exact}" in
          exact)
            printf '%s\n' \
              '{"MetricAlarms":[' \
              '{"AlarmName":"portfolio-lambda-dev-api-5xx","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-api-latency","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-duration","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-errors","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-throttles","StateValue":"OK"}' \
              ']}'
            ;;
          missing)
            printf '%s\n' '{"MetricAlarms":[]}'
            ;;
          extra)
            printf '%s\n' \
              '{"MetricAlarms":[' \
              '{"AlarmName":"portfolio-lambda-dev-api-5xx","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-api-latency","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-duration","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-errors","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-throttles","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-unapproved","StateValue":"OK"}' \
              ']}'
            ;;
          insufficient)
            printf '%s\n' \
              '{"MetricAlarms":[' \
              '{"AlarmName":"portfolio-lambda-dev-api-5xx","StateValue":"INSUFFICIENT_DATA"},' \
              '{"AlarmName":"portfolio-lambda-dev-api-latency","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-duration","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-errors","StateValue":"OK"},' \
              '{"AlarmName":"portfolio-lambda-dev-lambda-throttles","StateValue":"OK"}' \
              ']}'
            ;;
          *)
            echo "unexpected fake alarm scenario: $FAKE_ALARM_SCENARIO" >&2
            exit 2
            ;;
        esac
        ;;
      *)
        echo "unexpected fake aws command: $service $operation" >&2
        exit 2
        ;;
    esac
    ;;
  curl)
    [ -z "${FAKE_CURL_ARGUMENT_LOG:-}" ] || printf 'curl %s\n' "$*" >> "$FAKE_CURL_ARGUMENT_LOG"
    headers=
    body=
    write_out=
    url=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -D)
          headers=$2
          shift 2
          ;;
        -o)
          body=$2
          shift 2
          ;;
        -w | --write-out)
          write_out=$2
          shift 2
          ;;
        --connect-timeout | --max-time | --max-redirs | --connect-to) shift 2 ;;
        -*) shift ;;
        *)
          url=$1
          shift
          ;;
      esac
    done
    [ -z "${FAKE_CURL_LOG:-}" ] || printf '%s\n' "$url" >> "$FAKE_CURL_LOG"
    status=200
    content_type=text/html
    case "$url" in
      */healthz) content_type=application/json ;;
      */static/css/tailwind.css) content_type=text/css ;;
      */static/images/backgrounds/home-hero.jpg) content_type=image/jpeg ;;
    esac
    case "${FAKE_ROUTE_SCENARIO:-exact}:$url" in
      redirect:*/soccer) status=302 ;;
      wrong-content:*/static/css/tailwind.css) content_type=text/html ;;
    esac
    if [ "${url##*/}" = healthz ]; then
      case "${FAKE_HEALTH_SCENARIO:-exact}" in
        exact) ;;
        redirect) status=302 ;;
        wrong-content) content_type=text/html ;;
        degraded) ;;
        *)
          echo "unexpected fake health scenario: $FAKE_HEALTH_SCENARIO" >&2
          exit 2
          ;;
      esac
    fi
    [ -z "$headers" ] || printf 'HTTP/1.1 %s Test\r\nContent-Type: %s\r\n\r\n' \
      "$status" "$content_type" > "$headers"
    if [ -n "$body" ]; then
      case "$url" in
        */healthz)
          health_status=ok
          [ "${FAKE_HEALTH_SCENARIO:-exact}" != degraded ] || health_status=degraded
          printf '{"status":"%s","revision":"%s"}\n' \
            "$health_status" "${FAKE_HEALTH_SHA:?set FAKE_HEALTH_SHA}" > "$body"
          ;;
        *)
          case "$content_type:$url" in
            image/jpeg:*) printf '\377\330\377fake-jpeg' > "$body" ;;
            text/css:*) printf 'body { color: black; }\n' > "$body" ;;
            *) printf '<!doctype html><html><body>ok</body></html>\n' > "$body" ;;
          esac
          ;;
      esac
    fi
    [ -z "$write_out" ] || printf '%s\n%s\n' "$status" "$content_type"
    ;;
  docker)
    [ -z "${FAKE_DOCKER_LOG:-}" ] || printf 'docker %s\n' "$*" >> "$FAKE_DOCKER_LOG"
    case "${1:?missing fake docker command}" in
      login) cat > /dev/null ;;
      buildx) ;;
      push)
        if [ "${FAKE_DOCKER_SIGNAL_PARENT_ON_PUSH:-false}" = true ]; then
          kill -TERM "$PPID"
        fi
        [ -z "${FAKE_ECR_PUSHED_MARKER:-}" ] || : > "$FAKE_ECR_PUSHED_MARKER"
        ;;
      *)
        echo "unexpected fake docker command: $1" >&2
        exit 2
        ;;
    esac
    ;;
  sleep)
    [ -z "${FAKE_SLEEP_LOG:-}" ] || printf 'sleep %s\n' "$*" >> "$FAKE_SLEEP_LOG"
    ;;
  tofu)
    [ -z "${FAKE_TOFU_LOG:-}" ] || printf 'tofu %s\n' "$*" >> "$FAKE_TOFU_LOG"
    case "$*" in
      *' apply '*)
        if [ "${FAKE_TOFU_APPLY_SIGNAL:-false}" = true ]; then
          kill -TERM "$PPID"
          exit 143
        fi
        [ "${FAKE_TOFU_APPLY_FAILURE:-false}" != true ] || {
          echo 'simulated apply failure' >&2
          exit 1
        }
        ;;
      *' output -json')
        [ "${FAKE_TOFU_OUTPUT_FAILURE:-false}" != true ] || {
          echo 'simulated output failure' >&2
          exit 1
        }
        printf '%s\n' '{
          "api_gateway_domain_targets": {
            "value": {
              "dev.craigdevjohnson.com": "origin.example.invalid"
            }
          }
        }'
        ;;
      *)
        echo "unexpected fake tofu command: $*" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    echo "unexpected fake command: $command_name" >&2
    exit 2
    ;;
esac
