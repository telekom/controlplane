// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package envoy

import (
	"context"
	"fmt"
	"sort"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
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

	// RequireJWT declares that incoming tokens must be validated and their
	// issuer must be one of the given trusted issuers.
	RequireJWT(issuers []string)
	// AllowConsumers declares the consumer allow-list matched against the JWT
	// azp claim. An empty (non-nil) slice means deny-all.
	AllowConsumers(names []string)
}

var _ EnvoyFeatureBuilder = &Builder{}

// Builder is the go-control-plane counterpart to features.Builder. It reads the
// same Route/Consumer source fields, runs each enabled EnvoyFeature's
// IsUsed/Apply to accumulate intent, then renders and publishes an xDS
// snapshot.
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
	b.intent.trustedIssuers = issuers
}

func (b *Builder) AllowConsumers(names []string) {
	b.intent.accessControl = true
	if names == nil {
		names = []string{}
	}
	b.intent.allowConsumers = names
}

// Build runs the enabled features, then renders the accumulated intent into an
// xDS snapshot and publishes it.
//
// ponytail: single PocNodeID + whole-snapshot overwrite → one route at a time.
// Upgrade path: per-node accumulation of a node's routes.
func (b *Builder) Build(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("envoy.builder")
	if b.route == nil {
		return features.ErrNoRoute
	}
	log = log.WithValues("route", b.route.Name)

	for _, f := range sortFeatures(b.features) {
		if f.IsUsed(ctx, b) {
			log.V(1).Info("Applying feature", "name", f.Name())
			if err := f.Apply(ctx, b); err != nil {
				return fmt.Errorf("applying feature %s: %w", f.Name(), err)
			}
		} else {
			log.V(1).Info("Feature is not used", "name", f.Name())
		}
	}

	if b.upstream == nil {
		return fmt.Errorf("upstream is not set")
	}

	bundle, err := b.render()
	if err != nil {
		return fmt.Errorf("rendering xDS bundle for route %s: %w", b.route.Name, err)
	}

	if err := b.xds.SetSnapshotFor(ctx, PocNodeID, bundle); err != nil {
		return fmt.Errorf("publishing snapshot for route %s: %w", b.route.Name, err)
	}
	return nil
}

// BuildForConsumer is not yet implemented for the Envoy path. Consumer-scoped
// features (e.g. IpRestriction) come later in the migration order.
func (b *Builder) BuildForConsumer(ctx context.Context) error {
	return fmt.Errorf("BuildForConsumer is not implemented for the Envoy feature builder yet")
}

// render turns the accumulated intent into xDS resources.
func (b *Builder) render() (ResourceBundle, error) {
	routeName := b.route.Name
	clusterName := routeName

	filters, err := buildAccessControlFilters(b.intent)
	if err != nil {
		return ResourceBundle{}, err
	}

	r := b.route
	listener, _, err := buildListener(routeName, clusterName, filters, r.GetHostnames(), r.GetPaths(), b.upstream.GetPath())
	if err != nil {
		return ResourceBundle{}, err
	}

	cluster := buildCluster(clusterName, b.upstream.GetHostname(), uint32(b.upstream.GetPort()))
	clusters := []*clusterv3.Cluster{cluster}

	// remote_jwks needs a TLS cluster per issuer host in the same snapshot.
	jwksClusters, err := jwksClustersFor(b.intent.trustedIssuers)
	if err != nil {
		return ResourceBundle{}, err
	}
	clusters = append(clusters, jwksClusters...)

	// ponytail: RouteConfig is inlined in the HCM, so it is not published as a
	// standalone RDS resource. Upgrade path: switch to RDS (Rds{RouteConfigName})
	// and populate ResourceBundle.Routes when routes go dynamic.
	return ResourceBundle{
		Listeners: []*listenerv3.Listener{listener},
		Clusters:  clusters,
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
		return list[i].Priority() < list[j].Priority()
	})
	return list
}
