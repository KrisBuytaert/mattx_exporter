// SPDX-License-Identifier: GPL-2.0-only

package migration

import (
	"reflect"
	"regexp"
	"testing"
	"time"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procfs"
)

// fakeProc is a mutable stand-in for /proc/<pid>.
type fakeProc map[int]ProcInfo

func (f fakeProc) lookup(pid int) (ProcInfo, bool) { p, ok := f[pid]; return p, ok }

type harness struct {
	t    *testing.T
	proc fakeProc
	tr   *Tracker
	now  time.Time
}

func newHarness(t *testing.T, o Options) *harness {
	h := &harness{t: t, proc: fakeProc{}, now: time.Unix(1_000_000, 0)}
	h.tr = New(o, h.proc.lookup)
	return h
}

// step observes a snapshot with the given guests/remote and advances the clock 1s.
func (h *harness) step(guests, remote []procfs.Entry) {
	h.t.Helper()
	h.tr.Observe(h.now, procfs.Snapshot{Available: true, Guests: guests, Remote: remote})
	h.now = h.now.Add(time.Second)
}

func e(pid, node int) procfs.Entry { return procfs.Entry{PID: pid, Node: node} }

func (h *harness) wantTotals(want map[TotalKey]uint64) {
	h.t.Helper()
	got := h.tr.Totals()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		h.t.Errorf("totals = %v, want %v", got, want)
	}
}

func (h *harness) pid(pid int) PIDStat {
	h.t.Helper()
	for _, p := range h.tr.PIDs() {
		if p.PID == pid {
			return p
		}
	}
	h.t.Fatalf("pid %d not in PIDs() = %v", pid, h.tr.PIDs())
	return PIDStat{}
}

func TestBaselineIsNotCounted(t *testing.T) {
	h := newHarness(t, Options{})
	h.proc[100] = ProcInfo{"migtest", 1}
	h.step([]procfs.Entry{e(900, 2)}, []procfs.Entry{e(100, 2)})
	h.wantTotals(nil)
	if got := h.tr.Remote(); !reflect.DeepEqual(got, []Placement{{100, "migtest", 2}}) {
		t.Errorf("Remote() = %v", got)
	}
	if got := h.pid(100); got.Migrations != 0 || got.Role != RoleRemote {
		t.Errorf("baseline pid = %+v, want 0 migrations, remote", got)
	}
}

func TestUnavailableBeforeFirstSnapshotDoesNotConsumeBaseline(t *testing.T) {
	h := newHarness(t, Options{})
	h.tr.Observe(h.now, procfs.Snapshot{Available: false})
	if h.tr.Available() {
		t.Error("Available() = true after unavailable snapshot")
	}
	h.step(nil, nil) // first available snapshot is the baseline
	h.proc[100] = ProcInfo{"migtest", 1}
	h.step(nil, []procfs.Entry{e(100, 2)})
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1})
}

func TestOutAndReturn(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1})
	if p := h.pid(100); p.Role != RoleRemote || p.Node != 2 || p.Migrations != 1 {
		t.Errorf("after out: %+v", p)
	}
	// Process is still alive in /proc: it came back.
	h.step(nil, nil)
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1, {Return, "migtest"}: 1})
	if p := h.pid(100); p.Role != RoleHome || p.Node != -1 || p.Migrations != 2 {
		t.Errorf("after return: %+v", p)
	}
	// And goes out again: the same pid has now migrated three times.
	h.step(nil, []procfs.Entry{e(100, 3)})
	if p := h.pid(100); p.Role != RoleRemote || p.Node != 3 || p.Migrations != 3 {
		t.Errorf("after second out: %+v", p)
	}
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 2, {Return, "migtest"}: 1})
}

func TestRemoteExitIsNotAReturn(t *testing.T) {
	h := newHarness(t, Options{PIDRetention: time.Minute})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	delete(h.proc, 100) // the deputy is reaped after the surrogate exited
	h.step(nil, nil)
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1, {Exit, "migtest"}: 1})
	if p := h.pid(100); p.Role != RoleGone {
		t.Errorf("exited pid role = %v, want gone", p.Role)
	}
}

