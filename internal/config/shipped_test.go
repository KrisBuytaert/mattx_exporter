// SPDX-License-Identifier: GPL-2.0-only

package config

import (
	"os"
	"reflect"
	"testing"
)

// The default config shipped in the RPM must parse and equal the built-in defaults.
func TestShippedConfigEqualsDefaults(t *testing.T) {
	data, err := os.ReadFile("../../packaging/mattx_exporter.yml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Errorf("shipped config differs from defaults:\n got %+v\nwant %+v", got, Default())
	}
	if got.PerPIDMetrics || got.ListenAddress != ":9105" {
		t.Errorf("per_pid_metrics must be false and listen :9105, got %v %q", got.PerPIDMetrics, got.ListenAddress)
	}
}
