// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"net"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Server is the xDS-serving component of the control plane.
//
// Unlike the reconcilers, it runs on EVERY replica (not just the leader) so that
// each replica can serve its own connected Envoy pods from the shared in-memory
// snapshot cache. It therefore opts out of leader election via
// NeedLeaderElection() == false and is registered on the manager with
// mgr.Add(...). K8s writes (status, finalizers) stay on the leader-gated
// reconcilers; this component only serves an in-memory projection, so running it
// everywhere causes no API-server contention.
//
// ponytail: the cache is currently populated by the leader-gated route handler
// only. The all-replicas informer-driven snapshot builder (so every replica
// populates its own cache) is deferred until the EnvoyFeatureBuilder produces
// real resources — see internal/features/envoy/README.md ("CP HA & scale").
type Server struct {
	cache cachev3.Cache
	addr  string
}

var (
	_ manager.Runnable               = (*Server)(nil)
	_ manager.LeaderElectionRunnable = (*Server)(nil)
)

// NewServer constructs the ADS xDS server serving from the shared cache on addr.
func NewServer(cache cachev3.Cache, addr string) *Server {
	return &Server{cache: cache, addr: addr}
}

// NeedLeaderElection implements [manager.LeaderElectionRunnable].
//
// Always false: the xDS server runs on all replicas, not only the leader.
func (*Server) NeedLeaderElection() bool {
	return false
}

// Start implements [manager.Runnable]. It serves the ADS gRPC API until ctx is
// cancelled, then stops gracefully.
func (s *Server) Start(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.xds-server")

	srv := serverv3.NewServer(ctx, s.cache, newServerCallbacks(log))
	grpcServer := grpc.NewServer()
	discoveryv3.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)

	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening on %q: %w", s.addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("Envoy xDS server listening", "addr", s.addr)
		errCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		log.Info("Envoy xDS server stopping")
		grpcServer.GracefulStop()
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("xDS server serve: %w", err)
		}
		return nil
	}
}

// newServerCallbacks returns callbacks that log stream lifecycle at debug level.
func newServerCallbacks(log logr.Logger) serverv3.CallbackFuncs {
	return serverv3.CallbackFuncs{
		StreamOpenFunc: func(_ context.Context, id int64, typeURL string) error {
			log.V(1).Info("xDS stream opened", "streamID", id, "typeURL", typeURL)
			return nil
		},
		StreamClosedFunc: func(id int64, node *corev3.Node) {
			log.V(1).Info("xDS stream closed", "streamID", id, "nodeID", node.GetId())
		},
	}
}
