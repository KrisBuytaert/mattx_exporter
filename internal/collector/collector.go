// Package collector exposes tracker, node table and host state as Prometheus metrics.
package collector

import (
	"github.com/prometheus/client_golang/prometheus"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procfs"
)

// Options selects what is exported. It mirrors the YAML config; policy lives there.
type Options struct {
	Nodes     bool // node table metrics
	Migration bool // migration state and counters
	Host      bool // host collectors (module loads, kdump, service)
	Legacy    bool // legacy metric names of the textfile scripts
	PerPID    bool // per-pid metrics
}

// SnapshotSource provides the latest polled /proc/mattx snapshot.
type SnapshotSource interface {
	Latest() procfs.Snapshot
}

// HostSource provides the host-level facts the node script used to collect.
type HostSource interface {
	ModuleLoads() (int, error)
	ExecmemErrors() (int, error)
	KdumpDirs() (int, error)
	ServiceActive() (bool, error)
}

// Build is reported as mattx_build_info.
type Build struct {
	Version   string
	Commit    string
	GoVersion string
}

// Collector implements prometheus.Collector.
type Collector struct{}

// New returns a Collector. h may be nil when Options.Host is false.
func New(o Options, snap SnapshotSource, tr *migration.Tracker, h HostSource, b Build) *Collector {
	return &Collector{}
}

// Describe implements prometheus.Collector. The collector is unchecked.
func (c *Collector) Describe(chan<- *prometheus.Desc) {}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(chan<- prometheus.Metric) {}
