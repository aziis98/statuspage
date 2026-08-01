# statuspage

A status page that monitors the machines in your lab (or home). It probes
machines over ICMP and TCP, runs a script over SSH to collect checks and
numeric metrics, and keeps recent history in SQLite.

The backend is a single Go binary, the frontend is a Preact + Vite page, and
everything is configured with one YAML file.

![statuspage](assets/screenshots/status-collage.png)

## Features

- ICMP + TCP probes per machine, combined into a single status (`up`,
  `degraded`, `down`, `unknown`).
- Optional SSH check that runs a script and parses:
  - check lines like `name:status:info` (e.g. `vulkan:on:AMD Radeon`);
  - metric lines like `name:metric:value` (e.g. `cpu_pct:metric:42.5`) that get
    accumulated into a SQLite time series;
  - free-form text after a `---` separator (shown raw in the modal).
- Per-machine history (status ticks + incidents) and per-metric line plots with
  hover tooltips.
- Interactive refresh button (opt-in) to re-probe a machine on demand.
- A neo-brutalist web UI with no runtime JS framework bloat (Preact + Vite).

## Layout

```
statuspage
├── server/          the Go backend
│   ├── main.go      entrypoint, flags, routes
│   ├── config.go    yaml config loading + defaults
│   ├── monitor.go   probes, ssh runs, status/history/metrics handlers
│   ├── mock.go      deterministic mock data for --mock / screenshots
│   └── history.go   sqlite history + metrics storage and downsampling
├── go.mod           module definition (statuspage)
├── src/             the frontend (Preact + TS + CSS)
├── example.config.yaml      documented, fully commented-out example config
├── config.local.yaml      local config (gitignored, contains ssh keys)
└── data.volume/    created at runtime, holds history.db
```

## Requirements

- Go 1.26+ (see the `go` directive in `go.mod`; the backend uses `http.ServeMux`
  method patterns, added in 1.22).
- A C compiler for `mattn/go-sqlite3` (the `CGO_ENABLED=1` build).
- `bun` (or `npm`) for the frontend build.

## Building

```sh
bun install
bun run build
CGO_ENABLED=1 go build -o bin/statuspage ./server
```

Run it:

```sh
./bin/statuspage -c config.local.yaml
```

Then open `http://localhost:5000`.

### Docker

Clone the repo and set up your config inside it:

```sh
git clone https://git.phc.dm.unipi.it/aziis98/statuspage
cd statuspage
cp example.config.yaml config.local.yaml   # your config goes here
docker compose up -d --build
```

The container mounts `./data.volume` over `/app/data.volume` (the default
database location) and `./config.local.yaml` read-only at
`/app/config.local.yaml`. `NET_RAW` is added so ICMP probes work; without it
the app falls back to TCP-only.

## Development

Three flags change how the app runs while developing — they control what the
backend serves and whether it talks to real machines.

### `--dev`

Builds the frontend (`bun run build`) before serving `dist/`, so one command
gets you a running page without a separate build step:

```sh
./bin/statuspage -c config.local.yaml --dev
```

### `--dev-server`

Skips serving `dist/` entirely and lets the Vite dev server handle the
frontend, with hot reloading as you edit `src/`:

```sh
./bin/statuspage -c config.local.yaml --dev-server   # terminal 1
bun dev                                            # terminal 2, http://localhost:5173
```

`vite.config.ts` proxies `/api` to `http://localhost:5000`, so the backend must
be running too.

### `--mock`

Serves fully generated data (six `server-N` machines with noisy metric series)
instead of probing real machines — no config, SSH keys, or network access
needed. It's primarily used by `scripts/generate-screenshot` to produce the
README image, but is handy for previewing the UI quickly:

```sh
./bin/statuspage --mock --dev
```

### Screenshot

`scripts/generate-screenshot` builds the frontend and server, starts the app in
mock mode, captures the top of the page with headless Chromium in both light and
dark, and frames each with a transparent background, border, and shadow. It
needs `chromium` and `python3-PIL` on PATH:

```sh
./scripts/generate-screenshot
```

This writes three PNGs to `assets/screenshots/`:

| file                        | contents                                          |
| --------------------------- | ------------------------------------------------- |
| `status.png`                | light capture                                     |
| `status-dark.png`           | dark capture                                      |
| `status-collage.png`        | dark behind, offset top-left, light on top        |

Runs are incremental: a capture is skipped when no frontend/server source is
newer than the existing PNG, and the collage is rebuilt only when the mtime of a
light/dark variant is newer than the collage. `status-collage.png` is the image
at the top of this README.

### Flags

| flag                | default        | description                                        |
| ------------------- | -------------- | -------------------------------------------------- |
| `-c, --config`      | `config.local.yaml`  | path to the yaml config file               |
| `--addr`            | `:5000`        | listen address                                     |
| `--db`              | `data.volume/history.db`  | sqlite database file                  |
| `--dev`             | `false`        | run `bun run build` before serving `dist/`         |
| `--dev-server`      | `false`        | don't serve `dist/` (Vite dev server handles it)   |
| `--mock`            | `false`        | serve generated mock data (no real probes)         |
| `--check`           | `false`        | load + expand the config, print a machine summary, then exit |