func TestHopBetweenRemoteNodes(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	h.step(nil, []procfs.Entry{e(100, 3)})
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1, {Hop, "migtest"}: 1})
	if p := h.pid(100); p.Node != 3 || p.Migrations != 2 {
		t.Errorf("after hop: %+v", p)
	}
}

func TestGuestsInAndDeparted(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[900] = ProcInfo{"stress", 5}
	h.step([]procfs.Entry{e(900, 2)}, nil)
	h.wantTotals(map[TotalKey]uint64{{In, "stress"}: 1})
	if got := h.tr.Guests(); !reflect.DeepEqual(got, []Placement{{900, "stress", 2}}) {
		t.Errorf("Guests() = %v", got)
	}
	h.step(nil, nil)
	h.wantTotals(map[TotalKey]uint64{{In, "stress"}: 1, {Departed, "stress"}: 1})
	if len(h.tr.Guests()) != 0 {
		t.Errorf("Guests() = %v after departure", h.tr.Guests())
	}
}

func TestSameCommDuplicatesAreTrackedByPid(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 1}
	h.proc[101] = ProcInfo{"migtest", 2}
	h.step(nil, []procfs.Entry{e(100, 2), e(101, 2)})
	h.step(nil, []procfs.Entry{e(101, 2)}) // only 100 returns
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 2, {Return, "migtest"}: 1})
	if got := h.tr.Remote(); !reflect.DeepEqual(got, []Placement{{101, "migtest", 2}}) {
		t.Errorf("Remote() = %v", got)
	}
	if h.pid(100).Role != RoleHome || h.pid(101).Role != RoleRemote {
		t.Errorf("roles: 100=%v 101=%v", h.pid(100).Role, h.pid(101).Role)
	}
}

func TestPIDReuseStartsANewIdentity(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	// The remote process exits; a new, unrelated process takes pid 100 on this node
	// and is migrated, all between two polls: the entry never disappears.
	h.proc[100] = ProcInfo{"stress", 99}
	h.step(nil, []procfs.Entry{e(100, 2)})
	got := h.tr.Totals()
	if got[TotalKey{Exit, "migtest"}] != 1 {
		t.Errorf("old identity not counted as exit: %v", got)
	}
	p := h.pid(100)
	if p.Comm != "stress" || p.Migrations != 1 || p.Role != RoleRemote {
		t.Errorf("new identity = %+v, want stress, 1 migration, remote", p)
	}
}

func TestPIDReuseAfterGap(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	// Entry vanishes and pid 100 now belongs to a different process: exit, not return.
	h.proc[100] = ProcInfo{"bash", 50}
	h.step(nil, nil)
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1, {Exit, "migtest"}: 1})
}

func TestCommCachedAtFirstSight(t *testing.T) {
	h := newHarness(t, Options{PIDRetention: time.Minute})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	delete(h.proc, 100) // /proc/100 gone: comm can no longer be read
	h.step(nil, nil)
	if got := h.pid(100).Comm; got != "migtest" {
		t.Errorf("comm = %q, want cached migtest", got)
	}
	if _, ok := h.tr.Totals()[TotalKey{Exit, "migtest"}]; !ok {
		t.Errorf("exit counted under wrong comm: %v", h.tr.Totals())
	}
}

func TestUnknownCommWhenProcIsGone(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.step(nil, []procfs.Entry{e(100, 2)}) // never in /proc
	h.wantTotals(map[TotalKey]uint64{{Out, "unknown"}: 1})
}

