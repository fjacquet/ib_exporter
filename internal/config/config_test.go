package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.yaml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestLoad_Defaults(t *testing.T) {
	c, err := Load(write(t, "sources: {localHCA: true}\n"))
	require.NoError(t, err)
	require.Equal(t, 9449, c.Server.Port)
	require.Equal(t, "/metrics", c.Server.URI)
	require.Equal(t, 60*time.Second, c.Collection.Interval)
	require.Equal(t, 10*time.Minute, c.Collection.TopologyInterval)
	require.Equal(t, 5*time.Minute, c.Collection.PortInfoInterval)
	require.Equal(t, 4, c.Collection.MaxConcurrent)
	require.Equal(t, 5, c.Collection.OptionalEvery)
}

func TestLoad_ExpandsEnv(t *testing.T) {
	t.Setenv("IB_MAP", "/etc/ib/map")
	t.Setenv("IB_CA", "mlx5_3")
	c, err := Load(write(t, "fabrics:\n  - {name: a, ca: \"${IB_CA}\", port: 1, nodeNameMap: \"${IB_MAP}\"}\n"))
	require.NoError(t, err)
	require.Equal(t, "mlx5_3", c.Fabrics[0].CA)
	require.Equal(t, "/etc/ib/map", c.Fabrics[0].NodeNameMap)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
}

func TestValidate_RejectsReservedURI(t *testing.T) {
	for _, uri := range []string{"/", "/health", "/livez", "/readyz", "metrics"} {
		c := &Config{Server: Server{Port: 9449, URI: uri}, Collection: validCollection(), Sources: Sources{LocalHCA: true}}
		require.Error(t, c.Validate(), uri)
	}
	c := &Config{Server: Server{Port: 9449, URI: "/metrics"}, Collection: validCollection(), Sources: Sources{LocalHCA: true}}
	require.NoError(t, c.Validate())
}

func TestValidate_DuplicateFabricName(t *testing.T) {
	c := &Config{Server: Server{Port: 9449, URI: "/metrics"}, Collection: validCollection(),
		Fabrics: []Fabric{{Name: "a", CA: "mlx5_0", Port: 1}, {Name: "a", CA: "mlx5_1", Port: 1}}}
	require.ErrorContains(t, c.Validate(), "duplicate")
}

func TestValidate_EmptyFabricsAllowedWithLocalHCA(t *testing.T) {
	c := &Config{Server: Server{Port: 9449, URI: "/metrics"}, Collection: validCollection()}
	require.Error(t, c.Validate())
	c.Sources.LocalHCA = true
	require.NoError(t, c.Validate())
}

func validCollection() Collection {
	return Collection{Interval: time.Minute, TopologyInterval: time.Minute, PortInfoInterval: time.Minute, MaxConcurrent: 1, OptionalEvery: 1}
}

func TestValidate_CollectionAndFabricFields(t *testing.T) {
	cases := map[string]func(c *Config){
		"interval":         func(c *Config) { c.Collection.Interval = 0 },
		"topologyInterval": func(c *Config) { c.Collection.TopologyInterval = -time.Second },
		"portInfoInterval": func(c *Config) { c.Collection.PortInfoInterval = 0 },
		"maxConcurrent":    func(c *Config) { c.Collection.MaxConcurrent = 0 },
		"optionalEvery":    func(c *Config) { c.Collection.OptionalEvery = 0 },
		"ca":               func(c *Config) { c.Fabrics[0].CA = "" },
		"port":             func(c *Config) { c.Fabrics[0].Port = 0 },
	}
	for want, mutate := range cases {
		c := &Config{Server: Server{Port: 9449, URI: "/metrics"}, Collection: validCollection(),
			Fabrics: []Fabric{{Name: "a", CA: "mlx5_0", Port: 1}}}
		require.NoError(t, c.Validate())
		mutate(c)
		require.ErrorContains(t, c.Validate(), want, want)
	}
}

func TestValidate_SampleConfigAndDefaults(t *testing.T) {
	c, err := Load("../../config.yaml")
	require.NoError(t, err)
	require.NoError(t, c.Validate())
	c, err = Load(write(t, "fabrics:\n  - {name: a, ca: mlx5_0, port: 1}\n"))
	require.NoError(t, err)
	require.NoError(t, c.Validate())
}
