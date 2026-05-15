#!/usr/bin/env bash
# Bring up everything astral-spy needs and then launch it.
#
# Responsibilities:
#   1. Ensure the i2c-dev kernel module is loaded (provides /dev/i2c-N
#      character devices that the ASUS Astral SMBus sensor lives on).
#   2. Ensure the NVIDIA driver is loaded (modprobe nvidia if needed).
#   3. Build bin/astral-spy if it does not yet exist.
#   4. Re-exec the binary as root since /dev/i2c-* is root-only by default.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$SCRIPT_DIR/bin/astral-spy"

log() { printf '\033[2m[astral-spy]\033[0m %s\n' "$*"; }
err() { printf '\033[31m[astral-spy]\033[0m %s\n' "$*" >&2; }

ensure_module() {
    local mod="$1"
    if lsmod | awk '{print $1}' | grep -qx "${mod//-/_}"; then
        return 0
    fi
    log "loading kernel module: $mod"
    if [[ $EUID -eq 0 ]]; then
        modprobe "$mod"
    else
        sudo modprobe "$mod"
    fi
}

ensure_i2c() {
    ensure_module i2c-dev
    # On some boards the NVIDIA driver creates its i2c adapters lazily;
    # nudge it so /sys/bus/pci/devices/<gpu>/i2c-* shows up.
    if ! ls /dev/i2c-* >/dev/null 2>&1; then
        err "no /dev/i2c-* devices present after modprobe — check BIOS/SMBus settings"
        exit 1
    fi
}

ensure_nvidia() {
    if ! command -v nvidia-smi >/dev/null 2>&1; then
        err "nvidia-smi not found — install the NVIDIA driver first"
        exit 1
    fi
    if ! nvidia-smi -L >/dev/null 2>&1; then
        log "nvidia-smi failed, attempting modprobe nvidia"
        ensure_module nvidia || true
        nvidia-smi -L >/dev/null
    fi
}

ensure_built() {
    if ! command -v go >/dev/null 2>&1; then
        err "go toolchain not found — install Go to build astral-spy"
        exit 1
    fi
    # Always invoke make — it's a no-op when the binary is up to date,
    # and rebuilds when any source file (e.g. the device-ID list in
    # internal/sus/sus.go) has changed since the last build.
    log "checking build"
    (cd "$SCRIPT_DIR" && make --quiet)
}

ensure_i2c
ensure_nvidia
ensure_built

if [[ $EUID -ne 0 ]]; then
    log "re-executing as root (sudo) — required for /dev/i2c access"
    exec sudo "$BIN" "$@"
fi

exec "$BIN" "$@"
