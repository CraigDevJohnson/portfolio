#!/bin/sh
set -eu

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

stat_metadata() {
	metadata_target=$1
	if metadata_output=$(LC_ALL=C stat -c '%u:%a:%h' "$metadata_target" 2>/dev/null); then
		:
	elif metadata_output=$(LC_ALL=C stat -f '%u:%Mp%Lp:%l' "$metadata_target" 2>/dev/null); then
		:
	else
		fail "Unable to read file metadata with GNU or BSD stat: $metadata_target"
	fi
	printf '%s\n' "$metadata_output"
}

parse_metadata() {
	metadata_value=$1
	metadata_label=$2

	metadata_owner=${metadata_value%%:*}
	metadata_remainder=${metadata_value#*:}
	[ "$metadata_remainder" != "$metadata_value" ] || fail "$metadata_label metadata is malformed"
	metadata_mode=${metadata_remainder%%:*}
	metadata_links=${metadata_remainder#*:}
	[ "$metadata_links" != "$metadata_remainder" ] || fail "$metadata_label metadata is malformed"
	case "$metadata_links" in
		*:*) fail "$metadata_label metadata is malformed" ;;
	esac

	case "$metadata_owner" in
		'' | *[!0-9]*) fail "$metadata_label owner metadata is malformed" ;;
	esac
	case "$metadata_mode" in
		'' | *[!0-7]*) fail "$metadata_label mode metadata is malformed" ;;
	esac
	case "$metadata_mode" in
		0[0-7][0-7][0-7]) metadata_mode=${metadata_mode#0} ;;
	esac
	case "$metadata_links" in
		'' | *[!0-9]*) fail "$metadata_label hard-link metadata is malformed" ;;
	esac
}

[ "$#" -eq 2 ] || fail "usage: $0 parent|created|approved PLAN_FILE"
check_type=$1
plan_file=$2
case "$check_type" in
	parent | created | approved) ;;
	*) fail "unknown retirement plan file check: $check_type" ;;
esac
case "$plan_file" in
	/*) ;;
	*) fail "PLAN_FILE must be absolute" ;;
esac

plan_dir=$(dirname "$plan_file")
[ -d "$plan_dir" ] && [ ! -L "$plan_dir" ] || \
	fail "PLAN_FILE parent must be an existing non-symlink directory"
parse_metadata "$(stat_metadata "$plan_dir")" "PLAN_FILE parent"
current_uid=$(id -u)
[ "$metadata_owner" = "$current_uid" ] || fail "PLAN_FILE parent must be owned by the current user"
[ "$metadata_mode" = 700 ] || fail "PLAN_FILE parent must have mode 700"

[ "$check_type" != parent ] || exit 0

[ -f "$plan_file" ] && [ ! -L "$plan_file" ] || \
	fail "PLAN_FILE must be an existing regular non-symlink file: $plan_file"
parse_metadata "$(stat_metadata "$plan_file")" "PLAN_FILE"
[ "$metadata_owner" = "$current_uid" ] || fail "PLAN_FILE must be owned by the current user"
[ "$metadata_links" = 1 ] || fail "PLAN_FILE must have exactly one hard link"

if [ "$check_type" = approved ]; then
	[ "$metadata_mode" = 400 ] || fail "PLAN_FILE must have mode 400"
fi
