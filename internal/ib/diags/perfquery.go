package diags

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// PortCounters holds the numeric fields of one perfquery block (one port).
// GUID is set only when the tool addressed the port by GUID; otherwise the caller
// resolves the node. Field names are the libibmad labels (e.g. "PortXmitData").
type PortCounters struct {
	GUID   string
	Port   int
	Fields map[string]uint64
}

// Block header (perfquery.c): "# <name> counters: <portid> port <n> (CapMask: ...)".
var (
	pqHeader = regexp.MustCompile(`^#.*counters:\s*(.*?)\s+port\s+(\d+)`)
	pqGUID   = regexp.MustCompile(`(?i)\bGuid\s+(0x[0-9a-f]+)`)
)

// ParsePerfquery parses perfquery output (basic, -x, -T, -E, -D, congestion; one
// or many port blocks, as printed with -a -l). Value lines look like
// "Name:....<dots to col 32>value" (libibmad _mad_dump_field). N/A, empty or
// non-numeric values are left out of Fields.
func ParsePerfquery(out []byte) ([]PortCounters, error) {
	var res []PortCounters
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	nonEmpty := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		nonEmpty = true
		if m := pqHeader.FindStringSubmatch(line); m != nil {
			port, _ := strconv.Atoi(m[2])
			pc := PortCounters{Port: port, Fields: map[string]uint64{}}
			if g := pqGUID.FindStringSubmatch(m[1]); g != nil {
				pc.GUID = strings.ToLower(g[1])
			}
			res = append(res, pc)
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok || len(res) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		v, err := strconv.ParseUint(strings.TrimLeft(strings.TrimSpace(rest), ". "), 0, 64)
		if err != nil {
			continue
		}
		res[len(res)-1].Fields[strings.TrimSpace(name)] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan perfquery: %w", err)
	}
	if nonEmpty && len(res) == 0 {
		return nil, fmt.Errorf("no perfquery counter blocks found: unknown format")
	}
	return res, nil
}

// MergeCounters merges ext into basic per (GUID, port); ext values win (64-bit
// data/packets beat the 32-bit ones). Ext-only blocks are appended.
func MergeCounters(basic, ext []PortCounters) []PortCounters {
	type key struct {
		guid string
		port int
	}
	out := make([]PortCounters, 0, len(basic))
	idx := map[key]int{}
	add := func(pc PortCounters) {
		k := key{pc.GUID, pc.Port}
		i, ok := idx[k]
		if !ok {
			idx[k] = len(out)
			out = append(out, PortCounters{GUID: pc.GUID, Port: pc.Port, Fields: map[string]uint64{}})
			i = len(out) - 1
		}
		for f, v := range pc.Fields {
			out[i].Fields[f] = v
		}
	}
	for _, pc := range basic {
		add(pc)
	}
	for _, pc := range ext {
		add(pc)
	}
	return out
}

type metricDef struct {
	name  string
	mult  float64 // 4 for PMA data counters (units of 4 octets)
	lane  string  // value of the "lane" label; "" when not per lane
	lanes bool    // metric carries the "lane" label key
}

// perfFields maps perfquery field names to metrics. Unlisted fields are ignored.
var perfFields = buildPerfFields()

