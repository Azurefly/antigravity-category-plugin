#!/bin/sh
# Builds the plugin shared library in CI for the runner's OS into dist/.
# Usage: sh scripts/ci-build.sh <goarch>
set -eu

PLUGIN_ID="antigravity-category-plugin"
GOARCH=$1

case "${GITHUB_REF:-}" in
  refs/tags/v*) VERSION=${GITHUB_REF#refs/tags/v} ;;
  *) VERSION=0.1.0-dev.$(printf %.7s "${GITHUB_SHA:-local}") ;;
esac

GOOS=$(go env GOHOSTOS)
case $GOOS in
  linux)
    EXT=so
    if [ "$GOARCH" = "arm64" ] && [ "$(go env GOHOSTARCH)" = "amd64" ]; then
      export CC=aarch64-linux-gnu-gcc
    fi
    ;;
  darwin)
    EXT=dylib
    case $GOARCH in
      amd64) export CC="clang -arch x86_64" ;;
      *)     export CC="clang -arch arm64" ;;
    esac
    ;;
  windows)
    EXT=dll
    ;;
  *)
    echo "unsupported OS $GOOS" >&2
    exit 1
    ;;
esac

mkdir -p dist
export CGO_ENABLED=1 GOARCH
go build -buildvcs=false -trimpath -buildmode=c-shared \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o "dist/${PLUGIN_ID}.${EXT}" .
rm -f dist/*.h
