# CI trigger for the pull mirror

`vox/mattx_exporter` on Forgejo is a pull mirror of GitHub `KrisBuytaert/mattx_exporter`.
Mirror syncs are not pushes, so `.forgejo/workflows/ci.yml` (which also has
`workflow_dispatch`) is started by `ci/poll-and-dispatch.sh` from a 15 minute timer.

The poller compares the Forgejo repo's own master tip (what the job clones and tests)
with the last tested sha, skips while a run is queued or running, and dispatches
`ci.yml` on `master`. If GitHub is ahead of the mirror it requests a mirror sync and
dispatches on the next tick, so CI does not wait for the 8h sync interval.

Tests: `bats ci/test-poll.bats` (fake curl and git, no network).

## What puppetry (puppetry-88) must provide on the runner host

- The script deployed, for example `/var/lib/forgejo-runner-mattx/mattx_exporter/ci/poll-and-dispatch.sh`
  (clone or package), and a oneshot `forgejo-runner-mattx-exporter-poller.service`:
  `ExecStart=/usr/bin/bash <path>/ci/poll-and-dispatch.sh`, run as the runner user,
  with `Environment=HOME=...` or `STATE_DIR=` pointing at a writable directory
  (own files `last-tested.mattx_exporter` and `pending.mattx_exporter`).
- A `.timer` with `OnBootSec=5min`, `OnUnitActiveSec=15min`, `Persistent=true`.
- A root-only `EnvironmentFile` with `FORGEJO_URL`, `FORGEJO_TOKEN` and `FORGEJO_CA`
  (CA bundle of the self-signed instance). Token scope: read and write on repository
  `vox/mattx_exporter` (Actions dispatch and mirror-sync) plus read on its Actions tasks.
- `bash`, `curl`, `git`, `python3` on the host.
