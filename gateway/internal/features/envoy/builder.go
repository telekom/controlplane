// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/go-logr/logr"

	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

// EnvoyFeature mirrors the Kong features.Feature pattern for the Envoy path.
// IsUsed reads the neutral base (same inputs as the Kong features), while Apply
// declares backend-agnostic intent onto the Envoy builder instead of mutating
// Kong plugin structs.
type EnvoyFeature interface {
	Name() gatewayv1.FeatureType
	Priority() int
	IsUsed(ctx context.Context, builder features.FeatureBuilder) bool
	Apply(ctx context.Context, builder EnvoyFeatureBuilder) error
}

// EnvoyFeatureBuilder is the Envoy-specific feature builder. It embeds the
// neutral features.FeatureBuilder base and adds the Envoy intent-writers that
// feature Apply implementations use, plus feature registration and the build
// lifecycle.
//
// The writer methods express intent (require JWT, allow consumers), not Envoy
// proto shapes; Build renders the accumulated intent into xDS resources. This
// keeps feature logic in one place, exactly as the Kong Apply does.
type EnvoyFeatureBuilder interface {
	features.FeatureBuilder

	EnableFeature(f EnvoyFeature)
	Build(context.Context) error
	BuildForConsumer(context.Context) error
	Render(context.Context) (ResourceBundle, error)

	// RequireJWT declares that incoming tokens must be validated and their
	// issuer must be one of the given trusted issuers.
	RequireJWT(issuers []string)
	// AllowConsumers declares the consumer allow-list matched against the JWT
	// azp claim. An empty (non-nil) slice means deny-all.
	AllowConsumers(names []string)
	// RequireLMSToken declares that a LastMileSecurity token must be minted per
	// request by the external issuer (ext_authz) and injected upstream. realm
	// and environment identify the token context (which signing realm applies).
	RequireLMSToken(realm, environment string)
}

var _ EnvoyFeatureBuilder = &Builder{}

// Builder is the go-control-plane counterpart to features.Builder. It reads the
// same Route/Consumer source fields and renders xDS resources without side effects.
type Builder struct {
	xds XdsClient

	route            *gatewayv1.Route
	consumer         *gatewayv1.Consumer
	gateway          *gatewayv1.Gateway
	allowedConsumers []*gatewayv1.ConsumeRoute
	upstream         client.Upstream

	features map[gatewayv1.FeatureType]EnvoyFeature

	// accumulated intent (set by feature Apply)
	intent accessControlIntent
	lms    lmsIntent
}

// lmsIntent is the intent accumulated by the LastMileSecurity feature's Apply:
// mint an LMS token per request via the external issuer and inject it upstream.
type lmsIntent struct {
	enabled     bool
	realm       string
	environment string
}

// accessControlIntent is the backend-agnostic intent accumulated by the
// AccessControl feature's Apply.
type accessControlIntent struct {
	trustedIssuers []string
	// allowConsumers is nil when access control is disabled, and a (possibly
	// empty) slice otherwise. Empty slice => deny-all.
	allowConsumers []string
	accessControl  bool
}

// NewFeatureBuilder constructs the Envoy builder. Signature mirrors
// features.NewFeatureBuilder but takes an XdsClient in place of the KongClient.
var NewFeatureBuilder = func(xds XdsClient, route *gatewayv1.Route, consumer *gatewayv1.Consumer, gateway *gatewayv1.Gateway) EnvoyFeatureBuilder {
	return &Builder{
		xds:              xds,
		route:            route,
		consumer:         consumer,
		gateway:          gateway,
		allowedConsumers: []*gatewayv1.ConsumeRoute{},
		features:         map[gatewayv1.FeatureType]EnvoyFeature{},
	}
}

// --- neutral base (features.FeatureBuilder) ---

func (b *Builder) GetRoute() (*gatewayv1.Route, bool) {
	if b.route == nil {
		return nil, false
	}
	return b.route, true
}

func (b *Builder) GetConsumer() (*gatewayv1.Consumer, bool) {
	if b.consumer == nil {
		return nil, false
	}
	return b.consumer, true
}

func (b *Builder) GetGateway() *gatewayv1.Gateway {
	return b.gateway
}

func (b *Builder) GetAllowedConsumers() []*gatewayv1.ConsumeRoute {
	return b.allowedConsumers
}

func (b *Builder) AddAllowedConsumers(consumers ...*gatewayv1.ConsumeRoute) {
	b.allowedConsumers = append(b.allowedConsumers, consumers...)
}

func (b *Builder) SetUpstream(upstream client.Upstream) {
	b.upstream = upstream
}

// --- Envoy-specific ---

func (b *Builder) EnableFeature(f EnvoyFeature) {
	b.features[f.Name()] = f
}

