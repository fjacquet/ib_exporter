package sysfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

const fixture = "testdata/sysfs"

func bind(dev string, port int) string {
	if dev == "mlx5_0" && port == 1 {
		return "prod"
	}
	return ""
}

func find(ss []ib.Sample, name, dev string, labels ...string) (ib.Sample, bool) {
	for _, s := range ss {
		if s.Name != name || label(s, "device") != dev {
			continue
		}
		ok := true
		for i := 0; i+1 < len(labels); i += 2 {
			ok = ok && label(s, labels[i]) == labels[i+1]
		}
		if ok {
			return s, true
		}
	}
	return ib.Sample{}, false
}

func label(s ib.Sample, k string) string {
	for _, l := range s.Labels {
		if l.Key == k {
			return l.Value
		}
	}
	return ""
}

func TestReadHCA_EnumeratesAllHwCounters(t *testing.T) {
	ss, err := ReadHCA(fixture, bind)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{"out_of_buffer": 4, "np_cnp_sent": 9, "zz_unknown_counter": 77} {
		s, ok := find(ss, "ib_hca_hw_counter_total", "mlx5_0", "name", name)
		if !ok || s.Value != want || !s.Counter {
			t.Errorf("hw counter %s: %+v ok=%v", name, s, ok)
		}
		if label(s, "fabric") != "prod" || label(s, "guid") != "0x0002c90300123457" {
			t.Errorf("labels: %+v", s.Labels)
		}
	}
}

func TestReadHCA_NoDeviceNameAssumption(t *testing.T) {
	ss, err := ReadHCA(fixture, bind)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := find(ss, "ib_hca_hw_counter_total", "foo_0", "name", "zz_unknown_counter")
	if !ok || s.Value != 77 {
		t.Fatalf("foo_0 missing: %+v", s)
	}
	if label(s, "fabric") != "" {
		t.Errorf("unbound port must have empty fabric, got %q", label(s, "fabric"))
	}
	info, ok := find(ss, "ib_hca_info", "foo_0")
	if !ok || label(info, "node_guid") != "0x0002c90300abcdef" || label(info, "board_id") != "MT_0000000999" || label(info, "firmware") != "28.40.1000" || info.Value != 1 {
		t.Errorf("info: %+v", info)
	}
}

func TestReadHCA_DataCountersTimes4AndGauges(t *testing.T) {
	ss, err := ReadHCA(fixture, bind)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]float64{
		"ib_hca_rcv_data_bytes_total":  20000, // counters_ext _64 (5000) x4 overrides counters/ (1000)
		"ib_hca_xmit_data_bytes_total": 24000,
		"ib_hca_link_downed_total":     2,
		"ib_hca_xmit_packets_total":    20,
		"ib_hca_state":                 4,
		"ib_hca_phys_state":            5,
		"ib_hca_rate_bytes_per_second": 12.5e9,
		"ib_hca_lid":                   3,
		"ib_hca_sm_lid":                1,
		"ib_hca_link_layer_info":       1,
	} {
		s, ok := find(ss, name, "mlx5_0")
		if !ok || s.Value != want {
			t.Errorf("%s = %+v ok=%v want %v", name, s, ok, want)
		}
	}
	if s, _ := find(ss, "ib_hca_link_layer_info", "mlx5_0"); label(s, "link_layer") != "InfiniBand" {
		t.Errorf("link_layer: %+v", s.Labels)
	}
}

func TestReadHCA_UnreadableFileSkipped(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	c := filepath.Join(root, "mlx5_0/ports/1/counters/link_downed")
	if err := os.Remove(c); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/nonexistent", c); err != nil { // listed by ReadDir, unreadable
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mlx5_0/ports/1/rate"), []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ss, err := ReadHCA(root, bind)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"ib_hca_link_downed_total", "ib_hca_rate_bytes_per_second"} {
		if s, ok := find(ss, n, "mlx5_0"); ok {
			t.Errorf("%s must be absent, got %+v", n, s)
		}
	}
	if _, ok := find(ss, "ib_hca_state", "mlx5_0"); !ok {
		t.Error("other samples must survive")
	}
}

func TestReadHCA_MissingRoot(t *testing.T) {
	if _, err := ReadHCA(filepath.Join(t.TempDir(), "nope"), bind); err == nil {
		t.Fatal("want error")
	}
}
