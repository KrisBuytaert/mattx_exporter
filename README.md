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

## Releases

A release is created by `.github/workflows/release.yml` when a tag `vX.Y.Z` is pushed to
GitHub. It runs `make release-assets` (the same recipe as locally): a
`mattx_exporter-<ver>-linux-amd64.tar.gz` (static binary, LICENSE, README), the RPM, the
DEB and `SHA256SUMS`, and attaches them with generated release notes. Verify with
`sha256sum -c SHA256SUMS`. Running the workflow by hand (workflow_dispatch) only builds and
uploads workflow artifacts; it never creates a release. Releases are not signed yet.
The Forgejo mirror and internal CI are unaffected (Forgejo reads `.forgejo/workflows` only).

## License

GPL v2 only, the same license as MattX; see `LICENSE`. Go sources carry
`SPDX-License-Identifier: GPL-2.0-only`.

## Dashboards and alerts

Generic example dashboards and alert rules (use `instance`, no site-specific labels):

- `dashboards/grafana/mattx-exporter.json`: Grafana import (Dashboards, New, Import). Variables:
  `datasource` (any Prometheus data source), `job` (text box, regex, default `.*mattx.*`),
  `instance` (multi-select from `mattx_build_info`).
- `dashboards/perses/mattx-exporter.json`: Perses dashboard resource
  (`percli apply -f dashboards/perses/mattx-exporter.json`, project `default`; change
  `metadata.project` if needed). Uses the project's or global's default Prometheus datasource.
  Variables: `job` (default `.*mattx.*`), `instance`.
- `alerts/mattx.rules.yml`: example Prometheus alert rules (`promtool check rules` clean).

### Example alert rules

These are examples. The exporter itself sends nothing anywhere; the rules only fire inside
your Prometheus, and whether anyone is notified depends entirely on your own Alertmanager
routing. You can leave the file out. Thresholds and `for:` durations are examples to tune.

| Alert | Detects | Severity | `for:` | Fires in normal use? | What to do | Does NOT mean |
|---|---|---|---|---|---|---|
| MattxExporterDown | Prometheus cannot scrape the exporter | warning | 5m (a reboot or upgrade takes 1-2 min) | not for reboots or deploys | check the node and `systemctl status mattx_exporter` | the cluster is broken, only that the node is not observed |
| MattxNodeDown | a node that was in `/proc/mattx/nodes` is gone | warning | 5m (nodes reboot during deploys) | not for a reboot that completes in 5m | check that node and its mattx service | a node never seen since the exporter started |
| MattxExecmemErrors | `execmem: unable to allocate` in this boot's kernel log (mattx.ko failed to load) | warning | 1m | after a failed module load until the next reboot | reboot the node, load the module once | the module is failing right now |
| MattxKdumpDirsIncreased | a new directory in the kdump dir within 1h (the kernel crashed) | critical | none (it is an increase over 1h) | only on a real crash | keep the dump and kernel log | dumps that were there before the exporter started; clears after 1h |
| MattxExporterScrapeErrors | the exporter's `mattx_exporter_scrape_errors_total` keeps growing (a collector fails) | warning | 10m (one bad read is not worth a notification) | not in normal use | read `journalctl -u mattx_exporter`, check the module is loaded | the cluster is unhealthy; other collectors stay valid |

The "Migrations per pid" panel needs `per_pid_metrics: true` in `/etc/mattx_exporter.yml`
(off by default); it is empty otherwise. The dashboards are generated by
`python3 dashboards/gen.py` from one panel table; `go test ./internal/dashboards` checks
that every query only uses metrics the exporter can emit. The packages install these
files under `/usr/share/mattx_exporter/` and do not provision them. Not tested against a
real Grafana or Perses.

## Packaging

`make rpm` builds `dist/mattx_exporter-<ver>-<rel>.x86_64.rpm` and `make deb` builds
`dist/mattx-exporter_<ver>_amd64.deb` (Debian names cannot contain underscores) with fpm,
each checked by `packaging/test-rpm.sh` / `packaging/test-deb.sh`. Both ship the static
binary `/usr/bin/mattx_exporter`, the systemd unit (runs as user `mattx_exporter`, not
enabled by the package), `/etc/mattx_exporter.yml` (preserved on upgrade: `%config(noreplace)`
/ dpkg conffile) and the examples under `/usr/share/mattx_exporter/`. Deb version: `<tag>-1`,
or `<tag or 0.0.0>+<commits>.g<sha>-1`. Removing the deb keeps the user; purge removes the
config only.

Packages are published internally by copying them into a directory-based repository (on
the lab: the RPM repo `/opt/local-repo/vox-extras`, then `createrepo_c --update` on it);
nothing is published publicly.
