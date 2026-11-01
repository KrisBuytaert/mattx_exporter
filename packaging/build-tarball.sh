#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Static linux/amd64 binary + LICENSE + README as $OUT_DIR/mattx_exporter-<ver>-linux-amd64.tar.gz
# (<ver> is the tag without v on a tag, else <tag or 0.0.0>+<commits>.g<sha>, like the deb).
set -euo pipefail
source "$(dirname "$0")/common.sh"
if [ "$n" = 0 ] && [ -n "$tag" ]; then v="$ver"; else v="$ver+$n.g$sha"; fi
name="mattx_exporter-$v-linux-amd64"
w=$(mktemp -d); trap 'rm -rf "$w"' EXIT
mkdir "$w/$name"
build_binary "$w/$name/mattx_exporter"
cp LICENSE README.md "$w/$name/"
mkdir -p "$OUT_DIR"
tar -C "$w" --owner=0 --group=0 --numeric-owner -czf "$OUT_DIR/$name.tar.gz" "$name"
echo "$OUT_DIR/$name.tar.gz"
