package ib

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// help holds exact-name help strings; helpPrefix covers families, longest match first.
var help = map[string]string{
	"ib_up":                       "1 when the fabric topology was read this cycle, else 0.",
	"ib_collect_duration_seconds": "Sum of the reader's exec durations in the last cycle, including waits for a free exec slot.",
	"ib_collect_errors_total":     "Reader failures since start (one per failing node or command group).",
	"ib_collect_last_success_timestamp_seconds": "Unix time of the reader's last successful cycle.",
	"ib_node_info":            "Fabric node from ibnetdiscover; constant 1.",
	"ib_link_info":            "Port-to-port link from ibnetdiscover; constant 1.",
	"ib_sm_info":              "Subnet manager identity from sminfo; constant 1.",
	"ib_hca_hw_counter_total": "Local HCA driver counter from sysfs hw_counters/.",
	"ib_hca_info":             "Local HCA identity from sysfs; constant 1.",
	"ib_hca_link_layer_info":  "Local HCA port link layer from sysfs; constant 1.",
}

var helpPrefix = []struct{ prefix, help string }{
	{"ib_port_", "InfiniBand port value from perfquery (counters) or smpquery portinfo (state)."},
	{"ib_sm_", "Subnet manager value from sminfo."},
	{"ib_switch_", "Switch health value from ibswinfo."},
	{"ib_hca_", "Local HCA port value from sysfs."},
}

// helpFor returns the help text for a metric name.
func helpFor(name string) string {
	if h, ok := help[name]; ok {
		return h
	}
	for _, p := range helpPrefix {
		if strings.HasPrefix(name, p.prefix) {
			return p.help
		}
	}
	return "InfiniBand exporter metric " + name + "."
}

// Legacy Prometheus charset: also valid OTel instrument names and attribute keys.
var (
	validName  = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	validLabel = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// exportable flattens snap (fabrics in name order, then HCA) and drops samples that
// would make an exposition invalid: bad names, labels or UTF-8, a label-key set or
// counter/gauge kind differing from the first sample of that name, duplicate series.
// Both exporters use it, so they always export the same series.
func exportable(snap *Snapshot) []Sample {
	type kind struct {
		keys    string
		counter bool
	}
	var all []Sample
	for _, f := range slices.Sorted(maps.Keys(snap.Fabrics)) {
		all = append(all, snap.Fabrics[f].Samples...)
	}
	all = append(all, snap.HCA...)

	kinds := map[string]kind{}
	seen := map[string]bool{}
	out := make([]Sample, 0, len(all))
	for _, s := range all {
		if !validSample(s) {
			continue
		}
		keys, vals := splitLabels(s.Labels)
		k := kind{strings.Join(keys, ","), s.Counter}
		if want, ok := kinds[s.Name]; ok && want != k {
			continue
		}
		kinds[s.Name] = k
		id := s.Name + "\xff" + strings.Join(vals, "\xff")
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, s)
	}
	return out
}

func validSample(s Sample) bool {
	if !validName.MatchString(s.Name) {
		return false
	}
	keys := map[string]bool{}
	for _, l := range s.Labels {
		if !validLabel.MatchString(l.Key) || strings.HasPrefix(l.Key, "__") || keys[l.Key] || !utf8.ValidString(l.Value) {
			return false
		}
		keys[l.Key] = true
	}
	return true
}

func splitLabels(ls []Label) (keys, vals []string) {
	keys, vals = make([]string, len(ls)), make([]string, len(ls))
	for i, l := range ls {
		keys[i], vals[i] = l.Key, l.Value
	}
	return keys, vals
}
