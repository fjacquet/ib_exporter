// Package replay serves infiniband-diags output from fixture files, so the demo
// stack and tests run without InfiniBand hardware. See README.md for the layout.
package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
)

// Runner implements diags.Runner from Dir/<fabric>/<tool>__<args>.txt. It executes nothing.
type Runner struct{ Dir string }

var _ diags.Runner = Runner{}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// Run returns the fixture for the tool and args (without the -C/-P ExecRunner appends).
func (r Runner) Run(_ context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	if f.Name == "" || f.Name == "." || f.Name == ".." {
		return nil, fmt.Errorf("replay: invalid fabric name %q", f.Name)
	}
	file := unsafe.ReplaceAllString(tool+"__"+strings.Join(args, "_"), "-") + ".txt"
	path := filepath.Join(r.Dir, unsafe.ReplaceAllString(f.Name, "-"), file)
	if rel, err := filepath.Rel(r.Dir, path); err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("replay: fixture path %s escapes %s", path, r.Dir)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("replay: no fixture %s: %w", path, err)
	}
	return b, nil
}
