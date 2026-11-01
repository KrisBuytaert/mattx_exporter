// SPDX-License-Identifier: GPL-2.0-only

package procfs

import "os"

func readFile(p string) ([]byte, error) { return os.ReadFile(p) }
func writeFile(p, s string) error       { return os.WriteFile(p, []byte(s), 0o644) }
