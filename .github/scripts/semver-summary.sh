#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Writes a semver summary to GITHUB_STEP_SUMMARY.
#
# Args: TAG TAG_TYPE

error() { echo "::error::$*"; exit 1; }

TAG="${1:-}"
TAG_TYPE="${2:-}"

if test -z "$TAG"; then
	error "usage: semver-summary.sh <tag> <tag-type>"
fi

{
	echo "### Semver Summary"
	echo "- **Tag**: ${TAG}"
	echo "- **Type**: ${TAG_TYPE}"
} >> "$GITHUB_STEP_SUMMARY"
