// Command ib_exporter exports InfiniBand fabric metrics to Prometheus and OTLP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"

	"github.com/fjacquet/ib_exporter/internal/collect"
	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/server"
	"github.com/fjacquet/ib_exporter/internal/telemetry"
)

// version is set at build time with -X main.version.
var version = "dev"

type options struct {
	configPath         string
	debug, once, trace bool
}

func main() {
	var o options
	cmd := &cobra.Command{
		Use:           "ib_exporter",
		Short:         "InfiniBand fabric exporter for Prometheus and OTLP",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(*cobra.Command, []string) error { return run(o) },
	}
	cmd.Flags().StringVar(&o.configPath, "config", "config.yaml", "path to the configuration file")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "debug logging; with --once, print every sample")
	cmd.Flags().BoolVar(&o.once, "once", false, "run one collection cycle, print and exit (no HTTP server)")
	cmd.Flags().BoolVar(&o.trace, "trace", false, "log every infiniband-diags command with its stdout")
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	level := slog.LevelInfo
	if o.debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	holder, err := config.NewHolder(o.configPath)
	if err != nil {
		return err
	}
	cfg := holder.Get()
	store := &ib.SnapshotStore{}
	newLoop := func(c *config.Config) *collect.Loop {
		return collect.NewCollectionLoop(c, newRunner(c, o.trace), store)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if o.once {
		err := newLoop(cfg).Once(ctx)
		if o.debug {
			printSamples(os.Stdout, store.Load())
		}
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(ib.NewSnapshotCollector(store), ib.NewBuildInfoCollector(version, runtime.Version()))
	mux := http.NewServeMux()
	server.RegisterEndpoints(mux, cfg, store, reg)
	srv := &http.Server{Addr: ":" + strconv.Itoa(cfg.Server.Port), Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.ListenAndServe() }() // listening before the first cycle

	shutdownOTLP, err := telemetry.Setup(ctx, store)
	if err != nil {
		slog.Warn("otlp disabled", "err", err)
	}

	sup := &supervisor{start: func(ctx context.Context, c *config.Config) { newLoop(c).Run(ctx) }}
	sup.restart(ctx, cfg)
	if err := holder.Watch(ctx, 500*time.Millisecond, func(old, cur *config.Config) {
		if old.Server != cur.Server {
			slog.Warn("server.port/server.uri changes need a restart; still serving the old values")
		}
		sup.restart(ctx, cur)
	}); err != nil {
		slog.Warn("config reload disabled", "err", err)
	}
	slog.Info("ib_exporter started", "version", version, "port", cfg.Server.Port)

	select {
	case <-ctx.Done():
	case err := <-srvErr:
		stop()
		sup.stop()
		return err
	}
	sc, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sup.stop()
	err = errors.Join(srv.Shutdown(sc), shutdownOTLP(sc))
	return err
}

// supervisor runs one collection loop at a time; restart replaces it with a
// loop over a new config on the same store.
type supervisor struct {
	start       func(context.Context, *config.Config)
	stopTimeout time.Duration // bound on waiting for a cancelled loop; 0 means 15s
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
}

func (s *supervisor) restart(parent context.Context, c *config.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	s.cancel, s.done = cancel, done
	go func() {
		defer close(done)
		s.start(ctx, c)
	}()
}

func (s *supervisor) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

// stopLocked cancels the running loop and waits for it, so two do not overlap, but
// only up to stopTimeout: a stuck loop must not block reload or shutdown. Its
// cancelled cycle never swaps a snapshot, so an overlap cannot publish stale data.
func (s *supervisor) stopLocked() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	d := s.stopTimeout
	if d == 0 {
		d = 15 * time.Second
	}
	select {
	case <-s.done:
	case <-time.After(d):
		slog.Warn("collection loop did not stop in time; continuing without it", "timeout", d)
	}
}
