#!/bin/sh
# Creates the service account. Does NOT enable or start the service (puppet does).
set -e
getent group mattx_exporter >/dev/null || groupadd --system mattx_exporter
getent passwd mattx_exporter >/dev/null || useradd --system --gid mattx_exporter --no-create-home --shell /sbin/nologin --comment 'mattx_exporter service account' mattx_exporter
systemctl daemon-reload >/dev/null 2>&1 || true