## Configuration

Everything lives in a single yaml file. A fully documented, commented-out
example with every option is in `example.config.yaml` — copy it to
`config.local.yaml` and fill it in. The top-level layout:

```yaml
title: "My machines"
interactive: true
shared_metric_window: false

ping: { ... }
ssh:  { ... }

metrics:
  cpu_pct: { min: 0, max: 100 }

groups:
  - name: "Lab"
    machines:
      - host: a1.example.net
```

### `title`

```yaml
title: "Laboratori Dipartimento di Matematica"
```

Shown in the header and the browser tab. Defaults to `Status Page`.

### `interactive`

```yaml
interactive: true
```

Whether clients can trigger an on-demand probe via the refresh button on each
machine card. When `false`, the endpoint refuses refresh requests.

### `shared_metric_window`

```yaml
shared_metric_window: false
```

Controls how metric plots share their x (time) axis:

- `false` (default): each metric plot scales to its own recorded time span, so
  a brand new metric isn't drowned out by an older one.
- `true`: all plots are aligned to the same window (the last ~24h) and are
  downsampled together, so you can compare shapes directly.

### `ping`

```yaml
ping:
  interval: 10s
  timeout: 2s
  tcp_port: 22
```

| key        | default | description                               |
| ---------- | ------- | ----------------------------------------- |
| `interval` | `10s`   | how often to probe                        |
| `timeout`  | `2s`    | per-probe timeout                         |
| `tcp_port` | `22`    | port to TCP-connect for the tcp check     |

Only IPv4 addresses are probed (IPv6 is ignored).

### `ssh`

```yaml
ssh:
  interval: 5m
  user: labuser
  port: 22
  key: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
  script: |
    echo "tmp:on:$(df -h /tmp 2>/dev/null | awk 'NR==2 {print $2}')"
    echo "cpu_pct:metric:$(top -bn1 2>/dev/null | awk '/%Cpu/ {printf "%.1f", 100-$8}')"
    echo "---"
    echo "kernel: $(uname -sr)"
```

| key        | default   | description                              |
| ---------- | --------- | ---------------------------------------- |
| `interval` | `10m`     | how often to run the script over ssh     |
| `user`     | `root`    | ssh user                                 |
| `port`     | `22`      | ssh port                                 |
| `key`      | —         | PEM private key (the only supported auth)|
| `script`   | —         | shell script to run; see script format   |

If `key` or `script` is empty, the ssh check is skipped for that machine.

#### Script format

One line per check, `name:status:info`:

```
vulkan:on:AMD Radeon RX 570
nvidia:off:no-nvidia-gpus
```

`status` can be `on`, `off`, `down`, or anything else (shown as `unknown`).

A line with `name:metric:value` is treated as a numeric metric, accumulated in
SQLite and drawn as a line plot:

```
cpu_pct:metric:42.5
ram_pct:metric:61.3
disk_free:metric:198.2GB
```

The value is parsed as a leading number with an optional unit suffix.

Everything after the first line containing `---` is treated as free-form output
and shown raw in the machine modal:

```
---
hostname: a1
kernel: Linux 6.8.0
uptime: up 3 days
```

#### `metrics`

Suggested `min`/`max` ranges for metric plots, keyed by metric name:

```yaml
metrics:
  cpu_pct:
    min: 0
    max: 100
  ram_pct:
    min: 0
    max: 100
  disk_free:
    min: 0
```

These are soft bounds: the plot scales to fit the data if a value goes outside
the range, and the current value is highlighted (with a wavy underline) when
out of range.

#### Copy-paste: Linux examples

Small, ready-to-paste snippets for a Linux box. Wrap commands that may not
exist on every machine (like `sensors` or `nvidia-smi`) in `2>/dev/null`; a
line that produces no output is skipped.

Status chips — `name:status:info`:

```yaml
ssh:
  script: |
    echo "hostname:on:$(hostname)"
    echo "kernel:on:$(uname -sr)"
    echo "vulkan:on:$(vulkaninfo --summary 2>/dev/null | sed -n '/deviceName/p' | head -1 | cut -d: -f2-)"
    echo "docker:$(systemctl is-active docker 2>/dev/null | sed 's/active/on/; s/inactive/off/; s/failed/down/'):docker daemon"
```

CPU & memory metrics:

```yaml
ssh:
  script: |
    echo "cpu_pct:metric:$(top -bn1 2>/dev/null | awk '/%Cpu/ {printf "%.1f", 100-$8}')"
    echo "load1:metric:$(cut -d' ' -f1 /proc/loadavg)"
    echo "ram_pct:metric:$(free -m 2>/dev/null | awk '/^Mem:/ {printf "%.1f", $3/$2*100}')"
    echo "ram_used:metric:$(free -m 2>/dev/null | awk '/^Mem:/ {print $3}')MB"
```

Disk usage:

