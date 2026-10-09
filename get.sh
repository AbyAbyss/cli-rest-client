#!/bin/sh
# Download a prebuilt term-rest-client and install it on your PATH. No Go needed.
#
#   curl -fsSL https://raw.githubusercontent.com/AbyAbyss/cli-rest-client/main/get.sh | sh
#
# Environment:
#   VERSION=v0.2.0       install that release instead of the latest
#   INSTALL_DIR=~/bin    install there (default: /usr/local/bin, /opt/homebrew/bin
#                        or ~/.local/bin, whichever is writable first)
#
# Works on macOS (Apple Silicon and Intel) and Linux (x86_64 and arm64). The
# download is checked against the release's SHA256SUMS before installing.

set -eu

REPO="AbyAbyss/cli-rest-client"
APP="term-rest-client"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
    Darwin) os=darwin ;;
    Linux)  os=linux ;;
    *) fail "unsupported system $(uname -s); on Windows download the .zip from https://github.com/$REPO/releases" ;;
esac
case "$(uname -m)" in
    x86_64|amd64)  arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) fail "unsupported CPU $(uname -m)" ;;
esac
# Apple Silicon running this shell under Rosetta reports x86_64; prefer native.
if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
fi

if [ -n "${VERSION:-}" ]; then
    base="https://github.com/$REPO/releases/download/$VERSION"
else
    base="https://github.com/$REPO/releases/latest/download"
fi
name="$APP-$os-$arch"

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
else
    fail "curl or wget is needed"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

info "Downloading $name.tar.gz ${VERSION:-(latest release)}"
fetch "$base/$name.tar.gz" "$tmp/$name.tar.gz" || fail "download failed: $base/$name.tar.gz"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"

expected="$(grep " $name.tar.gz\$" "$tmp/SHA256SUMS" | cut -d' ' -f1)"
[ -n "$expected" ] || fail "$name.tar.gz is not listed in SHA256SUMS"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)"
else
    actual="$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1)"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $name.tar.gz"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
bin="$tmp/$name/$APP"
[ -f "$bin" ] || fail "archive has no $APP binary"

if [ -n "${INSTALL_DIR:-}" ]; then
    dir="$INSTALL_DIR"
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
elif [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ]; then
    dir=/opt/homebrew/bin
else
    dir="$HOME/.local/bin"
fi

info "Installing to $dir"
if mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
    install -m 0755 "$bin" "$dir/$APP"
else
    command -v sudo >/dev/null 2>&1 || fail "$dir is not writable; choose another with INSTALL_DIR=..."
    sudo mkdir -p "$dir"
    sudo install -m 0755 "$bin" "$dir/$APP"
fi

ok "Installed $("$dir/$APP" -version)"
case ":$PATH:" in
    *":$dir:"*)
        echo
        echo "  Run it from any terminal:  $APP"
        ;;
    *)
        echo
        echo "  $dir is not on your PATH yet. Add this line to your shell's startup file"
        echo "  (~/.zshrc on macOS, ~/.bashrc on most Linux systems), then open a new terminal:"
        echo
        echo "    export PATH=\"$dir:\$PATH\""
        ;;
esac
