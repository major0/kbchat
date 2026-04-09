#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Checks for coverage regression between PR and main branch.
# Expects PR_COVERAGE and MAIN_COVERAGE environment variables.
# Fails if coverage drops by more than 1%.

error() { echo "::error::$*"; exit 1; }

unfloat() {
	set -- "$(printf '%.02f' "${1}")"
	set -- "${1##0}"
	echo "${1}" | sed -e 's/\.//'
}

if test "${SKIP_REGRESSION:-}" = "true"; then
	echo "Skipping regression check due to main branch build failures"
	echo "PR branch coverage: ${PR_COVERAGE}%"
	exit 0
fi

echo "Main: ${MAIN_COVERAGE}%, PR: ${PR_COVERAGE}%"

MAIN_INT="$(unfloat "${MAIN_COVERAGE}")"
PR_INT="$(unfloat "${PR_COVERAGE}")"

# Allow up to 1% regression (100 basis points in our integer scale)
THRESHOLD=100
DIFF=$((MAIN_INT - PR_INT))

if test "$DIFF" -gt "$THRESHOLD"; then
	error "Coverage regression: main ${MAIN_COVERAGE}% → PR ${PR_COVERAGE}%"
fi

echo "Coverage check passed"
