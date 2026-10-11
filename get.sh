#!/bin/sh
# Download a prebuilt term-rest-client and install it on your PATH as `rest`.
# No Go needed.
#
#   curl -fsSL https://raw.githubusercontent.com/AbyAbyss/cli-rest-client/main/get.sh | sh
#
# Environment:
#   VERSION=v0.6.0       install that release instead of the latest
#   INSTALL_DIR=~/bin    install there (default: /usr/local/bin, /opt/homebrew/bin
#                        or ~/.local/bin, whichever is writable first)
#
# Works on macOS (Apple Silicon and Intel) and Linux (x86_64 and arm64). The
# download is checked against the release's SHA256SUMS before installing.
# The old command name, term-rest-client, is kept as a link to rest so
# existing scripts keep working.

set -eu

REPO="AbyAbyss/cli-rest-client"
APP="rest"
OLD="term-rest-client" # the name releases before v0.6.0 used

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
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

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
else
    fail "curl or wget is needed"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

info "Downloading term-rest-client for $os/$arch ${VERSION:-(latest release)}"
fetch "$base/SHA256SUMS" "$tmp/SHA256SUMS" || fail "download failed: $base/SHA256SUMS"
# Releases from v0.6.0 ship rest-<os>-<arch>; older ones term-rest-client-<os>-<arch>.
name=""
for prefix in "$APP" "$OLD"; do
    if grep -q " $prefix-$os-$arch.tar.gz\$" "$tmp/SHA256SUMS"; then
        name="$prefix-$os-$arch"
        inner="$prefix"
        break
    fi
done
[ -n "$name" ] || fail "no build for $os/$arch in this release"
fetch "$base/$name.tar.gz" "$tmp/$name.tar.gz" || fail "download failed: $base/$name.tar.gz"

expected="$(grep " $name.tar.gz\$" "$tmp/SHA256SUMS" | cut -d' ' -f1)"
if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$tmp/$name.tar.gz" | cut -d' ' -f1)"
else
    actual="$(shasum -a 256 "$tmp/$name.tar.gz" | cut -d' ' -f1)"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $name.tar.gz"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
bin="$tmp/$name/$inner"
[ -f "$bin" ] || fail "archive has no $inner binary"

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
sudo=""
if ! { mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; }; then
    command -v sudo >/dev/null 2>&1 || fail "$dir is not writable; choose another with INSTALL_DIR=..."
    sudo="sudo"
    $sudo mkdir -p "$dir"
fi
$sudo install -m 0755 "$bin" "$dir/$APP"
# term-rest-client keeps working: a link to rest (replacing an older copy).
$sudo rm -f "$dir/$OLD"
$sudo ln -s "$APP" "$dir/$OLD"

ok "Installed $("$dir/$APP" -version) as $dir/$APP"
case ":$PATH:" in
    *":$dir:"*)
        found="$(command -v "$APP" 2>/dev/null || true)"
        if [ -n "$found" ] && [ "$found" != "$dir/$APP" ]; then
            echo
            warn "another '$APP' comes first on your PATH: $found"
            echo "  Run this one as $dir/$APP, or as $OLD."
        else
            echo
            echo "  Run it from any terminal:  $APP        (or $APP -gui for the browser UI)"
        fi
        ;;
    *)
        echo
        echo "  $dir is not on your PATH yet. Add this line to your shell's startup file"
        echo "  (~/.zshrc on macOS, ~/.bashrc on most Linux systems), then open a new terminal:"
        echo
        echo "    export PATH=\"$dir:\$PATH\""
        ;;
esac