func (b *Builder) RequireJWT(issuers []string) {
	b.intent.trustedIssuers = append([]string(nil), issuers...)
	sort.Strings(b.intent.trustedIssuers)
}

func (b *Builder) AllowConsumers(names []string) {
	b.intent.accessControl = true
	if names == nil {
		names = []string{}
	}
	b.intent.allowConsumers = append([]string(nil), names...)
	sort.Strings(b.intent.allowConsumers)
}

func (b *Builder) RequireLMSToken(realm, environment string) {
	b.lms.enabled = true
	b.lms.realm = realm
	b.lms.environment = environment
}

// Build runs the enabled features, then renders the accumulated intent into an
// xDS snapshot and publishes it.
//
// ponytail: single PocNodeID + whole-snapshot overwrite → one route at a time.
// Upgrade path: per-node accumulation of a node's routes.
func (b *Builder) Build(ctx context.Context) error {
	bundle, err := b.Render(ctx)
	if err != nil {
		return err
	}
	if b.xds == nil {
		// Gateway reconciliation publishes the complete aggregate. Route
		// reconciliation only validates that this individual route renders.
		return nil
	}
	if err := b.xds.Publish(ctx, bundle.Target, bundle.Source, &bundle); err != nil {
		return fmt.Errorf("publishing snapshot for route %s: %w", b.route.Name, err)
	}
	return nil
}

// Render applies enabled features and returns a deterministic bundle without publishing it.
func (b *Builder) Render(ctx context.Context) (ResourceBundle, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.builder")
	if b.route == nil {
		return ResourceBundle{}, features.ErrNoRoute
	}
	log = log.WithValues("route", b.route.Name)

	for _, f := range sortFeatures(b.features) {
		if f.IsUsed(ctx, b) {
			log.V(1).Info("Applying feature", "name", f.Name())
			if err := f.Apply(ctx, b); err != nil {
				return ResourceBundle{}, fmt.Errorf("applying feature %s: %w", f.Name(), err)
			}
		} else {
			log.V(1).Info("Feature is not used", "name", f.Name())
		}
	}

	if b.upstream == nil {
		return ResourceBundle{}, fmt.Errorf("upstream is not set")
	}

	bundle, err := b.render()
	if err != nil {
		return ResourceBundle{}, fmt.Errorf("rendering xDS bundle for route %s: %w", b.route.Name, err)
	}
	bundle.Sort()
	return bundle, nil
}

// BuildForConsumer is not yet implemented for the Envoy path. Consumer-scoped
// features (e.g. IpRestriction) come later in the migration order.
func (b *Builder) BuildForConsumer(ctx context.Context) error {
	return fmt.Errorf("BuildForConsumer is not implemented for the Envoy feature builder yet")
}

// render turns the accumulated intent into xDS resources.
func (b *Builder) render() (ResourceBundle, error) {
	routeName := resourceName(b.route.Namespace, b.route.Name)
	clusterName := routeName

	filters, err := buildFilters(b.intent, b.lms)
	if err != nil {
		return ResourceBundle{}, err
	}

	r := b.route

	perFilterConfig, err := namedAccessControlPerFilterConfig(b.intent, routeName)
	if err != nil {
		return ResourceBundle{}, err
	}
	lmsConfig, err := lmsVhostPerFilterConfig(b.lms)
	if err != nil {
		return ResourceBundle{}, err
	}
	for name, config := range lmsConfig {
		perFilterConfig[name] = config
	}

	vhost := &routev3.VirtualHost{
		Name: routeName, Domains: routeDomains(r.GetHostnames()),
		Routes:               routeEntries(clusterName, r.GetPaths(), b.upstream.GetPath()),
		TypedPerFilterConfig: perFilterConfig,
	}
	routeConfigName := routeName + "-routes"
	listener, err := buildListener(routeName+"-listener", routeConfigName, filters)
	if err != nil {
		return ResourceBundle{}, err
	}

	cluster := buildEDSCluster(clusterName)
	clusters := []*clusterv3.Cluster{cluster}

	// remote_jwks needs a TLS cluster per issuer host in the same snapshot.
	jwksClusters, err := jwksClustersFor(b.intent.trustedIssuers)
	if err != nil {
		return ResourceBundle{}, err
	}
	clusters = append(clusters, jwksClusters...)

	// ext_authz needs the LMS issuer cluster (http2/gRPC) in the same snapshot.
	if b.lms.enabled {
		issuerCluster, issuerErr := buildLMSIssuerCluster()
		if issuerErr != nil {
			return ResourceBundle{}, issuerErr
		}
		clusters = append(clusters, issuerCluster)
	}

	port, err := upstreamPort(b.upstream)
	if err != nil {
		return ResourceBundle{}, err
	}
	return ResourceBundle{
		Target:    b.targetIdentity(),
		Source:    b.sourceMetadata(),
		Listeners: []*listenerv3.Listener{listener},
		Clusters:  clusters,
		Routes:    []*routev3.RouteConfiguration{buildRouteConfiguration(routeConfigName, []*routev3.VirtualHost{vhost})},
		Endpoints: []*endpointv3.ClusterLoadAssignment{
			buildEndpoint(clusterName, b.upstream.GetHostname(), port),
		},
	}, nil
}

