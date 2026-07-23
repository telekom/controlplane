// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	rbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	httprbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/rbac/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	matcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"

	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
)

var _ EnvoyFeature = &AccessControlFeature{}

// AccessControlFeature is the Envoy counterpart of
// feature.AccessControlFeature. It reads the same Route source fields and
// declares the equivalent intent (require JWT for trusted issuers; allow a
// consumer set, empty => deny-all) onto the Envoy builder.
type AccessControlFeature struct {
	priority int
}

// InstanceAccessControlFeature is the shared instance, mirroring the Kong
// feature.InstanceAccessControlFeature (priority 10).
var InstanceAccessControlFeature = &AccessControlFeature{priority: 10}

func (f *AccessControlFeature) Name() gatewayv1.FeatureType {
	return gatewayv1.FeatureTypeAccessControl
}

func (f *AccessControlFeature) Priority() int { return f.priority }

// IsUsed mirrors feature.AccessControlFeature.IsUsed
// (internal/features/feature/access_control.go:33-39): used when the route has
// trusted issuers.
func (f *AccessControlFeature) IsUsed(ctx context.Context, builder features.FeatureBuilder) bool {
	route, ok := builder.GetRoute()
	if !ok {
		return false
	}
	return len(route.GetTrustedIssuers()) > 0
}

// Apply mirrors feature.AccessControlFeature.Apply
// (internal/features/feature/access_control.go:41-77): require JWT for the
// route's trusted issuers, and (unless access control is disabled) build the
// consumer allow-list from default consumers plus route-matched allowed
// consumers. An empty allow-list means deny-all.
func (f *AccessControlFeature) Apply(ctx context.Context, builder EnvoyFeatureBuilder) error {
	route, ok := builder.GetRoute()
	if !ok {
		return features.ErrNoRoute
	}

	if len(route.GetTrustedIssuers()) > 0 {
		builder.RequireJWT(route.GetTrustedIssuers())
	}

	if route.Spec.Security.DisableAccessControl {
		return nil
	}

	seen := map[string]struct{}{}
	allowed := make([]string, 0)
	add := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		allowed = append(allowed, name)
	}

	for _, dc := range route.Spec.Security.DefaultConsumers {
		add(dc)
	}
	for _, cr := range builder.GetAllowedConsumers() {
		if cr.Spec.Route.Equals(route) {
			add(cr.Spec.ConsumerName)
		}
	}

	builder.AllowConsumers(allowed)
	return nil
}

// Canonical Envoy filter names — must match exactly.
const (
	filterJwtAuthn = "envoy.filters.http.jwt_authn"
	filterRBAC     = "envoy.filters.http.rbac"
	filterRouter   = "envoy.filters.http.router"
	filterHCM      = "envoy.filters.network.http_connection_manager"
)

// jwtPayloadMetadataKey is the metadata key under which jwt_authn stores the
// verified payload (JwtProvider.payload_in_metadata). RBAC then descends
// jwtPayloadMetadataKey -> "azp" to read the consumer identity.
const jwtPayloadMetadataKey = "jwt_payload"

// consumerMatchClaim mirrors the Kong path's plugin.ConsumerMatchClaim
// (pkg/kong/client/plugin/jwt.go:17): the consumer identity lives in "azp".
const consumerMatchClaim = "azp"

// buildAccessControlFilters emits the AccessControl HTTP filters (no terminal
// router): jwt_authn (issuer allowlist + azp -> metadata) when trusted issuers
// exist, then rbac (consumer allow-list, empty => deny-all) when access control
// is enabled. The caller (buildFilters) appends any other feature filters and
// the terminal router, preserving ordering.
//
// Behavioral parity with the Kong path:
//   - JWT validation is only added when trusted issuers exist
//     (access_control.go:46-50).
//   - If access control is disabled (intent.accessControl == false), no RBAC
//     filter is emitted (access_control.go:53).
//   - Empty allow-list => deny-all. Under RBAC action ALLOW an empty policy
//     map matches nothing, so all requests are denied — the native equivalent
//     of Kong's DenyAllGroup sentinel (access_control.go:73-75), without the
//     sentinel.
func buildAccessControlFilters(in accessControlIntent) ([]*hcmv3.HttpFilter, error) {
	filters := make([]*hcmv3.HttpFilter, 0, 2)

	if len(in.trustedIssuers) > 0 {
		f, err := mkFilter(filterJwtAuthn, buildJwtAuthn(in.trustedIssuers))
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}

	if in.accessControl {
		f, err := mkFilter(filterRBAC, buildRBAC(in.allowConsumers))
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}

	return filters, nil
}

