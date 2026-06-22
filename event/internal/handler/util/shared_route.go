// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"context"
	"net/url"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

// DeleteRouteIfExists fetches a Route by ObjectRef and deletes it if found.
// Returns nil if the Route is already gone (NotFound).
func DeleteRouteIfExists(ctx context.Context, ref *ctypes.ObjectRef) error {
	if ref == nil {
		return nil
	}

	c := cclient.ClientFromContextOrDie(ctx)

	route := &gatewayapi.Route{}
	err := c.Get(ctx, ref.K8s(), route)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to get Route %q", ref.String())
	}

	if err := c.Delete(ctx, route); err != nil {
		return errors.Wrapf(err, "failed to delete Route %q", ref.String())
	}

	return nil
}

// parseUpstream parses a raw URL string into a gateway Upstream.
func parseUpstream(rawUrl string) (gatewayapi.Upstream, error) {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return gatewayapi.Upstream{}, errors.Wrapf(err, "failed to parse URL %s", rawUrl)
	}
	return gatewayapi.Upstream{
		Scheme:   u.Scheme,
		Hostname: u.Hostname(),
		Port:     int32(gatewayapi.GetPortOrDefaultFromScheme(u)),
		Path:     u.Path,
	}, nil
}

type Options struct {
	Owner         metav1.Object
	IsProxyTarget bool

	// TrustedIssuers is the list of trusted token issuers for this route.
	// For primary routes: includes the zone's IDP issuer + LMS issuers from proxy zones.
	// For proxy routes: includes the source zone's LMS issuer (mesh-client authentication).
	TrustedIssuers []string

	// RealmName is the identity realm name used by the Jumper sidecar for
	// Last-Mile-Security token issuance. Typically equals the environment name.
	RealmName string
}

type Option func(*Options)

func WithOwner(owner metav1.Object) Option {
	return func(o *Options) {
		o.Owner = owner
	}
}

func WithProxyTarget(isProxyTarget bool) Option {
	return func(o *Options) {
		o.IsProxyTarget = isProxyTarget
	}
}

// WithTrustedIssuers sets the trusted token issuers for the route.
// These issuers are used by the gateway's JWT plugin to validate incoming tokens.
func WithTrustedIssuers(issuers []string) Option {
	return func(o *Options) {
		o.TrustedIssuers = issuers
	}
}

// WithRealmName sets the identity realm name on the route's Security.
// The Jumper sidecar uses this to determine which realm to use for LMS token issuance.
func WithRealmName(realmName string) Option {
	return func(o *Options) {
		o.RealmName = realmName
	}
}

func (o *Options) apply(ctx context.Context, route *gatewayapi.Route) error {
	c := cclient.ClientFromContextOrDie(ctx)
	if o.Owner != nil {
		return controllerutil.SetControllerReference(o.Owner, route, c.Scheme())
	}
	return nil
}

// applySecurity sets TrustedIssuers and RealmName on the route's Security from options.
func (o *Options) applySecurity(route *gatewayapi.Route) {
	if len(o.TrustedIssuers) > 0 {
		route.Spec.Security.TrustedIssuers = o.TrustedIssuers
	}
	if o.RealmName != "" {
		route.Spec.Security.RealmName = o.RealmName
	}
}

// RouteDownstreamURL constructs the external-facing URL from a Route's
// Hostnames[0] and Paths[0]. Returns empty string if either slice is empty.
func RouteDownstreamURL(route *gatewayapi.Route) string {
	if len(route.Spec.Hostnames) == 0 || len(route.Spec.Paths) == 0 {
		return ""
	}
	return "https://" + route.Spec.Hostnames[0] + route.Spec.Paths[0]
}
