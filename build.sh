#!/bin/bash
# Build script for Linux/macOS

set -e

APP_NAME="term-rest-client"
CMD_DIR="cmd/term-rest-client"
BIN_DIR="bin"
DIST_DIR="dist"

build_local() {
    echo "Building $APP_NAME..."
    mkdir -p "$BIN_DIR"
    
    go build -o "$BIN_DIR/$APP_NAME" "$CMD_DIR/main.go"
    
    if [ $? -eq 0 ]; then
        echo "Build complete: $BIN_DIR/$APP_NAME"
    else
        echo "Build failed!"
        exit 1
    fi
}

build_cross() {
    local os=$1
    local arch=$2
    local ext=${3:-""}
    
    echo "Building for $os/$arch..."
    mkdir -p "$DIST_DIR"
    
    GOOS=$os GOARCH=$arch go build -o "$DIST_DIR/$APP_NAME-$os-$arch$ext" "$CMD_DIR/main.go"
    
    if [ $? -eq 0 ]; then
        echo "  ✓ $DIST_DIR/$APP_NAME-$os-$arch$ext"
    else
        echo "  ✗ Failed to build for $os/$arch"
    fi
}

build_all() {
    echo "Building for all platforms..."
    
    build_cross "linux" "amd64"
    build_cross "windows" "amd64" ".exe"
    build_cross "darwin" "amd64"
    build_cross "darwin" "arm64"
    
    echo ""
    echo "Build complete for all platforms!"
}

case "${1:-local}" in
    all)
        build_all
        ;;
    local|*)
        build_local
        ;;
esac


