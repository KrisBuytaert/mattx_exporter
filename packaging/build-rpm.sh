#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Builds the static binary and packages it with fpm. Output: $OUT_DIR/*.rpm
# Version: nearest git tag (v prefix stripped), 0.0.0 without tags; release 0.<commits>.g<sha>.<dist>
# (or 1.<dist> exactly on a tag). DIST defaults to fc44 like the other vox-extras packages;
# the binary is static, the package has no library dependencies.
set -euo pipefail
source "$(dirname "$0")/common.sh"
DIST="${DIST:-fc44}"
if [ "$n" = 0 ] && [ -n "$tag" ]; then rel="1.$DIST"; else rel="0.$n.g$sha.$DIST"; fi
stage_tree usr/lib/systemd/system; trap 'rm -rf "$stage"' EXIT

mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/mattx_exporter-"$ver"-"$rel".x86_64.rpm
fpm -s dir -t rpm --name mattx_exporter --version "$ver" --iteration "$rel" --architecture x86_64 \
  --license GPL-2.0-only --url https://github.com/KrisBuytaert/mattx_exporter \
  --description "Prometheus exporter for the MattX SSI cluster kernel module" \
  --depends systemd --config-files /etc/mattx_exporter.yml \
  --after-install packaging/postinstall.sh --before-remove packaging/preuninstall.sh \
  --package "$OUT_DIR/" --chdir "$stage" usr etc >/dev/null
rpm=$(ls "$OUT_DIR"/mattx_exporter-"$ver"-"$rel".x86_64.rpm)
echo "$rpm"
