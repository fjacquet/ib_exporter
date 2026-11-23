// Package diags runs infiniband-diags tools and parses their text output.
package diags

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fjacquet/ib_exporter/internal/config"
)

// Runner executes an infiniband-diags tool against a fabric's CA/port.
type Runner interface {
	Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error)
}

// diagsTools are the infiniband-diags tools that take -C <CA> -P <Port>. Other tools
// (ibswinfo, a third-party script) get their arguments unchanged.
var diagsTools = map[string]bool{
	"ibnetdiscover": true, "perfquery": true, "smpquery": true, "sminfo": true, "saquery": true,
	"ibstat": true, "ibaddr": true, "iblinkinfo": true, "ibqueryerrors": true,
}

// ExecRunner runs tools through os/exec, appending -C <CA> -P <Port> for infiniband-diags tools.
type ExecRunner struct {
	// Timeout bounds each command (0: none). A timeout is a reader error, not a cancellation.
	Timeout time.Duration
	// Trace, when set, receives every command line with its stdout and error.
	Trace func(cmd string, out []byte, err error)
}

// Run implements Runner. It returns stdout only; stderr is folded into the error.
func (r *ExecRunner) Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) {
	full := append([]string{}, args...)
	if diagsTools[filepath.Base(tool)] {
		full = append(full, "-C", f.CA, "-P", strconv.Itoa(f.Port))
	}
	cctx := ctx
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		cctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(cctx, tool, full...)
	cmd.WaitDelay = 2 * time.Second // a killed script's child may still hold stdout
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	switch {
	case err == nil:
	case ctx.Err() != nil: // shutdown or reload: report the cancellation itself
		err = fmt.Errorf("%s: %w", tool, ctx.Err())
	case cctx.Err() != nil:
		err = fmt.Errorf("%s: timed out after %s: %w", tool, r.Timeout, cctx.Err())
	default:
		err = fmt.Errorf("%s: %w: %s", tool, err, strings.TrimSpace(stderr.String()))
	}
	if r.Trace != nil {
		r.Trace(tool+" "+strings.Join(full, " "), stdout.Bytes(), err)
	}
	return stdout.Bytes(), err
}
