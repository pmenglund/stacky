#!/usr/bin/env bash
# Cross-compile stacky for a set of platforms and produce release archives.
# Usage: ./tools/package_release.sh [version] [output_dir]
# Defaults: version="dev" and output_dir="dist".
set -euo pipefail

VERSION=${1:-dev}
OUTPUT_DIR=${2:-dist}
WORKSPACE="${BUILD_WORKSPACE_DIRECTORY:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$WORKSPACE"

mkdir -p "$OUTPUT_DIR"

if command -v sha256sum >/dev/null 2>&1; then
  HASH_CMD=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  HASH_CMD=(shasum -a 256)
else
  echo 'Required hash utility (sha256sum or shasum) not found' >&2
  exit 1
fi

platforms=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
)

for platform in "${platforms[@]}"; do
  read -r GOOS GOARCH <<<"$platform"
  pkg="stacky_${VERSION}_${GOOS}_${GOARCH}"
  build_dir="${OUTPUT_DIR}/${pkg}"
  archive_base="${OUTPUT_DIR}/${pkg}"

  rm -rf "$build_dir"
  mkdir -p "$build_dir"

  bin_name="stacky"
  if [[ "$GOOS" == "windows" ]]; then
    bin_name="stacky.exe"
  fi

  echo "Building ${pkg}/${bin_name}" >&2
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w" -o "${build_dir}/${bin_name}" .

  cp LICENSE.txt "${build_dir}/"
  if [[ -f README.md ]]; then
    cp README.md "${build_dir}/"
  fi

  if [[ "$GOOS" == "windows" ]]; then
    (cd "$OUTPUT_DIR" && zip -rq "${archive_base}.zip" "$pkg")
  else
    (cd "$OUTPUT_DIR" && tar -czf "${archive_base}.tar.gz" "$pkg")
  fi

  rm -rf "$build_dir"
done

pushd "$OUTPUT_DIR" >/dev/null
"${HASH_CMD[@]}" stacky_${VERSION}_*.tar.gz stacky_${VERSION}_*.zip > "stacky_${VERSION}_SHA256SUMS.txt"
popd >/dev/null

echo "Release artifacts written to ${OUTPUT_DIR}" >&2
