#!/usr/bin/env bash
# Build release archives for every supported platform into dist/.
#
#   scripts/release.sh            version from `git describe`
#   scripts/release.sh v0.2.0     explicit version
#
# Produces dist/term-rest-client-<os>-<arch>.tar.gz (.zip for Windows), each
# holding the binary, README.md and LICENSE, plus dist/SHA256SUMS. The names
# carry no version, so ".../releases/latest/download/<name>" always works.

set -euo pipefail
cd "$(dirname "$0")/.."

APP=term-rest-client
VERSION="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
BUILD_TIME="$(date -u '+%Y-%m-%d_%H:%M:%S')"
LDFLAGS="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"
TARGETS="darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64"

rm -rf dist
mkdir -p dist
for target in $TARGETS; do
    os="${target%/*}"
    arch="${target#*/}"
    name="${APP}-${os}-${arch}"
    stage="dist/${name}"
    ext=""
    [ "$os" = windows ] && ext=".exe"
    mkdir -p "$stage"
    echo "==> ${os}/${arch}"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$LDFLAGS" \
        -o "${stage}/${APP}${ext}" ./cmd/term-rest-client
    cp README.md LICENSE "$stage/"
    if [ "$os" = windows ]; then
        (cd dist && zip -qr "${name}.zip" "$name")
    else
        tar -C dist -czf "dist/${name}.tar.gz" "$name"
    fi
    rm -rf "$stage"
done

(cd dist && if command -v sha256sum >/dev/null; then sha256sum ./*.tar.gz ./*.zip; else shasum -a 256 ./*.tar.gz ./*.zip; fi | sed 's# \./# #' > SHA256SUMS)
echo "==> ${VERSION}"
ls -lh dist
