package diags

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/config"
)

func TestExecRunner_AppendsCAAndPort(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\"\necho oops >&2\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "faketool"), []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var traced string
	r := &ExecRunner{Trace: func(cmd string, _ []byte, _ error) { traced = cmd }}
	out, err := r.Run(context.Background(), config.Fabric{CA: "mlx5_3", Port: 2}, "faketool", "-x", "5")
	require.NoError(t, err)
	require.Equal(t, "-x 5 -C mlx5_3 -P 2\n", string(out)) // stdout only
	require.Contains(t, traced, "faketool")
}

func TestExecRunner_ErrorCarriesStderr(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "failtool"), []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o700))
	t.Setenv("PATH", dir)
	_, err := (&ExecRunner{}).Run(context.Background(), config.Fabric{CA: "a", Port: 1}, "failtool")
	require.ErrorContains(t, err, "boom")
}
