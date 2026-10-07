#!/usr/bin/env bash
# Build term-rest-client and install it on your PATH (macOS and Linux).
#
#   ./install.sh              build and install
#   ./install.sh uninstall    remove the installed binary
#
# The install directory is chosen automatically:
#   1. $INSTALL_DIR, if set
#   2. /usr/local/bin, if writable
#   3. /opt/homebrew/bin, if writable (Apple Silicon Homebrew)
#   4. ~/.local/bin (created if needed)
# If you pick a directory you can't write to (e.g. INSTALL_DIR=/usr/local/bin),
# only the final copy is run with sudo and you'll be asked for your password.

set -euo pipefail

APP_NAME="term-rest-client"
CMD_DIR="./cmd/term-rest-client"

cd "$(dirname "$0")"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
ok()    { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
fail()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

pick_dir() {
    if [ -n "${INSTALL_DIR:-}" ]; then
        echo "$INSTALL_DIR"
    elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
        echo /usr/local/bin
    elif [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ]; then
        echo /opt/homebrew/bin
    else
        echo "$HOME/.local/bin"
    fi
}

on_path() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
    esac
    return 1
}

path_hint() {
    local dir="$1" rc
    case "${SHELL:-}" in
        */zsh)  rc="$HOME/.zshrc" ;;
        */bash) rc="$HOME/.bash_profile" ;;
        *)      rc="your shell's startup file" ;;
    esac
    echo
    echo "  $dir is not on your PATH yet. Add it with:"
    echo
    echo "    echo 'export PATH=\"$dir:\$PATH\"' >> $rc"
    echo "    source $rc"
    echo
}

install_app() {
    command -v go >/dev/null 2>&1 || fail "Go is not installed. On macOS run: brew install go"

    local version build_time dir
    version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
    build_time="$(date -u '+%Y-%m-%d_%H:%M:%S')"
    dir="$(pick_dir)"

    info "Building $APP_NAME $version"
    mkdir -p bin
    go build -trimpath -ldflags "-s -w -X main.Version=$version -X main.BuildTime=$build_time" \
        -o "bin/$APP_NAME" "$CMD_DIR"

    info "Installing to $dir"
    if mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
        install -m 0755 "bin/$APP_NAME" "$dir/$APP_NAME"
    else
        command -v sudo >/dev/null 2>&1 || fail "$dir is not writable; choose another with INSTALL_DIR=..."
        info "$dir needs admin rights, using sudo for the copy"
        sudo mkdir -p "$dir"
        sudo install -m 0755 "bin/$APP_NAME" "$dir/$APP_NAME"
    fi

    ok "Installed $dir/$APP_NAME"
    if on_path "$dir"; then
        echo
        echo "  Run it from any terminal:"
        echo
        echo "    $APP_NAME              # open the REST client"
        echo "    $APP_NAME list         # list saved requests"
        echo "    $APP_NAME -version"
        echo
    else
        path_hint "$dir"
    fi
}

uninstall_app() {
    local removed=0 dir gopath_bin=""
    if command -v go >/dev/null 2>&1; then
        gopath_bin="$(go env GOPATH)/bin"
    fi
    for dir in "${INSTALL_DIR:-}" /usr/local/bin /opt/homebrew/bin "$HOME/.local/bin" "$gopath_bin"; do
        [ -n "$dir" ] || continue
        if [ -f "$dir/$APP_NAME" ]; then
            if [ -w "$dir" ]; then
                rm -f "$dir/$APP_NAME"
            else
                sudo rm -f "$dir/$APP_NAME" || fail "cannot remove $dir/$APP_NAME"
            fi
            ok "Removed $dir/$APP_NAME"
            removed=1
        fi
    done
    [ "$removed" -eq 1 ] || info "$APP_NAME is not installed in any known location"
    echo "  Your collections are kept in the workspace file (see Settings in the app)."
}

case "${1:-install}" in
    install)   install_app ;;
    uninstall) uninstall_app ;;
    -h|--help|help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//' ;;
    *) fail "unknown command '$1' (use install or uninstall)" ;;
esac
