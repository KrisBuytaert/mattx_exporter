// SPDX-License-Identifier: GPL-2.0-only

// Package collector exposes tracker, node table and host state as Prometheus metrics.
package collector

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/KrisBuytaert/mattx_exporter/internal/migration"
	"github.com/KrisBuytaert/mattx_exporter/internal/procfs"
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
type Collector struct {
	o     Options
	snap  SnapshotSource
	tr    *migration.Tracker
	host  HostSource
	build Build

	mu       sync.Mutex
	errs     map[string]float64 // scrape errors by collector
	seenNode map[int]string     // node id -> ip of every node seen in the table
	lastSeq  uint64
	seqSeen  bool
}

// Collector names used as label values of mattx_exporter_scrape_errors_total.
const (
	nameNodes     = "nodes"
	nameMigration = "migration"
	nameHost      = "host"
)

func desc(name, help string, labels ...string) *prometheus.Desc {
	return prometheus.NewDesc(name, help, labels, nil)
}

var (
	descBuildInfo = desc("mattx_build_info", "Build information of mattx_exporter.", "version", "commit", "goversion")
	descScrapeErr = desc("mattx_exporter_scrape_errors_total", "Errors encountered while collecting, by collector.", "collector")

	descNodeLoad     = desc("mattx_node_load", "CPU load of the node as reported in /proc/mattx/nodes.", "node", "ip")
	descNodeMem      = desc("mattx_node_mem_free_bytes", "Free memory of the node in bytes.", "node", "ip")
	descNodeAffinity = desc("mattx_node_affinity", "Scheduling affinity weight of the node.", "node", "ip")
	descNodeUp       = desc("mattx_node_up", "1 if the node is in the cluster table, 0 if it was seen before and is gone.", "node", "ip")

	descProcAvail   = desc("mattx_migration_proc_available", "1 if /proc/mattx/{guests,remote} is readable.")
	descGuestsHeld  = desc("mattx_guests_hosted", "Processes of other nodes currently hosted here.")
	descRemoteProcs = desc("mattx_remote_processes", "Own processes currently running on other nodes.")
	descGuestProc   = desc("mattx_guest_process", "Hosted guest processes by comm and home node.", "comm", "home_node")
	descRemoteProc  = desc("mattx_remote_process", "Own processes running elsewhere by comm and target node.", "comm", "target_node")
	descMigrations  = desc("mattx_migrations_total", "Migration transitions seen here since the exporter started (out, hop, return, exit: own processes; in, departed: hosted guests).", "direction", "comm")

	descPIDMigrations = desc("mattx_pid_migrations_total", "Times this pid changed location (out, hop, return) since the exporter saw it.", "pid", "comm")
	descPIDNode       = desc("mattx_pid_node", "Node a tracked pid runs on (role remote) or comes from (role guest).", "pid", "comm", "role")

	descModuleLoads = desc("mattx_module_loads_since_boot", "Times mattx.ko was loaded since boot.")
	descExecmem     = desc("mattx_execmem_errors_since_boot", "Kernel 'execmem: unable to allocate' lines since boot.")
	descKdump       = desc("mattx_kdump_dirs", "Crash dump directories in the kdump directory.")
	descServiceUp   = desc("mattx_service_active", "1 if the mattx systemd service is active.")
)

// New returns a Collector. h may be nil when Options.Host is false.
func New(o Options, snap SnapshotSource, tr *migration.Tracker, h HostSource, b Build) *Collector {
	return &Collector{
		o: o, snap: snap, tr: tr, host: h, build: b,
		errs:     map[string]float64{nameNodes: 0, nameMigration: 0, nameHost: 0},
		seenNode: map[int]string{},
	}
}

// Describe implements prometheus.Collector. The collector is unchecked: the set of
// series depends on the cluster.
func (c *Collector) Describe(chan<- *prometheus.Desc) {}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := c.snap.Latest()
	ch <- prometheus.MustNewConstMetric(descBuildInfo, prometheus.GaugeValue, 1, c.build.Version, c.build.Commit, c.build.GoVersion)

	if !c.seqSeen || snap.Seq != c.lastSeq {
		c.errs[nameMigration] += float64(snap.Malformed)
		c.lastSeq, c.seqSeen = snap.Seq, true
	}
	if c.o.Nodes {
		c.collectNodes(ch, snap)
	}
	if c.o.Migration {
		c.collectMigration(ch, snap)
	}
	if c.o.Host && c.host != nil {
		c.collectHost(ch)
	}
	for _, name := range []string{nameHost, nameMigration, nameNodes} {
		ch <- prometheus.MustNewConstMetric(descScrapeErr, prometheus.CounterValue, c.errs[name], name)
	}
}

