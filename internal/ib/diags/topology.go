package diags

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

// Format of `ibnetdiscover -p` (rdma-core dump_ports_report):
//
//	SW   101  1 0x... 4x EDR - CA 11 1 0x... ( 'local' - 'remote' )
var portLine = regexp.MustCompile(`^(SW|CA|RT)\s+(\d+)\s+(\d+)\s+(0x[0-9a-fA-F]+)\s+\S+\s+\S+` +
	`(?:\s+-\s+(?:SW|CA|RT)\s+\d+\s+(\d+)\s+(0x[0-9a-fA-F]+)\s+\(\s*'(.*)'\s+-\s+'(.*)'\s*\))?` +
	`\s*(?:'(.*)')?\s*$`)

var nodeTypes = map[string]ib.NodeType{"SW": ib.NodeSwitch, "CA": ib.NodeHCA, "RT": ib.NodeRouter}

// ParseTopology parses `ibnetdiscover -p` output. Unrecognised lines are skipped.
// Names come from nameMap (keyed by lower-case GUID), else the tool's name, else the GUID.
func ParseTopology(out []byte, nameMap map[string]string) ([]ib.Node, []ib.Link, error) {
	var nodes []ib.Node
	idx := map[string]int{}
	names := map[string]string{} // guid -> tool-reported name
	var links []ib.Link
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		m := portLine.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		lid, _ := strconv.ParseUint(m[2], 10, 16)
		port, _ := strconv.Atoi(m[3])
		guid := strings.ToLower(m[4])
		i, ok := idx[guid]
		if !ok {
			i = len(nodes)
			idx[guid] = i
			nodes = append(nodes, ib.Node{GUID: guid, Type: nodeTypes[m[1]], LID: uint16(lid)})
		}
		nodes[i].Ports++
		name := m[9]
		if m[5] != "" {
			name = m[7]
			rport, _ := strconv.Atoi(m[5])
			rg := strings.ToLower(m[6])
			if _, seen := names[rg]; !seen {
				names[rg] = m[8]
			}
			links = append(links, ib.Link{GUID: guid, Port: port, RemoteGUID: rg, RemotePort: rport})
		}
		if name != "" {
			names[guid] = name
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan topology: %w", err)
	}
	resolve := func(guid string) string {
		if n := nameMap[guid]; n != "" {
			return n
		}
		if n := names[guid]; n != "" {
			return n
		}
		return guid
	}
	for i := range nodes {
		nodes[i].Name = resolve(nodes[i].GUID)
	}
	for i := range links {
		links[i].RemoteName = resolve(links[i].RemoteGUID)
	}
	return nodes, links, nil
}

// LoadNodeNameMap reads an ibnetdiscover node-name-map (`0xGUID "name"` lines).
// An empty or missing path yields an empty map, not an error.
func LoadNodeNameMap(path string) (map[string]string, error) {
	m := map[string]string{}
	if path == "" {
		return m, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read node name map: %w", err)
	}
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		guid, rest, ok := strings.Cut(ln, " ")
		if !ok {
			continue
		}
		name := strings.Trim(strings.TrimSpace(rest), `"`)
		if name != "" {
			m[strings.ToLower(guid)] = name
		}
	}
	return m, nil
}
