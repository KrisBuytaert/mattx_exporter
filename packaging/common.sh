# SPDX-License-Identifier: GPL-2.0-only
# Sourced by build-rpm.sh and build-deb.sh: version derivation and staging.
# Sets: sha tag ver n describe stage (stage is a temp dir, caller sets the trap).
cd "$(dirname "${BASH_SOURCE[0]}")/.."
OUT_DIR="${OUT_DIR:-$PWD/dist}"
sha=$(git rev-parse --short HEAD)
tag=$(git describe --tags --abbrev=0 2>/dev/null || true)
if [ -n "$tag" ]; then ver=${tag#v}; n=$(git rev-list --count "$tag"..HEAD)
else ver=0.0.0; n=$(git rev-list --count HEAD); fi
describe=$(git describe --tags --always --dirty)

# stage_tree UNITDIR: static binary, config and unit under $stage
stage_tree() {
  stage=$(mktemp -d)
  install -d "$stage/usr/bin" "$stage/etc" "$stage/$1"
  CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$describe -X main.commit=$sha" \
    -o "$stage/usr/bin/mattx_exporter" ./cmd/mattx_exporter
  install -m 0644 packaging/mattx_exporter.yml "$stage/etc/mattx_exporter.yml"
  install -m 0644 packaging/mattx_exporter.service "$stage/$1/mattx_exporter.service"
  # example dashboards and alert rules: not auto-provisioned, import by hand
  local share="$stage/usr/share/mattx_exporter"
  install -d "$share/dashboards/grafana" "$share/dashboards/perses" "$share/alerts"
  install -m 0644 dashboards/grafana/mattx-exporter.json "$share/dashboards/grafana/"
  install -m 0644 dashboards/perses/mattx-exporter.json "$share/dashboards/perses/"
  install -m 0644 alerts/mattx.rules.yml "$share/alerts/"
}
