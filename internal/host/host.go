// Package host collects the host-level facts of the legacy mattx-node-metrics.sh:
// module loads and execmem errors since boot, kdump directories and service state.
package host

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// RunFunc runs a command and returns its combined stdout.
type RunFunc func(name string, args ...string) ([]byte, error)

// Source reads the facts. The zero value is not usable; use New.
type Source struct {
	Run          RunFunc
	KdumpDir     string
	JournalUnits []string
	ServiceUnit  string
	ModuleLoad   *regexp.Regexp
	Execmem      *regexp.Regexp
}

// New returns a Source that runs the real journalctl and systemctl.
func New(kdumpDir string, journalUnits []string, serviceUnit string, moduleLoad, execmem *regexp.Regexp) *Source {
	return &Source{
		Run: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).Output()
		},
		KdumpDir:     kdumpDir,
		JournalUnits: journalUnits,
		ServiceUnit:  serviceUnit,
		ModuleLoad:   moduleLoad,
		Execmem:      execmem,
	}
}

// ModuleLoads counts matching journal lines of the configured units since boot.
func (s *Source) ModuleLoads() (int, error) {
	total := 0
	for _, u := range s.JournalUnits {
		n, err := s.count(s.ModuleLoad, "-b", "-u", u, "--no-pager", "-q")
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// ExecmemErrors counts matching kernel log lines since boot.
func (s *Source) ExecmemErrors() (int, error) {
	return s.count(s.Execmem, "-b", "-k", "--no-pager", "-q")
}

func (s *Source) count(re *regexp.Regexp, args ...string) (int, error) {
	out, err := s.Run("journalctl", args...)
	if err != nil {
		return 0, fmt.Errorf("journalctl %s: %w", strings.Join(args, " "), err)
	}
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if re.MatchString(line) {
			n++
		}
	}
	return n, nil
}

// KdumpDirs counts directories directly below the kdump directory; a missing one is zero.
func (s *Source) KdumpDirs() (int, error) {
	entries, err := os.ReadDir(s.KdumpDir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n, nil
}

// ServiceActive reports whether the service unit is active. A non-zero exit of
// systemctl means inactive; failing to run systemctl at all is an error.
func (s *Source) ServiceActive() (bool, error) {
	_, err := s.Run("systemctl", "is-active", "--quiet", s.ServiceUnit)
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee):
		return false, nil
	default:
		return false, err
	}
}
