// SPDX-License-Identifier: GPL-2.0-only

package procfs

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseEntries(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		want      []Entry
		malformed int
	}{
		{"empty", "", nil, 0},
		{"blank lines only", "\n\n  \n", nil, 0},
		{"single", "2345:378\n", []Entry{{2345, 378}}, 0},
		{"no trailing newline", "1:2", []Entry{{1, 2}}, 0},
		{"two", "1234:378\n1235:379\n", []Entry{{1234, 378}, {1235, 379}}, 0},
		{"crlf and padding", "  77:2  \r\n1235:379\r\n", []Entry{{77, 2}, {1235, 379}}, 0},
		{"node zero is valid", "5:0\n", []Entry{{5, 0}}, 0},
		{"not a number", "abc:1\n", nil, 1},
		{"no colon", "12\n", nil, 1},
		{"missing pid", ":3\n", nil, 1},
		{"missing node", "99:\n", nil, 1},
		{"too many fields", "1:2:3\n", nil, 1},
		{"negative pid", "-5:1\n", nil, 1},
		{"pid zero", "0:1\n", nil, 1},
		{"overflow", "4294967296:1\n", nil, 1},
		{"good lines survive bad ones", "1:2\nbad\n3:4\n", []Entry{{1, 2}, {3, 4}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, malformed, err := ParseEntries(strings.NewReader(tc.in))
			if err != nil {
				t.Fatalf("ParseEntries: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("entries = %v, want %v", got, tc.want)
			}
			if malformed != tc.malformed {
				t.Errorf("malformed = %d, want %d", malformed, tc.malformed)
			}
		})
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := readFile(filepath.Join("..", "..", "testdata", "proc", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseNodes(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		got, malformed, err := ParseNodes(strings.NewReader(readFixture(t, "normal/nodes")))
		if err != nil {
			t.Fatalf("ParseNodes: %v", err)
		}
		if malformed != 0 {
			t.Errorf("malformed = %d, want 0", malformed)
		}
		want := []Node{
			{ID: 377, Local: true, IP: "192.168.100.11", CPULoad: 12, Affinity: 4000, MemFreeMB: 1500},
			{ID: 378, IP: "192.168.100.12", CPULoad: 3, Affinity: 2000, MemFreeMB: 1400},
			{ID: 379, IP: "192.168.100.13", CPULoad: 0, Affinity: 1000, MemFreeMB: 900},
		}
		if !reflect.DeepEqual(got.Nodes, want) {
			t.Errorf("nodes = %+v, want %+v", got.Nodes, want)
		}
		for k, v := range map[string]string{
			"Balancer Enabled":   "YES",
			"Accept Guests":      "YES",
			"DSM Mode":           "2",
			"Node Affinity":      "0 (0 = Auto)",
			"Migration Excludes": "sshd,systemd",
			"Migration Includes": "migtest,stress, python3",
		} {
			if got.Settings[k] != v {
				t.Errorf("Settings[%q] = %q, want %q", k, got.Settings[k], v)
			}
		}
	})
	t.Run("empty", func(t *testing.T) {
		got, malformed, err := ParseNodes(strings.NewReader(""))
		if err != nil || malformed != 0 || len(got.Nodes) != 0 {
			t.Errorf("got %+v, %d, %v; want no nodes, 0, nil", got, malformed, err)
		}
	})
	t.Run("malformed rows are skipped and counted", func(t *testing.T) {
		got, malformed, err := ParseNodes(strings.NewReader(readFixture(t, "malformed/nodes")))
		if err != nil {
			t.Fatalf("ParseNodes: %v", err)
		}
		if malformed != 2 {
			t.Errorf("malformed = %d, want 2", malformed)
		}
		ids := []int{}
		for _, n := range got.Nodes {
			ids = append(ids, n.ID)
		}
		if !reflect.DeepEqual(ids, []int{377, 379}) {
			t.Errorf("node ids = %v, want [377 379]", ids)
		}
		if got.Settings["Balancer Enabled"] != "YES" {
			t.Errorf("settings lost after bad rows: %v", got.Settings)
		}
	})
}

func TestRead(t *testing.T) {
	fx := func(name string) string { return filepath.Join("..", "..", "testdata", "proc", name) }

	t.Run("normal", func(t *testing.T) {
		s, err := Read(fx("normal"))
		if err != nil {
			t.Fatal(err)
		}
		if !s.Available || !s.NodesOK {
			t.Errorf("Available=%v NodesOK=%v, want both true", s.Available, s.NodesOK)
		}
		if !reflect.DeepEqual(s.Guests, []Entry{{2345, 378}}) {
			t.Errorf("guests = %v", s.Guests)
		}
		if !reflect.DeepEqual(s.Remote, []Entry{{1234, 378}, {1235, 379}}) {
			t.Errorf("remote = %v", s.Remote)
		}
		if len(s.Nodes.Nodes) != 3 {
			t.Errorf("nodes = %d, want 3", len(s.Nodes.Nodes))
		}
	})
	t.Run("missing dir is not an error", func(t *testing.T) {
		s, err := Read(filepath.Join(t.TempDir(), "nope"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if s.Available || s.NodesOK || len(s.Guests) != 0 || len(s.Remote) != 0 {
			t.Errorf("snapshot of missing dir = %+v, want zero value", s)
		}
	})
	t.Run("empty files are available with no entries", func(t *testing.T) {
		s, err := Read(fx("empty"))
		if err != nil {
			t.Fatal(err)
		}
		if !s.Available || !s.NodesOK {
			t.Errorf("Available=%v NodesOK=%v, want both true", s.Available, s.NodesOK)
		}
		if len(s.Guests)+len(s.Remote)+len(s.Nodes.Nodes) != 0 || s.Malformed != 0 {
			t.Errorf("snapshot = %+v, want empty", s)
		}
	})
	t.Run("malformed lines are counted across files", func(t *testing.T) {
		s, err := Read(fx("malformed"))
		if err != nil {
			t.Fatal(err)
		}
		// nodes: 2, guests: 8, remote: 0
		if s.Malformed != 10 {
			t.Errorf("Malformed = %d, want 10", s.Malformed)
		}
		if len(s.Guests) != 2 || len(s.Remote) != 2 {
			t.Errorf("guests=%v remote=%v, want 2 and 2", s.Guests, s.Remote)
		}
	})
	t.Run("no nodes file", func(t *testing.T) {
		s, err := Read(fx("nonodes"))
		if err != nil {
			t.Fatal(err)
		}
		if !s.Available || s.NodesOK {
			t.Errorf("Available=%v NodesOK=%v, want true,false", s.Available, s.NodesOK)
		}
	})
	t.Run("only one of guests/remote is not available", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeFile(filepath.Join(dir, "guests"), "1:2\n"); err != nil {
			t.Fatal(err)
		}
		s, err := Read(dir)
		if err != nil {
			t.Fatal(err)
		}
		if s.Available {
			t.Error("Available = true with remote missing, want false (matches the legacy script)")
		}
	})
}
