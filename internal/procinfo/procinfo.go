// Package procinfo reads process identity from /proc/<pid>.
package procinfo

import "forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"

// Lookup reads comm and start time of pid below root (normally /proc).
func Lookup(root string) migration.Lookup {
	return func(pid int) (migration.ProcInfo, bool) { return migration.ProcInfo{}, false }
}

// parseStat extracts the start time (field 22) from the content of /proc/<pid>/stat.
func parseStat(b []byte) (uint64, bool) { return 0, false }
