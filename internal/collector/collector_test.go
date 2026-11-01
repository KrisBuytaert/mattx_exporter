// SPDX-License-Identifier: GPL-2.0-only

package collector

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"github.com/KrisBuytaert/mattx_exporter/internal/migration"
	"github.com/KrisBuytaert/mattx_exporter/internal/procfs"
)

type fakeSnap struct{ s procfs.Snapshot }

func (f *fakeSnap) Latest() procfs.Snapshot { return f.s }

type fakeHost struct {
	loads, execmem, kdump int
	active                bool
	err                   error
}

func (f fakeHost) ModuleLoads() (int, error)    { return f.loads, f.err }
func (f fakeHost) ExecmemErrors() (int, error)  { return f.execmem, f.err }
func (f fakeHost) KdumpDirs() (int, error)      { return f.kdump, f.err }
func (f fakeHost) ServiceActive() (bool, error) { return f.active, f.err }

var allOn = Options{Nodes: true, Migration: true, Host: true, Legacy: true, PerPID: true}

func nodeTable() procfs.Nodes {
	return procfs.Nodes{Nodes: []procfs.Node{
		{ID: 377, Local: true, IP: "192.168.100.11", CPULoad: 12, Affinity: 4000, MemFreeMB: 1500},
		{ID: 378, IP: "192.168.100.12", CPULoad: 3, Affinity: 2000, MemFreeMB: 1400},
		{ID: 379, IP: "192.168.100.13", CPULoad: 0, Affinity: 1000, MemFreeMB: 900},
	}}
}

// scenario returns a collector after one baseline snapshot and one snapshot in which two
// own pids went remote and one guest arrived (the state described by testdata/golden/full.prom).
func scenario(t *testing.T, o Options, h HostSource) (*Collector, *fakeSnap) {
	t.Helper()
	proc := map[int]migration.ProcInfo{
		1234: {Comm: "migtest", Start: 1}, 1235: {Comm: "migtest", Start: 2}, 2345: {Comm: "stress", Start: 3},
	}
	tr := migration.New(migration.Options{}, func(pid int) (migration.ProcInfo, bool) { p, ok := proc[pid]; return p, ok })
	now := time.Unix(1_000_000, 0)
	tr.Observe(now, procfs.Snapshot{Available: true})
	snap := procfs.Snapshot{
		Available: true, NodesOK: true, Nodes: nodeTable(),
		Guests: []procfs.Entry{{PID: 2345, Node: 378}},
		Remote: []procfs.Entry{{PID: 1234, Node: 378}, {PID: 1235, Node: 379}},
	}
	tr.Observe(now.Add(time.Second), snap)
	src := &fakeSnap{s: snap}
	return New(o, src, tr, h, Build{Version: "0.0.1", Commit: "abc123", GoVersion: "go1.26"}), src
}

func okHost() HostSource { return fakeHost{loads: 2, execmem: 1, kdump: 3, active: true} }

func metricNames(t *testing.T, c prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]*dto.MetricFamily{}
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

