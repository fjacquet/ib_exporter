package ib

import "time"

// FabricSnapshot is one fabric's state at a collection instant.
type FabricSnapshot struct {
	Fabric  string
	Up      bool
	Nodes   []Node
	Links   []Link
	Samples []Sample
	At      time.Time
}

// Snapshot is an immutable view of all fabrics; never mutate after Swap.
type Snapshot struct {
	Fabrics map[string]*FabricSnapshot
	HCA     []Sample
	BuiltAt time.Time
}
