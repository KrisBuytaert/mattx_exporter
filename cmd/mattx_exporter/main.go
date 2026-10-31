// Command mattx_exporter serves Prometheus metrics for the MattX kernel module.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"

	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/collector"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/config"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/host"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/migration"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/poller"
	"forgejo.lab.kribu.be/vox/mattx_exporter/internal/procinfo"
)

// Set at build time with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "unknown"
)

const defaultConfig = "/etc/mattx_exporter.yml"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mattx_exporter:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mattx_exporter", flag.ContinueOnError)
	cfgPath := fs.String("config", defaultConfig, "path of the YAML configuration file")
	showVersion := fs.Bool("version", false, "print the version and exit")
	printMetrics := fs.Bool("print-metrics", false, "poll once, print the metrics in text format and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "mattx_exporter %s (%s, %s)\n", version, commit, runtime.Version())
		return nil
	}

	explicit := false
	fs.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "config" })
	if _, err := os.Stat(*cfgPath); !explicit && errors.Is(err, os.ErrNotExist) {
		*cfgPath = "" // the default file is optional
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	tr := migration.New(migration.Options{
		Filter:        cfg.CommFilter(),
		MaxCommLabels: cfg.MaxCommLabels,
		MaxPIDs:       cfg.MaxPIDs,
		PIDRetention:  cfg.PIDRetention,
	}, procinfo.Lookup("/proc"))
	p := poller.New(cfg.ProcPath, cfg.PollInterval, tr)

	hostSrc := host.New(cfg.KdumpDir, cfg.JournalUnits, cfg.ServiceUnit,
		regexp.MustCompile(cfg.ModuleLoadRegex), regexp.MustCompile(cfg.ExecmemRegex))
	col := collector.New(collector.Options{
		Nodes: cfg.Collectors.Nodes, Migration: cfg.Collectors.Migration, Host: cfg.Collectors.Host,
		Legacy: cfg.LegacyMetrics, PerPID: cfg.PerPIDMetrics,
	}, p, tr, hostSrc, collector.Build{Version: version, Commit: commit, GoVersion: runtime.Version()})

	reg := prometheus.NewRegistry()
	reg.MustRegister(col, collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	if *printMetrics {
		p.Poll(time.Now())
		mfs, err := reg.Gather()
		if err != nil {
			return err
		}
		enc := expfmt.NewEncoder(stdout, expfmt.NewFormat(expfmt.TypeTextPlain))
		for _, mf := range mfs {
			if err := enc.Encode(mf); err != nil {
				return err
			}
		}
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go p.Run(ctx)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	slog.Info("listening", "address", cfg.ListenAddress, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
