#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-2.0-only
# Checks the built RPM: file list, config flag, requirements, scriptlets, binary runs.
set -euo pipefail
RPM="$(realpath "${1:?usage: test-rpm.sh path.rpm}")"
fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "PASS: $*"; }

want=$'/etc/mattx_exporter.yml\n/usr/bin/mattx_exporter\n/usr/lib/systemd/system/mattx_exporter.service'
got=$(rpm -qlp "$RPM" | grep -v '^/usr/lib/\.build-id' | sort)
[ "$got" = "$want" ] || fail "file list differs:"$'\n'"$got"
pass "file list"

rpm -qp --configfiles "$RPM" | grep -qx /etc/mattx_exporter.yml || fail "config not flagged as config"
rpm -qp --queryformat '[%{FILEFLAGS:fflags} %{FILENAMES}\n]' "$RPM" | grep -q '^c.*n.* /etc/mattx_exporter.yml' || fail "config is not noreplace"
pass "config(noreplace)"

req=$(rpm -qp --requires "$RPM" | grep -v '^rpmlib(' | sort)
echo "$req" | grep -qx 'systemd' || fail "missing systemd requirement: $req"
echo "$req" | grep -v -E '^(systemd|/bin/sh|/usr/bin/sh|shadow-utils|/usr/sbin/useradd|/usr/sbin/groupadd)$' | grep -q . && fail "unexpected requirements: $req"
pass "requires: $(echo $req | tr '\n' ' ')"

scripts=$(rpm -qp --scripts "$RPM")
echo "$scripts" | grep -q 'useradd' || fail "postinstall does not create the user"
echo "$scripts" | grep -q 'systemctl enable' && fail "package must not enable the service"
pass "scriptlets create user, do not enable"

rpm -qp --info "$RPM" | grep -q '^License *: GPL-2.0-only' || fail "license tag"
pass "license tag"

d=$(mktemp -d); trap 'rm -rf "$d"' EXIT
(cd "$d" && rpm2cpio "$RPM" | cpio -idm --quiet)
"$d/usr/bin/mattx_exporter" -version | grep -q '^mattx_exporter ' || fail "binary -version"
file "$d/usr/bin/mattx_exporter" | grep -q 'statically linked' || fail "binary is not static"
grep -q '^User=mattx_exporter' "$d/usr/lib/systemd/system/mattx_exporter.service" || fail "unit user"
grep -q '^WantedBy' "$d/usr/lib/systemd/system/mattx_exporter.service" || fail "unit has no [Install]"
pass "binary runs and is static; unit runs as mattx_exporter"
