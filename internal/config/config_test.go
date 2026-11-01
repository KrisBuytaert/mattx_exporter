// SPDX-License-Identifier: GPL-2.0-only

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c := Default()
	want := Config{
		ListenAddress:   ":9105",
		ProcPath:        "/proc/mattx",
		PollInterval:    time.Second,
		MaxCommLabels:   50,
		MaxPIDs:         200,
		PIDRetention:    5 * time.Minute,
		PerPIDMetrics:   false,
		LegacyMetrics:   true,
		KdumpDir:        "/var/crash",
		JournalUnits:    []string{"mattx"},
		ServiceUnit:     "mattx",
		ModuleLoadRegex: "Loading kernel module",
		ExecmemRegex:    "execmem: unable",
		Collectors:      Collectors{Nodes: true, Migration: true, Host: true},
	}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("Default() = %+v\nwant %+v", c, want)
	}
}

func TestParseEmptyGivesDefaults(t *testing.T) {
	for _, in := range []string{"", "# only a comment\n", "---\n"} {
		c, err := Parse([]byte(in))
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if !reflect.DeepEqual(c, Default()) {
			t.Errorf("Parse(%q) = %+v, want defaults", in, c)
		}
	}
}

func TestParseOverridesOnlyWhatIsSet(t *testing.T) {
	c, err := Parse([]byte(`
listen_address: "127.0.0.1:9200"
poll_interval: 500ms
comm_include: ["^migtest$", "^stress"]
comm_exclude: ["^stress-ng"]
max_comm_labels: 10
per_pid_metrics: true
legacy_metrics: false
collectors:
  host: false
`))
	if err != nil {
		t.Fatal(err)
	}
	d := Default()
	d.ListenAddress = "127.0.0.1:9200"
	d.PollInterval = 500 * time.Millisecond
	d.CommInclude = []string{"^migtest$", "^stress"}
	d.CommExclude = []string{"^stress-ng"}
	d.MaxCommLabels = 10
	d.PerPIDMetrics = true
	d.LegacyMetrics = false
	d.Collectors.Host = false // nodes and migration stay on
	if !reflect.DeepEqual(c, d) {
		t.Errorf("got %+v\nwant %+v", c, d)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"unknown key", "listen_adress: \":1\"\n", "listen_adress"},
		{"bad regex include", "comm_include: [\"(\"]\n", "comm_include"},
		{"bad regex exclude", "comm_exclude: [\"[\"]\n", "comm_exclude"},
		{"zero poll interval", "poll_interval: 0s\n", "poll_interval"},
		{"negative poll interval", "poll_interval: -1s\n", "poll_interval"},
		{"bad duration", "poll_interval: soon\n", "line 1"},
		{"negative max pids", "max_pids: -1\n", "max_pids"},
		{"negative max comm labels", "max_comm_labels: -1\n", "max_comm_labels"},
		{"negative retention", "pid_retention: -1s\n", "pid_retention"},
		{"empty listen address", "listen_address: \"\"\n", "listen_address"},
		{"empty proc path", "proc_path: \"\"\n", "proc_path"},
		{"bad module regex", "module_load_regex: \"(\"\n", "module_load_regex"},
		{"not yaml", "a: [\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.in))
			if err == nil {
				t.Fatal("no error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	t.Run("empty path is defaults", func(t *testing.T) {
		c, err := Load("")
		if err != nil || !reflect.DeepEqual(c, Default()) {
			t.Errorf("Load(\"\") = %+v, %v", c, err)
		}
	})
	t.Run("missing file is an error", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "nope.yml")); err == nil {
			t.Error("no error for a missing file")
		}
	})
	t.Run("file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "c.yml")
		if err := os.WriteFile(p, []byte("max_pids: 7\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Load(p)
		if err != nil || c.MaxPIDs != 7 {
			t.Errorf("Load = %+v, %v", c, err)
		}
	})
}

func TestCommFilter(t *testing.T) {
	tests := []struct {
		name             string
		include, exclude []string
		comm             string
		want             bool
	}{
		{"no rules tracks all", nil, nil, "anything", true},
		{"include match", []string{"^migtest$"}, nil, "migtest", true},
		{"include miss", []string{"^migtest$"}, nil, "bash", false},
		{"any include matches", []string{"^a$", "^b$"}, nil, "b", true},
		{"exclude only", nil, []string{"^sshd"}, "sshd", false},
		{"exclude only keeps others", nil, []string{"^sshd"}, "bash", true},
		{"exclude wins", []string{"^stress"}, []string{"^stress-ng"}, "stress-ng", false},
		{"unanchored regex", []string{"mig"}, nil, "xmigx", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.CommInclude, c.CommExclude = tc.include, tc.exclude
			f := c.CommFilter()
			if f == nil {
				t.Fatal("CommFilter() = nil")
			}
			if got := f(tc.comm); got != tc.want {
				t.Errorf("filter(%q) = %v, want %v", tc.comm, got, tc.want)
			}
		})
	}
}
