package diags

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "perfquery", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sampleVal(ss []ib.Sample, name string, lbl ...string) (float64, bool) {
next:
	for _, s := range ss {
		if s.Name != name {
			continue
		}
		for i := 0; i+1 < len(lbl); i += 2 {
			found := false
			for _, l := range s.Labels {
				if l.Key == lbl[i] && l.Value == lbl[i+1] {
					found = true
				}
			}
			if !found {
				continue next
			}
		}
		return s.Value, true
	}
	return 0, false
}

func TestParse_PerPortLoopOutput(t *testing.T) {
	pcs, err := ParsePerfquery(readFixture(t, "basic.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pcs) != 2 || pcs[0].Port != 1 || pcs[1].Port != 2 {
		t.Fatalf("want ports 1,2 got %+v", pcs)
	}
	if pcs[1].Fields["SymbolErrorCounter"] != 6 || pcs[0].Fields["PortRcvData"] != 1000 {
		t.Fatalf("fields: %+v", pcs[1].Fields)
	}
}

func TestParse_NAValueIsAbsent(t *testing.T) {
	in := "# Port counters: Lid 5 port 1 (CapMask: 0x5A00)\nPortXmitWait:....................N/A\nVL15Dropped:.....................\nPortXmitPkts:....................9\n"
	pcs, err := ParsePerfquery([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pcs[0].Fields["PortXmitWait"]; ok {
		t.Fatal("N/A must be absent")
	}
	if _, ok := pcs[0].Fields["VL15Dropped"]; ok {
		t.Fatal("empty must be absent")
	}
	if pcs[0].Fields["PortXmitPkts"] != 9 {
		t.Fatal("neighbour lost")
	}
	ss := ToSamples("f", ib.Node{GUID: "0x1", Type: ib.NodeSwitch}, pcs[0])
	if _, ok := sampleVal(ss, "ib_port_xmit_wait_total"); ok {
		t.Fatal("absent value must yield no sample")
	}
}

func TestParse_SaturatedCounterKept(t *testing.T) {
	in := "# Port counters: Lid 5 port 1\nSymbolErrorCounter:..............65535\nPortRcvData:.....................4294967295\nLinkDownedCounter:...............255\n"
	pcs, err := ParsePerfquery([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	ss := ToSamples("f", ib.Node{GUID: "0x1"}, pcs[0])
	if v, ok := sampleVal(ss, "ib_port_symbol_error_total"); !ok || v != 65535 {
		t.Fatalf("symbol: %v %v", v, ok)
	}
	if v, ok := sampleVal(ss, "ib_port_link_downed_total"); !ok || v != 255 {
		t.Fatalf("downed: %v %v", v, ok)
	}
	if v, _ := sampleVal(ss, "ib_port_rcv_data_bytes_total"); v != 4294967295*4 {
		t.Fatalf("rcv: %v", v)
	}
}

func TestMerge_BasicErrorsPlusExtendedData(t *testing.T) {
	b, _ := ParsePerfquery(readFixture(t, "basic.txt"))
	x, _ := ParsePerfquery(readFixture(t, "extended.txt"))
	m := MergeCounters(b, x)
	if len(m) != 2 {
		t.Fatalf("len %d", len(m))
	}
	if m[0].Fields["PortXmitData"] != 10000000000 {
		t.Fatalf("64-bit data must win: %d", m[0].Fields["PortXmitData"])
	}
	if m[0].Fields["PortXmitDiscards"] != 7 || m[0].Fields["PortUnicastXmitPkts"] != 299000000 {
		t.Fatalf("merge lost fields: %+v", m[0].Fields)
	}
}

func TestToSamples_DataTimesFour(t *testing.T) {
	x, _ := ParsePerfquery(readFixture(t, "extended.txt"))
	n := ib.Node{GUID: "0xabc", Name: "sw1", Type: ib.NodeSwitch}
	ss := ToSamples("fab", n, x[0])
	v, ok := sampleVal(ss, "ib_port_xmit_data_bytes_total", "guid", "0xabc", "port", "1", "fabric", "fab")
	if !ok || v != 40000000000 {
		t.Fatalf("got %v %v", v, ok)
	}
	for _, s := range ss {
		if !s.Counter {
			t.Fatalf("%s not counter", s.Name)
		}
	}
	if v, _ := sampleVal(ss, "ib_port_xmit_packets_total"); v != 300000000 {
		t.Fatalf("packets must not be x4: %v", v)
	}
}

func TestToSamples_SpeedsLaneLabelsAndUnion(t *testing.T) {
	pcs, err := ParsePerfquery(readFixture(t, "speeds.txt"))
	if err != nil || len(pcs) != 2 {
		t.Fatalf("%v %d", err, len(pcs))
	}
	n := ib.Node{GUID: "0x1", Type: ib.NodeSwitch}
	ss := append(ToSamples("f", n, pcs[0]), ToSamples("f", n, pcs[1])...)
	if v, ok := sampleVal(ss, "ib_port_fec_corrected_blocks_total", "lane", "2"); !ok || v != 20 {
		t.Fatalf("lane2: %v %v", v, ok)
	}
	if v, ok := sampleVal(ss, "ib_port_fec_uncorrectable_blocks_total", "lane", "", "port", "2"); !ok || v != 2 {
		t.Fatalf("rsfec port-level: %v %v", v, ok)
	}
	keys := map[string]string{}
	for _, s := range ss {
		k := ""
		for _, l := range s.Labels {
			k += l.Key + ","
		}
		if prev, ok := keys[s.Name]; ok && prev != k {
			t.Fatalf("%s label keys differ: %q vs %q", s.Name, prev, k)
		}
		keys[s.Name] = k
	}
}

func TestParse_OptionalFixtures(t *testing.T) {
	for file, metric := range map[string]string{
		"rcverr.txt":     "ib_port_local_physical_errors_total",
		"xmitdisc.txt":   "ib_port_xmit_inactive_discards_total",
		"congestion.txt": "ib_port_xmit_time_cong_total",
	} {
		pcs, err := ParsePerfquery(readFixture(t, file))
		if err != nil || len(pcs) == 0 {
			t.Fatalf("%s: %v", file, err)
		}
		var all []ib.Sample
		for _, pc := range pcs {
			all = append(all, ToSamples("f", ib.Node{GUID: "0x1"}, pc)...)
		}
		if _, ok := sampleVal(all, metric); !ok {
			t.Fatalf("%s: missing %s in %+v", file, metric, all)
		}
	}
}

func TestParse_UnknownFormatErrors(t *testing.T) {
	if _, err := ParsePerfquery([]byte("garbage\n")); err == nil {
		t.Fatal("want error")
	}
	if pcs, err := ParsePerfquery(nil); err != nil || len(pcs) != 0 {
		t.Fatalf("empty: %v %v", pcs, err)
	}
}
