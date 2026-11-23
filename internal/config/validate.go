package config

import (
	"errors"
	"fmt"
	"strings"
)

// ReservedURIs are served by the exporter itself; the metrics URI must not collide.
var ReservedURIs = map[string]bool{"/": true, "/health": true, "/livez": true, "/readyz": true}

// Validate checks the configuration for unusable values.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range", c.Server.Port)
	}
	if !strings.HasPrefix(c.Server.URI, "/") {
		return fmt.Errorf("server.uri %q must start with /", c.Server.URI)
	}
	if ReservedURIs[c.Server.URI] {
		return fmt.Errorf("server.uri %q is reserved", c.Server.URI)
	}
	if len(c.Fabrics) == 0 && !c.Sources.LocalHCA {
		return errors.New("no fabrics configured and sources.localHCA is false")
	}
	seen := map[string]bool{}
	for _, f := range c.Fabrics {
		if f.Name == "" {
			return errors.New("fabric name must not be empty")
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate fabric name %q", f.Name)
		}
		seen[f.Name] = true
	}
	return nil
}
