#!/usr/bin/env bash
set -euo pipefail

WORKSPACE="${BUILD_WORKSPACE_DIRECTORY:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$WORKSPACE"

GO_BUILD_CACHE="$WORKSPACE/.gocache/build"
GO_MOD_CACHE="$WORKSPACE/.gocache/mod"

mkdir -p "$GO_BUILD_CACHE" "$GO_MOD_CACHE"

GOCACHE="$GO_BUILD_CACHE" GOMODCACHE="$GO_MOD_CACHE" GOFLAGS="-mod=readonly" go test ./...
