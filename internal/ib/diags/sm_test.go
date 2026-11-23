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

func TestParseSMInfo_UnpaddedGUIDIsNormalised(t *testing.T) {
	line := "sminfo: sm lid 1 sm guid 0x2C9030012ab01, activity count 1 priority 0 state 3 SMINFO_MASTER\n"
	smir := "SMInfoRecord dump:\n\t\tLID...................1\n\t\tGUID..................0x0002c9030012ab01\n" +
		"\t\tActCount..............1\n\t\tPriority..............0\n\t\tSMState...............3\n"
	a, err := ParseSMInfo([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseSMInfo([]byte(smir))
	if err != nil {
		t.Fatal(err)
	}
	if a.GUID != "0x0002c9030012ab01" || a.GUID != b.GUID {
		t.Fatalf("sminfo %q smir %q", a.GUID, b.GUID)
	}
	if _, ok := sampleVal(SMSamples("f", a), "ib_sm_info", "sm_guid", "0x0002c9030012ab01"); !ok {
		t.Fatal("sm_guid label not normalised")
	}
}
