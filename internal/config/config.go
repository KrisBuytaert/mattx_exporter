// Package config loads /etc/mattx_exporter.yml. All policy (which processes to track,
// limits, paths) lives in that file; the program only supplies defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
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
func Default() Config {
	return Config{
		ListenAddress:   ":9105",
		ProcPath:        "/proc/mattx",
		PollInterval:    time.Second,
		MaxCommLabels:   50,
		MaxPIDs:         200,
		PIDRetention:    5 * time.Minute,
		LegacyMetrics:   true,
		KdumpDir:        "/var/crash",
		JournalUnits:    []string{"mattx"},
		ServiceUnit:     "mattx",
		ModuleLoadRegex: "Loading kernel module",
		ExecmemRegex:    "execmem: unable",
		Collectors:      Collectors{Nodes: true, Migration: true, Host: true},
	}
}

// Parse decodes YAML over the defaults and validates the result. Unknown keys are errors.
func Parse(data []byte) (Config, error) {
	c := Default()
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Load reads path. An empty path returns the defaults; a path that cannot be read is an error.
func Load(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Validate reports the first invalid setting.
func (c Config) Validate() error {
	switch {
	case c.ListenAddress == "":
		return errors.New("config: listen_address must not be empty")
	case c.ProcPath == "":
		return errors.New("config: proc_path must not be empty")
	case c.PollInterval <= 0:
		return fmt.Errorf("config: poll_interval must be positive, got %s", c.PollInterval)
	case c.MaxCommLabels < 0:
		return errors.New("config: max_comm_labels must not be negative")
	case c.MaxPIDs < 0:
		return errors.New("config: max_pids must not be negative")
	case c.PIDRetention < 0:
		return errors.New("config: pid_retention must not be negative")
	}
	for _, r := range []struct {
		field string
		exprs []string
	}{
		{"comm_include", c.CommInclude},
		{"comm_exclude", c.CommExclude},
		{"module_load_regex", []string{c.ModuleLoadRegex}},
		{"execmem_regex", []string{c.ExecmemRegex}},
	} {
		for _, e := range r.exprs {
			if _, err := regexp.Compile(e); err != nil {
				return fmt.Errorf("config: %s: %w", r.field, err)
			}
		}
	}
	return nil
}

// CommFilter returns the predicate selecting which comms are tracked. The config must
// have passed Validate; invalid expressions are ignored.
func (c Config) CommFilter() func(comm string) bool {
	include, exclude := compileAll(c.CommInclude), compileAll(c.CommExclude)
	return func(comm string) bool {
		for _, re := range exclude {
			if re.MatchString(comm) {
				return false
			}
		}
		if len(include) == 0 {
			return true
		}
		for _, re := range include {
			if re.MatchString(comm) {
				return true
			}
		}
		return false
	}
}

func compileAll(exprs []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, e := range exprs {
		if re, err := regexp.Compile(e); err == nil {
			out = append(out, re)
		}
	}
	return out
}
