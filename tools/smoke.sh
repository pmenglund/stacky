#!/usr/bin/env bash
set -euo pipefail

WORKSPACE="${BUILD_WORKSPACE_DIRECTORY:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$WORKSPACE"

GO_BUILD_CACHE="$WORKSPACE/.gocache/build"

mkdir -p "$GO_BUILD_CACHE"

# Allow overriding the go test arguments while providing sensible defaults.
if [ "$#" -eq 0 ]; then
  set -- ./internal/e2e -run 'TestStackInfoSmoke|TestStackPushPushesBranches' -count=1
else
  set -- "$@"
fi

GOCACHE="$GO_BUILD_CACHE" \
GOFLAGS="-mod=readonly" \
go test "$@"
