// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extauthzv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	upstreamsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"

	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ EnvoyFeature = &LastMileSecurityFeature{}

// LastMileSecurityFeature is the Envoy counterpart of
// feature.LastMileSecurityFeature (internal/features/feature/last_mile_security.go).
// The Kong path repoints the upstream to the local Jumper sidecar, which mints
// the last-mile token. In Envoy, an external issuer service mints and signs the
// token; this feature declares that intent, and the builder emits an ext_authz
// filter that calls the issuer and injects the returned Authorization header.
type LastMileSecurityFeature struct {
	priority int
}

// InstanceLastMileSecurityFeature mirrors the Kong
// feature.InstanceLastMileSecurityFeature priority (100,
// internal/features/feature/last_mile_security.go:27).
var InstanceLastMileSecurityFeature = &LastMileSecurityFeature{priority: 100}

func (f *LastMileSecurityFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeLastMileSecurity
}

func (f *LastMileSecurityFeature) Priority() int { return f.priority }

// IsUsed mirrors feature.LastMileSecurityFeature.IsUsed
// (internal/features/feature/last_mile_security.go:38-45): used when the route
// is neither pass-through nor a failover route.
func (f *LastMileSecurityFeature) IsUsed(ctx context.Context, builder features.FeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return !route.Spec.PassThrough && route.Spec.Traffic.Failover == nil
}

// Apply reads the same source fields as the Kong path's Apply
// (internal/features/feature/last_mile_security.go:47-63): the route's realm
// name and the request environment. It declares LMS token minting intent; the
// builder renders the ext_authz filter + issuer cluster.
func (f *LastMileSecurityFeature) Apply(ctx context.Context, builder EnvoyFeatureBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}
	// Same source as the Kong path (feature/last_mile_security.go:53,63): the
	// environment from context and the realm name from the route security spec.
	env := contextutil.EnvFromContextOrDie(ctx)
	builder.RequireLMSToken(route.Spec.Security.RealmName, env)
	return nil
}

// Canonical Envoy ext_authz filter name and the issuer cluster name (must match
// the cluster emitted by buildLMSIssuerCluster).
const (
	filterExtAuthz   = "envoy.filters.http.ext_authz"
	lmsIssuerCluster = "lms_ext_authz"
	lmsIssuerHost    = "lms-issuer"
	lmsIssuerPort    = 9002
)

// buildFilters assembles the ordered HTTP filter chain:
//
//	jwt_authn -> rbac -> ext_authz -> router
//
// ext_authz runs after jwt_authn so the verified consumer-token payload is
// available, and before the terminal router so the minted Authorization header
// reaches the upstream. Router is always last (terminal).
//
// Ordering/citations (envoy-expert): ext_authz after jwt_authn
// https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto#envoy-v3-api-field-extensions-filters-http-ext-authz-v3-extauthz-metadata-context-namespaces ;
// router must be terminal/last
// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter
func buildFilters(ac accessControlIntent, lms lmsIntent) ([]*hcmv3.HttpFilter, error) {
	filters, err := buildAccessControlFilters(ac)
	if err != nil {
		return nil, err
	}

	if lms.enabled {
		f, err := mkFilter(filterExtAuthz, buildExtAuthz())
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}

	router, err := mkFilter(filterRouter, &routerv3.Router{})
	if err != nil {
		return nil, err
	}
	filters = append(filters, router)

	return filters, nil
}