func buildPerfFields() map[string]metricDef {
	m := map[string]metricDef{
		"PortXmitData":                   {name: "ib_port_xmit_data_bytes_total", mult: 4},
		"PortRcvData":                    {name: "ib_port_rcv_data_bytes_total", mult: 4},
		"PortXmitPkts":                   {name: "ib_port_xmit_packets_total"},
		"PortRcvPkts":                    {name: "ib_port_rcv_packets_total"},
		"PortUnicastXmitPkts":            {name: "ib_port_unicast_xmit_packets_total"},
		"PortUnicastRcvPkts":             {name: "ib_port_unicast_rcv_packets_total"},
		"PortMulticastXmitPkts":          {name: "ib_port_multicast_xmit_packets_total"},
		"PortMulticastRcvPkts":           {name: "ib_port_multicast_rcv_packets_total"},
		"SymbolErrorCounter":             {name: "ib_port_symbol_error_total"},
		"LinkErrorRecoveryCounter":       {name: "ib_port_link_error_recovery_total"},
		"LinkDownedCounter":              {name: "ib_port_link_downed_total"},
		"PortRcvErrors":                  {name: "ib_port_rcv_errors_total"},
		"PortRcvRemotePhysicalErrors":    {name: "ib_port_rcv_remote_physical_errors_total"},
		"PortRcvSwitchRelayErrors":       {name: "ib_port_rcv_switch_relay_errors_total"},
		"PortXmitDiscards":               {name: "ib_port_xmit_discards_total"},
		"PortXmitConstraintErrors":       {name: "ib_port_xmit_constraint_errors_total"},
		"PortRcvConstraintErrors":        {name: "ib_port_rcv_constraint_errors_total"},
		"LocalLinkIntegrityErrors":       {name: "ib_port_local_link_integrity_errors_total"},
		"ExcessiveBufferOverrunErrors":   {name: "ib_port_excessive_buffer_overrun_errors_total"},
		"VL15Dropped":                    {name: "ib_port_vl15_dropped_total"},
		"QP1Dropped":                     {name: "ib_port_qp1_dropped_total"},
		"PortXmitWait":                   {name: "ib_port_xmit_wait_total"},
		"PortInactiveDiscards":           {name: "ib_port_xmit_inactive_discards_total"},
		"PortNeighborMTUDiscards":        {name: "ib_port_xmit_neighbor_mtu_discards_total"},
		"PortSwLifetimeLimitDiscards":    {name: "ib_port_xmit_sw_lifetime_limit_discards_total"},
		"PortSwHOQLifetimeLimitDiscards": {name: "ib_port_xmit_sw_hoq_lifetime_limit_discards_total"},
		"PortLocalPhysicalErrors":        {name: "ib_port_local_physical_errors_total"},
		"PortMalformedPktErrors":         {name: "ib_port_malformed_packet_errors_total"},
		"PortBufferOverrunErrors":        {name: "ib_port_buffer_overrun_errors_total"},
		"PortDLIDMappingErrors":          {name: "ib_port_dlid_mapping_errors_total"},
		"PortVLMappingErrors":            {name: "ib_port_vl_mapping_errors_total"},
		"PortLoopingErrors":              {name: "ib_port_looping_errors_total"},
		"PortPktRcvFECN":                 {name: "ib_port_rcv_fecn_total"},
		"PortPktRcvBECN":                 {name: "ib_port_rcv_becn_total"},
		"PortXmitTimeCong":               {name: "ib_port_xmit_time_cong_total"},
		// -T (PortExtendedSpeedsCounters); see docs/metrics.md.
		"SyncHeaderErrorCounter":       {name: "ib_port_sync_header_errors_total"},
		"UnknownBlockCounter":          {name: "ib_port_unknown_blocks_total"},
		"PortFECCorrectableBlockCtr":   {name: "ib_port_fec_corrected_blocks_total", lanes: true},
		"PortFECUncorrectableBlockCtr": {name: "ib_port_fec_uncorrectable_blocks_total", lanes: true},
		"PortFECCorrectedSymbolCtr":    {name: "ib_port_fec_corrected_symbols_total", lanes: true},
	}
	perLane := map[string]string{
		"ErrorDetectionCounterLane":    "ib_port_error_detection_total",
		"FECCorrectableBlockCtrLane":   "ib_port_fec_corrected_blocks_total",
		"FECUncorrectableBlockCtrLane": "ib_port_fec_uncorrectable_blocks_total",
		"FECCorrectableSymbolCtrLane":  "ib_port_fec_corrected_symbols_total",
	}
	for prefix, name := range perLane {
		for i := 0; i < 12; i++ {
			l := strconv.Itoa(i)
			m[prefix+l] = metricDef{name: name, lane: l, lanes: true}
		}
	}
	return m
}

// ToSamples converts one port's counters into cumulative samples. Labels are
// ib.PortLabels plus, for per-lane metrics, a trailing "lane" label (empty when
// the value is port-level).
func ToSamples(fabric string, node ib.Node, pc PortCounters) []ib.Sample {
	names := make([]string, 0, len(pc.Fields))
	for f := range pc.Fields {
		if _, ok := perfFields[f]; ok {
			names = append(names, f)
		}
	}
	sort.Strings(names)
	out := make([]ib.Sample, 0, len(names))
	for _, f := range names {
		d := perfFields[f]
		labels := ib.PortLabels(fabric, node, pc.Port)
		if d.lanes {
			labels = append(labels, ib.Label{Key: "lane", Value: d.lane})
		}
		mult := d.mult
		if mult == 0 {
			mult = 1
		}
		out = append(out, ib.Sample{Name: d.name, Labels: labels, Value: float64(pc.Fields[f]) * mult, Counter: true})
	}
	return out
}
