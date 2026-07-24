// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package xdsserver

import (
	"context"
	"fmt"
	"sync"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const unauthorizedMessage = "xDS request is not authorized"

type streamIdentity struct {
	uri     string
	typeURL string
	nodeID  string
}

type callbacks struct {
	secure       bool
	assignments  relayAssignments
	metrics      *callbackMetrics
	mu           sync.RWMutex
	streams      map[int64]streamIdentity
	deltaStreams map[int64]streamIdentity
}

func newCallbacks(secure bool, assignments relayAssignments, registry prometheus.Registerer) (*callbacks, error) {
	callbackMetrics, err := newCallbackMetrics(registry)
	if err != nil {
		return nil, err
	}
	return &callbacks{
		secure: secure, assignments: assignments, metrics: callbackMetrics,
		streams: map[int64]streamIdentity{}, deltaStreams: map[int64]streamIdentity{},
	}, nil
}

var _ serverv3.Callbacks = (*callbacks)(nil)

func (c *callbacks) OnStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	identity := ""
	if c.secure {
		var ok bool
		identity, ok = verifiedURIIdentity(ctx)
		if !ok {
			c.metrics.unauthorized.WithLabelValues(metricType(typeURL)).Inc()
			return status.Error(codes.PermissionDenied, unauthorizedMessage)
		}
	}
	c.mu.Lock()
	c.streams[streamID] = streamIdentity{uri: identity, typeURL: typeURL}
	c.mu.Unlock()
	c.metrics.activeStreams.WithLabelValues(metricType(typeURL)).Inc()
	return nil
}

func (c *callbacks) OnStreamClosed(streamID int64, _ *core.Node) {
	c.mu.Lock()
	stream, ok := c.streams[streamID]
	delete(c.streams, streamID)
	c.mu.Unlock()
	if ok {
		c.metrics.activeStreams.WithLabelValues(metricType(stream.typeURL)).Dec()
	}
}

func (c *callbacks) OnStreamRequest(streamID int64, req *discovery.DiscoveryRequest) error {
	typeURL := metricType(req.GetTypeUrl())
	c.metrics.requests.WithLabelValues(typeURL).Inc()
	if !c.authorized(c.streams, streamID, req.GetNode().GetId()) {
		c.metrics.unauthorized.WithLabelValues(typeURL).Inc()
		return status.Error(codes.PermissionDenied, unauthorizedMessage)
	}
	if req.GetResponseNonce() != "" {
		if req.GetErrorDetail() == nil {
			c.metrics.acks.WithLabelValues(typeURL).Inc()
		} else {
			c.metrics.nacks.WithLabelValues(typeURL).Inc()
		}
	}
	return nil
}

func (c *callbacks) OnStreamResponse(_ context.Context, _ int64, _ *discovery.DiscoveryRequest, response *discovery.DiscoveryResponse) {
	c.metrics.responses.WithLabelValues(metricType(response.GetTypeUrl())).Inc()
}

func (c *callbacks) OnDeltaStreamOpen(ctx context.Context, streamID int64, typeURL string) error {
	identity := ""
	if c.secure {
		var ok bool
		identity, ok = verifiedURIIdentity(ctx)
		if !ok {
			c.metrics.unauthorized.WithLabelValues(metricType(typeURL)).Inc()
			return status.Error(codes.PermissionDenied, unauthorizedMessage)
		}
	}
	c.mu.Lock()
	c.deltaStreams[streamID] = streamIdentity{uri: identity, typeURL: typeURL}
	c.mu.Unlock()
	c.metrics.activeStreams.WithLabelValues(metricType(typeURL)).Inc()
	return nil
}

func (c *callbacks) OnDeltaStreamClosed(streamID int64, _ *core.Node) {
	c.mu.Lock()
	stream, ok := c.deltaStreams[streamID]
	delete(c.deltaStreams, streamID)
	c.mu.Unlock()
	if ok {
		c.metrics.activeStreams.WithLabelValues(metricType(stream.typeURL)).Dec()
	}
}

