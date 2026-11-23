// Package diags runs infiniband-diags tools and parses their text output.
package diags

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/config"
)

// Runner executes an infiniband-diags tool against a fabric's CA/port.
type Runner interface {
	Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error)
}

// ExecRunner runs tools through os/exec, appending -C <CA> -P <Port>.
type ExecRunner struct {
	// Trace, when set, receives every command line with its stdout and error.
	Trace func(cmd string, out []byte, err error)
}

// Run implements Runner. It returns stdout only; stderr is folded into the error.
func (r *ExecRunner) Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	full := append(append([]string{}, args...), "-C", f.CA, "-P", strconv.Itoa(f.Port))
	cmd := exec.CommandContext(ctx, tool, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("%s: %w: %s", tool, err, strings.TrimSpace(stderr.String()))
	}
	if r.Trace != nil {
		r.Trace(tool+" "+strings.Join(full, " "), stdout.Bytes(), err)
	}
	return stdout.Bytes(), err
}
