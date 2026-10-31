package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPrintMetricsFromFixture(t *testing.T) {
	proc, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "proc", "normal"))
	cfg := writeConfig(t, "proc_path: "+proc+"\ncollectors:\n  host: false\nper_pid_metrics: true\n")
	var out bytes.Buffer
	if err := run([]string{"-config", cfg, "-print-metrics"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`mattx_build_info{`,
		`mattx_node_load{ip="192.168.100.11",node="377"} 12`,
		`mattx_node_up{ip="192.168.100.13",node="379"} 1`,
		`mattx_remote_processes 2`, // baseline snapshot: gauges show state, counters stay 0
		`mattx_guests_hosted 1`,
		`mattx_migration_proc_available 1`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out.String(), "mattx_module_loads_since_boot") {
		t.Error("host collector output present although disabled")
	}
}

func TestVersionFlag(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-version"}, &out); err != nil || !strings.Contains(out.String(), "mattx_exporter") {
		t.Errorf("-version: %q, %v", out.String(), err)
	}
}

func TestBadConfigIsAnError(t *testing.T) {
	cfg := writeConfig(t, "bogus_key: 1\n")
	if err := run([]string{"-config", cfg, "-print-metrics"}, &bytes.Buffer{}); err == nil {
		t.Error("no error for an invalid config")
	}
}
