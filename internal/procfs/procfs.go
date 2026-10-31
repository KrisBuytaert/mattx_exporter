// Package procfs parses the files the MattX kernel module exposes under /proc/mattx.
package procfs

import (
	"errors"
	"io"
)

// ErrNotImplemented marks API that exists but has no implementation yet.
var ErrNotImplemented = errors.New("not implemented")

// Node is one row of /proc/mattx/nodes.
type Node struct {
	ID        int
	Local     bool
	IP        string
	CPULoad   uint32
	Affinity  uint32
	MemFreeMB uint32
}

// Nodes is the parsed /proc/mattx/nodes file: the node table plus the
// "Key: value" settings printed below it.
type Nodes struct {
	Nodes    []Node
	Settings map[string]string
}

// Entry is one "pid:node" line of /proc/mattx/guests or /proc/mattx/remote.
// In guests, PID is the local pid of a hosted process and Node its home node.
// In remote, PID is the local pid of an own process and Node the node it runs on.
type Entry struct {
	PID  int
	Node int
}

// Snapshot is one read of the proc directory.
type Snapshot struct {
	// Available is true when both guests and remote were readable.
	Available bool
	// NodesOK is true when the nodes file was readable.
	NodesOK bool
	Nodes   Nodes
	Guests  []Entry
	Remote  []Entry
	// Malformed counts lines that were skipped because they could not be parsed.
	Malformed int
}

// ParseNodes parses /proc/mattx/nodes. Unparsable table rows are skipped and counted.
func ParseNodes(r io.Reader) (Nodes, int, error) {
	return Nodes{}, 0, ErrNotImplemented
}

// ParseEntries parses a guests or remote file. Unparsable lines are skipped and counted;
// blank lines are ignored without counting.
func ParseEntries(r io.Reader) ([]Entry, int, error) {
	return nil, 0, ErrNotImplemented
}

// Read reads nodes, guests and remote below dir. A missing dir or missing guests/remote
// files is not an error: the snapshot is then simply not Available.
func Read(dir string) (Snapshot, error) {
	return Snapshot{}, ErrNotImplemented
}
