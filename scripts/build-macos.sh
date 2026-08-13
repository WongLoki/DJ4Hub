#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST_DIR="${ROOT_DIR}/dist"

mkdir -p "${DIST_DIR}"

cd "${ROOT_DIR}"

ARCH=$(go env GOARCH)
PKG_CONFIG_PATH="${PKG_CONFIG_PATH:-/opt/homebrew/lib/pkgconfig:/usr/local/lib/pkgconfig}"
export PKG_CONFIG_PATH

CGO_ENABLED=1 GOOS=darwin GOARCH="${ARCH}" go build \
  -p 2 \
  -trimpath -ldflags="-s -w" \
  -o "${DIST_DIR}/dj4ghub-macos-${ARCH}" ./cmd/dj4ghub-macos

cp "${DIST_DIR}/dj4ghub-macos-${ARCH}" "${DIST_DIR}/dj4ghub-macos"

echo "macOS binaries written to ${DIST_DIR}"
