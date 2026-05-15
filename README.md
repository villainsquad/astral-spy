# astral-spy

Allows users to get a live power pin breakdown for ASTRAL 5090 GPUs

Credit: https://github.com/jan-provaznik/sus

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/villainsquad/astral-spy/main/install.sh | sh
```

## Usage

Interactive terminal dashboard:

```bash
~/.local/share/astral-spy/start.sh
```

Run as a Prometheus exporter (for fleet monitoring) instead of the TUI:

```bash
~/.local/share/astral-spy/start.sh --exporter
# now exposing /metrics on :9835
```

To keep the exporter running across reboots, install the bundled systemd unit:

```bash
sudo cp ~/.local/share/astral-spy/bin/astral-spy /usr/local/bin/astral-spy
sudo cp ~/.local/share/astral-spy/contrib/systemd/astral-spy-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now astral-spy-exporter
```

Verify: `curl -s localhost:9835/metrics | head`.

## Requirements

**Hardware**

- ASUS ROG Astral RTX 5090, if you have an Astral card and you see `No compatible ASUS ROG Astral RTX 5090 devices found.` run `nvidia-smi --query-gpu=name,pci.sub_device_id --format=csv` and paste the output into a pull request.

**Runtime**

- Linux. The dashboard reads `/dev/i2c-*` and walks `/sys/bus/pci/devices/`, so Windows/macOS are not supported.
- NVIDIA proprietary driver (provides `libnvidia-ml.so` and `nvidia-smi`).
- `i2c-dev` kernel module — `start.sh` will `modprobe` it for you.
- Root / `sudo` — required for `/dev/i2c-*` access. `start.sh` re-execs itself under `sudo` automatically.

- Go ≥ **1.25.6** (set by `go.mod`'s `go` directive; older toolchains refuse to build).
- A C compiler (`gcc` or `clang`) — the NVML bindings use CGO.
- `git` — `install.sh` clones the repo into `$HOME/.local/share/astral-spy`.



