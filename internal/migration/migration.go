// Package migration turns successive /proc/mattx snapshots into migration events,
// counters and per-pid state. Entries are tracked by pid, not by comm.
package migration

import (
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

// Tracker is safe for concurrent use.
type Tracker struct{}

// New returns a Tracker. lookup is used to cache comm at first sight and to tell a
// returned process from an exited one.
func New(o Options, lookup Lookup) *Tracker { return &Tracker{} }

// Observe feeds one snapshot taken at now. The first Available snapshot is only a
// baseline: nothing is counted for entries that already exist.
func (t *Tracker) Observe(now time.Time, s procfs.Snapshot) {}

// Available reports whether the last observed snapshot was Available.
func (t *Tracker) Available() bool { return false }

// Totals returns a copy of the aggregated counters.
func (t *Tracker) Totals() map[TotalKey]uint64 { return nil }

// Remote returns own processes currently running elsewhere, ordered by pid.
func (t *Tracker) Remote() []Placement { return nil }

// Guests returns hosted foreign processes, ordered by pid.
func (t *Tracker) Guests() []Placement { return nil }

// PIDs returns per-pid state ordered by pid, honouring MaxPIDs and PIDRetention.
func (t *Tracker) PIDs() []PIDStat { return nil }
