#!/usr/bin/env bats
# Tests for poll-and-dispatch.sh with fake curl and git on PATH.

setup() {
  T="$BATS_TEST_TMPDIR"; mkdir -p "$T/bin" "$T/state"
  export STATE_DIR="$T/state" LOG="$T/calls.log" FORGEJO_URL=http://forgejo.test FORGEJO_TOKEN=x
  export GH_SHA=aaaa111 FG_SHA=aaaa111 TASKS='{"workflow_runs":[]}'
  : > "$LOG"
  cat > "$T/bin/git" <<'F'
#!/bin/bash
[ "${GH_FAIL:-0}" = 1 ] && exit 128
printf '%s\trefs/heads/master\n' "$GH_SHA"
F
  cat > "$T/bin/curl" <<'F'
#!/bin/bash
echo "curl $*" >> "$LOG"
case "$*" in
  *actions/tasks*) echo "$TASKS" ;;
  *branches/master*) printf '{"commit":{"id":"%s"}}' "$FG_SHA" ;;
esac
F
  chmod +x "$T/bin/git" "$T/bin/curl"
  export PATH="$T/bin:$PATH"
  S="$BATS_TEST_DIRNAME/poll-and-dispatch.sh"
}

dispatches() { grep -c 'workflows/ci.yml/dispatches' "$LOG" || true; }

@test "new sha dispatches ci.yml on master" {
  run "$S"; [ "$status" -eq 0 ]
  [ "$(dispatches)" = 1 ]
  grep -q '"ref":"master"' "$LOG"
}

@test "same sha as last tested does not dispatch" {
  echo aaaa111 > "$STATE_DIR/last-tested.mattx_exporter"
  run "$S"; [ "$status" -eq 0 ]
  [ "$(dispatches)" = 0 ]
}

@test "finished run is recorded, then nothing is dispatched again" {
  "$S"
  run "$S"; [ "$status" -eq 0 ]
  [ "$(dispatches)" = 1 ]
  [ "$(cat "$STATE_DIR/last-tested.mattx_exporter")" = aaaa111 ]
}

@test "active run skips and records nothing" {
  export TASKS='{"workflow_runs":[{"status":"running"}]}'
  run "$S"; [ "$status" -eq 0 ]
  [ "$(dispatches)" = 0 ]
  [ ! -e "$STATE_DIR/last-tested.mattx_exporter" ]
}

@test "ls-remote failure exits non-zero and dispatches nothing" {
  export GH_FAIL=1
  run "$S"; [ "$status" -ne 0 ]
  [ "$(dispatches)" = 0 ]
}

@test "DRY_RUN dispatches nothing" {
  DRY_RUN=1 run "$S"; [ "$status" -eq 0 ]
  [[ "$output" == *"would dispatch"* ]]
  [ "$(dispatches)" = 0 ]
}

@test "lagging mirror triggers a mirror sync and no dispatch" {
  export GH_SHA=bbbb222
  run "$S"; [ "$status" -eq 0 ]
  [ "$(dispatches)" = 0 ]
  grep -q 'POST.*mirror-sync' "$LOG"
}
