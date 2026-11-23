package diags

import "testing"

func TestParseSMInfo_Master(t *testing.T) {
	sm, err := ParseSMInfo(readTD(t, "sminfo", "master.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := SMInfo{GUID: "0x0002c9030012ab01", LID: 1, ActivityCount: 482910, Priority: 14, State: 3}
	if sm != want {
		t.Fatalf("got %+v want %+v", sm, want)
	}
	ss := SMSamples("f", sm)
	l := []string{"fabric", "f", "sm_guid", "0x0002c9030012ab01", "sm_lid", "1"}
	for name, want := range map[string]float64{"ib_sm_info": 1, "ib_sm_state": 3, "ib_sm_priority": 14, "ib_sm_activity_count": 482910} {
		if v, ok := sampleVal(ss, name, l...); !ok || v != want {
			t.Errorf("%s = %v,%v want %v", name, v, ok, want)
		}
	}
}

func TestParseSMInfo_SMIR(t *testing.T) {
	sm, err := ParseSMInfo(readTD(t, "sminfo", "smir.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := SMInfo{GUID: "0x0002c9030012ab02", LID: 2, ActivityCount: 77, Priority: 5, State: 2}
	if sm != want {
		t.Fatalf("got %+v want %+v", sm, want)
	}
}

func TestParseSMInfo_Garbage(t *testing.T) {
	for _, in := range []string{"", "sminfo: ibwarn: can't open UMAD\n", "sminfo: sm lid 1 sm guid 0x1, activity count x priority 1 state 3"} {
		if _, err := ParseSMInfo([]byte(in)); err == nil {
			t.Errorf("want error for %q", in)
		}
	}
}
