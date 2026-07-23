// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"sync"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// NodeMapping maps explicitly configured Envoy node IDs to target IDs.
type NodeMapping struct {
	mapping map[string]string
}

// NewNodeMapping copies the configured static mapping.
func NewNodeMapping(mapping map[string]string) NodeMapping {
	copied := make(map[string]string, len(mapping))
	for nodeID, targetID := range mapping {
		if nodeID != "" && targetID != "" {
			copied[nodeID] = targetID
		}
	}
	return NodeMapping{mapping: copied}
}

// ID implements cache.NodeHash. Unknown nodes map to no snapshot and are rejected by callbacks.
func (m NodeMapping) ID(node *corev3.Node) string {
	if node == nil {
		return ""
	}
	return m.mapping[node.GetId()]
}

func (m NodeMapping) target(nodeID string) (string, bool) {
	targetID, ok := m.mapping[nodeID]
	return targetID, ok
}

func (m NodeMapping) nodes(targetID string) []string {
	nodes := make([]string, 0)
	for nodeID, mappedTarget := range m.mapping {
		if mappedTarget == targetID {
			nodes = append(nodes, nodeID)
		}
	}
	return nodes
}

type connections struct {
	mu      sync.RWMutex
	streams map[int64]string
	counts  map[string]int
}

func newConnections() *connections {
	return &connections{streams: make(map[int64]string), counts: make(map[string]int)}
}

func (c *connections) connect(streamID int64, nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if previous := c.streams[streamID]; previous == nodeID {
		return
	} else if previous != "" {
		c.counts[previous]--
	}
	c.streams[streamID] = nodeID
	c.counts[nodeID]++
}

func (c *connections) disconnect(streamID int64, nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if nodeID == "" {
		nodeID = c.streams[streamID]
	}
	delete(c.streams, streamID)
	if c.counts[nodeID] <= 1 {
		delete(c.counts, nodeID)
	} else {
		c.counts[nodeID]--
	}
}

func (c *connections) connected(nodeID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.counts[nodeID] > 0
}

func (c *connections) streamNode(streamID int64) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.streams[streamID]
}
