// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

// Command xdsdemo is a standalone local harness for the Envoy feature builder.
// It builds an xDS snapshot for a single fake Route (no operator, no k8s) and
// serves it over ADS so a real Envoy can connect, fetch config, and proxy to a
// dummy upstream. See gateway/cmd/xdsdemo/README.md for the docker-compose run.
//
// ponytail: hardcoded fake route + upstream, no trusted issuers (avoids the
// empty-JWKS placeholder that would reject every request). Exercises the
// listener -> cluster -> upstream path end to end.
package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features/envoy"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
)

func main() {
	addr := flag.String("addr", ":18000", "xDS ADS gRPC listen address")
	upstreamURL := flag.String("upstream", "http://upstream:80", "upstream URL Envoy proxies to")
	issuers := flag.String("issuers", envOr("ISSUERS", "https://iris-distcp1-dataplane1.dev.dhei.telekom.de/auth/realms/rover"), "comma-separated trusted token issuers (empty => no JWT/RBAC)")
	consumers := flag.String("consumers", envOr("CONSUMERS", "dev-luminary"), "comma-separated allowed consumers matched against the token azp claim")
	defaultScopes := flag.String("default-scopes", envOr("DEFAULT_SCOPES", ""), "comma-separated OAuth2 scopes added to every consumer's LMS token")
	consumerScopes := flag.String("consumer-scopes", envOr("CONSUMER_SCOPES", "dev-luminary=read"), "per-consumer scopes as name=scope[ scope...] entries, comma-separated (e.g. \"foo=read,bar=write admin\")")
	flag.Parse()

	zl, _ := zap.NewDevelopment()
	log := zapr.NewLogger(zl)
	ctx := logr.NewContext(context.Background(), log)
	// LMS reads the environment from context, like the Kong path.
	ctx = contextutil.WithEnv(ctx, envOr("ENVIRONMENT", "poc"))

	// Shared cache: the builder writes snapshots into it, the ADS server reads.
	cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, nil)
	xds := envoy.NewXdsClient(cache)

	// Fake inputs: a named route carrying the trusted issuers and allowed
	// consumers, plus the upstream. With issuers set, AccessControl emits the
	// JWT (remote_jwks) + RBAC (azp allow-list) filter chain.
	route := &gatewayv1.Route{}
	route.Name = "demo-route"
	route.Spec.Type = gatewayv1.RouteTypePrimary
	route.Spec.Hostnames = []string{"demo-route.local"}
	route.Spec.Paths = []string{"/get"}
	route.Spec.Security.TrustedIssuers = splitCSV(*issuers)
	route.Spec.Security.DefaultConsumers = splitCSV(*consumers)
	route.Spec.Security.RealmName = envOr("REALM", "poc-realm")

	// CustomScopes source fields (same as the Kong path): route default scopes
	// live on the route's M2M security; per-consumer scopes live on each
	// allowed ConsumeRoute's M2M security. The LMS issuer selects the scope set
	// by the verified azp claim and adds it to the minted token.
	if def := splitCSV(*defaultScopes); len(def) > 0 {
		route.Spec.Security.M2M = &gatewayv1.Machine2MachineAuthentication{Scopes: def}
	}
	allowedConsumers := consumeRoutesFor(route, *consumerScopes)

	builder := envoy.NewFeatureBuilder(xds, route, nil, &gatewayv1.Gateway{})
	builder.EnableFeature(envoy.InstanceAccessControlFeature)
	builder.EnableFeature(envoy.InstanceLastMileSecurityFeature)
	builder.EnableFeature(envoy.InstanceCustomScopesFeature)
	builder.AddAllowedConsumers(allowedConsumers...)
	builder.SetUpstream(client.NewUpstreamOrDie(*upstreamURL))

	if err := builder.Build(ctx); err != nil {
		log.Error(err, "building xDS snapshot")
		os.Exit(1)
	}
	log.Info("Snapshot published; starting ADS server",
		"node", envoy.PocNodeID, "addr", *addr, "upstream", *upstreamURL)

	srv := serverv3.NewServer(ctx, cache, nil)
	grpcServer := grpc.NewServer()
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, srv)
	clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, srv)
	endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, srv)
	listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, srv)
	routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, srv)

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Error(err, "listen")
		os.Exit(1)
	}

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Error(err, "serve")
		os.Exit(1)
	}
}

// envOr returns the env var value if set (non-empty), else the fallback.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// splitCSV splits a comma-separated flag into trimmed, non-empty values.
// Returns nil for an empty string so the feature stays disabled.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// consumeRoutesFor parses the per-consumer scopes flag ("name=scope[ scope...]"
// entries, comma-separated) into ConsumeRoutes carrying M2M scopes, which is
// the source field CustomScopesFeature reads via GetAllowedConsumers. Entries
// without "=" or with empty scopes are skipped.
func consumeRoutesFor(route *gatewayv1.Route, consumerScopes string) []*gatewayv1.ConsumeRoute {
	out := make([]*gatewayv1.ConsumeRoute, 0)
	for _, entry := range splitCSV(consumerScopes) {
		name, scopeStr, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		scopes := strings.Fields(scopeStr)
		if len(scopes) == 0 {
			continue
		}
		cr := &gatewayv1.ConsumeRoute{}
		cr.Spec.ConsumerName = name
		cr.Spec.Route.Name = route.Name
		cr.Spec.Route.Namespace = route.Namespace
		cr.Spec.Security = &gatewayv1.ConsumeRouteSecurity{
			M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{Scopes: scopes},
		}
		out = append(out, cr)
	}
	return out
}