// buildJwtAuthn creates one JwtProvider per trusted issuer (issuer allowlist:
// only tokens whose iss matches a listed provider validate) and requires any
// of them per route. The verified payload — including azp — is exported to
// dynamic metadata under jwtPayloadMetadataKey.
//
// Keys are fetched live via remote_jwks from each issuer's derived JWKS URI
// ({issuer}/protocol/openid-connect/certs, Keycloak convention), through a
// per-host TLS cluster named jwksClusterName(host). Those clusters are emitted
// by jwksClustersFor and must be present in the same snapshot.
func buildJwtAuthn(trustedIssuers []string) *jwtauthnv3.JwtAuthentication {
	providers := make(map[string]*jwtauthnv3.JwtProvider, len(trustedIssuers))
	providerNames := make([]*jwtauthnv3.JwtRequirement, 0, len(trustedIssuers))

	for i, issuer := range trustedIssuers {
		name := fmt.Sprintf("provider-%d", i)
		providers[name] = &jwtauthnv3.JwtProvider{
			Issuer:            issuer,
			PayloadInMetadata: jwtPayloadMetadataKey,
			ClaimToHeaders: []*jwtauthnv3.JwtClaimToHeader{{
				HeaderName: rateLimitConsumerHeader,
				ClaimName:  consumerMatchClaim,
			}},
			JwksSourceSpecifier: &jwtauthnv3.JwtProvider_RemoteJwks{
				RemoteJwks: &jwtauthnv3.RemoteJwks{
					HttpUri: &corev3.HttpUri{
						Uri:              jwksURIFromIssuer(issuer),
						HttpUpstreamType: &corev3.HttpUri_Cluster{Cluster: jwksClusterName(issuerHost(issuer))},
						Timeout:          durationpb.New(5 * time.Second),
					},
					// ponytail: fixed 5m cache; tune if key rotation is faster.
					CacheDuration: durationpb.New(5 * time.Minute),
				},
			},
		}
		providerNames = append(providerNames, &jwtauthnv3.JwtRequirement{
			RequiresType: &jwtauthnv3.JwtRequirement_ProviderName{ProviderName: name},
		})
	}

	// Any trusted issuer is acceptable; a token from an unlisted issuer has no
	// matching provider and is rejected.
	requirement := providerNames[0]
	if len(providerNames) > 1 {
		requirement = &jwtauthnv3.JwtRequirement{
			RequiresType: &jwtauthnv3.JwtRequirement_RequiresAny{
				RequiresAny: &jwtauthnv3.JwtRequirementOrList{Requirements: providerNames},
			},
		}
	}

	return &jwtauthnv3.JwtAuthentication{
		Providers: providers,
		Rules: []*jwtauthnv3.RequirementRule{{
			Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"}},
			RequirementType: &jwtauthnv3.RequirementRule_Requires{
				Requires: requirement,
			},
		}},
	}
}

// buildRBAC creates an ALLOW policy with one principal per allowed consumer,
// matching the azp claim from jwt_authn dynamic metadata. An empty allow-list
// yields an empty policy map => deny-all (Kong DenyAllGroup equivalent).
func buildRBAC(allowedConsumers []string) *httprbacv3.RBAC {
	policies := map[string]*rbacv3.Policy{}

	if len(allowedConsumers) > 0 {
		principals := make([]*rbacv3.Principal, 0, len(allowedConsumers))
		for _, consumer := range allowedConsumers {
			principals = append(principals, azpPrincipal(consumer))
		}
		policies["consumer-allowlist"] = &rbacv3.Policy{
			Principals:  principals,
			Permissions: []*rbacv3.Permission{{Rule: &rbacv3.Permission_Any{Any: true}}},
		}
	}

	return &httprbacv3.RBAC{
		Rules: &rbacv3.RBAC{
			Action:   rbacv3.RBAC_ALLOW,
			Policies: policies,
		},
	}
}

// azpPrincipal matches a single consumer name against the azp claim inside the
// jwt_authn payload metadata (namespace -> jwtPayloadMetadataKey -> azp).
func azpPrincipal(consumerName string) *rbacv3.Principal {
	return &rbacv3.Principal{
		Identifier: &rbacv3.Principal_Metadata{
			Metadata: &matcherv3.MetadataMatcher{
				Filter: filterJwtAuthn,
				Path: []*matcherv3.MetadataMatcher_PathSegment{
					{Segment: &matcherv3.MetadataMatcher_PathSegment_Key{Key: jwtPayloadMetadataKey}},
					{Segment: &matcherv3.MetadataMatcher_PathSegment_Key{Key: consumerMatchClaim}},
				},
				Value: &matcherv3.ValueMatcher{
					MatchPattern: &matcherv3.ValueMatcher_StringMatch{
						StringMatch: &matcherv3.StringMatcher{
							MatchPattern: &matcherv3.StringMatcher_Exact{Exact: consumerName},
						},
					},
				},
			},
		},
	}
}

// mkFilter wraps an HTTP filter config message into an HttpFilter.
func mkFilter(name string, msg proto.Message) (*hcmv3.HttpFilter, error) {
	typed, err := anypb.New(msg)
	if err != nil {
		return nil, fmt.Errorf("marshalling %s config: %w", name, err)
	}
	return &hcmv3.HttpFilter{
		Name:       name,
		ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: typed},
	}, nil
}

