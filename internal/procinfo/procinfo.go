// SPDX-License-Identifier: GPL-2.0-only

// Package procinfo reads process identity from /proc/<pid>.
package procinfo

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
)

// Lookup reads comm and start time of pid below root (normally /proc).
func Lookup(root string) migration.Lookup {
	return func(pid int) (migration.ProcInfo, bool) {
		dir := filepath.Join(root, strconv.Itoa(pid))
		comm, err := os.ReadFile(filepath.Join(dir, "comm"))
		if err != nil {
			return migration.ProcInfo{}, false
		}
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			return migration.ProcInfo{}, false
		}
		start, ok := parseStat(stat)
		if !ok {
			return migration.ProcInfo{}, false
		}
		return migration.ProcInfo{Comm: strings.TrimSpace(string(comm)), Start: start}, true
	}
}

// parseStat extracts the start time (field 22) from the content of /proc/<pid>/stat.
// The comm field (2) may contain spaces and parentheses, so fields are counted after
// the last closing parenthesis, where field 3 (state) comes first.
func parseStat(b []byte) (uint64, bool) {
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(string(b[i+1:]))
	const startIdx = 22 - 3
	if len(f) <= startIdx {
		return 0, false
	}
	v, err := strconv.ParseUint(f[startIdx], 10, 64)
	return v, err == nil
}
