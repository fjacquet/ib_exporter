// Package config loads and validates the exporter configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Server struct {
	Port int    `yaml:"port"`
	URI  string `yaml:"uri"`
}

type Collection struct {
	Interval         time.Duration `yaml:"interval"`
	TopologyInterval time.Duration `yaml:"topologyInterval"`
	PortInfoInterval time.Duration `yaml:"portInfoInterval"`
	MaxConcurrent    int           `yaml:"maxConcurrent"`
	OptionalEvery    int           `yaml:"optionalEvery"`
}

type Fabric struct {
	Name        string `yaml:"name"`
	CA          string `yaml:"ca"`
	Port        int    `yaml:"port"`
	NodeNameMap string `yaml:"nodeNameMap"`
}

type Ibswinfo struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type Sources struct {
	LocalHCA bool     `yaml:"localHCA"`
	Ibswinfo Ibswinfo `yaml:"ibswinfo"`
}

type Replay struct {
	Dir string `yaml:"dir"`
}

type Config struct {
	Server     Server     `yaml:"server"`
	Collection Collection `yaml:"collection"`
	Fabrics    []Fabric   `yaml:"fabrics"`
	Sources    Sources    `yaml:"sources"`
	Replay     Replay     `yaml:"replay"`
}

// Load reads path, applies defaults and ${ENV} expansion on string fields.
// It does not validate; call Validate.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	c := &Config{
		Server: Server{Port: 9449, URI: "/metrics"},
		Collection: Collection{
			Interval: 60 * time.Second, TopologyInterval: 10 * time.Minute,
			PortInfoInterval: 5 * time.Minute, MaxConcurrent: 4, OptionalEvery: 5,
		},
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.Server.URI = os.ExpandEnv(c.Server.URI)
	for i := range c.Fabrics {
		f := &c.Fabrics[i]
		f.Name, f.CA, f.NodeNameMap = os.ExpandEnv(f.Name), os.ExpandEnv(f.CA), os.ExpandEnv(f.NodeNameMap)
	}
	c.Sources.Ibswinfo.Path = os.ExpandEnv(c.Sources.Ibswinfo.Path)
	c.Replay.Dir = os.ExpandEnv(c.Replay.Dir)
	return c, nil
}
