// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/internal/features"
	"github.com/telekom/controlplane/gateway/pkg/kong/client"
)

// KGatewayFeature mirrors envoy.EnvoyFeature: IsUsed reads the neutral base,
// Apply declares intent onto the kgateway builder. No features are wired yet
// (basic routing only), but the seam matches the Kong/Envoy pattern so features
// can be added the same way.
type KGatewayFeature interface {
	Name() gatewayv1.FeatureType
	Priority() int
	IsUsed(ctx context.Context, builder features.FeatureBuilder) bool
	Apply(ctx context.Context, builder KGatewayFeatureBuilder) error
}

// KGatewayFeatureBuilder is the kgateway-specific feature builder. It embeds the
// neutral features.FeatureBuilder base and adds feature registration and the
// build lifecycle, analog of features.FeaturesBuilder (Kong) and
// envoy.EnvoyFeatureBuilder (Envoy).
//
// The writer methods express intent (require JWT, allow consumers), not
// kgateway CR shapes; render turns the accumulated intent into a TrafficPolicy
// + GatewayExtension attached to the HTTPRoute. This mirrors the Envoy builder's
// intent seam so feature logic stays in one place.
type KGatewayFeatureBuilder interface {
	features.FeatureBuilder

	EnableFeature(f KGatewayFeature)

	// RequireJWT declares that incoming tokens must be validated and their
	// issuer must be one of the given trusted issuers.
	RequireJWT(issuers []string)
	// AllowConsumers declares the consumer allow-list matched against the JWT
	// azp claim. An empty (non-nil) slice means deny-all.
	AllowConsumers(names []string)
}

var _ KGatewayFeatureBuilder = &Builder{}

// Builder renders a Route into Gateway-API resources and applies them to the
// target cluster via the kgateway Client.
type Builder struct {
	cl Client

	route            *gatewayv1.Route
	consumer         *gatewayv1.Consumer
	gateway          *gatewayv1.Gateway
	allowedConsumers []*gatewayv1.ConsumeRoute
	upstream         client.Upstream

	features map[gatewayv1.FeatureType]KGatewayFeature

	// accumulated intent (set by feature Apply)
	intent accessControlIntent
}

// accessControlIntent is the backend-agnostic intent accumulated by the
// AccessControl feature's Apply, mirroring envoy.accessControlIntent.
type accessControlIntent struct {
	trustedIssuers []string
	// allowConsumers is nil when access control is disabled, and a (possibly
	// empty) slice otherwise. Empty slice => deny-all.
	allowConsumers []string
	accessControl  bool
}

// NewFeatureBuilder constructs the kgateway builder. Signature mirrors
// features.NewFeatureBuilder / envoy.NewFeatureBuilder but takes a kgateway
// Client (a controller-runtime client for the target cluster).
var NewFeatureBuilder = func(cl Client, route *gatewayv1.Route, consumer *gatewayv1.Consumer, gateway *gatewayv1.Gateway) KGatewayFeatureBuilder {
	return &Builder{
		cl:               cl,
		route:            route,
		consumer:         consumer,
		gateway:          gateway,
		allowedConsumers: []*gatewayv1.ConsumeRoute{},
		features:         map[gatewayv1.FeatureType]KGatewayFeature{},
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

// --- kgateway-specific ---

func (b *Builder) EnableFeature(f KGatewayFeature) {
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

// Build runs the enabled features (none yet), then renders the route into
// Gateway-API resources and applies them.
func (b *Builder) Build(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx).WithName("kgateway.builder")
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

	// Default the upstream to the route's first configured backend when no
	// feature set one, mirroring the envoy builder.
	// ponytail: first upstream only. Upgrade path: weighted backendRefs across
	// Upstreams[1:] when load balancing is needed.
	if b.upstream == nil {
		if len(b.route.Spec.Backend.Upstreams) == 0 {
			return fmt.Errorf("upstream is not set")
		}
		b.upstream = b.route.Spec.Backend.Upstreams[0]
	}

	bundle, err := b.render()
	if err != nil {
		return fmt.Errorf("rendering Gateway-API resources for route %s: %w", b.route.Name, err)
	}

	if err := b.cl.Apply(ctx, bundle); err != nil {
		return fmt.Errorf("applying Gateway-API resources for route %s: %w", b.route.Name, err)
	}
	return nil
}

// BuildForConsumer is not yet implemented for the kgateway path.
func (b *Builder) BuildForConsumer(ctx context.Context) error {
	return fmt.Errorf("BuildForConsumer is not implemented for the kgateway feature builder yet")
}

// render turns the route + upstream into a Backend + HTTPRoute.
func (b *Builder) render() (ResourceBundle, error) {
	return ResourceBundle{Objects: b.buildResources()}, nil
}

// sortFeatures orders features by ascending priority, matching the other
// builders.
func sortFeatures(m map[gatewayv1.FeatureType]KGatewayFeature) []KGatewayFeature {
	list := make([]KGatewayFeature, 0, len(m))
	for _, f := range m {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Priority() < list[j].Priority()
	})
	return list
}
