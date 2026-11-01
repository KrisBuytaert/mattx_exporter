// SPDX-License-Identifier: GPL-2.0-only

// Package migration turns successive /proc/mattx snapshots into migration events,
// counters and per-pid state. Entries are tracked by pid, not by comm.
package migration

import (
	"sort"
	"sync"
	"time"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procfs"
)

// Direction classifies a migration event.
type Direction string

const (
	// Out: an own process started running on another node (remote entry appeared).
	Out Direction = "out"
	// Hop: an own, already remote process moved from one remote node to another.
	Hop Direction = "hop"
	// Return: a remote entry vanished and the process is still alive at home.
	Return Direction = "return"
	// Exit: a remote entry vanished and the process is gone (exited remotely).
	Exit Direction = "exit"
	// In: a guest of another node appeared here.
	In Direction = "in"
	// Departed: a hosted guest disappeared.
	Departed Direction = "departed"
)

// Role says where a tracked pid currently is.
type Role string

const (
	RoleRemote Role = "remote" // own process running on another node
	RoleGuest  Role = "guest"  // foreign process hosted here
	RoleHome   Role = "home"   // own process that came back and is alive here
	RoleGone   Role = "gone"   // process ended (exited remotely or guest departed)
)

// ProcInfo identifies a live process: Start is its start time from /proc/<pid>/stat,
// used to tell a reused pid from the original process.
type ProcInfo struct {
	Comm  string
	Start uint64
}

// Lookup returns information about a local pid, or false if it does not exist.
type Lookup func(pid int) (ProcInfo, bool)

// Options configures a Tracker. Policy comes from the YAML config, not from here.
type Options struct {
	// Filter reports whether processes with this comm are tracked; nil tracks all.
	Filter func(comm string) bool
	// MaxCommLabels limits distinct comm label values; later ones become "other".
	// Zero means unlimited.
	MaxCommLabels int
	// MaxPIDs limits how many pids PIDs() reports. Zero means unlimited.
	MaxPIDs int
	// PIDRetention is how long a pid that is gone stays in PIDs(). Zero drops it at once.
	PIDRetention time.Duration
}

// TotalKey keys the aggregated migration counters.
type TotalKey struct {
	Direction Direction
	Comm      string
}

// Placement is a currently remote or hosted process.
type Placement struct {
	PID  int
	Comm string
	// Node is the target node for remote entries and the home node for guests.
	Node int
}

// PIDStat is the per-pid view.
type PIDStat struct {
	PID  int
	Comm string
	// Migrations counts location changes of this pid identity: out, hop and return.
	Migrations uint64
	Role       Role
	// Node is the node the pid runs on (remote) or comes from (guest); -1 otherwise.
	Node int
}

// track is the state of one pid identity.
type track struct {
	comm       string
	start      uint64
	role       Role
	node       int
	migrations uint64
	goneAt     time.Time
}

// Tracker is safe for concurrent use.
type Tracker struct {
	opts   Options
	lookup Lookup

	mu        sync.Mutex
	baselined bool
	available bool
	remote    map[int]*track // active remote entries
	guests    map[int]*track // active guest entries
	pids      map[int]*track // everything reported per pid
	totals    map[TotalKey]uint64
	comms     map[string]struct{} // distinct comm labels handed out
}

// New returns a Tracker. lookup is used to cache comm at first sight and to tell a
// returned process from an exited one.
func New(o Options, lookup Lookup) *Tracker {
	return &Tracker{
		opts:   o,
		lookup: lookup,
		remote: map[int]*track{},
		guests: map[int]*track{},
		pids:   map[int]*track{},
		totals: map[TotalKey]uint64{},
		comms:  map[string]struct{}{},
	}
}

const (
	commUnknown = "unknown"
	commOther   = "other"
)

// Observe feeds one snapshot taken at now. The first Available snapshot is only a
// baseline: nothing is counted for entries that already exist.
func (t *Tracker) Observe(now time.Time, s procfs.Snapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !s.Available {
		t.available = false
		for _, m := range []map[int]*track{t.remote, t.guests} {
			for pid, tr := range m {
				tr.role, tr.node, tr.goneAt = RoleGone, -1, now
				delete(m, pid)
			}
		}
		t.purge(now)
		return
	}
	t.available = true
	count := t.baselined
	t.baselined = true

	t.sync(now, count, s.Remote, t.remote, RoleRemote)
	t.sync(now, count, s.Guests, t.guests, RoleGuest)
	t.purge(now)
}

