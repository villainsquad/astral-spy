# astral-spy

Allows users to get a live power pin breakdown for ASTRAL 5090 GPUs

Credit: https://github.com/jan-provaznik/sus

## Requirements

**Hardware**

- ASUS ROG Astral RTX 5090 (PCI subsystem IDs `0x89e31043` / `0x8a3c1043`). Only Astral cards expose the per-pin shunt sensor over SMBus; non-Astral 5090s are not supported.

**Runtime**

- Linux. The dashboard reads `/dev/i2c-*` and walks `/sys/bus/pci/devices/`, so Windows/macOS are not supported.
- NVIDIA proprietary driver (provides `libnvidia-ml.so` and `nvidia-smi`).
- `i2c-dev` kernel module — `start.sh` will `modprobe` it for you.
- Root / `sudo` — required for `/dev/i2c-*` access. `start.sh` re-execs itself under `sudo` automatically.

- Go ≥ **1.25.6** (set by `go.mod`'s `go` directive; older toolchains refuse to build).
- A C compiler (`gcc` or `clang`) — the NVML bindings use CGO.
- `git` — `install.sh` clones the repo into `$HOME/.local/share/astral-spy`.


## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/villainsquad/astral-spy/main/install.sh | sh
```

## Usage

```bash
./start.sh
```