func (b *Builder) targetIdentity() TargetIdentity {
	if b.gateway != nil && b.gateway.Name != "" {
		return TargetIdentity{Namespace: b.gateway.Namespace, Name: b.gateway.Name, UID: b.gateway.UID}
	}
	if b.route != nil && !b.route.Spec.GatewayRef.IsEmpty() {
		return TargetIdentity{
			Namespace: b.route.Spec.GatewayRef.Namespace, Name: b.route.Spec.GatewayRef.Name, UID: b.route.Spec.GatewayRef.UID,
		}
	}
	if b.route != nil {
		return TargetIdentity{Namespace: b.route.Namespace, Name: b.route.Name, UID: b.route.UID}
	}
	return TargetIdentity{}
}

func (b *Builder) sourceMetadata() SourceMetadata {
	metadata := SourceMetadata{}
	if b.gateway != nil && b.gateway.Name != "" {
		metadata.Resources = append(metadata.Resources, sourceReference("Gateway", b.gateway))
	}
	if b.route != nil {
		metadata.Resources = append(metadata.Resources, sourceReference("Route", b.route))
	}
	return metadata
}

type routeRender struct {
	name     string
	vhost    *routev3.VirtualHost
	cluster  *clusterv3.Cluster
	endpoint *endpointv3.ClusterLoadAssignment
	clusters []*clusterv3.Cluster
	access   accessControlIntent
	lms      lmsIntent
}

func (b *Builder) renderRoute(ctx context.Context) (routeRender, error) {
	if b.route == nil {
		return routeRender{}, features.ErrNoRoute
	}
	for _, f := range sortFeatures(b.features) {
		if f.IsUsed(ctx, b) {
			if err := f.Apply(ctx, b); err != nil {
				return routeRender{}, fmt.Errorf("applying feature %s: %w", f.Name(), err)
			}
		}
	}
	if b.upstream == nil {
		return routeRender{}, fmt.Errorf("upstream is not set")
	}
	port, err := upstreamPort(b.upstream)
	if err != nil {
		return routeRender{}, err
	}
	name := resourceName(b.route.Namespace, b.route.Name)
	perFilterConfig, err := namedAccessControlPerFilterConfig(b.intent, name)
	if err != nil {
		return routeRender{}, err
	}
	lmsConfig, err := lmsVhostPerFilterConfig(b.lms)
	if err != nil {
		return routeRender{}, err
	}
	for filterName, config := range lmsConfig {
		perFilterConfig[filterName] = config
	}
	additionalClusters, err := jwksClustersFor(b.intent.trustedIssuers)
	if err != nil {
		return routeRender{}, err
	}
	if b.lms.enabled {
		issuerCluster, err := buildLMSIssuerCluster()
		if err != nil {
			return routeRender{}, err
		}
		additionalClusters = append(additionalClusters, issuerCluster)
	}
	return routeRender{
		name: name,
		vhost: &routev3.VirtualHost{
			Name: name, Domains: routeDomains(b.route.GetHostnames()),
			Routes:               routeEntries(name, b.route.GetPaths(), b.upstream.GetPath()),
			TypedPerFilterConfig: perFilterConfig,
		},
		cluster:  buildEDSCluster(name),
		endpoint: buildEndpoint(name, b.upstream.GetHostname(), port),
		clusters: additionalClusters, access: b.intent, lms: b.lms,
	}, nil
}

// sortFeatures orders features by ascending priority (lower applies earlier),
// matching the Kong builder's sortFeatures.
func sortFeatures(m map[gatewayv1.FeatureType]EnvoyFeature) []EnvoyFeature {
	list := make([]EnvoyFeature, 0, len(m))
	for _, f := range m {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority() != list[j].Priority() {
			return list[i].Priority() < list[j].Priority()
		}
		return list[i].Name() < list[j].Name()
	})
	return list
}

func resourceName(namespace, name string) string {
	return strings.TrimPrefix(namespace+"/"+name, "/")
}

func upstreamPort(upstream client.Upstream) (uint32, error) {
	port := upstream.GetPort()
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("upstream port %d is outside the valid range", port)
	}
	return uint32(port), nil
}
