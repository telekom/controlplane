// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

// Package server implements durable xDS publication, restoration, status, and ADS callbacks.
package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
	"github.com/telekom/controlplane/gateway/internal/xds/storage"
)

var supportedTypeURLs = []string{
	"type.googleapis.com/envoy.config.listener.v3.Listener",
	"type.googleapis.com/envoy.config.route.v3.RouteConfiguration",
	"type.googleapis.com/envoy.config.cluster.v3.Cluster",
	"type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment",
}

type durableStore interface {
	Activate(context.Context, *xdsapi.Bundle) (storage.Activation, error)
	LoadActive(context.Context) (map[string]storage.ActiveBundle, error)
	Active(context.Context, string) (storage.ActiveBundle, bool, error)
	RecordObservation(context.Context, string, *xdsapi.DeliveryObservation) error
	Observations(context.Context, string) ([]*xdsapi.DeliveryObservation, error)
}

// Service implements both internal management RPCs and supplies ADS callbacks.
type Service struct {
	xdsapi.UnimplementedPublicationServiceServer
	xdsapi.UnimplementedStatusServiceServer
	store         durableStore
	cache         cachev3.SnapshotCache
	mapping       NodeMapping
	connections   *connections
	callbacks     *callbacks
	publicationMu sync.Mutex
	activationMu  sync.RWMutex
	activated     map[string]uint64
	ready         atomic.Bool
}

// New restores every active bundle before constructing the ADS server.
func New(ctx context.Context, store durableStore, cache cachev3.SnapshotCache, mapping NodeMapping) (*Service, error) {
	if store == nil || cache == nil {
		return nil, fmt.Errorf("store and cache are required")
	}
	connections := newConnections()
	service := &Service{
		store: store, cache: cache, mapping: mapping, connections: connections,
		activated: make(map[string]uint64),
	}
	service.callbacks = newCallbacks(mapping, store, connections)

	active, err := store.LoadActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading active bundles: %w", err)
	}
	for targetID, persisted := range active {
		snapshot, validationErrors := Validate(persisted.Bundle, strconv.FormatUint(persisted.Generation, 10))
		if len(validationErrors) > 0 {
			return nil, fmt.Errorf("active bundle for target %q is invalid: %w", targetID, validationErrors)
		}
		if err := cache.SetSnapshot(ctx, targetID, snapshot); err != nil {
			return nil, fmt.Errorf("restoring active snapshot for target %q: %w", targetID, err)
		}
		service.setActivated(targetID, persisted.Generation)
	}
	service.ready.Store(true)
	return service, nil
}

// ADS creates the go-control-plane server with mapping-enforcing callbacks.
func (s *Service) ADS(ctx context.Context) serverv3.Server {
	return serverv3.NewServer(ctx, s.cache, s.callbacks)
}

// Ready reports whether durable restoration completed.
func (s *Service) Ready() bool {
	return s.ready.Load()
}

// PublishBundle validates, persists, activates, then updates the disposable cache.
func (s *Service) PublishBundle(
	ctx context.Context,
	request *xdsapi.PublishBundleRequest,
) (*xdsapi.PublishBundleResponse, error) {
	s.publicationMu.Lock()
	defer s.publicationMu.Unlock()

	bundle := request.GetBundle()
	_, validationErrors := Validate(bundle, "candidate")
	if len(validationErrors) > 0 {
		grpcStatus := status.New(codes.InvalidArgument, validationErrors.Error())
		withDetails, err := addStatusDetails(grpcStatus, validationErrors)
		if err == nil {
			grpcStatus = withDetails
		}
		return nil, grpcStatus.Err()
	}

	activation, err := s.store.Activate(ctx, bundle)
	if errors.Is(err, storage.ErrGenerationConflict) {
		return nil, status.Error(codes.AlreadyExists, "publisher generation conflicts with existing content")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "persisting and activating bundle failed")
	}
	if !activation.Active {
		return &xdsapi.PublishBundleResponse{
			PersistedGeneration: activation.Generation,
			Digest:              bundle.Digest,
			Idempotent:          true,
			Activated:           false,
		}, nil
	}
	snapshot, validationErrors := Validate(bundle, strconv.FormatUint(activation.Generation, 10))
	if len(validationErrors) > 0 {
		return nil, status.Error(codes.Internal, "activated bundle could not be rebuilt")
	}
	if err := s.cache.SetSnapshot(ctx, bundle.TargetId, snapshot); err != nil {
		s.setActivated(bundle.TargetId, 0)
		return nil, status.Error(codes.Internal, "bundle persisted but cache activation failed")
	}
	s.setActivated(bundle.TargetId, activation.Generation)

	return &xdsapi.PublishBundleResponse{
		PersistedGeneration: activation.Generation,
		Digest:              bundle.Digest,
		Idempotent:          activation.Idempotent,
		Activated:           activation.Active,
	}, nil
}

// GetStatus returns durable activation and convergence over currently connected mapped nodes.
func (s *Service) GetStatus(
	ctx context.Context,
	request *xdsapi.GetStatusRequest,
) (*xdsapi.GetStatusResponse, error) {
	if request.GetTargetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "target ID is required")
	}
	active, exists, err := s.store.Active(ctx, request.GetTargetId())
	if err != nil {
		return nil, status.Error(codes.Internal, "reading active bundle failed")
	}
	if !exists {
		return nil, status.Error(codes.NotFound, "target has no active bundle")
	}
	observations, err := s.store.Observations(ctx, request.GetTargetId())
	if err != nil {
		return nil, status.Error(codes.Internal, "reading delivery observations failed")
	}

	connected := make([]string, 0)
	for _, nodeID := range s.mapping.nodes(request.GetTargetId()) {
		if s.connections.connected(nodeID) {
			connected = append(connected, nodeID)
		}
	}
	sort.Strings(connected)
	converged := converged(active.Generation, connected, observations)
	return &xdsapi.GetStatusResponse{
		TargetId: request.GetTargetId(), PersistedGeneration: active.Generation,
		Digest: active.Bundle.Digest, Activated: s.isActivated(request.GetTargetId(), active.Generation),
		Converged:        converged,
		ConnectedNodeIds: connected, Observations: observations,
	}, nil
}

func (s *Service) setActivated(targetID string, generation uint64) {
	s.activationMu.Lock()
	defer s.activationMu.Unlock()
	s.activated[targetID] = generation
}

func (s *Service) isActivated(targetID string, generation uint64) bool {
	s.activationMu.RLock()
	defer s.activationMu.RUnlock()
	return s.activated[targetID] == generation
}

func converged(generation uint64, connected []string, observations []*xdsapi.DeliveryObservation) bool {
	acks := make(map[string]map[string]bool, len(connected))
	for _, observation := range observations {
		if observation.Generation != generation || observation.State != xdsapi.DeliveryState_DELIVERY_STATE_ACK {
			continue
		}
		if acks[observation.NodeId] == nil {
			acks[observation.NodeId] = make(map[string]bool)
		}
		acks[observation.NodeId][observation.TypeUrl] = true
	}
	for _, nodeID := range connected {
		for _, typeURL := range supportedTypeURLs {
			if !acks[nodeID][typeURL] {
				return false
			}
		}
	}
	return true
}
