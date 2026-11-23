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
	LastErr string // latest error of the first failing reader, "" when none; for /health
}

// Snapshot is an immutable view of all fabrics; never mutate after Swap.
type Snapshot struct {
	Fabrics map[string]*FabricSnapshot
	HCA     []Sample // nil when sources.localHCA is off; else at least the sysfs meta samples
	HCAOK   bool     // the local HCA was read this cycle
	HCAErr  string   // latest sysfs error, "" when none
	BuiltAt time.Time
}