// sync reconciles one entry list with the active map of that kind.
func (t *Tracker) sync(now time.Time, count bool, entries []procfs.Entry, active map[int]*track, role Role) {
	seen := make(map[int]bool, len(entries))
	for _, e := range entries {
		seen[e.PID] = true
		info, alive := t.lookup(e.PID)
		if prev, ok := active[e.PID]; ok {
			if alive && prev.start != 0 && info.Start != prev.start {
				// The pid was reused while the entry never went away: the old process is
				// gone, the entry now belongs to a new one.
				t.leave(now, count, prev, e.PID, role, false)
				delete(active, e.PID)
			} else {
				if prev.node != e.Node {
					prev.node = e.Node
					if role == RoleRemote {
						prev.migrations++
						t.add(count, Hop, prev.comm)
					}
				}
				continue
			}
		}
		if tr := t.enter(now, count, e, info, alive, role); tr != nil {
			active[e.PID] = tr
		}
	}
	for pid, tr := range active {
		if !seen[pid] {
			info, alive := t.lookup(pid)
			t.leave(now, count, tr, pid, role, alive && (tr.start == 0 || info.Start == tr.start))
			delete(active, pid)
		}
	}
}

// enter registers a new entry, or nil if the comm is filtered out.
func (t *Tracker) enter(now time.Time, count bool, e procfs.Entry, info ProcInfo, alive bool, role Role) *track {
	comm, start := commUnknown, uint64(0)
	if alive {
		if t.opts.Filter != nil && !t.opts.Filter(info.Comm) {
			return nil
		}
		comm, start = t.label(info.Comm), info.Start
	}
	tr := t.pids[e.PID]
	if tr == nil || tr.role != RoleHome || tr.start != start || start == 0 {
		tr = &track{comm: comm, start: start}
		t.pids[e.PID] = tr
	}
	tr.role, tr.node = role, e.Node
	if role == RoleRemote {
		if count {
			tr.migrations++
		}
		t.add(count, Out, tr.comm)
	} else {
		t.add(count, In, tr.comm)
	}
	return tr
}

// leave handles an entry that vanished. stillAlive means the process is alive on this
// node, which for an own process means it came back.
func (t *Tracker) leave(now time.Time, count bool, tr *track, pid int, role Role, stillAlive bool) {
	if role == RoleGuest {
		t.add(count, Departed, tr.comm)
		tr.role, tr.node, tr.goneAt = RoleGone, -1, now
		return
	}
	if stillAlive {
		tr.migrations++
		t.add(count, Return, tr.comm)
		tr.role, tr.node = RoleHome, -1
		return
	}
	t.add(count, Exit, tr.comm)
	tr.role, tr.node, tr.goneAt = RoleGone, -1, now
}

func (t *Tracker) add(count bool, d Direction, comm string) {
	if count {
		t.totals[TotalKey{d, comm}]++
	}
}

// label maps a comm to its label value, folding into "other" past MaxCommLabels.
func (t *Tracker) label(comm string) string {
	if _, ok := t.comms[comm]; ok || t.opts.MaxCommLabels == 0 {
		return comm
	}
	if len(t.comms) >= t.opts.MaxCommLabels {
		return commOther
	}
	t.comms[comm] = struct{}{}
	return comm
}

// purge ends returned processes that died at home and drops gone pids past retention.
func (t *Tracker) purge(now time.Time) {
	for pid, tr := range t.pids {
		if tr.role == RoleHome {
			if info, ok := t.lookup(pid); !ok || info.Start != tr.start {
				tr.role, tr.goneAt = RoleGone, now
			}
		}
		if tr.role == RoleGone && now.Sub(tr.goneAt) >= t.opts.PIDRetention {
			delete(t.pids, pid)
		}
	}
}

// Available reports whether the last observed snapshot was Available.
func (t *Tracker) Available() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.available
}

// Totals returns a copy of the aggregated counters.
func (t *Tracker) Totals() map[TotalKey]uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[TotalKey]uint64, len(t.totals))
	for k, v := range t.totals {
		out[k] = v
	}
	return out
}

// Remote returns own processes currently running elsewhere, ordered by pid.
func (t *Tracker) Remote() []Placement {
	return t.placements(func() map[int]*track { return t.remote })
}

// Guests returns hosted foreign processes, ordered by pid.
func (t *Tracker) Guests() []Placement {
	return t.placements(func() map[int]*track { return t.guests })
}

func (t *Tracker) placements(m func() map[int]*track) []Placement {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Placement
	for pid, tr := range m() {
		out = append(out, Placement{PID: pid, Comm: tr.comm, Node: tr.node})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// PIDs returns per-pid state ordered by pid, honouring MaxPIDs and PIDRetention.
func (t *Tracker) PIDs() []PIDStat {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]PIDStat, 0, len(t.pids))
	for pid, tr := range t.pids {
		out = append(out, PIDStat{PID: pid, Comm: tr.comm, Migrations: tr.migrations, Role: tr.role, Node: tr.node})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	if t.opts.MaxPIDs > 0 && len(out) > t.opts.MaxPIDs {
		out = out[:t.opts.MaxPIDs]
	}
	return out
}
