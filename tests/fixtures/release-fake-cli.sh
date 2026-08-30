#!/bin/sh
set -eu

command_name=${0##*/}

case "$command_name" in
	gh)
		endpoint=
		for argument in "$@"; do
			case "$argument" in
				repos/*) endpoint=$argument ;;
			esac
		done
		case "$endpoint" in
			*/commits/main)
				printf '%s\n' "${FAKE_MAIN_SHA:?set FAKE_MAIN_SHA}"
				;;
			*/commits/*/pulls)
				printf '%s\n' "${FAKE_PULLS_JSON:?set FAKE_PULLS_JSON}"
				;;
			*/deployments/*/statuses)
				printf '%s\n' "${FAKE_STATUSES_JSON:?set FAKE_STATUSES_JSON}"
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
		service=${1:?missing fake aws service}
		operation=${2:?missing fake aws operation}
		case "$service/$operation" in
			s3api/get-bucket-versioning)
				printf '%s\n' "${FAKE_BUCKET_VERSIONING:-None}"
				;;
			ecr/describe-images)
				printf '%s\n' "${FAKE_TAG_DIGEST:?set FAKE_TAG_DIGEST}"
				;;
			lambda/get-alias)
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
				printf '{"MetricAlarms":[]}\n'
				;;
			*)
				echo "unexpected fake aws command: $service $operation" >&2
				exit 2
				;;
		esac
		;;
	curl)
		headers=
		body=
		url=
		while [ "$#" -gt 0 ]; do
			case "$1" in
				-D) headers=$2; shift 2 ;;
				-o) body=$2; shift 2 ;;
				-*) shift ;;
				*) url=$1; shift ;;
			esac
		done
		if [ "${url##*/}" = healthz ]; then
			printf '{"revision":"%s"}\n' "${FAKE_HEALTH_SHA:?set FAKE_HEALTH_SHA}"
		else
			[ -z "$headers" ] || printf 'HTTP/1.1 200 OK\n' >"$headers"
			[ -z "$body" ] || printf 'ok\n' >"$body"
		fi
		;;
	*)
		echo "unexpected fake command: $command_name" >&2
		exit 2
		;;
esac
