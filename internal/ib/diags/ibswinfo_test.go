package diags

import "testing"

func TestParseIbswinfo_Basic(t *testing.T) {
	h, err := ParseIbswinfo(readTD(t, "ibswinfo", "basic.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ss := SwitchHealthSamples("f", testNode(), h)
	b := []string{"fabric", "f"}
	chk := func(name string, want float64, extra ...string) {
		t.Helper()
		if v, ok := sampleVal(ss, name, append(b, extra...)...); !ok || v != want {
			t.Errorf("%s %v = %v,%v want %v", name, extra, v, ok, want)
		}
	}
	chk("ib_switch_temperature_celsius", 47)
	chk("ib_switch_uptime_seconds", 12*86400+3*3600+4*60+5)
	chk("ib_switch_fan_rpm", 8200, "fan", "fan1")
	chk("ib_switch_fan_rpm", 8050, "fan", "fan2")
	chk("ib_switch_psu_watts", 112, "psu", "psu1")
	chk("ib_switch_psu_status_info", 1, "psu", "psu2", "state", "OK")
	chk("ib_switch_hardware_info", 1, "part_number", "MQM8700-HS2F", "serial_number", "MT2001T01234", "firmware_version", "27.2008.2102")
	if _, ok := sampleVal(ss, "ib_switch_fan_rpm", "fan", "fan3"); ok {
		t.Error("N/A fan must be absent")
	}
	if _, ok := sampleVal(ss, "ib_switch_psu_watts", "psu", "psu2"); ok {
		t.Error("N/A psu power must be absent")
	}
	// one label-key set per metric name
	keys := map[string]string{}
	for _, s := range ss {
		k := ""
		for _, l := range s.Labels {
			k += l.Key + ","
		}
		if p, ok := keys[s.Name]; ok && p != k {
			t.Errorf("%s label keys differ: %q vs %q", s.Name, p, k)
		}
		keys[s.Name] = k
	}
}

func TestParseIbswinfo_Garbage_Absent(t *testing.T) {
	for _, in := range [][]byte{readTD(t, "ibswinfo", "garbage.txt"), nil, []byte("temperature : hot\nuptime : soon\n")} {
		h, err := ParseIbswinfo(in)
		if err == nil {
			t.Errorf("want error for %q, got %+v", in, h)
		}
		if n := len(SwitchHealthSamples("f", testNode(), h)); n != 0 {
			t.Errorf("garbage produced %d samples", n)
		}
	}
}
