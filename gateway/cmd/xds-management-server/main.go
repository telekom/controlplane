// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
// SPDX-License-Identifier: Apache-2.0

// Command xds-management-server runs the plaintext durable xDS POC service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoveryservice "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"google.golang.org/grpc"

	xdsapi "github.com/telekom/controlplane/gateway/internal/xds/api/v1"
	xdsserver "github.com/telekom/controlplane/gateway/internal/xds/server"
	"github.com/telekom/controlplane/gateway/internal/xds/storage"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "xDS management server failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	grpcAddress := flag.String("grpc-address", ":18000", "plaintext publication, status, and ADS listen address")
	healthAddress := flag.String("health-address", ":8080", "health and readiness HTTP listen address")
	databasePath := flag.String("database", "./data/xds.db", "SQLite database path")
	historyLimit := flag.Int("history-limit", storage.DefaultHistoryLimit, "retained generations per target")
	nodeMappings := flag.String("node-mappings", "", "comma-separated nodeID=targetID mappings")
	flag.Parse()

	mapping, err := parseMappings(*nodeMappings)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(ctx, *databasePath, *historyLimit)
	if err != nil {
		return fmt.Errorf("opening storage: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "closing xDS storage failed: %v\n", closeErr)
		}
	}()

	nodeMapping := xdsserver.NewNodeMapping(mapping)
	cache := cachev3.NewSnapshotCache(true, nodeMapping, nil)
	service, err := xdsserver.New(ctx, store, cache, nodeMapping)
	if err != nil {
		return fmt.Errorf("restoring management server: %w", err)
	}
	ads := service.ADS(ctx)
	grpcServer := grpc.NewServer()
	xdsapi.RegisterPublicationServiceServer(grpcServer, service)
	xdsapi.RegisterStatusServiceServer(grpcServer, service)
	discoveryservice.RegisterAggregatedDiscoveryServiceServer(grpcServer, ads)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, ads)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, ads)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, ads)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, ads)

	listener, err := net.Listen("tcp", *grpcAddress)
	if err != nil {
		return fmt.Errorf("listening for gRPC: %w", err)
	}
	healthServer := &http.Server{
		Addr:              *healthAddress,
		ReadHeaderTimeout: 5 * time.Second,
		Handler:           healthHandler(service),
	}
	errCh := make(chan error, 2)
	go func() { errCh <- grpcServer.Serve(listener) }()
	go func() { errCh <- healthServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		grpcStopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(grpcStopped)
		}()
		select {
		case <-grpcStopped:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down health server: %w", err)
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving: %w", err)
	}
}

func healthHandler(service *xdsserver.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(response http.ResponseWriter, _ *http.Request) {
		if !service.Ready() {
			http.Error(response, "not ready", http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusOK)
	})
	return mux
}

func parseMappings(value string) (map[string]string, error) {
	mappings := make(map[string]string)
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid node mapping %q, expected nodeID=targetID", entry)
		}
		nodeID, targetID := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if _, exists := mappings[nodeID]; exists {
			return nil, fmt.Errorf("duplicate node mapping for %q", nodeID)
		}
		mappings[nodeID] = targetID
	}
	return mappings, nil
}
