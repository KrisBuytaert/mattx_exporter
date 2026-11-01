// SPDX-License-Identifier: GPL-2.0-only

// Package poller reads /proc/mattx on an interval and feeds the migration tracker.
package poller

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procfs"
)

// Poller implements collector.SnapshotSource.
type Poller struct {
	dir      string
	interval time.Duration
	tr       *migration.Tracker

	mu     sync.Mutex
	latest procfs.Snapshot
}

// New returns a Poller reading dir every interval into tr.
func New(dir string, interval time.Duration, tr *migration.Tracker) *Poller {
	return &Poller{dir: dir, interval: interval, tr: tr}
}

// Poll reads the proc directory once, as of now. A read error leaves the snapshot
// unavailable and is logged.
func (p *Poller) Poll(now time.Time) {
	s, err := procfs.Read(p.dir)
	if err != nil {
		slog.Warn("reading proc dir failed", "dir", p.dir, "err", err)
		s = procfs.Snapshot{}
	}
	p.mu.Lock()
	s.Seq = p.latest.Seq + 1
	p.latest = s
	p.mu.Unlock()
	p.tr.Observe(now, s)
}

// Latest returns the most recent snapshot, with Seq counting the polls.
func (p *Poller) Latest() procfs.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latest
}

// Run polls until ctx is cancelled; the first poll happens immediately.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.Poll(time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.Poll(now)
		}
	}
}
