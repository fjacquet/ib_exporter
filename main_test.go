package main

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
)

func TestOnceDebug_PrintsSortedSamples(t *testing.T) {
	snap := &ib.Snapshot{
		Fabrics: map[string]*ib.FabricSnapshot{
			"b": {Samples: []ib.Sample{{Name: "ib_up", Labels: []ib.Label{{Key: "fabric", Value: "b"}}, Value: 1}}},
			"a": {Samples: []ib.Sample{
				{Name: "ib_port_rcv_data_total", Labels: []ib.Label{{Key: "fabric", Value: "a"}, {Key: "port", Value: "1"}}, Value: 12.5, Counter: true},
				{Name: "ib_up", Labels: []ib.Label{{Key: "fabric", Value: "a"}}, Value: 0},
			}},
		},
		HCA: []ib.Sample{{Name: "ib_hca_info", Value: 1}},
	}
	var b bytes.Buffer
	printSamples(&b, snap)
	assert.Equal(t, "ib_hca_info 1\n"+
		"ib_port_rcv_data_total{fabric=\"a\",port=\"1\"} 12.5\n"+
		"ib_up{fabric=\"a\"} 0\n"+
		"ib_up{fabric=\"b\"} 1\n", b.String())
}

func TestSupervisor_RestartReplacesLoop(t *testing.T) {
	var starts, running atomic.Int32
	s := &supervisor{start: func(ctx context.Context, _ *config.Config) {
		starts.Add(1)
		running.Add(1)
		defer running.Add(-1)
		<-ctx.Done()
	}}
	ctx := context.Background()
	s.restart(ctx, &config.Config{})
	s.restart(ctx, &config.Config{})
	assert.Eventually(t, func() bool { return starts.Load() == 2 && running.Load() == 1 }, 2e9, 5e6)
	s.stop()
	assert.Zero(t, running.Load())
}

func TestSupervisor_StopIsBounded(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	s := &supervisor{stopTimeout: 50 * time.Millisecond, start: func(context.Context, *config.Config) { <-block }}
	s.restart(context.Background(), &config.Config{})
	start := time.Now()
	s.stop()
	assert.Less(t, time.Since(start), 2*time.Second, "a loop that ignores cancellation must not block shutdown")
}

func TestNewRunner_CommandTimeout(t *testing.T) {
	r := newRunner(&config.Config{Collection: config.Collection{CommandTimeout: 7 * time.Second}}, false)
	assert.Equal(t, 7*time.Second, r.(*diags.ExecRunner).Timeout)
}
