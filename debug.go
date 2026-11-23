package main

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/config"
	"github.com/fjacquet/ib_exporter/internal/ib"
	"github.com/fjacquet/ib_exporter/internal/ib/diags"
	"github.com/fjacquet/ib_exporter/internal/ib/replay"
)

// printSamples writes every sample of snap, sorted, as `name{k="v",...} value`.
// It is the --once --debug output, meant to be diffed against docs/metrics.md.
func printSamples(w io.Writer, snap *ib.Snapshot) {
	var lines []string
	add := func(ss []ib.Sample) {
		for _, s := range ss {
			var b strings.Builder
			b.WriteString(s.Name)
			if len(s.Labels) > 0 {
				b.WriteByte('{')
				for i, l := range s.Labels {
					if i > 0 {
						b.WriteByte(',')
					}
					b.WriteString(l.Key + "=" + strconv.Quote(l.Value))
				}
				b.WriteByte('}')
			}
			lines = append(lines, b.String()+" "+strconv.FormatFloat(s.Value, 'g', -1, 64))
		}
	}
	for _, f := range snap.Fabrics {
		add(f.Samples)
	}
	add(snap.HCA)
	sort.Strings(lines)
	for _, l := range lines {
		_, _ = fmt.Fprintln(w, l)
	}
}

// newRunner returns the replay runner when cfg.Replay.Dir is set, else the real
// one. With trace, every command is logged with its stdout and error (no env).
func newRunner(cfg *config.Config, trace bool) diags.Runner {
	if cfg.Replay.Dir != "" {
		return replay.Runner{Dir: cfg.Replay.Dir}
	}
	r := &diags.ExecRunner{}
	if trace {
		r.Trace = func(cmd string, out []byte, err error) {
			slog.Info("trace", "cmd", cmd, "output", string(out), "err", err)
		}
	}
	return r
}