func (c *Collector) collectNodes(ch chan<- prometheus.Metric, snap procfs.Snapshot) {
	if !snap.NodesOK {
		return
	}
	present := map[int]bool{}
	for _, n := range snap.Nodes.Nodes {
		present[n.ID] = true
		c.seenNode[n.ID] = n.IP
		id := strconv.Itoa(n.ID)
		ch <- prometheus.MustNewConstMetric(descNodeLoad, prometheus.GaugeValue, float64(n.CPULoad), id, n.IP)
		ch <- prometheus.MustNewConstMetric(descNodeMem, prometheus.GaugeValue, float64(n.MemFreeMB)*1024*1024, id, n.IP)
		ch <- prometheus.MustNewConstMetric(descNodeAffinity, prometheus.GaugeValue, float64(n.Affinity), id, n.IP)
		ch <- prometheus.MustNewConstMetric(descNodeUp, prometheus.GaugeValue, 1, id, n.IP)
	}
	for id, ip := range c.seenNode {
		if !present[id] {
			ch <- prometheus.MustNewConstMetric(descNodeUp, prometheus.GaugeValue, 0, strconv.Itoa(id), ip)
		}
	}
}

func (c *Collector) collectMigration(ch chan<- prometheus.Metric, snap procfs.Snapshot) {
	if c.o.Legacy {
		c.collectLegacy(ch, snap)
	}
	if c.o.PerPID {
		for _, p := range c.tr.PIDs() {
			pid := strconv.Itoa(p.PID)
			ch <- prometheus.MustNewConstMetric(descPIDMigrations, prometheus.CounterValue, float64(p.Migrations), pid, p.Comm)
			if p.Role == migration.RoleRemote || p.Role == migration.RoleGuest {
				ch <- prometheus.MustNewConstMetric(descPIDNode, prometheus.GaugeValue, float64(p.Node), pid, p.Comm, string(p.Role))
			}
		}
	}
}

func (c *Collector) collectLegacy(ch chan<- prometheus.Metric, snap procfs.Snapshot) {
	avail := 0.0
	if snap.Available {
		avail = 1
	}
	ch <- prometheus.MustNewConstMetric(descProcAvail, prometheus.GaugeValue, avail)

	guests, remote := c.tr.Guests(), c.tr.Remote()
	ch <- prometheus.MustNewConstMetric(descGuestsHeld, prometheus.GaugeValue, float64(len(guests)))
	ch <- prometheus.MustNewConstMetric(descRemoteProcs, prometheus.GaugeValue, float64(len(remote)))
	emitGrouped(ch, descGuestProc, guests)
	emitGrouped(ch, descRemoteProc, remote)
	for k, v := range c.tr.Totals() {
		ch <- prometheus.MustNewConstMetric(descMigrations, prometheus.CounterValue, float64(v), string(k.Direction), k.Comm)
	}
}

// emitGrouped emits one gauge per (comm, node) with the number of processes.
func emitGrouped(ch chan<- prometheus.Metric, d *prometheus.Desc, ps []migration.Placement) {
	type key struct {
		comm string
		node int
	}
	n := map[key]int{}
	for _, p := range ps {
		n[key{p.Comm, p.Node}]++
	}
	for k, v := range n {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, float64(v), k.comm, strconv.Itoa(k.node))
	}
}

func (c *Collector) collectHost(ch chan<- prometheus.Metric) {
	emit := func(d *prometheus.Desc, v float64, err error) {
		if err != nil {
			c.errs[nameHost]++
			return
		}
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	n, err := c.host.ModuleLoads()
	emit(descModuleLoads, float64(n), err)
	n, err = c.host.ExecmemErrors()
	emit(descExecmem, float64(n), err)
	n, err = c.host.KdumpDirs()
	emit(descKdump, float64(n), err)
	active, err := c.host.ServiceActive()
	v := 0.0
	if active {
		v = 1
	}
	emit(descServiceUp, v, err)
}
