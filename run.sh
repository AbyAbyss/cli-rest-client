#!/bin/bash
# Run script for Linux/macOS

set -e

APP_NAME="term-rest-client"
BIN_DIR="bin"

if [ ! -f "$BIN_DIR/$APP_NAME" ]; then
    echo "Building $APP_NAME first..."
    ./build.sh
    if [ $? -ne 0 ]; then
        echo "Build failed, cannot run."
        exit 1
    fi
fi

echo "Running $APP_NAME..."
./$BIN_DIR/$APP_NAME

