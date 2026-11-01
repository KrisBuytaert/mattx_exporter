// SPDX-License-Identifier: GPL-2.0-only

// Package procfs parses the files the MattX kernel module exposes under /proc/mattx.
package procfs

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	// Seq numbers the polls; the poller increments it per read so consumers can tell a
	// new snapshot from one they already saw.
	Seq uint64
}

// ParseNodes parses /proc/mattx/nodes. Unparsable table rows are skipped and counted.
func ParseNodes(r io.Reader) (Nodes, int, error) {
	n := Nodes{Settings: map[string]string{}}
	malformed := 0
	inTable := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			inTable = false
		case strings.HasPrefix(line, "Node ID"):
			inTable = true
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "MattX Cluster Nodes"):
		case inTable:
			node, ok := parseNodeRow(line)
			if !ok {
				malformed++
				continue
			}
			n.Nodes = append(n.Nodes, node)
		default:
			if k, v, ok := strings.Cut(line, ":"); ok {
				n.Settings[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	return n, malformed, sc.Err()
}

// parseNodeRow parses "ID [(Local)] IP load affinity mem_mb".
func parseNodeRow(line string) (Node, bool) {
	f := strings.Fields(line)
	var n Node
	if len(f) > 1 && f[1] == "(Local)" {
		n.Local = true
		f = append(f[:1], f[2:]...)
	}
	if len(f) != 5 {
		return Node{}, false
	}
	id, err := strconv.ParseInt(f[0], 10, 32)
	if err != nil || net.ParseIP(f[1]) == nil {
		return Node{}, false
	}
	var u [3]uint32
	for i := range u {
		v, err := strconv.ParseUint(f[2+i], 10, 32)
		if err != nil {
			return Node{}, false
		}
		u[i] = uint32(v)
	}
	n.ID, n.IP = int(id), f[1]
	n.CPULoad, n.Affinity, n.MemFreeMB = u[0], u[1], u[2]
	return n, true
}

// ParseEntries parses a guests or remote file. Unparsable lines are skipped and counted;
// blank lines are ignored without counting.
func ParseEntries(r io.Reader) ([]Entry, int, error) {
	var out []Entry
	malformed := 0
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		p, n, ok := strings.Cut(line, ":")
		pid, err1 := strconv.ParseInt(strings.TrimSpace(p), 10, 32)
		node, err2 := strconv.ParseInt(strings.TrimSpace(n), 10, 32)
		if !ok || err1 != nil || err2 != nil || pid <= 0 || node < 0 {
			malformed++
			continue
		}
		out = append(out, Entry{PID: int(pid), Node: int(node)})
	}
	return out, malformed, sc.Err()
}

// Read reads nodes, guests and remote below dir. A missing dir or missing guests/remote
// files is not an error: the snapshot is then simply not Available.
func Read(dir string) (Snapshot, error) {
	var s Snapshot
	guests, gok, gbad, err := readEntries(filepath.Join(dir, "guests"))
	if err != nil {
		return Snapshot{}, err
	}
	remote, rok, rbad, err := readEntries(filepath.Join(dir, "remote"))
	if err != nil {
		return Snapshot{}, err
	}
	s.Available = gok && rok
	if s.Available {
		s.Guests, s.Remote = guests, remote
		s.Malformed += gbad + rbad
	}
	f, err := os.Open(filepath.Join(dir, "nodes"))
	switch {
	case err == nil:
		defer f.Close()
		nodes, bad, err := ParseNodes(f)
		if err != nil {
			return Snapshot{}, err
		}
		s.NodesOK, s.Nodes = true, nodes
		s.Malformed += bad
	case !errors.Is(err, fs.ErrNotExist):
		return Snapshot{}, err
	}
	return s, nil
}

// readEntries reads one entry file; ok is false if it does not exist.
func readEntries(path string) (entries []Entry, ok bool, malformed int, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, 0, nil
	}
	if err != nil {
		return nil, false, 0, err
	}
	defer f.Close()
	entries, malformed, err = ParseEntries(f)
	return entries, err == nil, malformed, err
}
