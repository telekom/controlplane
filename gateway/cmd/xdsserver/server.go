// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Package xdsserver serves Envoy xDS with optional relay mTLS authorization.
package xdsserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config configures the xDS listener. Security is disabled only when all four
// security file options are empty.
type Config struct {
	Address               string
	ServerCertificateFile string
	ServerKeyFile         string
	ClientCAFile          string
	RelayAssignmentsFile  string
	Registry              prometheus.Registerer
}

// Server is a running xDS gRPC server.
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	callbacks  *callbacks
}

// Start starts an xDS server and arranges graceful shutdown when ctx is done.
func Start(ctx context.Context, cache cachev3.SnapshotCache, cfg Config) (*Server, error) {
	secure, err := cfg.validateSecurity()
	if err != nil {
		return nil, err
	}

	assignments := relayAssignments(nil)
	var grpcOptions []grpc.ServerOption
	if secure {
		assignments, err = loadRelayAssignments(cfg.RelayAssignmentsFile)
		if err != nil {
			return nil, fmt.Errorf("loading relay assignments: %w", err)
		}
		reloader, err := newTLSReloader(cfg.ServerCertificateFile, cfg.ServerKeyFile, cfg.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("loading xDS TLS configuration: %w", err)
		}
		grpcOptions = append(grpcOptions, grpc.Creds(credentials.NewTLS(&tls.Config{
			MinVersion:         tls.VersionTLS12,
			GetConfigForClient: reloader.getConfigForClient,
		})))
	}

	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("listening for xDS: %w", err)
	}

	callbacks, err := newCallbacks(secure, assignments, cfg.Registry)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("registering xDS metrics: %w", err)
	}
	xds := serverv3.NewServer(ctx, cache, callbacks)
	grpcServer := grpc.NewServer(grpcOptions...)
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, xds)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, xds)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, xds)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, xds)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, xds)

	server := &Server{grpcServer: grpcServer, listener: listener, callbacks: callbacks}
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()
	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			logr.FromContextOrDiscard(ctx).Error(serveErr, "xDS server stopped")
		}
	}()
	return server, nil
}

// Address returns the bound listener address.
func (s *Server) Address() net.Addr {
	return s.listener.Addr()
}

// Assign authorizes one relay URI SAN for one stable node ID.
func (s *Server) Assign(identity, nodeID string) error { return s.callbacks.assign(identity, nodeID) }

// RemoveNode revokes a node from every relay assignment.
func (s *Server) RemoveNode(nodeID string) { s.callbacks.removeNode(nodeID) }

func (cfg Config) validateSecurity() (bool, error) {
	values := []string{cfg.ServerCertificateFile, cfg.ServerKeyFile, cfg.ClientCAFile, cfg.RelayAssignmentsFile}
	configured := 0
	for _, value := range values {
		if value != "" {
			configured++
		}
	}
	if configured == 0 {
		return false, nil
	}
	if configured != len(values) {
		return false, fmt.Errorf("server certificate, server key, client CA, and relay assignments must all be configured")
	}
	return true, nil
}
