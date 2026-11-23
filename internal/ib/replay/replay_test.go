package replay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
)

var _ diags.Runner = (*Runner)(nil)

func TestReplayRunner_ReturnsFixture(t *testing.T) {
	dir := t.TempDir()
	fd := filepath.Join(dir, "prod")
	if err := os.MkdirAll(fd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fd, "perfquery__-x_-a_-l.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Runner{Dir: dir}
	out, err := r.Run(context.Background(), config.Fabric{Name: "prod", CA: "mlx5_0", Port: 1}, "perfquery", "-x", "-a", "-l")
	if err != nil || string(out) != "data" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestReplayRunner_MissingFixture(t *testing.T) {
	r := Runner{Dir: t.TempDir()}
	_, err := r.Run(context.Background(), config.Fabric{Name: "prod"}, "smpquery", "portinfo", "1/2")
	if err == nil || !strings.Contains(err.Error(), "replay: no fixture ") || !strings.Contains(err.Error(), "smpquery__portinfo_1-2.txt") {
		t.Fatalf("err=%v", err)
	}
}

func TestReplayRunner_FabricNameCannotEscapeDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "dir")
	if err := os.MkdirAll(filepath.Join(dir, "prod"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(parent, "perfquery__x.txt"), filepath.Join(dir, "perfquery__x.txt"), filepath.Join(parent, "perfquery__...txt")} {
		if err := os.WriteFile(p, []byte("SECRET"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r := Runner{Dir: dir}
	for _, c := range []struct {
		fabric string
		args   []string
	}{{"..", []string{"x"}}, {".", []string{"x"}}, {"../x", []string{"x"}}, {"", []string{"x"}}, {"prod", []string{"../.."}}, {"..", []string{"../.."}}} {
		out, err := r.Run(context.Background(), config.Fabric{Name: c.fabric}, "perfquery", c.args...)
		if err == nil || strings.Contains(string(out), "SECRET") {
			t.Errorf("fabric %q args %v: out=%q err=%v", c.fabric, c.args, out, err)
		}
	}
}