func TestUnavailableSnapshotClearsState(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step([]procfs.Entry{e(900, 2)}, []procfs.Entry{e(100, 2)})
	h.tr.Observe(h.now, procfs.Snapshot{Available: false})
	if h.tr.Available() || len(h.tr.Remote()) != 0 || len(h.tr.Guests()) != 0 {
		t.Errorf("state not cleared: avail=%v remote=%v guests=%v", h.tr.Available(), h.tr.Remote(), h.tr.Guests())
	}
	// Counters are monotonic and nothing extra is counted for the vanished module.
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1, {In, "unknown"}: 1})
}

func TestFilterIgnoresUnlistedComms(t *testing.T) {
	re := regexp.MustCompile(`^migtest$`)
	h := newHarness(t, Options{Filter: re.MatchString})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 1}
	h.proc[101] = ProcInfo{"bash", 2}
	h.step(nil, []procfs.Entry{e(100, 2), e(101, 2)})
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 1})
	if got := h.tr.Remote(); len(got) != 1 || got[0].PID != 100 {
		t.Errorf("Remote() = %v, want only pid 100", got)
	}
}

func TestMaxCommLabelsFoldsIntoOther(t *testing.T) {
	h := newHarness(t, Options{MaxCommLabels: 2})
	h.step(nil, nil)
	for i, c := range []string{"a", "b", "c", "d"} {
		h.proc[100+i] = ProcInfo{c, uint64(i)}
	}
	h.step(nil, []procfs.Entry{e(100, 2), e(101, 2), e(102, 2), e(103, 2)})
	h.wantTotals(map[TotalKey]uint64{{Out, "a"}: 1, {Out, "b"}: 1, {Out, "other"}: 2})
	// The label chosen at first sight stays stable.
	h.step(nil, []procfs.Entry{e(101, 2), e(102, 2), e(103, 2)})
	if got := h.tr.Totals()[TotalKey{Return, "a"}]; got != 1 {
		t.Errorf("return for a = %d, want 1", got)
	}
	var comms []string
	for _, p := range h.tr.Remote() {
		comms = append(comms, p.Comm)
	}
	if !reflect.DeepEqual(comms, []string{"b", "other", "other"}) {
		t.Errorf("remote comms = %v", comms)
	}
}

func TestMaxPIDsLimitsPerPIDButNotTotals(t *testing.T) {
	h := newHarness(t, Options{MaxPIDs: 2})
	h.step(nil, nil)
	for pid := 100; pid < 103; pid++ {
		h.proc[pid] = ProcInfo{"migtest", uint64(pid)}
	}
	h.step(nil, []procfs.Entry{e(100, 2), e(101, 2), e(102, 2)})
	h.wantTotals(map[TotalKey]uint64{{Out, "migtest"}: 3})
	if got := len(h.tr.PIDs()); got != 2 {
		t.Errorf("len(PIDs()) = %d, want 2", got)
	}
	if got := len(h.tr.Remote()); got != 3 {
		t.Errorf("len(Remote()) = %d, want 3 (aggregates are not limited)", got)
	}
}

func TestPIDRetention(t *testing.T) {
	h := newHarness(t, Options{PIDRetention: 5 * time.Second})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	delete(h.proc, 100)
	h.step(nil, nil)
	if got := h.pid(100).Role; got != RoleGone {
		t.Fatalf("role = %v, want gone", got)
	}
	for i := 0; i < 3; i++ {
		h.step(nil, nil)
	}
	h.pid(100) // still retained after 3s
	for i := 0; i < 5; i++ {
		h.step(nil, nil)
	}
	for _, p := range h.tr.PIDs() {
		if p.PID == 100 {
			t.Errorf("pid 100 still reported after retention: %+v", p)
		}
	}
}

func TestZeroRetentionDropsGonePIDsAtOnce(t *testing.T) {
	h := newHarness(t, Options{})
	h.step(nil, nil)
	h.proc[100] = ProcInfo{"migtest", 7}
	h.step(nil, []procfs.Entry{e(100, 2)})
	delete(h.proc, 100)
	h.step(nil, nil)
	if got := h.tr.PIDs(); len(got) != 0 {
		t.Errorf("PIDs() = %v, want none", got)
	}
}
