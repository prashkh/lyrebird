#!/usr/bin/env bash
# Lyrebird installer.
#
# Usage:
#   ./install.sh                         (when run from inside a cloned repo)
#   curl -fsSL <url>/install.sh | sh     (eventually, once we have releases)
#
# What it does:
#   1. Picks a writable directory on your $PATH (~/.local/bin, /opt/homebrew/bin,
#      or /usr/local/bin — whichever exists, is on PATH, and is writable).
#   2. Builds the `lyre` binary (requires Go 1.21+).
#   3. Copies it into the chosen install dir.
#   4. Optionally registers the Claude Code PostToolUse hook.
#
# Reads no arguments; honors $LYRE_INSTALL_DIR to force a destination.

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

# 1. Sanity: are we in the right place?
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
cd "$SCRIPT_DIR"
[ -f cmd/lyre/main.go ] || fail "install.sh must be run from a Lyrebird source checkout (cmd/lyre/main.go not found here)."

# 2. Need Go.
if ! command -v go >/dev/null 2>&1; then
    if [ -x /opt/homebrew/bin/go ]; then
        export PATH="/opt/homebrew/bin:$PATH"
    elif [ -x /usr/local/go/bin/go ]; then
        export PATH="/usr/local/go/bin:$PATH"
    else
        fail "Go is required (1.21+). Install via:  brew install go   or   https://go.dev/dl/"
    fi
fi
GO_VERSION=$(go version | awk '{print $3}')
info "Using $(go version)"

# 3. Pick install dir.
pick_dir() {
    if [ -n "${LYRE_INSTALL_DIR:-}" ]; then
        echo "$LYRE_INSTALL_DIR"
        return
    fi
    # Prefer ~/.local/bin if it's on PATH and writable (no sudo).
    case ":$PATH:" in
        *":$HOME/.local/bin:"*)
            mkdir -p "$HOME/.local/bin"
            if [ -w "$HOME/.local/bin" ]; then
                echo "$HOME/.local/bin"
                return
            fi
            ;;
    esac
    # Apple Silicon Homebrew prefix.
    if [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ]; then
        case ":$PATH:" in *":/opt/homebrew/bin:"*) echo /opt/homebrew/bin; return ;; esac
    fi
    # Intel/Linux Homebrew or system.
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
        case ":$PATH:" in *":/usr/local/bin:"*) echo /usr/local/bin; return ;; esac
    fi
    # Fallback: ~/.local/bin even if not on PATH; we'll warn the user.
    mkdir -p "$HOME/.local/bin"
    echo "$HOME/.local/bin"
}

DEST_DIR=$(pick_dir)
DEST="$DEST_DIR/lyre"
info "Installing to $DEST"

# 4. Build.
info "Building..."
go build -ldflags "-s -w" -o "$DEST" ./cmd/lyre
ok "Built lyre"

# 5. Verify it's on PATH.
case ":$PATH:" in
    *":$DEST_DIR:"*)
        ok "$DEST_DIR is on \$PATH"
        ;;
    *)
        warn "$DEST_DIR is NOT on your \$PATH."
        warn "Add this line to your ~/.zshrc (or ~/.bashrc):"
        printf '    export PATH="%s:$PATH"\n' "$DEST_DIR"
        ;;
esac

# 6. Smoke test.
INSTALLED_VERSION=$("$DEST" version 2>/dev/null || true)
ok "Installed: $INSTALLED_VERSION"

# 7. Offer to install the Claude Code hook.
if [ -d "$HOME/.claude" ]; then
    if [ -t 0 ]; then
        printf "\nInstall Claude Code PostToolUse hook now? [Y/n] "
        read -r ans
    else
        ans="y"  # non-interactive: opt-in by default
    fi
    case "${ans:-y}" in
        n|N|no|NO) info "Skipped. Run \`lyre install-hook\` later if you want it." ;;
        *) "$DEST" install-hook ;;
    esac
fi

cat <<EOF

${GREEN}Lyrebird is installed.${RESET}

Next steps:
  cd ~/your-project
  lyre init                  # start tracking this folder
  lyre watch &               # auto-snapshot on every change
  lyre ui                    # open the timeline at http://localhost:6789

Documentation: https://github.com/prashkh/lyrebird
EOF
