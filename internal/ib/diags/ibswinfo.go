package diags

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// SwitchHealth is the tolerant parse of `ibswinfo -d lid-<LID>` text. The real format is
// UNVERIFIED (see testdata/ibswinfo/README.md). Maps are keyed by the lowercased,
// space-stripped line key (e.g. "fan1", "psu1"). Zero UptimeSeconds means absent.
type SwitchHealth struct {
	Temperatures  map[string]float64 // sensor -> degrees C
	FansRPM       map[string]float64
	PSUWatts      map[string]float64
	PSUStatus     map[string]string
	UptimeSeconds float64
	Hardware      map[string]string // part_number, serial_number, firmware_version, psid
}

var (
	swNum    = regexp.MustCompile(`^-?\d+(?:\.\d+)?`)
	swWatts  = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s*W\b`)
	swRPM    = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)\s*(?:rpm)?\s*$`)
	swUptime = regexp.MustCompile(`(?i)(\d+)\s*(day|d|hour|h|min|m|sec|s)[a-z]*`)
	swHW     = map[string]string{"partnumber": "part_number", "serialnumber": "serial_number",
		"firmwareversion": "firmware_version", "psid": "psid"}
)

// ParseIbswinfo parses ibswinfo text ("key : value" lines). N/A and non-numeric readings are
// dropped; input with nothing recognised is an error, never zeros.
func ParseIbswinfo(out []byte) (SwitchHealth, error) {
	h := SwitchHealth{Temperatures: map[string]float64{}, FansRPM: map[string]float64{},
		PSUWatts: map[string]float64{}, PSUStatus: map[string]string{}, Hardware: map[string]string{}}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(line, ":")
		v = strings.TrimSpace(v)
		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(k), " ", ""))
		if !ok || key == "" || v == "" || strings.EqualFold(v, "n/a") {
			continue
		}
		switch {
		case swHW[key] != "":
			h.Hardware[swHW[key]] = v
		case key == "uptime":
			h.UptimeSeconds = parseUptime(v)
		case strings.Contains(key, "temp"):
			if n := swNum.FindString(v); n != "" {
				h.Temperatures[key], _ = strconv.ParseFloat(n, 64)
			}
		case strings.HasPrefix(key, "fan"):
			if m := swRPM.FindStringSubmatch(v); m != nil {
				h.FansRPM[key], _ = strconv.ParseFloat(m[1], 64)
			}
		case strings.HasPrefix(key, "psu"):
			psu := strings.TrimSuffix(strings.TrimSuffix(key, "status"), "power")
			if m := swWatts.FindStringSubmatch(v); m != nil {
				h.PSUWatts[psu], _ = strconv.ParseFloat(m[1], 64)
			} else if strings.HasSuffix(key, "status") || swNum.FindString(v) == "" {
				h.PSUStatus[psu] = v
			}
		}
	}
	if len(h.Temperatures)+len(h.FansRPM)+len(h.PSUWatts)+len(h.PSUStatus)+len(h.Hardware) == 0 && h.UptimeSeconds == 0 {
		return SwitchHealth{}, errors.New("no ibswinfo fields recognised: unknown format")
	}
	return h, nil
}

// parseUptime accepts "12d 3h 4m 5s", "3 days 4 hours" or bare seconds; 0 when unparsable.
func parseUptime(v string) float64 {
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return n
	}
	mult := map[byte]float64{'d': 86400, 'h': 3600, 'm': 60, 's': 1}
	var total float64
	for _, m := range swUptime.FindAllStringSubmatch(v, -1) {
		n, _ := strconv.ParseFloat(m[1], 64)
		total += n * mult[strings.ToLower(m[2])[0]]
	}
	return total
}

// SwitchHealthSamples builds gauges for one switch. Series carry fabric,node_type,guid,node_name
// plus a per-metric extra (sensor, fan, psu, psu+state, or hardware fields) so each name has one key set.
func SwitchHealthSamples(fabric string, node ib.Node, h SwitchHealth) []ib.Sample {
	var out []ib.Sample
	base := func(extra ...ib.Label) []ib.Label {
		return append([]ib.Label{{Key: ib.LabelFabric, Value: fabric}, {Key: "node_type", Value: string(node.Type)},
			{Key: "guid", Value: node.GUID}, {Key: "node_name", Value: node.Name}}, extra...)
	}
	each := func(m map[string]float64, name, lbl string) {
		for _, k := range sortedKeys(m) {
			out = append(out, ib.Sample{Name: name, Labels: base(ib.Label{Key: lbl, Value: k}), Value: m[k]})
		}
	}
	each(h.Temperatures, "ib_switch_temperature_celsius", "sensor")
	each(h.FansRPM, "ib_switch_fan_rpm", "fan")
	each(h.PSUWatts, "ib_switch_psu_watts", "psu")
	for _, k := range sortedKeys(h.PSUStatus) {
		out = append(out, ib.Sample{Name: "ib_switch_psu_status_info",
			Labels: base(ib.Label{Key: "psu", Value: k}, ib.Label{Key: "state", Value: h.PSUStatus[k]}), Value: 1})
	}
	if h.UptimeSeconds > 0 {
		out = append(out, ib.Sample{Name: "ib_switch_uptime_seconds", Labels: base(), Value: h.UptimeSeconds})
	}
	if len(h.Hardware) > 0 {
		out = append(out, ib.Sample{Name: "ib_switch_hardware_info", Value: 1, Labels: base(
			ib.Label{Key: "part_number", Value: h.Hardware["part_number"]}, ib.Label{Key: "serial_number", Value: h.Hardware["serial_number"]},
			ib.Label{Key: "firmware_version", Value: h.Hardware["firmware_version"]}, ib.Label{Key: "psid", Value: h.Hardware["psid"]})})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
