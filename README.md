# mattx_exporter

Prometheus exporter for the [MattX](https://github.com/brainmatt/mattx) SSI cluster
kernel module. It replaces the `mattx-node-metrics.sh` and `mattx-migration-metrics.sh`
node_exporter textfile scripts with one small static Go binary (nodes have little RAM)
that exposes `/metrics` on `:9105`.

Status: early development. Phase 1 reads `/proc/mattx/{nodes,guests,remote}` by polling.
Kernel counters (`/proc/mattx/stats`) and netlink events are planned as later sources.

## Metrics

- Node table (`/proc/mattx/nodes`): `mattx_node_load`, `mattx_node_mem_free_bytes`,
  `mattx_node_affinity`, `mattx_node_up{node,ip}`.
- Migration state (`/proc/mattx/{guests,remote}`): `mattx_guests_hosted`,
  `mattx_remote_processes`, `mattx_guest_process`, `mattx_remote_process`,
  `mattx_migrations_total{direction,comm}` (legacy names, kept for compatibility with the
  textfile scripts until the cut-over), and per-pid metrics such as
  `mattx_pid_migrations_total{pid,comm}`.
- Host: module loads and execmem errors since boot, kdump directories, service state,
  `mattx_build_info`, `mattx_exporter_scrape_errors_total`.

## Configuration

Everything is configured in a YAML file (default `/etc/mattx_exporter.yml`); the
program contains no policy such as process-name lists. Defaults are sane without a file.
Per-pid metrics (`per_pid_metrics`) are off by default; the legacy
`mattx_migrations_total` metric is always exported.

## Development

    go vet ./...
    go test ./...
    REQUIRE_PROMTOOL=1 go test ./...   # also lints the output with promtool

Tests use fixture `/proc/mattx` trees under `testdata/`; nothing needs the kernel module.

## License

GPL v2 only, the same license as MattX; see `LICENSE`. Go sources carry
`SPDX-License-Identifier: GPL-2.0-only`.

## Packaging

`make rpm` builds `dist/mattx_exporter-<ver>-<rel>.x86_64.rpm` (static binary, systemd
unit running as user `mattx_exporter`, `%config(noreplace) /etc/mattx_exporter.yml`) with
fpm and checks it with `packaging/test-rpm.sh`. The unit is not enabled by the package.
