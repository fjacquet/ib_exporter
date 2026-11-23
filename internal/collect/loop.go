// Package collect runs the collection loop: each cycle it polls every fabric
// through a diags.Runner, reads the local HCA from sysfs, and swaps an immutable
// ib.Snapshot into the store. It lives outside package ib because diags imports ib.
package collect

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
	"github.com/fjacquet/ib_exporter/internal/ib/sysfs"
)

// Option configures a Loop.
type Option func(*Loop)

// WithClock sets the clock used for scheduling and timestamps (default time.Now).
func WithClock(now func() time.Time) Option { return func(l *Loop) { l.now = now } }

// WithHCARoot sets the sysfs InfiniBand root (default /sys/class/infiniband).
func WithHCARoot(root string) Option { return func(l *Loop) { l.hcaRoot = root } }

// Loop collects all configured sources into snapshots. Run or Once must not be
// called concurrently: per-fabric state is owned by one cycle at a time.
type Loop struct {
	cfg     *config.Config
	r       diags.Runner
	store   *ib.SnapshotStore
	now     func() time.Time
	hcaRoot string
	cycle   int
	fabrics []*fabricState // parallel to cfg.Fabrics
	sysfs   readerStat
}

// NewCollectionLoop builds a loop over cfg; cfg must have passed Validate.
func NewCollectionLoop(cfg *config.Config, r diags.Runner, store *ib.SnapshotStore, opts ...Option) *Loop {
	l := &Loop{cfg: cfg, r: limitRunner{r: r, sem: make(chan struct{}, cfg.Collection.MaxConcurrent)},
		store: store, now: time.Now, hcaRoot: sysfs.DefaultRoot}
	for _, f := range cfg.Fabrics {
		l.fabrics = append(l.fabrics, newFabricState(f))
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Run runs a cycle immediately, then every collection.interval until ctx is cancelled.
func (l *Loop) Run(ctx context.Context) {
	t := time.NewTicker(l.cfg.Collection.Interval)
	defer t.Stop()
	for {
		if err := l.Once(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("collection cycle", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Once runs one cycle and swaps the snapshot, even when sources failed. It returns
// an error only when every fabric is down and the local HCA (if enabled) failed too.
func (l *Loop) Once(ctx context.Context) error {
	now := l.now()
	optional := l.cycle%l.cfg.Collection.OptionalEvery == 0
	l.cycle++

	snaps := make([]*ib.FabricSnapshot, len(l.fabrics))
	// All fabrics run at once; limitRunner bounds in-flight execs. Not WithContext:
	// one fabric failing must not cancel the others.
	var g errgroup.Group
	for i, fs := range l.fabrics {
		g.Go(func() error {
			snaps[i] = l.collectFabric(ctx, fs, now, optional)
			return nil
		})
	}
	_ = g.Wait() // goroutines always return nil
	// A cancelled cycle saw killed execs, not a down fabric: keep the last good snapshot.
	if err := ctx.Err(); err != nil {
		return err
	}

	snap := &ib.Snapshot{Fabrics: make(map[string]*ib.FabricSnapshot, len(snaps)), BuiltAt: now}
	up := false
	for _, fs := range snaps {
		snap.Fabrics[fs.Fabric] = fs
		up = up || fs.Up
	}
	if l.cfg.Sources.LocalHCA {
		snap.HCA, snap.HCAOK = l.readHCA(now)
		snap.HCAErr = l.sysfs.lastErr
	}
	l.store.Swap(snap)
	if !up && !snap.HCAOK && (len(snaps) > 0 || l.cfg.Sources.LocalHCA) {
		return errors.New("collect: every fabric is down and no local HCA was read")
	}
	return nil
}

// readHCA reads sysfs; on error the HCA samples are empty, only meta samples remain.
func (l *Loop) readHCA(now time.Time) ([]ib.Sample, bool) {
	start := time.Now()
	ss, err := sysfs.ReadHCA(l.hcaRoot, l.fabricFor)
	l.sysfs.cycDur, l.sysfs.cycRan = time.Since(start), true
	l.sysfs.record(err)
	if err != nil {
		ss = nil
	}
	l.sysfs.finish(now, "", "sysfs")
	return append(ss, l.sysfs.samples("", "sysfs")...), err == nil
}

// fabricFor returns the first configured fabric bound to the local CA/port, else "".
func (l *Loop) fabricFor(dev string, port int) string {
	for _, f := range l.cfg.Fabrics {
		if f.CA == dev && f.Port == port {
			return f.Name
		}
	}
	return ""
}

// limitRunner bounds in-flight execs across all fabrics and readers to cap(sem):
// collection.maxConcurrent limits PMA/SMP load on the fabric, not goroutines.
type limitRunner struct {
	r   diags.Runner
	sem chan struct{}
}

func (lr limitRunner) Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	select {
	case lr.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-lr.sem }()
	return lr.r.Run(ctx, f, tool, args...)
}