func TestGolden(t *testing.T) {
	c, _ := scenario(t, allOn, okHost())
	f, err := os.Open(filepath.Join("..", "..", "testdata", "golden", "full.prom"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := testutil.CollectAndCompare(c, f); err != nil {
		t.Error(err)
	}
}

func TestPromtoolCheck(t *testing.T) {
	path, err := exec.LookPath("promtool")
	if err != nil {
		if os.Getenv("REQUIRE_PROMTOOL") != "" {
			t.Fatal("REQUIRE_PROMTOOL is set but promtool is not installed")
		}
		t.Skip("promtool not installed")
	}
	c, _ := scenario(t, allOn, okHost())
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	enc := expfmt.NewEncoder(&sb, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			t.Fatal(err)
		}
	}
	if sb.Len() == 0 {
		t.Fatal("collector produced no metrics")
	}
	cmd := exec.Command(path, "check", "metrics")
	cmd.Stdin = strings.NewReader(sb.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("promtool check metrics: %v\n%s", err, out)
	}
}

// TestLegacyNamesStayCompatible compares against output recorded from the real
// mattx-migration-metrics.sh and mattx-node-metrics.sh: every metric they emitted keeps
// its name, type and label names, so dashboards keep working until the cut-over.
func TestLegacyNamesStayCompatible(t *testing.T) {
	c, _ := scenario(t, allOn, okHost())
	got := metricNames(t, c)
	for _, file := range []string{"legacy-migration.prom", "legacy-node.prom"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		parser := expfmt.NewTextParser(model.UTF8Validation)
		legacy, err := parser.TextToMetricFamilies(strings.NewReader(string(b)))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		for name, lmf := range legacy {
			gmf, ok := got[name]
			if !ok {
				t.Errorf("%s: legacy metric %s is missing", file, name)
				continue
			}
			if gmf.GetType() != lmf.GetType() {
				t.Errorf("%s: type = %v, legacy %v", name, gmf.GetType(), lmf.GetType())
			}
			for _, lm := range lmf.Metric {
				if !hasLabelNames(gmf, labelNames(lm)) {
					t.Errorf("%s: no sample with legacy label names %v", name, labelNames(lm))
				}
			}
		}
	}
}

func mustFamily(t *testing.T, got map[string]*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	mf, ok := got[name]
	if !ok {
		t.Fatalf("metric family %s missing", name)
	}
	return mf
}

func labelNames(m *dto.Metric) []string {
	var n []string
	for _, l := range m.Label {
		n = append(n, l.GetName())
	}
	sort.Strings(n)
	return n
}

func hasLabelNames(mf *dto.MetricFamily, want []string) bool {
	for _, m := range mf.Metric {
		if strings.Join(labelNames(m), ",") == strings.Join(want, ",") {
			return true
		}
	}
	return false
}

func TestOptionsSelectMetrics(t *testing.T) {
	legacyOnly := []string{
		"mattx_migration_proc_available", "mattx_guests_hosted", "mattx_remote_processes",
		"mattx_guest_process", "mattx_remote_process", "mattx_migrations_total",
	}
	perPID := []string{"mattx_pid_migrations_total", "mattx_pid_node"}
	nodes := []string{"mattx_node_load", "mattx_node_mem_free_bytes", "mattx_node_affinity", "mattx_node_up"}
	host := []string{"mattx_module_loads_since_boot", "mattx_execmem_errors_since_boot", "mattx_kdump_dirs", "mattx_service_active"}
	always := []string{"mattx_build_info", "mattx_exporter_scrape_errors_total"}

	tests := []struct {
		name    string
		o       Options
		present [][]string
		absent  [][]string
	}{
		{"all", allOn, [][]string{always, nodes, host, legacyOnly, perPID}, nil},
		{"legacy off", Options{Nodes: true, Migration: true, Host: true, PerPID: true},
			[][]string{always, nodes, host, perPID}, [][]string{legacyOnly}},
		{"per pid off", Options{Nodes: true, Migration: true, Host: true, Legacy: true},
			[][]string{always, nodes, host, legacyOnly}, [][]string{perPID}},
		{"nodes off", Options{Migration: true, Host: true, Legacy: true, PerPID: true},
			[][]string{always, host, legacyOnly, perPID}, [][]string{nodes}},
		{"host off", Options{Nodes: true, Migration: true, Legacy: true, PerPID: true},
			[][]string{always, nodes, legacyOnly, perPID}, [][]string{host}},
		{"migration off", Options{Nodes: true, Host: true, Legacy: true, PerPID: true},
			[][]string{always, nodes, host}, [][]string{legacyOnly, perPID}},
		{"everything off", Options{}, [][]string{always}, [][]string{nodes, host, legacyOnly, perPID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := scenario(t, tc.o, okHost())
			got := metricNames(t, c)
			for _, g := range tc.present {
				for _, n := range g {
					if _, ok := got[n]; !ok {
						t.Errorf("%s missing", n)
					}
				}
			}
			for _, g := range tc.absent {
				for _, n := range g {
					if _, ok := got[n]; ok {
						t.Errorf("%s present, want absent", n)
					}
				}
			}
		})
	}
}

func TestNodeUpDropsToZeroWhenNodeLeaves(t *testing.T) {
	c, src := scenario(t, Options{Nodes: true}, nil)
	metricNames(t, c) // first scrape: all three seen
	tbl := nodeTable()
	tbl.Nodes = tbl.Nodes[:2] // node 379 left the cluster table
	src.s.Nodes = tbl
	got := metricNames(t, c)
	var up = map[string]float64{}
	for _, m := range mustFamily(t, got, "mattx_node_up").Metric {
		up[m.Label[1].GetValue()] = m.Gauge.GetValue()
	}
	if up["379"] != 0 || up["378"] != 1 || up["377"] != 1 {
		t.Errorf("mattx_node_up = %v, want 377:1 378:1 379:0", up)
	}
	for _, m := range mustFamily(t, got, "mattx_node_load").Metric {
		if m.Label[1].GetValue() == "379" {
			t.Error("mattx_node_load still reported for the departed node")
		}
	}
}

func TestNodeMetricsAbsentWhenProcUnavailable(t *testing.T) {
	c, src := scenario(t, Options{Nodes: true, Migration: true, Legacy: true}, nil)
	src.s = procfs.Snapshot{}
	got := metricNames(t, c)
	if mf, ok := got["mattx_node_load"]; ok && len(mf.Metric) > 0 {
		t.Errorf("node samples with unavailable proc: %v", mf.Metric)
	}
	if v := mustFamily(t, got, "mattx_migration_proc_available").Metric[0].Gauge.GetValue(); v != 0 {
		t.Errorf("mattx_migration_proc_available = %v, want 0", v)
	}
}

func TestHostErrorCountsAsScrapeError(t *testing.T) {
	c, _ := scenario(t, Options{Host: true}, fakeHost{err: errors.New("journalctl failed")})
	got := metricNames(t, c)
	for _, n := range []string{"mattx_module_loads_since_boot", "mattx_execmem_errors_since_boot", "mattx_kdump_dirs", "mattx_service_active"} {
		if mf, ok := got[n]; ok && len(mf.Metric) > 0 {
			t.Errorf("%s reported despite a failing source", n)
		}
	}
	var hostErrs float64 = -1
	for _, m := range mustFamily(t, got, "mattx_exporter_scrape_errors_total").Metric {
		if m.Label[0].GetValue() == "host" {
			hostErrs = m.Counter.GetValue()
		}
	}
	if hostErrs < 1 {
		t.Errorf("scrape errors for host = %v, want >= 1", hostErrs)
	}
	// The counter is monotonic across scrapes.
	got = metricNames(t, c)
	for _, m := range mustFamily(t, got, "mattx_exporter_scrape_errors_total").Metric {
		if m.Label[0].GetValue() == "host" && m.Counter.GetValue() <= hostErrs {
			t.Errorf("scrape error counter did not grow on a second failing scrape: %v", m.Counter.GetValue())
		}
	}
}

func TestMalformedProcLinesAreCountedAsScrapeErrors(t *testing.T) {
	c, src := scenario(t, Options{Migration: true}, nil)
	src.s.Malformed = 3
	got := metricNames(t, c)
	for _, m := range mustFamily(t, got, "mattx_exporter_scrape_errors_total").Metric {
		if m.Label[0].GetValue() == "migration" && m.Counter.GetValue() != 3 {
			t.Errorf("migration scrape errors = %v, want 3", m.Counter.GetValue())
		}
	}
}

// MetricNames feeds the dashboard and alert tests, so it must match what is emitted.
func TestMetricNamesMatchEmitted(t *testing.T) {
	c, _ := scenario(t, allOn, okHost())
	got := metricNames(t, c)
	want := map[string]bool{}
	for _, n := range MetricNames() {
		want[n] = true
	}
	for n := range got {
		if !want[n] {
			t.Errorf("emitted but not in MetricNames: %s", n)
		}
	}
	for n := range want {
		if _, ok := got[n]; !ok && n != "mattx_migration_proc_available" {
			t.Errorf("in MetricNames but not emitted: %s", n)
		}
	}
}
