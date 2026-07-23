// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kgwv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// buildResources renders the Route into a kgateway Backend (the upstream target,
// which may be out-of-cluster) plus a Gateway-API HTTPRoute that matches on the
// route's hostnames and paths and forwards to that Backend.
//
// When the AccessControl feature accumulated intent (trusted issuers and/or a
// consumer allow-list), it also emits a TrafficPolicy (jwtAuth + rbac) and a
// GatewayExtension (the JWT providers), and attaches the TrafficPolicy to the
// HTTPRoute rule via an ExtensionRef filter. Everything is derived from the
// Route CR; nothing is hardcoded except the Keycloak JWKS path convention.
//
// ponytail: single static Backend, prefix path matches, no header/method
// matching. Upgrade path: weighted backendRefs (load balancing), exact/regex
// path matches, and more HTTPRouteFilters as features are ported.
func (b *Builder) buildResources() []client.Object {
	backend := b.buildBackend()

	var acFilters []gwapiv1.HTTPRouteFilter
	objs := []client.Object{backend}

	if b.intent.trustedIssuers != nil || b.intent.accessControl {
		ext := b.buildJWTGatewayExtension()
		policy := b.buildTrafficPolicy(ext.Name)
		objs = append(objs, ext, policy)
		objs = append(objs, b.buildJWKSBackends()...)
		acFilters = append(acFilters, extensionRefFilter(policy.Name))
	}

	route := b.buildHTTPRoute(backend.Name, acFilters)
	objs = append(objs, route)
	return objs
}

// buildBackend models the selected upstream as a kgateway static Backend. Using
// a Backend (rather than assuming an in-cluster Service) lets the upstream be an
// arbitrary host:port, including out-of-cluster targets.
func (b *Builder) buildBackend() *kgwv1alpha1.Backend {
	return &kgwv1alpha1.Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kgwv1alpha1.GroupVersion.String(),
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.route.Name,
			Namespace: b.route.Namespace,
		},
		Spec: kgwv1alpha1.BackendSpec{
			// Type is documented as deprecated/inferred, but the installed
			// CRD's CEL validation dereferences self.type in every branch, so
			// it must be set explicitly.
			Type: ptr.To(kgwv1alpha1.BackendTypeStatic),
			Static: &kgwv1alpha1.StaticBackend{
				Hosts: []kgwv1alpha1.Host{{
					Host: b.upstream.GetHostname(),
					Port: gwapiv1.PortNumber(b.upstream.GetPort()),
				}},
			},
		},
	}
}

// buildHTTPRoute renders the Gateway-API HTTPRoute. It binds to the kgateway
// Gateway via parentRefs and forwards matched traffic to the given Backend.
// filters, when non-empty, are attached to the rule (e.g. an ExtensionRef to a
// TrafficPolicy emitted by the AccessControl feature).
func (b *Builder) buildHTTPRoute(backendName string, filters []gwapiv1.HTTPRouteFilter) *gwapiv1.HTTPRoute {
	route := &gwapiv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: gwapiv1.GroupVersion.String(),
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.route.Name,
			Namespace: b.route.Namespace,
		},
		Spec: gwapiv1.HTTPRouteSpec{
			CommonRouteSpec: gwapiv1.CommonRouteSpec{
				// parentRefs binds this route to the kgateway Gateway. The
				// Gateway CR is assumed to carry the same name as our Gateway
				// resource.
				ParentRefs: []gwapiv1.ParentReference{{
					Name: gwapiv1.ObjectName(b.gateway.Name),
				}},
			},
			Rules: []gwapiv1.HTTPRouteRule{{
				Matches:     pathMatches(b.route.GetPaths()),
				Filters:     filters,
				BackendRefs: backendRefs(backendName),
			}},
		},
	}
	for _, h := range b.route.GetHostnames() {
		route.Spec.Hostnames = append(route.Spec.Hostnames, gwapiv1.Hostname(h))
	}
	return route
}

// pathMatches maps the route's path prefixes to HTTPRouteMatch entries. With no
// paths configured a single "/" PathPrefix matches all requests, mirroring the
// envoy builder's routeEntries default.
func pathMatches(paths []string) []gwapiv1.HTTPRouteMatch {
	if len(paths) == 0 {
		paths = []string{"/"}
	}
	matches := make([]gwapiv1.HTTPRouteMatch, 0, len(paths))
	for _, p := range paths {
		matches = append(matches, gwapiv1.HTTPRouteMatch{
			Path: &gwapiv1.HTTPPathMatch{
				Type:  ptr.To(gwapiv1.PathMatchPathPrefix),
				Value: ptr.To(p),
			},
		})
	}
	return matches
}

// backendRefs points the rule at the kgateway Backend CR. The Backend carries
// the host/port, so no port is set on the ref here.
func backendRefs(backendName string) []gwapiv1.HTTPBackendRef {
	return []gwapiv1.HTTPBackendRef{{
		BackendRef: gwapiv1.BackendRef{
			BackendObjectReference: gwapiv1.BackendObjectReference{
				Group: ptr.To(gwapiv1.Group(kgwv1alpha1.GroupName)),
				Kind:  ptr.To(gwapiv1.Kind("Backend")),
				Name:  gwapiv1.ObjectName(backendName),
			},
		},
	}}
}
