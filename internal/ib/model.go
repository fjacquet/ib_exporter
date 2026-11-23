// Package ib holds the InfiniBand fabric model shared by collectors and exporters.
package ib

import "strconv"

// LabelFabric is the label key identifying the fabric.
const LabelFabric = "fabric"

// NodeType is the kind of InfiniBand node.
type NodeType string

// Node types.
const (
	NodeSwitch NodeType = "switch"
	NodeHCA    NodeType = "hca"
	NodeRouter NodeType = "router"
)

// Label is a metric label pair.
type Label struct{ Key, Value string }

// Sample is one metric observation.
type Sample struct {
	Name    string
	Labels  []Label
	Value   float64
	Counter bool
}

// Node is a fabric node (switch, HCA or router).
type Node struct {
	GUID, Name string
	Type       NodeType
	LID        uint16
	Ports      int
}

// Link is a port-to-port connection.
type Link struct {
	GUID       string
	Port       int
	RemoteGUID string
	RemotePort int
	RemoteName string
}

// PortLabels returns the canonical per-port label set, in fixed order.
func PortLabels(fabric string, n Node, port int) []Label {
	return []Label{
		{LabelFabric, fabric},
		{"node_type", string(n.Type)},
		{"guid", n.GUID},
		{"node_name", n.Name},
		{"port", strconv.Itoa(port)},
	}
}
