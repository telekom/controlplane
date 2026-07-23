// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/protobuf/types/known/timestamppb"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
)

const maxErrorDetailLength = 512

type observationStore interface {
	RecordObservation(context.Context, string, *xdsapi.DeliveryObservation) error
}

type responseDetails struct {
	targetID   string
	nodeID     string
	typeURL    string
	generation uint64
}

type callbacks struct {
	serverv3.CallbackFuncs
	mapping     NodeMapping
	store       observationStore
	connections *connections
	mu          sync.Mutex
	responses   map[string]responseDetails
}

func newCallbacks(mapping NodeMapping, store observationStore, connections *connections) *callbacks {
	callbacks := &callbacks{
		mapping: mapping, store: store, connections: connections,
		responses: make(map[string]responseDetails),
	}
	callbacks.StreamRequestFunc = callbacks.onStreamRequest
	callbacks.StreamResponseFunc = callbacks.onStreamResponse
	callbacks.StreamClosedFunc = callbacks.onStreamClosed
	return callbacks
}

func (c *callbacks) onStreamRequest(streamID int64, request *discoveryv3.DiscoveryRequest) error {
	if request == nil {
		return fmt.Errorf("xDS request is required")
	}
	nodeID := request.GetNode().GetId()
	if nodeID == "" {
		nodeID = c.connections.streamNode(streamID)
	}
	if nodeID == "" {
		return fmt.Errorf("xDS node ID is required")
	}
	if _, ok := c.mapping.target(nodeID); !ok {
		return fmt.Errorf("xDS node %q is not mapped", nodeID)
	}
	if current := c.connections.streamNode(streamID); current != "" && current != nodeID {
		return fmt.Errorf("xDS stream cannot change node identity")
	}
	c.connections.connect(streamID, nodeID)
	if request.GetResponseNonce() == "" {
		return nil
	}

	c.mu.Lock()
	responseKey := nonceKey(streamID, request.GetResponseNonce())
	details, ok := c.responses[responseKey]
	delete(c.responses, responseKey)
	c.mu.Unlock()
	if !ok || details.nodeID != nodeID || details.typeURL != request.GetTypeUrl() {
		return nil
	}

	state := xdsapi.DeliveryState_DELIVERY_STATE_ACK
	errorDetail := ""
	if request.GetErrorDetail() != nil {
		state = xdsapi.DeliveryState_DELIVERY_STATE_NACK
		errorDetail = sanitizeError(request.GetErrorDetail().GetMessage())
	}
	observation := &xdsapi.DeliveryObservation{
		NodeId: nodeID, TypeUrl: details.typeURL, Generation: details.generation,
		State: state, Nonce: request.GetResponseNonce(), ErrorDetail: errorDetail,
		ObservedAt: timestamppb.New(time.Now().UTC()),
	}
	if err := c.store.RecordObservation(context.Background(), details.targetID, observation); err != nil {
		return fmt.Errorf("recording xDS delivery: %w", err)
	}
	return nil
}

func (c *callbacks) onStreamResponse(
	_ context.Context,
	streamID int64,
	request *discoveryv3.DiscoveryRequest,
	response *discoveryv3.DiscoveryResponse,
) {
	if request == nil || response == nil || response.GetNonce() == "" {
		return
	}
	nodeID := request.GetNode().GetId()
	if nodeID == "" {
		nodeID = c.connections.streamNode(streamID)
	}
	targetID, ok := c.mapping.target(nodeID)
	if !ok {
		return
	}
	generation, err := strconv.ParseUint(response.GetVersionInfo(), 10, 64)
	if err != nil {
		return
	}
	c.mu.Lock()
	c.responses[nonceKey(streamID, response.GetNonce())] = responseDetails{
		targetID: targetID, nodeID: nodeID, typeURL: response.GetTypeUrl(), generation: generation,
	}
	c.mu.Unlock()
}

func nonceKey(streamID int64, nonce string) string {
	return strconv.FormatInt(streamID, 10) + ":" + nonce
}

func (c *callbacks) onStreamClosed(streamID int64, node *corev3.Node) {
	c.connections.disconnect(streamID, node.GetId())
	prefix := strconv.FormatInt(streamID, 10) + ":"
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.responses {
		if strings.HasPrefix(key, prefix) {
			delete(c.responses, key)
		}
	}
}

func sanitizeError(detail string) string {
	detail = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, detail)
	if len(detail) > maxErrorDetailLength {
		detail = detail[:maxErrorDetailLength]
		for !utf8.ValidString(detail) {
			detail = detail[:len(detail)-1]
		}
	}
	return detail
}
