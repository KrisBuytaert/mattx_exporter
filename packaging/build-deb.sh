#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Debian package names cannot contain underscores: the package is mattx-exporter (binary, unit, user stay mattx_exporter).
# Builds the static binary and packages it as a .deb with fpm. Output: $OUT_DIR/*.deb
# Version: <tag>-1 on a tag, otherwise <tag or 0.0.0>+<commits>.g<sha>-1; the commit
# count makes later builds sort higher (dpkg compares digit runs numerically).
# Unit goes to /usr/lib/systemd/system: supported by systemd on all Debian/Ubuntu releases
# and what Debian trixie+ policy expects (avoids usrmerge /lib aliasing problems).
set -euo pipefail
source "$(dirname "$0")/common.sh"
ARCH="${ARCH:-amd64}"
if [ "$n" = 0 ] && [ -n "$tag" ]; then debver="$ver-1"; else debver="$ver+$n.g$sha-1"; fi
stage_tree usr/lib/systemd/system; trap 'rm -rf "$stage"' EXIT

mkdir -p "$OUT_DIR"
out="$OUT_DIR/mattx-exporter_${debver}_${ARCH}.deb"
rm -f "$out"
fpm -s dir -t deb --name mattx-exporter --version "${debver%-1}" --iteration 1 --architecture "$ARCH" \
  --license GPL-2.0-only --url https://github.com/KrisBuytaert/mattx_exporter \
  --maintainer "Kris Buytaert <KrisBuytaert@users.noreply.github.com>" \
  --description "Prometheus exporter for the MattX SSI cluster kernel module" \
  --depends systemd --config-files /etc/mattx_exporter.yml --deb-priority optional --category admin \
  --after-install packaging/deb/postinst --before-remove packaging/deb/prerm \
  --after-remove packaging/deb/postrm \
  --package "$out" --chdir "$stage" usr etc >/dev/null
echo "$out"
