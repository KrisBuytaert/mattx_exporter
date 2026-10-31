// Package host collects the host-level facts of the legacy mattx-node-metrics.sh:
// module loads and execmem errors since boot, kdump directories and service state.
package host

import (
	"errors"
	"regexp"
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
	return &Source{}
}

func (s *Source) ModuleLoads() (int, error)    { return 0, errors.New("not implemented") }
func (s *Source) ExecmemErrors() (int, error)  { return 0, errors.New("not implemented") }
func (s *Source) KdumpDirs() (int, error)      { return 0, errors.New("not implemented") }
func (s *Source) ServiceActive() (bool, error) { return false, errors.New("not implemented") }
