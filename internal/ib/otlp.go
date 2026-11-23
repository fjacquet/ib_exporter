package ib

import (
	"context"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// otlpBridge observes the snapshot through OTel observable instruments: a
// Float64ObservableCounter per counter name, a Float64ObservableGauge per gauge name.
//
// OTel instruments must exist before a collection, but names here are dynamic. The
// bridge creates instruments for every name in the snapshot at construction, and
// each collection re-scans for names it has no instrument for yet. The SDK holds
// its pipeline lock while running callbacks and registering a callback takes that
// lock, so a late name is registered by a goroutine that runs after the collection:
// a name that first appears in the snapshot is observed from the next collection on.
// Instruments are never removed; a name absent from the snapshot observes nothing.
type otlpBridge struct {
	store *SnapshotStore
	meter metric.Meter

	syncMu  sync.Mutex // serialises sync; never held by callbacks
	mu      sync.Mutex // guards known; never held across an SDK call
	known   map[string]bool
	pending atomic.Bool // a background sync is queued
}

// NewOTLPBridge registers observable instruments on meter for the store's snapshot.
func NewOTLPBridge(store *SnapshotStore, meter metric.Meter) error {
	b := &otlpBridge{store: store, meter: meter, known: map[string]bool{}}
	return b.sync(alwaysPresent...)
}

// alwaysPresent seeds the first registration: the SDK ignores a callback with no
// instruments, and only a registered callback can discover names, so a bridge
// built on an empty store would otherwise never export anything.
var alwaysPresent = []Sample{{Name: "ib_up"}, {Name: "ib_collect_errors_total", Counter: true}}

// sync creates instruments for names without one (seed first, then the snapshot)
// and registers one callback for them.
func (b *otlpBridge) sync(seed ...Sample) error {
	b.syncMu.Lock()
	defer b.syncMu.Unlock()
	insts := map[string]instrument{}
	var list []metric.Observable
	for _, s := range append(seed, exportable(b.store.Load())...) {
		if _, ok := insts[s.Name]; ok || b.isKnown(s.Name) {
			continue
		}
		inst, err := b.instrument(s)
		if err != nil {
			// Skip only this name, and never retry it: one bad name must not block the batch.
			otel.Handle(err)
			b.markKnown(s.Name)
			continue
		}
		insts[s.Name] = instrument{inst, s.Counter}
		list = append(list, inst)
	}
	if len(list) == 0 {
		return nil
	}
	if _, err := b.meter.RegisterCallback(b.observer(insts), list...); err != nil {
		return err
	}
	for n := range insts {
		b.markKnown(n)
	}
	return nil
}

func (b *otlpBridge) markKnown(name string) {
	b.mu.Lock()
	b.known[name] = true
	b.mu.Unlock()
}

// instrument records the kind: SDK instruments satisfy both the counter and gauge interfaces.
type instrument struct {
	obs     metric.Float64Observable
	counter bool
}

func (b *otlpBridge) instrument(s Sample) (metric.Float64Observable, error) {
	if s.Counter {
		return b.meter.Float64ObservableCounter(s.Name, metric.WithDescription(helpFor(s.Name)))
	}
	return b.meter.Float64ObservableGauge(s.Name, metric.WithDescription(helpFor(s.Name)))
}

func (b *otlpBridge) isKnown(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.known[name]
}

// observer returns the callback for one batch of instruments. A sample whose kind
// differs from its instrument's (the name changed kind since) is not observed.
func (b *otlpBridge) observer(insts map[string]instrument) metric.Callback {
	return func(_ context.Context, o metric.Observer) error {
		late := false
		for _, s := range exportable(b.store.Load()) {
			inst, ok := insts[s.Name]
			if !ok {
				late = late || !b.isKnown(s.Name)
				continue
			}
			if inst.counter != s.Counter {
				continue
			}
			o.ObserveFloat64(inst.obs, s.Value, metric.WithAttributes(attrs(s.Labels)...))
		}
		if late && b.pending.CompareAndSwap(false, true) {
			go func() {
				defer b.pending.Store(false)
				if err := b.sync(); err != nil {
					otel.Handle(err)
				}
			}()
		}
		return nil
	}
}

func attrs(ls []Label) []attribute.KeyValue {
	kv := make([]attribute.KeyValue, len(ls))
	for i, l := range ls {
		kv[i] = attribute.String(l.Key, l.Value)
	}
	return kv
}