// buildExtAuthz configures the ext_authz HTTP filter to call the LMS issuer
// over gRPC. The issuer mints the token and returns it in the CheckResponse
// OkHttpResponse.headers, which Envoy applies to the upstream request (no
// allow-list needed on the gRPC path). FailureModeAllow is left false: if the
// issuer is unreachable, the request is rejected (fail closed).
//
// Citations (envoy-expert, confirmed against envoy@v1.37.0):
//   - message: envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto#extensions-filters-http-ext-authz-v3-extauthz
//   - grpc_service.envoy_grpc.cluster_name
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/grpc_service.proto#envoy-v3-api-field-config-core-v3-grpcservice-envoygrpc-cluster-name
//   - failure_mode_allow (default false = fail closed)
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto#envoy-v3-api-field-extensions-filters-http-ext-authz-v3-extauthz-failure-mode-allow
//   - OkHttpResponse.headers applied upstream on the gRPC path
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/auth/v3/external_auth.proto#envoy-v3-api-msg-service-auth-v3-okhttpresponse
func buildExtAuthz() *extauthzv3.ExtAuthz {
	return &extauthzv3.ExtAuthz{
		Services: &extauthzv3.ExtAuthz_GrpcService{
			GrpcService: &corev3.GrpcService{
				TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: lmsIssuerCluster},
				},
				Timeout: durationpb.New(1 * time.Second),
			},
		},
		FailureModeAllow: false,
		// Give the issuer the verified JWT payload written by jwt_authn.
		MetadataContextNamespaces: []string{filterJwtAuthn},
	}
}

// lmsVhostPerFilterConfig returns the VirtualHost typed_per_filter_config map
// for the LMS ext_authz filter, carrying the route's realm and environment as
// context_extensions. These arrive in the gRPC Check at
// AttributeContext.context_extensions, letting the issuer mint a realm-scoped
// token. Returns nil when LMS is disabled (no override attached).
//
// CustomScopes folds its resolved scope map into the same context_extensions:
//   - "defaultScopes": space-separated scopes for consumers without an entry.
//   - "consumerScopes": a JSON object {consumerName: "space separated scopes"}.
//
// context_extensions is map<string,string> and opaque to Envoy, so the
// per-consumer sub-map is JSON-encoded into a single value; the issuer parses
// it and selects the scope set by the incoming clientId/azp. Scopes are only
// attached when LMS is enabled (the LMS token is what carries them).
//
// Citations (envoy-expert):
//   - ExtAuthzPerRoute.CheckSettings.context_extensions (map<string,string>)
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_authz/v3/ext_authz.proto#envoy-v3-api-field-extensions-filters-http-ext-authz-v3-checksettings-context-extensions
//   - context_extensions is opaque ("maps to the internal opaque context"),
//     so JSON-in-a-value is passed through unparsed
//     https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/auth/v3/attribute_context.proto#envoy-v3-api-field-service-auth-v3-attributecontext-context-extensions
func lmsVhostPerFilterConfig(lms lmsIntent, scopes customScopesIntent) (map[string]*anypb.Any, error) {
	if !lms.enabled {
		return nil, nil
	}

	ctxExt := map[string]string{
		"realm":       lms.realm,
		"environment": lms.environment,
	}

	if scopes.defaultScopes != "" {
		ctxExt["defaultScopes"] = scopes.defaultScopes
	}
	if len(scopes.perConsumer) > 0 {
		encoded, err := json.Marshal(scopes.perConsumer)
		if err != nil {
			return nil, fmt.Errorf("marshalling per-consumer scopes: %w", err)
		}
		ctxExt["consumerScopes"] = string(encoded)
	}

	perRoute, err := anypb.New(&extauthzv3.ExtAuthzPerRoute{
		Override: &extauthzv3.ExtAuthzPerRoute_CheckSettings{
			CheckSettings: &extauthzv3.CheckSettings{
				ContextExtensions: ctxExt,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling lms ext_authz per-route config: %w", err)
	}
	return map[string]*anypb.Any{filterExtAuthz: perRoute}, nil
}

// buildLMSIssuerCluster creates the http2 (gRPC) STRICT_DNS cluster the
// ext_authz filter calls. Mirrors the static lms_ext_authz cluster in the
// xdsdemo bootstrap.
//
// ponytail: static STRICT_DNS to lms-issuer:9002, plaintext h2c. Upgrade path:
// EDS + upstream TLS to the issuer.
func buildLMSIssuerCluster() (*clusterv3.Cluster, error) {
	c := buildCluster(lmsIssuerCluster, lmsIssuerHost, lmsIssuerPort)
	// gRPC requires HTTP/2 to the upstream.
	h2, err := anypb.New(&upstreamsv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamsv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling lms issuer http2 options: %w", err)
	}
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": h2,
	}
	return c, nil
}
