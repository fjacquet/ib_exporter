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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "perfquery"), []byte(script), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var traced string
	r := &ExecRunner{Trace: func(cmd string, _ []byte, _ error) { traced = cmd }}
	out, err := r.Run(context.Background(), config.Fabric{CA: "mlx5_3", Port: 2}, "perfquery", "-x", "5")
	require.NoError(t, err)
	require.Equal(t, "-x 5 -C mlx5_3 -P 2\n", string(out)) // stdout only
	require.Contains(t, traced, "perfquery")
}

// ibswinfo is not an infiniband-diags tool: it gets its arguments unchanged, by name or by path.
func TestExecRunner_NonDiagsToolArgsUnchanged(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "ibswinfo")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\necho \"$@\"\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"ibswinfo", tool} {
		out, err := (&ExecRunner{}).Run(context.Background(), config.Fabric{CA: "mlx5_3", Port: 2}, name, "-d", "lid-5")
		require.NoError(t, err)
		require.Equal(t, "-d lid-5\n", string(out), name)
	}
}

func TestExecRunner_ErrorCarriesStderr(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "failtool"), []byte("#!/bin/sh\necho boom >&2\nexit 3\n"), 0o700))
	t.Setenv("PATH", dir)
	_, err := (&ExecRunner{}).Run(context.Background(), config.Fabric{CA: "a", Port: 1}, "failtool")
	require.ErrorContains(t, err, "boom")
}
