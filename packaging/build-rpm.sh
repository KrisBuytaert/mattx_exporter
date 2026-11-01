#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Builds the static binary and packages it with fpm. Output: $OUT_DIR/*.rpm
# Version: nearest git tag (v prefix stripped), 0.0.0 without tags; release 0.<commits>.g<sha>.<dist>
# (or 1.<dist> exactly on a tag). DIST defaults to fc44 like the other vox-extras packages;
# the binary is static, the package has no library dependencies.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT_DIR="${OUT_DIR:-$PWD/dist}"
DIST="${DIST:-fc44}"
sha=$(git rev-parse --short HEAD)
tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
if [ -n "$tag" ]; then
  ver=${tag#v}
  n=$(git rev-list --count "$tag"..HEAD)
else
  ver=0.0.0; n=$(git rev-list --count HEAD)
fi
if [ "$n" = 0 ] && [ -n "$tag" ]; then rel="1.$DIST"; else rel="0.$n.g$sha.$DIST"; fi
describe=$(git describe --tags --always --dirty)

stage=$(mktemp -d); trap 'rm -rf "$stage"' EXIT
install -d "$stage/usr/bin" "$stage/etc" "$stage/usr/lib/systemd/system"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$describe -X main.commit=$sha" \
  -o "$stage/usr/bin/mattx_exporter" ./cmd/mattx_exporter
install -m 0644 packaging/mattx_exporter.yml "$stage/etc/mattx_exporter.yml"
install -m 0644 packaging/mattx_exporter.service "$stage/usr/lib/systemd/system/mattx_exporter.service"

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