// buildListener assembles the HCM listener carrying the given HTTP filter
// chain and an inline route to the given cluster. hostnames and paths select
// the request (RT-02 / RT-01); upstreamPath, when non-trivial, is prepended to
// the forwarded path (RV-04). vhostPerFilterConfig, when non-empty, is attached
// as the VirtualHost typed_per_filter_config (keyed by filter name) — the
// generic seam features use to attach per-route filter overrides without this
// listener code knowing which feature they belong to.
//
// ponytail: fixed 0.0.0.0:10000 bind + inline RouteConfig; POC single-listener.

func buildListener(routeName, clusterName string, filters []*hcmv3.HttpFilter, hostnames, paths []string, upstreamPath string, vhostPerFilterConfig map[string]*anypb.Any, rateLimits []*routev3.RateLimit) (*listenerv3.Listener, *routev3.RouteConfiguration, error) {
	routes := routeEntries(clusterName, paths, upstreamPath)
	for _, route := range routes {
		route.GetRoute().RateLimits = rateLimits
	}
	vhost := &routev3.VirtualHost{
		Name:    routeName,
		Domains: routeDomains(hostnames),
		Routes:  routes,
	}
	if len(vhostPerFilterConfig) > 0 {
		vhost.TypedPerFilterConfig = vhostPerFilterConfig
	}

	routeConfig := &routev3.RouteConfiguration{
		Name:         routeName,
		VirtualHosts: []*routev3.VirtualHost{vhost},
	}

	manager := &hcmv3.HttpConnectionManager{
		StatPrefix:     "ingress_http",
		HttpFilters:    filters,
		RouteSpecifier: &hcmv3.HttpConnectionManager_RouteConfig{RouteConfig: routeConfig},
	}
	hcmAny, err := anypb.New(manager)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling hcm config: %w", err)
	}

	listener := &listenerv3.Listener{
		Name: routeName,
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{SocketAddress: &corev3.SocketAddress{
			Address:       "0.0.0.0",
			PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 10000},
		}}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{{
				Name:       filterHCM,
				ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: hcmAny},
			}},
		}},
	}
	return listener, routeConfig, nil
}

// buildCluster creates a STRICT_DNS cluster to the upstream host:port.
//
// ponytail: static STRICT_DNS, no TLS/EDS. Upgrade path: EDS + upstream TLS.
func buildCluster(name, host string, port uint32) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 name,
		ConnectTimeout:       durationpb.New(5 * time.Second),
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints: []*endpointv3.LocalityLbEndpoints{{
				LbEndpoints: []*endpointv3.LbEndpoint{{
					HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
						Address: &corev3.Address{Address: &corev3.Address_SocketAddress{SocketAddress: &corev3.SocketAddress{
							Address:       host,
							PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
						}}},
					}},
				}},
			}},
		},
	}
}

// jwksURIFromIssuer derives the JWKS endpoint from a Keycloak issuer URL by the
// standard convention {issuer}/protocol/openid-connect/certs.
//
// ponytail: Keycloak-specific path. Upgrade path: read jwks_uri from each
// issuer's /.well-known/openid-configuration instead of hardcoding it.
func jwksURIFromIssuer(issuer string) string {
	return strings.TrimRight(issuer, "/") + "/protocol/openid-connect/certs"
}

// issuerHost extracts the host from an issuer URL, for naming the JWKS cluster
// and setting the TLS SNI. Falls back to the raw issuer if it does not parse.
func issuerHost(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return issuer
	}
	return u.Hostname()
}

// jwksClusterName is the cluster name used by remote_jwks for a given issuer
// host. Deduplicated by host so issuers sharing a host share one cluster.
func jwksClusterName(host string) string {
	return "jwks_" + host
}

// jwksClustersFor returns the TLS clusters backing remote_jwks for the given
// issuers, one per distinct host, deduplicated. Empty when there are none.
func jwksClustersFor(trustedIssuers []string) ([]*clusterv3.Cluster, error) {
	seen := map[string]struct{}{}
	clusters := make([]*clusterv3.Cluster, 0, len(trustedIssuers))
	for _, issuer := range trustedIssuers {
		host := issuerHost(issuer)
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		c, err := buildJwksCluster(host)
		if err != nil {
			return nil, err
		}
		clusters = append(clusters, c)
	}
	return clusters, nil
}

// buildJwksCluster creates a STRICT_DNS cluster to host:443 with an upstream
// TLS context (SNI = host). The issuer is served by a public CA, so Envoy's
// default trust bundle validates it — no validation_context needed.
//
// ponytail: fixed 443 + default trust. Upgrade path: configurable port and an
// explicit CA bundle for internal issuers.
func buildJwksCluster(host string) (*clusterv3.Cluster, error) {
	tlsCtx, err := anypb.New(&tlsv3.UpstreamTlsContext{Sni: host})
	if err != nil {
		return nil, fmt.Errorf("marshalling jwks tls context: %w", err)
	}
	c := buildCluster(jwksClusterName(host), host, 443)
	c.TransportSocket = &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: tlsCtx},
	}
	return c, nil
}
