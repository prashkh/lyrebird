#!/bin/sh
# Lyrebird installer — downloads a prebuilt binary and drops it on $PATH.
#
# Run:
#   curl -fsSL https://raw.githubusercontent.com/prashkh/lyrebird/main/install.sh | sh
#
# What this script does:
#   1. Detects your OS and CPU.
#   2. Downloads the matching `lyre` binary from the latest GitHub release.
#   3. Drops it in the first writable directory on your $PATH (preferring
#      ~/.local/bin, then /opt/homebrew/bin, then /usr/local/bin).
#   4. Optionally registers Lyrebird's Claude Code hook in
#      ~/.claude/settings.json (so chat threads are captured automatically).
#
# Honors LYRE_INSTALL_DIR if you want to force a destination.

set -eu

REPO="prashkh/lyrebird"
RELEASE_URL_BASE="https://github.com/${REPO}/releases/latest/download"

# Pretty output (only if stdout is a tty).
if [ -t 1 ]; then
    YELLOW=$(printf '\033[33m')
    GREEN=$(printf '\033[32m')
    RED=$(printf '\033[31m')
    DIM=$(printf '\033[2m')
    BOLD=$(printf '\033[1m')
    RESET=$(printf '\033[0m')
else
    YELLOW=""; GREEN=""; RED=""; DIM=""; BOLD=""; RESET=""
fi

info()  { printf "%s\n" "${DIM}$*${RESET}"; }
ok()    { printf "%s\n" "${GREEN}\xe2\x9c\x93${RESET} $*"; }
warn()  { printf "%s\n" "${YELLOW}!${RESET} $*"; }
fail()  { printf "%s\n" "${RED}\xe2\x9c\x97${RESET} $*" >&2; exit 1; }

# 1. Detect platform.
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64)  arch=amd64 ;;
    *) fail "Unsupported CPU architecture: $arch" ;;
esac

case "$os" in
    darwin) ;;
    linux)  fail "Linux support is coming soon. For now, run install-from-source.sh inside a clone of the repo (requires Go)." ;;
    *)      fail "Unsupported OS: $os. Lyrebird currently runs on macOS only." ;;
esac

ASSET="lyre-${os}-${arch}"
DOWNLOAD_URL="${RELEASE_URL_BASE}/${ASSET}"
info "Detected: ${BOLD}${os}/${arch}${RESET}"

# 2. Pick install dir.
pick_dir() {
    if [ -n "${LYRE_INSTALL_DIR:-}" ]; then
        printf '%s' "$LYRE_INSTALL_DIR"
        return
    fi
    case ":$PATH:" in
        *":$HOME/.local/bin:"*)
            mkdir -p "$HOME/.local/bin"
            if [ -w "$HOME/.local/bin" ]; then
                printf '%s' "$HOME/.local/bin"; return
            fi
            ;;
    esac
    if [ -d /opt/homebrew/bin ] && [ -w /opt/homebrew/bin ]; then
        case ":$PATH:" in *":/opt/homebrew/bin:"*) printf '%s' /opt/homebrew/bin; return ;; esac
    fi
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
        case ":$PATH:" in *":/usr/local/bin:"*) printf '%s' /usr/local/bin; return ;; esac
    fi
    mkdir -p "$HOME/.local/bin"
    printf '%s' "$HOME/.local/bin"
}

DEST_DIR=$(pick_dir)
DEST="$DEST_DIR/lyre"
info "Installing to ${BOLD}$DEST${RESET}"

# 3. Download via curl (or wget as fallback).
TMP=$(mktemp -t lyre.XXXXXX)
trap 'rm -f "$TMP"' EXIT INT TERM
if command -v curl >/dev/null 2>&1; then
    info "Downloading $DOWNLOAD_URL"
    curl -fsSL --retry 2 -o "$TMP" "$DOWNLOAD_URL" || fail "Download failed."
elif command -v wget >/dev/null 2>&1; then
    info "Downloading $DOWNLOAD_URL"
    wget -q -O "$TMP" "$DOWNLOAD_URL" || fail "Download failed."
else
    fail "Need either curl or wget on \$PATH to download the binary."
fi

# 4. Install.
chmod +x "$TMP"
mv "$TMP" "$DEST"
trap - EXIT INT TERM
ok "Installed lyre"

# macOS sometimes quarantines downloaded binaries. Strip the attribute so
# the binary doesn't trigger Gatekeeper on first run.
if [ "$os" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "$DEST" 2>/dev/null || true
fi

# 5. Verify it's on PATH.
case ":$PATH:" in
    *":$DEST_DIR:"*) ok "$DEST_DIR is on \$PATH" ;;
    *)
        warn "$DEST_DIR is NOT on your \$PATH."
        warn "Add this line to your ~/.zshrc (or ~/.bashrc):"
        printf '    export PATH="%s:$PATH"\n' "$DEST_DIR"
        ;;
esac

# 6. Smoke test.
INSTALLED_VERSION=$("$DEST" version 2>/dev/null || echo "(could not run lyre — see above)")
ok "$INSTALLED_VERSION"

# 7. Offer to install the Claude Code hook (only if interactive).
if [ -t 0 ] && [ -d "$HOME/.claude" ]; then
    printf "\nInstall Claude Code PostToolUse hook now? (captures chat threads alongside file changes) [Y/n] "
    read -r ans
    case "${ans:-y}" in
        n|N|no|NO) info "Skipped. Run \`lyre install-hook\` later if you want it." ;;
        *) "$DEST" install-hook ;;
    esac
fi

cat <<EOF

${GREEN}${BOLD}Lyrebird is installed.${RESET}

Next steps:
  cd ~/your-project
  lyre init                  # start tracking this folder
  lyre watch &               # auto-snapshot on every change
  lyre ui                    # open the timeline at http://localhost:6789

Documentation: https://github.com/${REPO}
EOF