func (c *callbacks) OnStreamDeltaRequest(streamID int64, req *discovery.DeltaDiscoveryRequest) error {
	typeURL := metricType(req.GetTypeUrl())
	c.metrics.requests.WithLabelValues(typeURL).Inc()
	if !c.authorized(c.deltaStreams, streamID, req.GetNode().GetId()) {
		c.metrics.unauthorized.WithLabelValues(metricType(typeURL)).Inc()
		return status.Error(codes.PermissionDenied, unauthorizedMessage)
	}
	return nil
}

func (c *callbacks) OnStreamDeltaResponse(_ int64, _ *discovery.DeltaDiscoveryRequest, response *discovery.DeltaDiscoveryResponse) {
	c.metrics.responses.WithLabelValues(metricType(response.GetTypeUrl())).Inc()
}

func (c *callbacks) OnFetchRequest(ctx context.Context, req *discovery.DiscoveryRequest) error {
	typeURL := metricType(req.GetTypeUrl())
	c.metrics.requests.WithLabelValues(typeURL).Inc()
	if !c.secure {
		return nil
	}
	identity, ok := verifiedURIIdentity(ctx)
	if !ok || !c.identityCanAccess(identity, req.GetNode().GetId()) {
		c.metrics.unauthorized.WithLabelValues(typeURL).Inc()
		return status.Error(codes.PermissionDenied, unauthorizedMessage)
	}
	return nil
}

func (c *callbacks) OnFetchResponse(_ *discovery.DiscoveryRequest, response *discovery.DiscoveryResponse) {
	c.metrics.responses.WithLabelValues(metricType(response.GetTypeUrl())).Inc()
}

func (c *callbacks) authorized(streams map[int64]streamIdentity, streamID int64, nodeID string) bool {
	if !c.secure {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	stream, ok := streams[streamID]
	if !ok {
		return false
	}
	if stream.nodeID == "" {
		if nodeID == "" || !c.identityCanAccessLocked(stream.uri, nodeID) {
			return false
		}
		stream.nodeID = nodeID
		streams[streamID] = stream
		return true
	}
	return nodeID == "" || nodeID == stream.nodeID
}

func (c *callbacks) identityCanAccess(identity, nodeID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.identityCanAccessLocked(identity, nodeID)
}

func (c *callbacks) identityCanAccessLocked(identity, nodeID string) bool {
	nodes, ok := c.assignments[identity]
	if !ok {
		return false
	}
	_, ok = nodes[nodeID]
	return ok
}

func (c *callbacks) assign(identity, nodeID string) error {
	if identity == "" || nodeID == "" {
		return fmt.Errorf("relay identity and node ID are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.assignments == nil {
		c.assignments = relayAssignments{}
	}
	for existingIdentity, nodes := range c.assignments {
		delete(nodes, nodeID)
		if len(nodes) == 0 {
			delete(c.assignments, existingIdentity)
		}
	}
	if c.assignments[identity] == nil {
		c.assignments[identity] = map[string]struct{}{}
	}
	c.assignments[identity][nodeID] = struct{}{}
	return nil
}

func (c *callbacks) removeNode(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for identity, nodes := range c.assignments {
		delete(nodes, nodeID)
		if len(nodes) == 0 {
			delete(c.assignments, identity)
		}
	}
}

func verifiedURIIdentity(ctx context.Context) (string, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", false
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		if pointer, pointerOK := p.AuthInfo.(*credentials.TLSInfo); pointerOK {
			tlsInfo = *pointer
			ok = true
		}
	}
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.PeerCertificates) == 0 {
		return "", false
	}
	leaf := tlsInfo.State.PeerCertificates[0]
	if len(leaf.URIs) != 1 {
		return "", false
	}
	// Require the authenticated leaf to be the leaf represented by a verified chain.
	if !tlsInfo.State.VerifiedChains[0][0].Equal(leaf) || tlsInfo.State.HandshakeComplete == false {
		return "", false
	}
	return leaf.URIs[0].String(), true
}
