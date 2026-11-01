// SPDX-License-Identifier: GPL-2.0-only

package host

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type call struct{ name, args string }

// fakeRun returns canned output per joined command line and records the calls.
func fakeRun(out map[string]string, errs map[string]error, calls *[]call) RunFunc {
	return func(name string, args ...string) ([]byte, error) {
		a := strings.Join(args, " ")
		*calls = append(*calls, call{name, a})
		key := name + " " + a
		return []byte(out[key]), errs[key]
	}
}

func newSource(run RunFunc) *Source {
	return &Source{
		Run:          run,
		KdumpDir:     "/nonexistent",
		JournalUnits: []string{"mattx"},
		ServiceUnit:  "mattx",
		ModuleLoad:   regexp.MustCompile("Loading kernel module"),
		Execmem:      regexp.MustCompile("execmem: unable"),
	}
}

func TestModuleLoads(t *testing.T) {
	var calls []call
	s := newSource(fakeRun(map[string]string{
		"journalctl -b -u mattx --no-pager -q": "a Loading kernel module mattx\nother\nb Loading kernel module mattx\n",
	}, nil, &calls))
	got, err := s.ModuleLoads()
	if err != nil || got != 2 {
		t.Errorf("ModuleLoads() = %d, %v; want 2, nil", got, err)
	}
	if len(calls) != 1 {
		t.Errorf("calls = %v", calls)
	}
}

func TestModuleLoadsSumsUnits(t *testing.T) {
	var calls []call
	s := newSource(fakeRun(map[string]string{
		"journalctl -b -u mattx --no-pager -q":      "Loading kernel module\n",
		"journalctl -b -u mattx-load --no-pager -q": "Loading kernel module\nLoading kernel module\n",
	}, nil, &calls))
	s.JournalUnits = []string{"mattx", "mattx-load"}
	got, err := s.ModuleLoads()
	if err != nil || got != 3 {
		t.Errorf("ModuleLoads() = %d, %v; want 3, nil", got, err)
	}
}

func TestModuleLoadsNoMatchIsZero(t *testing.T) {
	var calls []call
	s := newSource(fakeRun(nil, nil, &calls))
	if got, err := s.ModuleLoads(); err != nil || got != 0 {
		t.Errorf("= %d, %v; want 0, nil", got, err)
	}
}

func TestExecmemErrors(t *testing.T) {
	var calls []call
	s := newSource(fakeRun(map[string]string{
		"journalctl -b -k --no-pager -q": "x execmem: unable to allocate memory\ny\nexecmem: unable again\n",
	}, nil, &calls))
	got, err := s.ExecmemErrors()
	if err != nil || got != 2 {
		t.Errorf("ExecmemErrors() = %d, %v; want 2, nil", got, err)
	}
}

func TestJournalFailureIsAnError(t *testing.T) {
	var calls []call
	boom := errors.New("journalctl: permission denied")
	s := newSource(fakeRun(nil, map[string]error{
		"journalctl -b -u mattx --no-pager -q": boom,
		"journalctl -b -k --no-pager -q":       boom,
	}, &calls))
	if _, err := s.ModuleLoads(); err == nil {
		t.Error("ModuleLoads: no error")
	}
	if _, err := s.ExecmemErrors(); err == nil {
		t.Error("ExecmemErrors: no error")
	}
}

func TestKdumpDirs(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"127.0.0.1-2026-10-31", "127.0.0.1-2026-10-30"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "a-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var calls []call
	s := newSource(fakeRun(nil, nil, &calls))
	s.KdumpDir = dir
	if got, err := s.KdumpDirs(); err != nil || got != 2 {
		t.Errorf("KdumpDirs() = %d, %v; want 2 (files are not counted)", got, err)
	}
	s.KdumpDir = filepath.Join(dir, "missing")
	if got, err := s.KdumpDirs(); err != nil || got != 0 {
		t.Errorf("KdumpDirs(missing) = %d, %v; want 0, nil", got, err)
	}
}

func TestServiceActive(t *testing.T) {
	var calls []call
	key := "systemctl is-active --quiet mattx"
	tests := []struct {
		name    string
		err     error
		want    bool
		wantErr bool
	}{
		{"active", nil, true, false},
		{"inactive is an ExitError", &exec.ExitError{}, false, false},
		{"systemctl missing", errors.New("exec: systemctl: not found"), false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSource(fakeRun(nil, map[string]error{key: tc.err}, &calls))
			got, err := s.ServiceActive()
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("ServiceActive() = %v, %v; want %v, err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
