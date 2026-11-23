package diags

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

func readTD(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParsePortInfo_ActiveLink(t *testing.T) {
	pi, err := ParsePortInfo(readTD(t, "portinfo", "active.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := PortInfo{State: 4, PhysState: 5, WidthLanes: 4, SpeedGbps: 53.125, MTU: 4096}
	if pi != want {
		t.Fatalf("got %+v want %+v", pi, want)
	}
	ss := PortInfoSamples("f", testNode(), 1, pi)
	for name, want := range map[string]float64{
		"ib_port_state": 4, "ib_port_phys_state": 5, "ib_port_link_width_active": 4,
		"ib_port_link_speed_active": 53.125, "ib_port_rate_bytes_per_second": 25e9, "ib_port_mtu_bytes": 4096,
	} {
		if v, ok := sampleVal(ss, name, "fabric", "f", "port", "1"); !ok || v != want {
			t.Errorf("%s = %v,%v want %v", name, v, ok, want)
		}
	}
	for _, s := range ss {
		if s.Counter {
			t.Errorf("%s must be a gauge", s.Name)
		}
	}
}

func TestParsePortInfo_Down(t *testing.T) {
	pi, err := ParsePortInfo(readTD(t, "portinfo", "down.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if pi.State != 1 || pi.PhysState != 2 {
		t.Fatalf("got %+v", pi)
	}
	ss := PortInfoSamples("f", testNode(), 1, pi)
	if v, ok := sampleVal(ss, "ib_port_state"); !ok || v != 1 {
		t.Fatalf("state %v %v", v, ok)
	}
	for _, n := range []string{"ib_port_link_width_active", "ib_port_link_speed_active", "ib_port_rate_bytes_per_second"} {
		if _, ok := sampleVal(ss, n); ok {
			t.Errorf("%s must be absent on a down port", n)
		}
	}
}

func TestParsePortInfo_SpacedSpeedAndNA(t *testing.T) {
	in := "LinkState:....Active\nPhysLinkState:...LinkUp\nLinkWidthActive:....4X\nLinkSpeedActive:....10.0 Gbps\nNeighborMTU:....N/A\n"
	pi, err := ParsePortInfo([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if pi.SpeedGbps != 10 || pi.MTU != 0 {
		t.Fatalf("got %+v", pi)
	}
	ss := PortInfoSamples("f", testNode(), 2, pi)
	if v, _ := sampleVal(ss, "ib_port_rate_bytes_per_second"); v != 4e9 {
		t.Errorf("rate %v", v)
	}
	if _, ok := sampleVal(ss, "ib_port_mtu_bytes"); ok {
		t.Error("mtu N/A must be absent")
	}
}

func TestParsePortInfo_Garbage(t *testing.T) {
	if _, err := ParsePortInfo([]byte("ibwarn: [1] mad_rpc: can't open UMAD port\n")); err == nil {
		t.Fatal("want error")
	}
	if _, err := ParsePortInfo(nil); err == nil {
		t.Fatal("want error on empty")
	}
}

func testNode() ib.Node {
	return ib.Node{GUID: "0x0002c9030012ab01", Name: "leaf-01", Type: ib.NodeSwitch, LID: 5, Ports: 40}
}
