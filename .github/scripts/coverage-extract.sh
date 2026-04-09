#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Extracts coverage percentage from a coverage.out file.
# Outputs the numeric percentage (no % sign).
#
# Args: COVERAGE_FILE

error() { echo "::error::$*"; exit 1; }

COVERAGE_FILE="${1:-}"
if test -z "$COVERAGE_FILE"; then
	error "usage: coverage-extract.sh <coverage-file>"
fi

go tool cover -func="$COVERAGE_FILE" | grep "total:" | awk '{print $3}' | sed 's/%//'
