# Fleet monitoring for astral-spy

A two-side setup:

- **Render hosts** run `astral-spy --exporter` on `:9835`, plus optionally `node_exporter` on `:9100` for CPU temp/power/RAM/disk.
- **One central box** runs VictoriaMetrics (storage + scraper) and Grafana (graphs), wired together via `docker-compose.yml`.

The fleet-grid dashboard described in the README is then built in the Grafana UI using the PromQL snippets below.

---

## 0. One-time Docker setup on the central box

If `docker compose` complains with `permission denied while trying to connect to the Docker daemon socket`, your user isn't in the `docker` group yet:

```sh
sudo usermod -aG docker $USER
newgrp docker          # picks up the new group in this shell without re-login
```

After that, every future `docker` command works without `sudo`.

## 1. Central box — bring up the stack

First, create your scrape config from the template (the real `scrape.yml` is git-ignored so your fleet's IP layout never gets pushed to GitHub):

```sh
cd deploy
cp example-scrape.yml scrape.yml
$EDITOR scrape.yml          # replace example IPs/hostnames with your own
```

Then bring up the stack:

```sh
GF_ADMIN_PASSWORD=something-secret docker compose up -d
```

That's it — no need to pre-create data directories. Storage lives in Docker-managed named volumes (`vm-data`, `grafana-data`), so the containers' built-in users (Grafana uid 472, VM uid 1000) own their data correctly out of the box.

> **Don't commit `deploy/scrape.yml`.** It contains your fleet's IPs. The included `.gitignore` already excludes it; if you're forking and copying files manually, make sure that rule comes along too.

- Grafana: <http://localhost:3000> (admin / `$GF_ADMIN_PASSWORD`)
- VictoriaMetrics UI: <http://localhost:8428/vmui>

The VictoriaMetrics datasource is auto-provisioned. Once at least one render host is scraping, you'll see series under `astral_*` in Grafana's Explore tab.

## 2. Each render host — install the exporter

Build (or download — once releases exist) `astral-spy` and drop the binary at `/usr/local/bin/astral-spy`. From a host that already has the dashboard installed:

```sh
sudo cp ~/.local/share/astral-spy/bin/astral-spy /usr/local/bin/astral-spy
sudo cp ~/.local/share/astral-spy/deploy/astral-spy-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now astral-spy-exporter
```

Verify:

```sh
curl -s localhost:9835/metrics | head
```

For CPU/RAM/network/temp, also install `node_exporter` (Debian/Ubuntu: `apt install prometheus-node-exporter`). It listens on `:9100` by default — no further config needed.

## 3. Tell VictoriaMetrics about the new host

Edit `deploy/scrape.yml`, add the host under both `astral-spy` and `node` jobs, then reload VM:

```sh
docker compose kill -s HUP victoriametrics
```

(SIGHUP picks up scrape config changes without dropping data.)

## 4. NAT / residential boxes

VictoriaMetrics pulls — it has to reach each host on the listed port. Two clean answers:

- **Tailscale** (recommended): install on every box, reference hosts in `scrape.yml` by their `100.x.x.x` tailnet address. Zero firewall changes.
- **Push instead**: drop the scraper and have each host POST to VM's import endpoint on a timer. Lighter on infra but means writing a small pusher script. Worth doing if even Tailscale is off the table.

---

## 5. PromQL for the fleet-grid panels

Build these in Grafana → New Dashboard → Add panel. Datasource is `VictoriaMetrics` (the auto-provisioned one).

### Top stats row

| Panel | Type | Query |
|---|---|---|
| Σ GPU power across fleet | Stat | `sum(astral_gpu_power_watts)` |
| Σ CPU power across fleet | Stat | `sum(node_rapl_package_joules_total{} - node_rapl_package_joules_total{} offset 1m) / 60` *(or use `irate(node_rapl_package_joules_total[1m])`)* |
| Hosts up | Stat | `count(up{job="astral-spy"} == 1) / count(up{job="astral-spy"})` (format as ratio + show as `X/Y`) |

### Per-host table (with sparklines)

Panel type: **Table** (or **Bar gauge** for a more visual row).

```promql
astral_gpu_temp_celsius                              # column: Temp
astral_gpu_power_watts                               # column: Power
astral_pin_balance_ratio                             # column: Pin balance
```

Use Grafana's "Transform → Outer join by `instance`" to put one row per host. Add cell colour thresholds: temp >85 red, balance <0.85 yellow, <0.70 red.

### Pin-imbalance heatmap (the killer panel)

Panel type: **Heatmap**, Y-axis = `instance`, value = `1 - astral_pin_balance_ratio` (so "more red = more imbalanced").

```promql
1 - astral_pin_balance_ratio
```

A degrading 12V-2x6 connector drifts upward on its row over days/weeks — exactly the signal you want before a melt.

### Per-pin watts (drill-in panel)

Panel type: **Time series**, legend `Pin {{pin}}`.

```promql
astral_pin_watts{instance="$host"}
```

Add a Grafana variable `host = label_values(astral_gpu_up, instance)` so you can flip between machines from a dropdown.

### Alerts panel

Grafana → Alerts. Two starter rules:

```promql
# Pin imbalance sustained — likely connector degradation
(1 - astral_pin_balance_ratio) > 0.30
  and on(instance,gpu_uuid) astral_pin_total_watts > 60   # ignore idle
```

```promql
# GPU thermal pressure
astral_gpu_temp_celsius > 85
```

Both should be `for: 5m` to suppress transient spikes.

---

## 6. Backups / inspecting the data

Named volumes live under `/var/lib/docker/volumes/<project>_<name>/_data` on the host. The project prefix is the directory name compose ran from, so usually `deploy_vm-data` and `deploy_grafana-data`.

Snapshot VictoriaMetrics into a tarball you can copy off-box:

```sh
docker run --rm \
  -v deploy_vm-data:/data:ro \
  -v "$PWD":/backup \
  alpine tar czf /backup/vm-$(date +%F).tar.gz -C /data .
```

Same idea for Grafana:

```sh
docker run --rm \
  -v deploy_grafana-data:/data:ro \
  -v "$PWD":/backup \
  alpine tar czf /backup/grafana-$(date +%F).tar.gz -C /data .
```

Restore by reversing the `tar` (extract into the volume mount with the stack stopped). For ongoing protection, point your usual host backup at `/var/lib/docker/volumes/` — or schedule the above with cron.

## 7. Series cardinality / cost note

With 10 hosts × ~16 unique metric names × (6 pins for pin metrics, 1 otherwise), you're looking at roughly **~250 active series per host**, so ~2.5k total. VictoriaMetrics handles that in tens of MB of memory; a year of 15s-interval data is well under 10 GB on disk. You can crank the scrape interval down to 5s without trouble if you want sharper graphs.
