// Package sysfs reads local HCA state and counters from /sys/class/infiniband.
//
// The reader is OS-neutral (os.ReadDir/os.ReadFile on an injectable root), so it
// builds and is tested everywhere; on a host without the tree it returns an error.
package sysfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// DefaultRoot is the kernel's InfiniBand class directory.
const DefaultRoot = "/sys/class/infiniband"

// ReadHCA reads every device and port under root. fabricFor returns the fabric
// name bound to (device, port), or "" when the port is not bound in config.
// Unreadable or unparsable files give no sample, never zero. Ports without a
// readable gids/0 are skipped: the port GUID is the series identity.
func ReadHCA(root string, fabricFor func(dev string, port int) string) ([]ib.Sample, error) {
	devs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []ib.Sample
	for _, d := range devs {
		dir := filepath.Join(root, d.Name())
		ports, err := os.ReadDir(filepath.Join(dir, "ports"))
		if err != nil {
			continue
		}
		for _, p := range ports {
			n, err := strconv.Atoi(p.Name())
			if err != nil {
				continue
			}
			fabric := ""
			if fabricFor != nil {
				fabric = fabricFor(d.Name(), n)
			}
			out = append(out, readPort(dir, d.Name(), n, fabric)...)
		}
	}
	return out, nil
}

func readPort(devDir, dev string, port int, fabric string) []ib.Sample {
	pdir := filepath.Join(devDir, "ports", strconv.Itoa(port))
	guid, ok := guidOf(readStr(filepath.Join(pdir, "gids", "0")))
	if !ok {
		return nil
	}
	base := []ib.Label{{Key: ib.LabelFabric, Value: fabric}, {Key: "device", Value: dev}, {Key: "port", Value: strconv.Itoa(port)}, {Key: "guid", Value: guid}}
	with := func(extra ...ib.Label) []ib.Label { return append(append([]ib.Label{}, base...), extra...) }
	var out []ib.Sample
	add := func(name string, l []ib.Label, v float64, counter bool) {
		out = append(out, ib.Sample{Name: name, Labels: l, Value: v, Counter: counter})
	}

	// counters/ first, then counters_ext/: the 64-bit value replaces the 32-bit one.
	ctr := map[string]float64{}
	for _, sub := range []string{"counters", "counters_ext"} {
		for _, f := range files(filepath.Join(pdir, sub)) {
			if v, ok := readUint(f.path); ok {
				name, mul := counterName(f.name)
				ctr[name] = float64(v) * mul
			}
		}
	}
	for name, v := range ctr {
		add("ib_hca_"+name, base, v, true)
	}
	for _, f := range files(filepath.Join(pdir, "hw_counters")) {
		if v, ok := readUint(f.path); ok {
			add("ib_hca_hw_counter_total", with(ib.Label{Key: "name", Value: f.name}), float64(v), true)
		}
	}

	if v, ok := leadInt(readStr(filepath.Join(pdir, "state"))); ok {
		add("ib_hca_state", base, v, false)
	}
	if v, ok := leadInt(readStr(filepath.Join(pdir, "phys_state"))); ok {
		add("ib_hca_phys_state", base, v, false)
	}
	if v, ok := rateBytes(readStr(filepath.Join(pdir, "rate"))); ok {
		add("ib_hca_rate_bytes_per_second", base, v, false)
	}
	for _, f := range [][2]string{{"lid", "ib_hca_lid"}, {"sm_lid", "ib_hca_sm_lid"}} {
		if v, err := strconv.ParseUint(readStr(filepath.Join(pdir, f[0])), 0, 32); err == nil {
			add(f[1], base, float64(v), false)
		}
	}
	if ll := readStr(filepath.Join(pdir, "link_layer")); ll != "" {
		add("ib_hca_link_layer_info", with(ib.Label{Key: "link_layer", Value: ll}), 1, false)
	}

	// node_guid, board_id and fw_ver are optional: absent ones are skipped, not faked.
	if ng, ok := guidOf(readStr(filepath.Join(devDir, "node_guid"))); ok {
		add("ib_hca_info", with(
			ib.Label{Key: "board_id", Value: readStr(filepath.Join(devDir, "board_id"))},
			ib.Label{Key: "firmware", Value: readStr(filepath.Join(devDir, "fw_ver"))},
			ib.Label{Key: "node_guid", Value: ng},
		), 1, false)
	}
	return out
}

// counterName maps a counters/ or counters_ext/ file to the metric suffix, following
// ib_port_* names: "port_" prefix and "_64" suffix dropped, lowercased. Kernel docs
// (sysfs-class-infiniband) and node_exporter: port_{rcv,xmit}_data count units of 4
// octets (also the _64 variants), so they are exported x4 as bytes.
func counterName(file string) (string, float64) {
	n := strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(file), "port_"), "_64")
	if n == "rcv_data" || n == "xmit_data" {
		return n + "_bytes_total", 4
	}
	return n + "_total", 1
}

type entry struct{ name, path string }

// files lists regular-file and symlink entries of dir; a missing dir gives none.
func files(dir string) []entry {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []entry
	for _, e := range es {
		if !e.IsDir() {
			out = append(out, entry{e.Name(), filepath.Join(dir, e.Name())})
		}
	}
	return out
}

func readStr(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readUint(path string) (uint64, bool) {
	v, err := strconv.ParseUint(readStr(path), 10, 64)
	return v, err == nil
}

// guidOf formats the low 64 bits of "xxxx:xxxx:..." (a GID or sysfs node_guid) as 0x%016x.
func guidOf(s string) (string, bool) {
	h := strings.ReplaceAll(s, ":", "")
	if len(h) < 16 {
		return "", false
	}
	v, err := strconv.ParseUint(h[len(h)-16:], 16, 64)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("0x%016x", v), true
}

// leadInt parses the number before ':' in "4: ACTIVE".
func leadInt(s string) (float64, bool) {
	n, _, _ := strings.Cut(s, ":")
	v, err := strconv.Atoi(strings.TrimSpace(n))
	return float64(v), err == nil
}

// rateBytes parses "100 Gb/sec (4X EDR)" into bytes per second (Gb/s = 1e9 bit/s).
func rateBytes(s string) (float64, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	unit := 1e9
	if len(f) > 1 && strings.HasPrefix(f[1], "Mb") {
		unit = 1e6
	}
	return v * unit / 8, true
}
