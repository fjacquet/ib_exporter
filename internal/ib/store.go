package ib

import "sync"

// SnapshotStore holds the current snapshot; the zero value is ready to use.
type SnapshotStore struct {
	mu   sync.RWMutex
	snap *Snapshot
}

// Swap replaces the current snapshot. The caller must not mutate s afterwards.
func (st *SnapshotStore) Swap(s *Snapshot) {
	st.mu.Lock()
	st.snap = s
	st.mu.Unlock()
}

// Load returns the current snapshot, or an empty one before the first Swap.
func (st *SnapshotStore) Load() *Snapshot {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if st.snap == nil {
		return &Snapshot{Fabrics: map[string]*FabricSnapshot{}}
	}
	return st.snap
}
