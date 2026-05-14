#!/usr/bin/env bash
# astral-spy installer.
#
# One-liner:
#   curl -fsSL https://raw.githubusercontent.com/villainsquad/astral-spy/main/install.sh | sh
#
# What it does:
#   1. Checks for required tools (git, go, nvidia-smi).
#   2. Clones (or fast-forwards) the astral-spy repo into $ASTRAL_SPY_HOME
#      (default: $HOME/.local/share/astral-spy).
#   3. Hands off to start.sh, which loads kernel modules, builds the
#      binary, and re-execs it under sudo for /dev/i2c access.
#
# Overrides (env vars):
#   ASTRAL_SPY_HOME    install location (default: $HOME/.local/share/astral-spy)
#   ASTRAL_SPY_REPO    git URL (default: https://github.com/villainsquad/astral-spy.git)
#   ASTRAL_SPY_BRANCH  branch to track (default: main)
#   ASTRAL_SPY_NO_RUN  if set, install only — do not launch the dashboard

set -euo pipefail

REPO="${ASTRAL_SPY_REPO:-https://github.com/villainsquad/astral-spy.git}"
BRANCH="${ASTRAL_SPY_BRANCH:-main}"
HOME_DIR="${ASTRAL_SPY_HOME:-$HOME/.local/share/astral-spy}"

log() { printf '\033[2m[astral-spy]\033[0m %s\n' "$*"; }
err() { printf '\033[31m[astral-spy]\033[0m %s\n' "$*" >&2; }

require() {
    if ! command -v "$1" >/dev/null 2>&1; then
        err "missing dependency: $1${2:+ — $2}"
        exit 1
    fi
}

require git
require go "install the Go toolchain (https://go.dev/dl/)"
require nvidia-smi "install the NVIDIA driver"

mkdir -p "$(dirname "$HOME_DIR")"

if [[ -d "$HOME_DIR/.git" ]]; then
    log "updating existing checkout at $HOME_DIR"
    git -C "$HOME_DIR" fetch --depth 1 origin "$BRANCH"
    git -C "$HOME_DIR" checkout -q "$BRANCH"
    git -C "$HOME_DIR" reset --hard "origin/$BRANCH"
elif [[ -e "$HOME_DIR" ]]; then
    err "$HOME_DIR exists but is not a git checkout — refusing to overwrite"
    exit 1
else
    log "cloning $REPO into $HOME_DIR"
    git clone --depth 1 --branch "$BRANCH" "$REPO" "$HOME_DIR"
fi

chmod +x "$HOME_DIR/start.sh" "$HOME_DIR/install.sh" 2>/dev/null || true

if [[ -n "${ASTRAL_SPY_NO_RUN:-}" ]]; then
    log "installed at $HOME_DIR — run it with: $HOME_DIR/start.sh"
    exit 0
fi

log "launching dashboard"
exec "$HOME_DIR/start.sh" "$@"
