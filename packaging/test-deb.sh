#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Checks the built .deb: control fields, file list and modes, conffiles, maintainer
# scripts (sh -n, dash -n), no enable/start, binary runs and is static.
set -euo pipefail
DEB="$(realpath "${1:?usage: test-deb.sh path.deb}")"
fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

field() { dpkg-deb -f "$DEB" "$1"; }
[ "$(field Package)" = mattx_exporter ] || fail "Package: $(field Package)"
[[ "$(field Version)" =~ ^[0-9][0-9A-Za-z.+~]*-[0-9A-Za-z.+~]+$ ]] || fail "Version not dpkg-valid: $(field Version)"
[ "$(field Architecture)" = amd64 ] || fail "Architecture: $(field Architecture)"
[ "$(field Depends)" = systemd ] || fail "Depends: '$(field Depends)' (want exactly systemd)"
[ "$(field License)" = GPL-2.0-only ] || fail "License: '$(field License)'"
pass "control: $(field Package) $(field Version) $(field Architecture) Depends: $(field Depends)"

files=$(dpkg-deb -c "$DEB" | awk '$1 !~ /^d/ {print $1, $6}' | sed 's# \./# /#' | sort -k2)
want=$'-rw-r--r-- /etc/mattx_exporter.yml\n-rwxr-xr-x /usr/bin/mattx_exporter\n-rw-r--r-- /usr/lib/systemd/system/mattx_exporter.service'
[ "$files" = "$(echo "$want" | sort -k2)" ] || fail "files/modes differ:"$'\n'"$files"
pass "exactly three files with the right modes"

conf=$(dpkg-deb -I "$DEB" conffiles)
[ "$conf" = /etc/mattx_exporter.yml ] || fail "conffiles: $conf"
pass "conffiles: $conf"

d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
dpkg-deb -e "$DEB" "$d/ctl"; dpkg-deb -x "$DEB" "$d/root"
for s in postinst prerm postrm; do
  [ -x "$d/ctl/$s" ] || fail "missing maintainer script $s"
  sh -n "$d/ctl/$s" || fail "sh -n $s"
  if command -v dash >/dev/null; then dash -n "$d/ctl/$s" || fail "dash -n $s"; fi
done
pass "maintainer scripts parse (sh$(command -v dash >/dev/null && echo ', dash'))"

grep -E 'systemctl[^#]*(enable|start|restart)' "$d/ctl/postinst" && fail "postinst enables or starts the service"
grep -q -E 'adduser|useradd' "$d/ctl/postinst" || fail "postinst does not create the user"
pass "postinst creates the user, does not enable or start"

"$d/root/usr/bin/mattx_exporter" -version | grep -q '^mattx_exporter ' || fail "binary -version"
file "$d/root/usr/bin/mattx_exporter" | grep -q 'statically linked' || fail "binary is not static"
grep -q '^User=mattx_exporter' "$d/root/usr/lib/systemd/system/mattx_exporter.service" || fail "unit user"
grep -q '^SupplementaryGroups=systemd-journal' "$d/root/usr/lib/systemd/system/mattx_exporter.service" || fail "unit journal group"
pass "binary runs and is static; unit has user and journal group"
