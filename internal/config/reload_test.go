package config

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const validA = "sources: {localHCA: true}\ncollection: {maxConcurrent: 2}\n"
const validB = "sources: {localHCA: true}\ncollection: {maxConcurrent: 7}\n"

func TestHolder_ValidReloadSwaps(t *testing.T) {
	p := write(t, validA)
	h, err := NewHolder(p)
	require.NoError(t, err)
	require.Equal(t, 2, h.Get().Collection.MaxConcurrent)

	require.NoError(t, os.WriteFile(p, []byte(validB), 0o600))
	require.NoError(t, h.Reload())
	require.Equal(t, 7, h.Get().Collection.MaxConcurrent)
}

func TestHolder_InvalidReloadKeepsOld(t *testing.T) {
	p := write(t, validA)
	h, err := NewHolder(p)
	require.NoError(t, err)
	old := h.Get()

	for _, bad := range []string{"collection: {maxConcurent: 1}\n", "server: {port: 0}\nsources: {localHCA: true}\n", ":::"} {
		require.NoError(t, os.WriteFile(p, []byte(bad), 0o600))
		require.Error(t, h.Reload())
		require.Same(t, old, h.Get())
	}
}

func TestHolder_Watch_FileChangeNotifies(t *testing.T) {
	p := write(t, validA)
	h, err := NewHolder(p)
	require.NoError(t, err)
	var got atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, h.Watch(ctx, 20*time.Millisecond, func(old, cur *Config) {
		require.Equal(t, 2, old.Collection.MaxConcurrent)
		got.Store(int32(cur.Collection.MaxConcurrent))
	}))
	require.NoError(t, os.WriteFile(p, []byte(validB), 0o600))
	require.Eventually(t, func() bool { return got.Load() == 7 }, 3*time.Second, 10*time.Millisecond)
}
