package diags

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// PortInfo holds the decoded fields of one `smpquery portinfo` block. Zero means
// absent (N/A or unparsable): every valid value of these fields is non-zero.
type PortInfo struct {
	State      int     // PortState: 1 Down, 2 Init, 3 Armed, 4 Active, 5 ActiveDefer
	PhysState  int     // PhysicalState: 1 Sleep .. 7 PhyTest
	WidthLanes int     // active link width in lanes (1X=1, 4X=4, ...)
	SpeedGbps  float64 // per-lane signalling speed, from LinkSpeedActive or LinkSpeedExtActive
	MTU        int     // active (neighbor) MTU in bytes
}

// State strings are libibmad dump.c mad_dump_portstate / mad_dump_physportstate.
var (
	portStates = map[string]int{"down": 1, "initialize": 2, "init": 2, "armed": 3, "active": 4, "activedefer": 5}
	physStates = map[string]int{"sleep": 1, "polling": 2, "disabled": 3, "portconfigurationtraining": 4,
		"linkup": 5, "linkerrorrecovery": 6, "phytest": 7}
	piWidth = regexp.MustCompile(`^(\d+)\s*[xX]\b`)
	piSpeed = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*Gbps`)
)

func enumValue(m map[string]int, v string) int {
	if n, ok := m[strings.ToLower(strings.ReplaceAll(v, " ", ""))]; ok {
		return n
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 7 {
		return 0
	}
	return n
}

// ParsePortInfo parses `smpquery portinfo` output ("Name:....value" lines).
// Unknown lines and N/A values are ignored; output with no recognised field is an error.
func ParsePortInfo(out []byte) (PortInfo, error) {
	var pi PortInfo
	var ext float64
	extended, found := false, false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v := strings.TrimLeft(strings.TrimSpace(rest), ". ")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "linkstate":
			pi.State, found = enumValue(portStates, v), true
		case "physlinkstate":
			pi.PhysState, found = enumValue(physStates, v), true
		case "linkwidthactive":
			if m := piWidth.FindStringSubmatch(v); m != nil {
				pi.WidthLanes, _ = strconv.Atoi(m[1])
			}
			found = true
		case "linkspeedactive":
			if m := piSpeed.FindStringSubmatch(v); m != nil {
				pi.SpeedGbps, _ = strconv.ParseFloat(m[1], 64)
			}
			extended = strings.HasPrefix(strings.ToLower(v), "extended")
			found = true
		case "linkspeedextactive":
			if m := piSpeed.FindStringSubmatch(v); m != nil {
				ext, _ = strconv.ParseFloat(m[1], 64)
			}
		case "neighbormtu":
			pi.MTU, _ = strconv.Atoi(v)
			found = true
		}
	}
	if err := sc.Err(); err != nil {
		return PortInfo{}, fmt.Errorf("scan portinfo: %w", err)
	}
	if !found {
		return PortInfo{}, errors.New("no portinfo fields found: unknown format")
	}
	if extended {
		pi.SpeedGbps = ext
	}
	return pi, nil
}

// dataGbps maps per-lane signalling speed (x100, integer key) to the nominal data rate per lane
// after line encoding (SDR..NDR).
var dataGbps = map[int]float64{250: 2, 500: 4, 1000: 8, 1406: 14, 2578: 25, 5312: 50, 10625: 100}

// PortInfoSamples converts one port's PortInfo into gauge samples. Width, speed and rate are
// only meaningful on an Active port and are omitted otherwise; absent fields give no sample.
func PortInfoSamples(fabric string, node ib.Node, port int, pi PortInfo) []ib.Sample {
	var out []ib.Sample
	add := func(name string, v float64) {
		out = append(out, ib.Sample{Name: name, Labels: ib.PortLabels(fabric, node, port), Value: v})
	}
	if pi.State > 0 {
		add("ib_port_state", float64(pi.State))
	}
	if pi.PhysState > 0 {
		add("ib_port_phys_state", float64(pi.PhysState))
	}
	if pi.State == 4 {
		if pi.WidthLanes > 0 {
			add("ib_port_link_width_active", float64(pi.WidthLanes))
		}
		if pi.SpeedGbps > 0 {
			add("ib_port_link_speed_active", pi.SpeedGbps)
		}
		if d, ok := dataGbps[int(pi.SpeedGbps*100)]; ok && pi.WidthLanes > 0 {
			add("ib_port_rate_bytes_per_second", float64(pi.WidthLanes)*d*1e9/8)
		}
	}
	if pi.MTU > 0 {
		add("ib_port_mtu_bytes", float64(pi.MTU))
	}
	return out
}
