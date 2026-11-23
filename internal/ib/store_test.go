package ib

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_LoadBeforeSwapReturnsEmpty(t *testing.T) {
	var s SnapshotStore
	got := s.Load()
	require.NotNil(t, got)
	assert.NotNil(t, got.Fabrics)
	assert.Empty(t, got.Fabrics)
	assert.Empty(t, got.HCA)
}

func TestStore_SwapIsAtomic(t *testing.T) {
	var s SnapshotStore
	var wg sync.WaitGroup
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				snap := s.Load()
				// A snapshot is either wholly old or wholly new.
				for _, f := range snap.Fabrics {
					assert.Equal(t, len(f.Nodes), len(f.Samples))
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		n := i % 5
		f := &FabricSnapshot{Fabric: "f", Up: true, Nodes: make([]Node, n), Samples: make([]Sample, n)}
		s.Swap(&Snapshot{Fabrics: map[string]*FabricSnapshot{"f": f}, BuiltAt: time.Now()})
	}
	wg.Wait()
	assert.Contains(t, s.Load().Fabrics, "f")
}

func TestPortLabels_OrderAndKeys(t *testing.T) {
	n := Node{GUID: "0x1", Name: "sw1", Type: NodeSwitch}
	assert.Equal(t, []Label{
		{"fabric", "fab"}, {"node_type", "switch"}, {"guid", "0x1"}, {"node_name", "sw1"}, {"port", "7"},
	}, PortLabels("fab", n, 7))
}
