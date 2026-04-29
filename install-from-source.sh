#!/usr/bin/env bash
# Lyrebird — build-from-source installer.
#
# Use this if you've cloned the repo and want to build the binary yourself
# (for development, or if the prebuilt binary doesn't work for your platform).
# Most users should run the regular install.sh instead, which downloads
# a prebuilt binary.

set -euo pipefail

YELLOW=$'\033[33m'
GREEN=$'\033[32m'
RED=$'\033[31m'
DIM=$'\033[2m'
RESET=$'\033[0m'

info()  { printf "%s\n" "${DIM}$*${RESET}"; }
ok()    { printf "%s\n" "${GREEN}✓${RESET} $*"; }
warn()  { printf "%s\n" "${YELLOW}!${RESET} $*"; }
fail()  { printf "%s\n" "${RED}✗${RESET} $*" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"
[ -f cmd/lyre/main.go ] || fail "Run this from inside a Lyrebird source checkout."

if ! command -v go >/dev/null 2>&1; then
    if [ -x /opt/homebrew/bin/go ]; then
        export PATH="/opt/homebrew/bin:$PATH"
    elif [ -x /usr/local/go/bin/go ]; then
        export PATH="/usr/local/go/bin:$PATH"
    else
        fail "Go is required (1.21+). Install: brew install go"
    fi
fi
info "Using $(go version)"

pick_dir() {
    if [ -n "${LYRE_INSTALL_DIR:-}" ]; then echo "$LYRE_INSTALL_DIR"; return; fi
    case ":$PATH:" in
        *":$HOME/.local/bin:"*)
            mkdir -p "$HOME/.local/bin"
            [ -w "$HOME/.local/bin" ] && { echo "$HOME/.local/bin"; return; }
            ;;
    esac
    [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ] && {
        case ":$PATH:" in *":/opt/homebrew/bin:"*) echo /opt/homebrew/bin; return ;; esac
    }
    [ -d /usr/local/bin ] && [ -w /usr/local/bin ] && {
        case ":$PATH:" in *":/usr/local/bin:"*) echo /usr/local/bin; return ;; esac
    }
    mkdir -p "$HOME/.local/bin"
    echo "$HOME/.local/bin"
}

DEST_DIR=$(pick_dir)
DEST="$DEST_DIR/lyre"
info "Installing to $DEST"
info "Building..."
go build -ldflags '-s -w' -o "$DEST" ./cmd/lyre
ok "Built lyre"

case ":$PATH:" in
    *":$DEST_DIR:"*) ok "$DEST_DIR is on \$PATH" ;;
    *)
        warn "$DEST_DIR is NOT on your \$PATH."
        printf '    export PATH="%s:$PATH"\n' "$DEST_DIR"
        ;;
esac
ok "Installed: $("$DEST" version 2>/dev/null)"

if [ -d "$HOME/.claude" ]; then
    if [ -t 0 ]; then
        printf "\nInstall Claude Code PostToolUse hook? [Y/n] "
        read -r ans
    else
        ans="y"
    fi
    case "${ans:-y}" in
        n|N|no|NO) info "Skipped." ;;
        *) "$DEST" install-hook ;;
    esac
fi

echo
echo "${GREEN}Lyrebird is installed from source.${RESET}"
