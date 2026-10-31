// Command mattx_exporter serves Prometheus metrics for the MattX kernel module.
package main

import (
	"fmt"
	"io"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mattx_exporter:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error { return fmt.Errorf("not implemented") }
