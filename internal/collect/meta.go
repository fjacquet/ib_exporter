package collect

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// readerStat is one reader's exporter-health state. errors accumulates for the
// process lifetime; the cyc* fields cover the current cycle only.
type readerStat struct {
	errors float64
	ran    bool
	dur    time.Duration
	lastOK time.Time

	cycRan bool
	cycOK  bool
	cycDur time.Duration
}

// record counts err as one error, or marks the reader successful this cycle.
func (s *readerStat) record(err error) {
	if errors.Is(err, context.Canceled) {
		return // shutdown or reload, not a reader failure; the cycle is discarded
	}
	if err != nil {
		s.errors++
		slog.Debug("reader failed", "err", err)
		return
	}
	s.cycOK = true
}

// add merges a worker's cycle accounting into s.
func (s *readerStat) add(o *readerStat) {
	s.errors += o.errors
	s.cycDur += o.cycDur
	s.cycRan = s.cycRan || o.cycRan
	s.cycOK = s.cycOK || o.cycOK
}

// finish folds the cycle into the lasting state and resets the cycle fields.
func (s *readerStat) finish(now time.Time) {
	if s.cycRan {
		s.ran, s.dur = true, s.cycDur
	}
	if s.cycOK {
		s.lastOK = now
	}
	s.cycRan, s.cycOK, s.cycDur = false, false, 0
}

// samples emits errors_total always; duration once the reader ran, last success once it succeeded.
func (s *readerStat) samples(fabric, reader string) []ib.Sample {
	lbl := func() []ib.Label {
		return []ib.Label{{Key: ib.LabelFabric, Value: fabric}, {Key: "reader", Value: reader}}
	}
	out := []ib.Sample{{Name: "ib_collect_errors_total", Labels: lbl(), Value: s.errors, Counter: true}}
	if s.ran {
		out = append(out, ib.Sample{Name: "ib_collect_duration_seconds", Labels: lbl(), Value: s.dur.Seconds()})
	}
	if !s.lastOK.IsZero() {
		out = append(out, ib.Sample{Name: "ib_collect_last_success_timestamp_seconds", Labels: lbl(),
			Value: float64(s.lastOK.UnixNano()) / 1e9})
	}
	return out
}
