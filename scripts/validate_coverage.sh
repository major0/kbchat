#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Validates that test coverage meets the minimum floor (70%).
#
# Args: COVERAGE_FILE

error() { echo "::error::$*"; exit 1; }

unfloat() {
	set -- "$(printf '%.02f' "${1}")"
	set -- "${1##0}"
	echo "${1}" | sed -e 's/\.//'
}

COVERAGE_FILE="${1:-coverage.out}"

if ! test -f "$COVERAGE_FILE"; then
	error "coverage file not found: $COVERAGE_FILE"
fi

COVERAGE="$(go tool cover -func="$COVERAGE_FILE" | grep "total:" | awk '{print $3}' | sed 's/%//')"
COVERAGE_INT="$(unfloat "$COVERAGE")"
MIN_INT="$(unfloat "70.00")"

echo "Total coverage: ${COVERAGE}%"

if test "$COVERAGE_INT" -lt "$MIN_INT"; then
	error "Coverage ${COVERAGE}% is below minimum floor of 70%"
fi

echo "Coverage meets minimum floor (70%)"
