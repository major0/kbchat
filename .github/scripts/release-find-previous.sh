#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Finds the previous tag in the same series for changelog generation.
#
# RC tags: find the nearest ancestor tag in the series.
# Stable tags: skip past RC tags to find the previous stable release.
#
# Args: TAG PREFIX

error() { echo "::error::$*"; exit 1; }

TAG="${1:-}"
PREFIX="${2:-}"

if test -z "$TAG"; then
	error "usage: release-find-previous.sh <tag> <prefix>"
fi

PREV=""

case "$TAG" in
	*-rc*)
		PREV="$(git describe --tags --abbrev=0 --match "${PREFIX}*" \
			"${TAG}^" 2>/dev/null || true)"
		;;
	*)
		COMMIT="${TAG}^"
		I=0
		while test "$I" -lt 50; do
			CANDIDATE="$(git describe --tags --abbrev=0 --match "${PREFIX}*" \
				"$COMMIT" 2>/dev/null || true)"
			if test -z "$CANDIDATE"; then
				break
			fi
			case "$CANDIDATE" in
				*-rc*) COMMIT="${CANDIDATE}^" ;;
				*)     PREV="$CANDIDATE"; break ;;
			esac
			I=$((I + 1))
		done
		;;
esac

if test -z "$PREV"; then
	echo "::notice::No previous tag found for series ${PREFIX}*; changelog will cover full history"
else
	echo "::notice::Previous tag: $PREV"
fi
echo "tag=$PREV" >> "$GITHUB_OUTPUT"
