// SPDX-License-Identifier: GPL-2.0-only

// Package dashboards tests the example Grafana and Perses dashboards and the Prometheus
// alert rules shipped with the exporter: they must parse, only use metrics the exporter
// can emit, and have well-formed queries.
package dashboards

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/KrisBuytaert/mattx_exporter/internal/collector"
)

const root = "../.."

var metricRe = regexp.MustCompile(`\bmattx_[a-z0-9_]+`)

// queries walks any JSON value and collects Prometheus queries: "expr" (Grafana
// targets) and "query" under a PrometheusTimeSeriesQuery spec (Perses).
func queries(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "expr" {
				*out = append(*out, s)
			}
			if k == "plugin" {
				if p, ok := val.(map[string]any); ok && p["kind"] == "PrometheusTimeSeriesQuery" {
					if sp, ok := p["spec"].(map[string]any); ok {
						if q, ok := sp["query"].(string); ok {
							*out = append(*out, q)
						}
					}
				}
			}
			queries(val, out)
		}
	case []any:
		for _, e := range x {
			queries(e, out)
		}
	}
}

func load(t *testing.T, rel string) (any, []string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	var qs []string
	queries(v, &qs)
	return v, qs
}

func balanced(s string) bool {
	stack := []rune{}
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	inStr := false
	for _, r := range s {
		switch {
		case r == '"':
			inStr = !inStr
		case inStr:
		case r == '(' || r == '[' || r == '{':
			stack = append(stack, r)
		case r == ')' || r == ']' || r == '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return len(stack) == 0 && !inStr
}

func allowed() map[string]bool {
	m := map[string]bool{}
	for _, n := range collector.MetricNames() {
		m[n] = true
	}
	return m
}

func checkQueries(t *testing.T, name string, qs []string) {
	t.Helper()
	if len(qs) < 10 {
		t.Errorf("%s: only %d queries found", name, len(qs))
	}
	ok := allowed()
	used := map[string]bool{}
	for _, q := range qs {
		if strings.TrimSpace(q) == "" {
			t.Errorf("%s: empty query", name)
		}
		if !balanced(q) {
			t.Errorf("%s: unbalanced query %q", name, q)
		}
		for _, m := range metricRe.FindAllString(q, -1) {
			used[m] = true
			if !ok[m] {
				t.Errorf("%s: unknown metric %s in %q", name, m, q)
			}
		}
	}
	for _, must := range []string{"mattx_node_load", "mattx_node_mem_free_bytes", "mattx_migrations_total",
		"mattx_guests_hosted", "mattx_remote_processes", "mattx_service_active", "mattx_kdump_dirs",
		"mattx_module_loads_since_boot", "mattx_execmem_errors_since_boot", "mattx_exporter_scrape_errors_total", "mattx_build_info"} {
		if !used[must] {
			t.Errorf("%s: no panel uses %s", name, must)
		}
	}
}

func TestGrafanaDashboard(t *testing.T) {
	v, qs := load(t, "dashboards/grafana/mattx-exporter.json")
	checkQueries(t, "grafana", qs)
	vars := map[string]bool{}
	for _, e := range v.(map[string]any)["templating"].(map[string]any)["list"].([]any) {
		vars[e.(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"datasource", "job", "instance"} {
		if !vars[n] {
			t.Errorf("grafana: missing variable %s", n)
		}
	}
}

func TestPersesDashboard(t *testing.T) {
	v, qs := load(t, "dashboards/perses/mattx-exporter.json")
	checkQueries(t, "perses", qs)
	d := v.(map[string]any)
	if d["kind"] != "Dashboard" {
		t.Errorf("perses: kind %v", d["kind"])
	}
	vars := map[string]bool{}
	for _, e := range d["spec"].(map[string]any)["variables"].([]any) {
		vars[e.(map[string]any)["spec"].(map[string]any)["name"].(string)] = true
	}
	for _, n := range []string{"job", "instance"} {
		if !vars[n] {
			t.Errorf("perses: missing variable %s", n)
		}
	}
}

func TestNoLabSpecifics(t *testing.T) {
	for _, f := range []string{"dashboards/grafana/mattx-exporter.json", "dashboards/perses/mattx-exporter.json", "alerts/mattx.rules.yml"} {
		data, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "certname") {
			t.Errorf("%s uses the puppet certname label", f)
		}
	}
}

func TestAlertRules(t *testing.T) {
	file := filepath.Join(root, "alerts/mattx.rules.yml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	ok := allowed()
	for _, m := range metricRe.FindAllString(string(data), -1) {
		if !ok[m] {
			t.Errorf("rules: unknown metric %s", m)
		}
	}
	path, err := exec.LookPath("promtool")
	if err != nil {
		if os.Getenv("REQUIRE_PROMTOOL") != "" {
			t.Fatal("REQUIRE_PROMTOOL is set but promtool is not installed")
		}
		t.Skip("promtool not installed")
	}
	if out, err := exec.Command(path, "check", "rules", file).CombinedOutput(); err != nil {
		t.Errorf("promtool check rules: %v\n%s", err, out)
	}
}
