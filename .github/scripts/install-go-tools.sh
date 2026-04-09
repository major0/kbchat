#!/bin/sh
set -eu
POSIXLY_CORRECT='no bashing shell'

# Installs Go development tools (goimports, golangci-lint).
# Appends GOPATH/bin to GITHUB_PATH if running in CI.

error() { echo "::error::$*"; exit 1; }

go install golang.org/x/tools/cmd/goimports@latest || error "failed to install goimports"
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" latest || error "failed to install golangci-lint"

if test -n "${GITHUB_PATH:-}"; then
	echo "$(go env GOPATH)/bin" >> "$GITHUB_PATH"
fi
