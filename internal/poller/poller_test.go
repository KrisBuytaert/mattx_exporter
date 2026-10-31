package poller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTracker() *migration.Tracker {
	return migration.New(migration.Options{}, func(pid int) (migration.ProcInfo, bool) {
		return migration.ProcInfo{Comm: "migtest", Start: uint64(pid)}, true
	})
}

func TestPollFeedsTrackerAndNumbersSnapshots(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "guests", "")
	write(t, dir, "remote", "")
	tr := newTracker()
	p := New(dir, time.Second, tr)
	now := time.Unix(1000, 0)

	p.Poll(now) // baseline
	s1 := p.Latest()
	if !s1.Available || s1.Seq == 0 {
		t.Fatalf("Latest() = %+v, want available with a sequence number", s1)
	}
	write(t, dir, "remote", "100:2\n")
	p.Poll(now.Add(time.Second))
	s2 := p.Latest()
	if s2.Seq <= s1.Seq || len(s2.Remote) != 1 {
		t.Errorf("second snapshot = %+v (first seq %d)", s2, s1.Seq)
	}
	if got := tr.Totals()[migration.TotalKey{Direction: migration.Out, Comm: "migtest"}]; got != 1 {
		t.Errorf("out count = %d, want 1", got)
	}
}

func TestPollMissingDirIsUnavailableNotFatal(t *testing.T) {
	tr := newTracker()
	p := New(filepath.Join(t.TempDir(), "nope"), time.Second, tr)
	p.Poll(time.Unix(1, 0))
	if p.Latest().Available || tr.Available() {
		t.Error("available with a missing proc dir")
	}
}

func TestRunPollsUntilCancelled(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "guests", "")
	write(t, dir, "remote", "")
	p := New(dir, 5*time.Millisecond, newTracker())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	deadline := time.After(2 * time.Second)
	for p.Latest().Seq < 3 {
		select {
		case <-deadline:
			t.Fatalf("Seq stuck at %d", p.Latest().Seq)
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
