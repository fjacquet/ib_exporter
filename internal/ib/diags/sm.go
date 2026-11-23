package diags

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// SMInfo is the subnet manager answer. State: 0 NotActive, 1 Discovering, 2 Standby, 3 Master.
type SMInfo struct {
	GUID          string // lowercase 0x-prefixed
	LID           int
	ActivityCount uint32
	Priority      int
	State         int
}

var (
	// sminfo.c: "sminfo: sm lid %d sm guid 0x%lx, activity count %u priority %d state %d %s"
	smLine = regexp.MustCompile(`sm lid (\d+) sm guid (0x[0-9a-fA-F]+), activity count (\d+) priority (\d+) state (\d+)`)
	// saquery.c dump_sm_info_record: "\t\tLID....N", "GUID....0x..", "ActCount..", "Priority..", "SMState.."
	smirKV = regexp.MustCompile(`^\s*(LID|GUID|ActCount|Priority|SMState)\.+\s*(\S+)\s*$`)
)

// ParseSMInfo parses `sminfo` output or the first record of `saquery SMIR`. Any missing or
// non-numeric field is an error.
func ParseSMInfo(out []byte) (SMInfo, error) {
	if m := smLine.FindSubmatch(out); m != nil {
		lid, _ := strconv.Atoi(string(m[1]))
		act, err := strconv.ParseUint(string(m[3]), 10, 32)
		prio, _ := strconv.Atoi(string(m[4]))
		st, _ := strconv.Atoi(string(m[5]))
		if err == nil {
			return SMInfo{GUID: strings.ToLower(string(m[2])), LID: lid, ActivityCount: uint32(act), Priority: prio, State: st}, nil
		}
	}
	kv := map[string]string{}
	for _, l := range strings.Split(string(out), "\n") {
		if m := smirKV.FindStringSubmatch(l); m != nil {
			if _, dup := kv[m[1]]; !dup { // first record only
				kv[m[1]] = m[2]
			}
		}
	}
	lid, e1 := strconv.Atoi(kv["LID"])
	act, e2 := strconv.ParseUint(kv["ActCount"], 10, 32)
	prio, e3 := strconv.Atoi(kv["Priority"])
	st, e4 := strconv.Atoi(kv["SMState"])
	if kv["GUID"] == "" || errors.Join(e1, e2, e3, e4) != nil {
		return SMInfo{}, errors.New("no SM info found: unknown format")
	}
	return SMInfo{GUID: strings.ToLower(kv["GUID"]), LID: lid, ActivityCount: uint32(act), Priority: prio, State: st}, nil
}

// SMSamples converts SMInfo into gauges. Every series carries fabric,sm_guid,sm_lid.
func SMSamples(fabric string, sm SMInfo) []ib.Sample {
	lbl := func() []ib.Label {
		return []ib.Label{{Key: ib.LabelFabric, Value: fabric}, {Key: "sm_guid", Value: sm.GUID}, {Key: "sm_lid", Value: strconv.Itoa(sm.LID)}}
	}
	return []ib.Sample{
		{Name: "ib_sm_info", Labels: lbl(), Value: 1},
		{Name: "ib_sm_state", Labels: lbl(), Value: float64(sm.State)},
		{Name: "ib_sm_priority", Labels: lbl(), Value: float64(sm.Priority)},
		{Name: "ib_sm_activity_count", Labels: lbl(), Value: float64(sm.ActivityCount)},
	}
}
