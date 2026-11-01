#!/bin/bash
# SPDX-License-Identifier: GPL-2.0-only
# poll-and-dispatch.sh -- run from a systemd timer (15 min) on the Forgejo runner host.
#
# vox/mattx_exporter is a pull mirror of GitHub KrisBuytaert/mattx_exporter. Mirror syncs
# do not fire push workflows, so this poller dispatches .forgejo/workflows/ci.yml
# (workflow_dispatch, ref master) when there is a new commit.
#
# Which tip is compared: the Forgejo repo's own master, not GitHub's. The job clones
# from the Forgejo repo, so that is the code a run actually tests; dispatching on the
# GitHub tip would test stale code until the next mirror sync (default every 8h).
# To stay timely, when GitHub is ahead of the mirror the poller asks Forgejo to sync
# now (mirror-sync API) and dispatches on the next tick.
#
# State (STATE_DIR, default $HOME): last-tested.mattx_exporter holds the last sha whose
# run has finished; pending.mattx_exporter holds the sha of a dispatched run and is
# promoted to last-tested on a later tick, when no run is queued or running any more.
#
# Env: FORGEJO_URL, FORGEJO_TOKEN (read + dispatch Actions, write on the repo for
# mirror-sync), FORGEJO_CA (optional CA bundle), DRY_RUN=1 (print, change nothing).
set -euo pipefail
STATE_DIR="${STATE_DIR:-$HOME}"
REPO="${REPO:-vox/mattx_exporter}"
GITHUB_URL="${GITHUB_URL:-https://github.com/KrisBuytaert/mattx_exporter}"
LAST="$STATE_DIR/last-tested.mattx_exporter"
PENDING="$STATE_DIR/pending.mattx_exporter"
api() { curl -fsS ${FORGEJO_CA:+--cacert "$FORGEJO_CA"} -H "Authorization: token $FORGEJO_TOKEN" "$@"; }
base="$FORGEJO_URL/api/v1/repos/$REPO"

active=$(api "$base/actions/tasks?limit=20" | python3 -c '
import json,sys
print(sum(1 for t in json.load(sys.stdin).get("workflow_runs",[]) if t.get("status") in ("running","waiting","queued","blocked")))')
[ "$active" = 0 ] || { echo "run active ($active), skipping"; exit 0; }

# no run active: a pending dispatch has finished
if [ -s "$PENDING" ] && [ "${DRY_RUN:-0}" != 1 ]; then mv "$PENDING" "$LAST"; fi

gh=$(git ls-remote "$GITHUB_URL" refs/heads/master | cut -f1)
[ -n "$gh" ] || { echo "github ls-remote failed" >&2; exit 1; }
fg=$(api "$base/branches/master" | python3 -c 'import json,sys; print(json.load(sys.stdin)["commit"]["id"])')
[ -n "$fg" ] || { echo "forgejo master tip unreadable" >&2; exit 1; }

if [ "$gh" != "$fg" ]; then
    if [ "${DRY_RUN:-0}" = 1 ]; then echo "would sync mirror (github ${gh:0:7}, forgejo ${fg:0:7})"; exit 0; fi
    api -X POST "$base/mirror-sync" >/dev/null
    echo "mirror behind (github ${gh:0:7}, forgejo ${fg:0:7}), sync requested"
    exit 0
fi

[ "$(cat "$LAST" 2>/dev/null)" != "$fg" ] || exit 0
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would dispatch ci.yml master ${fg:0:7}"; exit 0; fi
api -X POST -H 'Content-Type: application/json' -d '{"ref":"master"}' \
    "$base/actions/workflows/ci.yml/dispatches" >/dev/null
echo "$fg" > "$PENDING"
echo "dispatched ci.yml master ${fg:0:7}"
