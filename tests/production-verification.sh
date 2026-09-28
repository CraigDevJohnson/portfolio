#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
PYTHONDONTWRITEBYTECODE=1 python3 "$root/tests/production-observation.py"
PYTHONDONTWRITEBYTECODE=1 python3 "$root/tests/production-public-observation.py"
