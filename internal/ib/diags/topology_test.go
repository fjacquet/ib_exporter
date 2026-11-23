package diags

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fjacquet/ib_exporter/internal/ib"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/ibnetdiscover/fabric.txt")
	require.NoError(t, err)
	return b
}

func byGUID(nodes []ib.Node) map[string]ib.Node {
	m := map[string]ib.Node{}
	for _, n := range nodes {
		m[n.GUID] = n
	}
	return m
}

func TestParseTopology_NodesAndLinks(t *testing.T) {
	nodes, links, err := ParseTopology(fixture(t), nil)
	require.NoError(t, err)
	require.Len(t, nodes, 6)
	m := byGUID(nodes)
	sw := m["0x0002c90300aa0001"]
	require.Equal(t, ib.NodeSwitch, sw.Type)
	require.Equal(t, "spine1", sw.Name)
	require.Equal(t, uint16(101), sw.LID)
	require.Equal(t, 4, sw.Ports)
	require.Equal(t, ib.NodeHCA, m["0x0002c90300bb0001"].Type)
	require.Len(t, links, 9)
	require.Contains(t, links, ib.Link{GUID: "0x0002c90300aa0001", Port: 3,
		RemoteGUID: "0x0002c90300aa0002", RemotePort: 3, RemoteName: "leaf2"})
}

func TestParseTopology_NameWithSpacesAndQuotes(t *testing.T) {
	nodes, _, err := ParseTopology(fixture(t), nil)
	require.NoError(t, err)
	m := byGUID(nodes)
	require.Equal(t, `node02 "mlx5_0"`, m["0x0002c90300bb0002"].Name)
	// intended per the spec Identity rule: HCA guid is the port GUID, HCAs group by node_name,
	// so one dual-port HCA yields two nodes with the same name.
	require.Equal(t, "node01 HCA-1", m["0x0002c90300bb0001"].Name)
	require.Equal(t, "node01 HCA-1", m["0x0002c90300bb0003"].Name)
}

func TestParseTopology_UnknownLineIgnored(t *testing.T) {
	nodes, _, err := ParseTopology([]byte("garbage\nSW 1 1 0x01 4x EDR\n"), nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "0x01", nodes[0].Name) // unnamed falls back to GUID
}

func TestParseTopology_NameMapOverrides(t *testing.T) {
	nodes, links, err := ParseTopology(fixture(t), map[string]string{"0x0002c90300aa0002": "leaf-two"})
	require.NoError(t, err)
	require.Equal(t, "leaf-two", byGUID(nodes)["0x0002c90300aa0002"].Name)
	require.Contains(t, links, ib.Link{GUID: "0x0002c90300aa0001", Port: 3,
		RemoteGUID: "0x0002c90300aa0002", RemotePort: 3, RemoteName: "leaf-two"})
}

func TestLoadNodeNameMap_MissingFileReturnsEmpty(t *testing.T) {
	m, err := LoadNodeNameMap(filepath.Join(t.TempDir(), "nope"))
	require.NoError(t, err)
	require.Empty(t, m)
	m, err = LoadNodeNameMap("")
	require.NoError(t, err)
	require.Empty(t, m)
}

func TestLoadNodeNameMap_Parses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "map")
	require.NoError(t, os.WriteFile(p, []byte("# c\n0x0002C90300AA0001 \"spine 1\"\n\n"), 0o600))
	m, err := LoadNodeNameMap(p)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"0x0002c90300aa0001": "spine 1"}, m)
}

func TestParseTopology_SpacedSpeed(t *testing.T) {
	in := "CA 14 1 0x0002c90300bb0004 4x 14.0625 Gbps - SW 101 5 0x0002c90300aa0001 ( 'h4' - 'spine1' )\n"
	nodes, links, err := ParseTopology([]byte(in), nil)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Len(t, links, 1)
	require.Equal(t, "h4", nodes[0].Name)
}

func TestParseTopology_NothingParsedIsError(t *testing.T) {
	_, _, err := ParseTopology([]byte("# c\nSW totally unrecognised\n"), nil)
	require.Error(t, err)
	nodes, _, err := ParseTopology([]byte("# only comment\n\n"), nil)
	require.NoError(t, err)
	require.Empty(t, nodes)
}
