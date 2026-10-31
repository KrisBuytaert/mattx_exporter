// Package poller reads /proc/mattx on an interval and feeds the migration tracker.
package poller

import (
	"context"
	"time"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procfs"
)

// Poller implements collector.SnapshotSource.
type Poller struct{}

// New returns a Poller reading dir every interval into tr.
func New(dir string, interval time.Duration, tr *migration.Tracker) *Poller { return &Poller{} }

// Poll reads the proc directory once, as of now.
func (p *Poller) Poll(now time.Time) {}

// Latest returns the most recent snapshot, with Seq counting the polls.
func (p *Poller) Latest() procfs.Snapshot { return procfs.Snapshot{} }

// Run polls until ctx is cancelled; the first poll happens immediately.
func (p *Poller) Run(ctx context.Context) {}
