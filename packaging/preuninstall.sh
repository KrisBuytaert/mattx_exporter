#!/bin/sh
# On final removal (not upgrade) stop and disable the service.
if [ "$1" = 0 ]; then
    systemctl disable --now mattx_exporter.service >/dev/null 2>&1 || true
fi
exit 0