```yaml
ssh:
  script: |
    echo "disk_root:metric:$(df -h / 2>/dev/null | awk 'NR==2 {print $5}' | tr -d '%')"
    echo "disk_root_free:metric:$(df -h / 2>/dev/null | awk 'NR==2 {print $4}' | tr -d 'G')"
```

Temperatures & GPU:

```yaml
ssh:
  script: |
    echo "cpu_temp:metric:$(sensors 2>/dev/null | awk '/Package id 0:/ {print $4; exit}' | tr -d '+°C')"
    echo "gpu_temp:metric:$(nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader 2>/dev/null | tr -d ' ')"
    echo "gpu_util:metric:$(nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader 2>/dev/null | tr -d ' %')"
```

Free-form output (everything after `---` is shown raw in the machine modal):

```yaml
ssh:
  script: |
    echo "---"
    echo "uptime: $(uptime -p)"
    echo "load: $(cat /proc/loadavg)"
    echo "mem: $(free -h | awk '/^Mem:/ {print $3 "/" $2}')"
    echo "top: $(ps -eo %cpu,comm --sort=-%cpu | head -4 | tr '\n' ';')"
```

Suggested bounds for the metrics above:

```yaml
metrics:
  cpu_pct:    { min: 0, max: 100 }
  ram_pct:    { min: 0, max: 100 }
  disk_root:  { min: 0, max: 100 }
  gpu_util:   { min: 0, max: 100 }
  gpu_temp:   { min: 0, max: 105 }
```

A per-machine `script:` overrides the global one, e.g. for the box that
actually has the NVIDIA GPU:

```yaml
groups:
  - name: "Lab"
    machines:
      - host: a1.example.net
      - host: gpu.example.net
        ssh:
          script: |
            echo "gpu_temp:metric:$(nvidia-smi --query-gpu=temperature.gpu --format=csv,noheader 2>/dev/null | tr -d ' ')"
```

### `groups`

```yaml
groups:
  - name: "Aula 3"
    machines:
      - host: a3-dott1.example.net
      - name: "front door pi"
        host: 192.168.1.10
```

A `machines` entry only needs `host`; `name` overrides the display name.
Machines listed at the top level (outside any group) go into a `Machines`
section instead.

`host` accepts brace patterns that are expanded at load time:

- `{1,2,3}` — enumeration
- `{2..5,8..10}` — inclusive numeric ranges
- `{a3,a4}` — labels (any non-range item is kept literally)
- several groups multiply in cartesian fashion; leading zeros are preserved

```yaml
groups:
  - name: "Server room"
    machines:
      - host: server-{a3,a4}-{1..10}.example.org
```

expands to `server-a3-1.example.org`, `server-a3-2.example.org`, ...,
`server-a4-10.example.org`. A `name:` applies to every expanded machine;
without one, each machine is named after its host. Ranges must go from a
smaller to a larger number, and their endpoints must be integers.

Malformed patterns make startup fail with an error instead of expanding
silently: unmatched or nested `{...}` groups, ranges with non-integer
endpoints, or ranges with extra dots (e.g. `{1..5..10}`). A single pattern may
expand to at most 10 000 machines; anything larger is rejected too.

Per-machine overrides for `ping` and `ssh` are supported, and inherit
unset values from the top level:

```yaml
groups:
  - name: "Lab"
    machines:
      - host: special.example.net
        ping:
          interval: 30s
        ssh:
          interval: 1m
```

## HTTP API

| method | path                     | description                                  |
| ------ | ------------------------ | -------------------------------------------- |
| `GET`  | `/api/status`            | current status, stats, metric ranges         |
| `GET`  | `/api/history/{machine}` | status tick history (`?limit=`, max 500)     |
| `GET`  | `/api/metrics/{machine}` | metric time series (see below)               |
| `POST` | `/api/refresh/{machine}` | schedule an on-demand probe (interactive)    |

`/api/metrics/{machine}` query params:

| param        | description                                      |
| ------------ | ------------------------------------------------ |
| `name`       | restrict to one metric name                      |
| `min`, `max` | unix seconds window to fetch                     |
| `max_points` | max returned points (downsampled, default 100)   |
| `shared`     | align downsampling to the `min`/`max` window     |

The frontend asks for the last ~24h at `max_points=100` per metric.

## Storage

A single SQLite file holds everything:

- `history` — status ticks per machine.
- `metrics` — accumulated metric samples per machine/name.

It's capped at 200k history rows (5000 per machine) and 5000 samples per
metric name, pruned every 10 minutes. The footer shows the database size and a
rough "size in a month" estimate.

## Notes

- The SSH host key check is disabled, since lab machines change frequently. Be
  aware of the implications if you expose this outside a trusted network.
- SSH key authentication only; password auth is not supported.
- ICMP requires a raw socket or `CAP_NET_RAW`. When unavailable, the app falls
  back to TCP-only probes.
- The frontend polls `/api/status` every 3 seconds.
- The database grows while the server runs; a prune loop keeps it bounded, but
  it is not a production-grade time-series store.

## License

GNU Affero General Public License v3.0 (AGPL-3.0). See [LICENSE](LICENSE).
