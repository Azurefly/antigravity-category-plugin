#!/bin/sh
# Builds multi-platform or current-platform plugin shared libraries locally or via Docker.
# Usage: sh scripts/build.sh [version]
set -eu

PLUGIN_ID="antigravity-category-plugin"
VERSION=${1:-0.1.0}
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$ROOT/dist"

HOST_OS=$(go env GOHOSTOS)
HOST_ARCH=$(go env GOHOSTARCH)

echo "==> Building for host ($HOST_OS/$HOST_ARCH)..."
case "$HOST_OS" in
  darwin)
    CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 CC="clang -arch arm64" \
      go build -buildvcs=false -trimpath -buildmode=c-shared \
      -ldflags "-s -w -X main.version=$VERSION" \
      -o "$ROOT/dist/${PLUGIN_ID}_darwin_arm64.dylib" .
    CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 CC="clang -arch x86_64" \
      go build -buildvcs=false -trimpath -buildmode=c-shared \
      -ldflags "-s -w -X main.version=$VERSION" \
      -o "$ROOT/dist/${PLUGIN_ID}_darwin_amd64.dylib" .
    ;;
  linux)
    CGO_ENABLED=1 GOOS=linux GOARCH="$HOST_ARCH" \
      go build -buildvcs=false -trimpath -buildmode=c-shared \
      -ldflags "-s -w -X main.version=$VERSION" \
      -o "$ROOT/dist/${PLUGIN_ID}_linux_${HOST_ARCH}.so" .
    ;;
esac
rm -f "$ROOT"/dist/*.h
echo "==> Output in $ROOT/dist"
