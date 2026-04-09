#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Ensures the pushed tag is annotated. If it's lightweight, replaces it
# with an annotated tag pointing at the same commit.
#
# Args: TAG
# Requires: GH_TOKEN or git push credentials

error() { echo "::error::$*"; exit 1; }

TAG="${1:-}"
if test -z "$TAG"; then
	error "usage: release-ensure-annotated.sh <tag>"
fi

TYPE="$(git cat-file -t "$TAG")"

if test "$TYPE" = "tag"; then
	echo "::notice::$TAG is already annotated"
	exit 0
fi

echo "::warning::$TAG is a lightweight tag — converting to annotated"

COMMIT="$(git rev-parse "${TAG}^{commit}")"

# Delete the lightweight tag locally and remotely
git tag -d "$TAG" || error "failed to delete local tag $TAG"
git push origin ":refs/tags/$TAG" || error "failed to delete remote tag $TAG"

# Re-create as annotated and push
git tag -a "$TAG" "$COMMIT" -m "$TAG" || error "failed to create annotated tag $TAG"
git push origin "$TAG" || error "failed to push annotated tag $TAG"

echo "::notice::$TAG converted to annotated tag on $COMMIT"
