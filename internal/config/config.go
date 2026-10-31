// Package config loads /etc/mattx_exporter.yml. All policy (which processes to track,
// limits, paths) lives in that file; the program only supplies defaults.
package config

import (
	"errors"
	"time"
)

// Collectors toggles whole metric groups.
type Collectors struct {
	Nodes     bool `yaml:"nodes"`
	Migration bool `yaml:"migration"`
	Host      bool `yaml:"host"`
}

// Config is the exporter configuration.
type Config struct {
	ListenAddress string        `yaml:"listen_address"`
	ProcPath      string        `yaml:"proc_path"`
	PollInterval  time.Duration `yaml:"poll_interval"`

	// CommInclude and CommExclude are regular expressions matched against a process comm.
	// An empty include list tracks every comm; exclude wins over include.
	CommInclude []string `yaml:"comm_include"`
	CommExclude []string `yaml:"comm_exclude"`

	// Cardinality limits. Zero means unlimited.
	MaxCommLabels int           `yaml:"max_comm_labels"`
	MaxPIDs       int           `yaml:"max_pids"`
	PIDRetention  time.Duration `yaml:"pid_retention"`

	PerPIDMetrics bool `yaml:"per_pid_metrics"`
	LegacyMetrics bool `yaml:"legacy_metrics"`

	KdumpDir     string   `yaml:"kdump_dir"`
	JournalUnits []string `yaml:"journal_units"`
	ServiceUnit  string   `yaml:"service_unit"`
	// ModuleLoadRegex and ExecmemRegex select the journal lines counted since boot.
	ModuleLoadRegex string `yaml:"module_load_regex"`
	ExecmemRegex    string `yaml:"execmem_regex"`

	Collectors Collectors `yaml:"collectors"`
}

// Default returns the configuration used when no file is given.
func Default() Config { return Config{} }

// Parse decodes YAML over the defaults and validates the result. Unknown keys are errors.
func Parse(data []byte) (Config, error) { return Config{}, errors.New("not implemented") }

// Load reads path. An empty path returns the defaults; a path that cannot be read is an error.
func Load(path string) (Config, error) { return Config{}, errors.New("not implemented") }

// CommFilter returns the predicate selecting which comms are tracked.
func (c Config) CommFilter() func(comm string) bool { return nil }
