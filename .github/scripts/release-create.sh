#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Creates a GitHub release with auto-generated changelog.
#
# Args: TAG NAME PREV
# Requires: GH_TOKEN (env)

error() { echo "::error::$*"; exit 1; }

TAG="${1:-}"
NAME="${2:-}"
PREV="${3:-}"

if test -z "$TAG"; then
	error "usage: release-create.sh <tag> <name> <prev>"
fi

# Build gh release args. We can't use arrays (bashism), so build the
# command incrementally.
TITLE="${NAME} ${TAG#*/}"
PRERELEASE=""

case "$TAG" in
	*-rc*) PRERELEASE="--prerelease" ;;
esac

if test -n "$PREV"; then
	gh release create "$TAG" --title "$TITLE" --generate-notes --notes-start-tag "$PREV" $PRERELEASE
else
	gh release create "$TAG" --title "$TITLE" --generate-notes $PRERELEASE
fi
