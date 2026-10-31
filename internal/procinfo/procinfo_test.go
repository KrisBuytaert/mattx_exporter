package procinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStat(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want uint64
		ok   bool
	}{
		{"plain", "100 (migtest) S 1 100 100 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 100 18446744073709551615", 987654, true},
		{"comm with spaces and parens", "100 (my (odd) prog) S 1 100 100 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 555 1000 100", 555, true},
		{"truncated", "100 (migtest) S 1 100", 0, false},
		{"no parens", "garbage", 0, false},
		{"empty", "", 0, false},
		{"start not a number", "100 (x) S 1 100 100 0 -1 4 100 0 0 0 1 2 0 0 20 0 1 0 abc 1", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseStat([]byte(tc.in))
			if got != tc.want || ok != tc.ok {
				t.Errorf("parseStat = %d, %v; want %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestLookupFixture(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "42")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "comm"), []byte("migtest\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "stat"), []byte("42 (migtest) S 1 42 42 0 -1 4 1 0 0 0 1 2 0 0 20 0 1 0 777 1 1"), 0o644)
	lk := Lookup(root)
	info, ok := lk(42)
	if !ok || info.Comm != "migtest" || info.Start != 777 {
		t.Errorf("Lookup(42) = %+v, %v", info, ok)
	}
	if _, ok := lk(43); ok {
		t.Error("Lookup(43) ok for a missing pid")
	}
}

func TestLookupSelf(t *testing.T) {
	info, ok := Lookup("/proc")(os.Getpid())
	if !ok || info.Comm == "" || info.Start == 0 {
		t.Errorf("Lookup(self) = %+v, %v", info, ok)
	}
}
