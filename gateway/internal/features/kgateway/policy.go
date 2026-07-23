// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kgateway

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	kgwv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
)

// consumerMatchClaim mirrors the envoy path's consumerMatchClaim
// (internal/features/envoy/access_control.go:123) and the Kong
// plugin.ConsumerMatchClaim: the consumer identity lives in the "azp" claim.
const consumerMatchClaim = "azp"

// jwtMetadataPayloadPath is the CEL accessor for the verified JWT payload that
// the jwt_authn filter stores in dynamic metadata. RBAC matchExpressions read
// claims from here, e.g. <payload>['azp'].
const jwtMetadataPayloadPath = "metadata.filter_metadata['envoy.filters.http.jwt_authn']['payload']"

// buildJWTGatewayExtension renders a GatewayExtension carrying one JWTProvider
// per trusted issuer from the Route. The TrafficPolicy's jwtAuth.extensionRef
// points at it. Every field is derived from Route.Spec.Security.TrustedIssuers.
//
// ponytail: the JWKS URL and its Backend host are derived from the issuer via
// the Keycloak convention {issuer}/protocol/openid-connect/certs, exactly as
// the envoy path's jwksURIFromIssuer. Upgrade path: read jwks_uri from each
// issuer's /.well-known/openid-configuration instead.
func (b *Builder) buildJWTGatewayExtension() *kgwv1alpha1.GatewayExtension {
	providers := make([]kgwv1alpha1.NamedJWTProvider, 0, len(b.intent.trustedIssuers))
	for i, issuer := range b.intent.trustedIssuers {
		host := issuerHost(issuer)
		providers = append(providers, kgwv1alpha1.NamedJWTProvider{
			Name: fmt.Sprintf("provider-%d", i),
			JWTProvider: kgwv1alpha1.JWTProvider{
				Issuer: issuer,
				JWKS: kgwv1alpha1.JWKS{
					RemoteJWKS: &kgwv1alpha1.RemoteJWKS{
						URL: jwksURIFromIssuer(issuer),
						// kgateway requires a BackendRef for the JWKS server; a
						// static Backend to the issuer host:443 is emitted by
						// buildJWKSBackend and referenced here by name.
						BackendRef: gwapiv1.BackendObjectReference{
							Group: ptr.To(gwapiv1.Group(kgwv1alpha1.GroupName)),
							Kind:  ptr.To(gwapiv1.Kind("Backend")),
							Name:  gwapiv1.ObjectName(jwksBackendName(host)),
						},
					},
				},
			},
		})
	}

	return &kgwv1alpha1.GatewayExtension{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kgwv1alpha1.GroupVersion.String(),
			Kind:       "GatewayExtension",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.route.Name + "-jwt",
			Namespace: b.route.Namespace,
		},
		Spec: kgwv1alpha1.GatewayExtensionSpec{
			Type: ptr.To(kgwv1alpha1.GatewayExtensionTypeJWT),
			JWT: &kgwv1alpha1.JWT{
				Providers: providers,
			},
		},
	}
}

// buildTrafficPolicy renders the TrafficPolicy that attaches to the HTTPRoute
// and carries jwtAuth (referencing the given GatewayExtension) and, when access
// control is enabled, rbac. All values come from the accumulated intent.
func (b *Builder) buildTrafficPolicy(extensionName string) *kgwv1alpha1.TrafficPolicy {
	spec := kgwv1alpha1.TrafficPolicySpec{
		TargetRefs: []shared.LocalPolicyTargetReferenceWithSectionName{{
			LocalPolicyTargetReference: shared.LocalPolicyTargetReference{
				Group: gwapiv1.GroupName,
				Kind:  "HTTPRoute",
				Name:  gwapiv1.ObjectName(b.route.Name),
			},
		}},
	}

	if len(b.intent.trustedIssuers) > 0 {
		spec.JWTAuth = &kgwv1alpha1.JWTAuth{
			ExtensionRef: &shared.NamespacedObjectReference{
				Name: gwapiv1.ObjectName(extensionName),
			},
		}
	}

	if b.intent.accessControl {
		spec.RBAC = &shared.Authorization{
			Action: shared.AuthorizationPolicyActionAllow,
			Policy: shared.AuthorizationPolicy{
				MatchExpressions: allowListExpressions(b.intent.allowConsumers),
			},
		}
	}

	return &kgwv1alpha1.TrafficPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kgwv1alpha1.GroupVersion.String(),
			Kind:       "TrafficPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.route.Name,
			Namespace: b.route.Namespace,
		},
		Spec: spec,
	}
}

// allowListExpressions turns the consumer allow-list into RBAC matchExpressions
// (CEL). Each allowed consumer becomes an equality against the azp claim in the
// jwt_authn payload metadata; the policy matches when any of them is true
// (kgateway OR-s the expressions).
//
// Empty allow-list => deny-all: matchExpressions requires at least one entry,
// so a single "false" expression is emitted, matching nothing (the kgateway
// equivalent of the envoy empty-policy / Kong DenyAllGroup deny-all).
func allowListExpressions(allowedConsumers []string) []shared.CELExpression {
	if len(allowedConsumers) == 0 {
		return []shared.CELExpression{"false"}
	}
	exprs := make([]shared.CELExpression, 0, len(allowedConsumers))
	for _, consumer := range allowedConsumers {
		exprs = append(exprs, shared.CELExpression(
			fmt.Sprintf("%s['%s'] == '%s'", jwtMetadataPayloadPath, consumerMatchClaim, consumer),
		))
	}
	return exprs
}

// buildJWKSBackends renders one static Backend per distinct issuer JWKS host,
// the targets of the RemoteJWKS BackendRefs. The host and port are derived from
// the issuer URL scheme (https => 443, http => 80, or an explicit :port). For
// https endpoints it also emits a BackendConfigPolicy that originates TLS
// (system CA) toward that Backend — a static Backend is plaintext by default,
// and pointing plaintext at an https (:443) issuer resets the JWKS fetch and
// stalls jwt_authn on any request that carries a token. Deduplicated by name so
// issuers sharing an endpoint share one Backend + policy.
func (b *Builder) buildJWKSBackends() []client.Object {
	seen := map[string]struct{}{}
	objs := make([]client.Object, 0, len(b.intent.trustedIssuers))
	for _, issuer := range b.intent.trustedIssuers {
		host, port, tls := issuerEndpoint(issuer)
		name := jwksBackendName(host)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		objs = append(objs, &kgwv1alpha1.Backend{
			TypeMeta: metav1.TypeMeta{
				APIVersion: kgwv1alpha1.GroupVersion.String(),
				Kind:       "Backend",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: b.route.Namespace,
			},
			Spec: kgwv1alpha1.BackendSpec{
				Type: ptr.To(kgwv1alpha1.BackendTypeStatic),
				Static: &kgwv1alpha1.StaticBackend{
					Hosts: []kgwv1alpha1.Host{{
						Host: host,
						Port: gwapiv1.PortNumber(port),
					}},
				},
			},
		})

		if tls {
			objs = append(objs, b.buildJWKSBackendTLSPolicy(name, host))
		}
	}
	return objs
}

// buildJWKSBackendTLSPolicy originates TLS toward the given JWKS Backend using
// the system CA bundle, with SNI set to the issuer host. Required for https
// JWKS issuers; a static Backend has no inline TLS knob, so TLS is configured
// via a BackendConfigPolicy targeting the Backend.
func (b *Builder) buildJWKSBackendTLSPolicy(backendName, host string) *kgwv1alpha1.BackendConfigPolicy {
	return &kgwv1alpha1.BackendConfigPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: kgwv1alpha1.GroupVersion.String(),
			Kind:       "BackendConfigPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      backendName,
			Namespace: b.route.Namespace,
		},
		Spec: kgwv1alpha1.BackendConfigPolicySpec{
			TargetRefs: []shared.LocalPolicyTargetReference{{
				Group: gwapiv1.Group(kgwv1alpha1.GroupName),
				Kind:  "Backend",
				Name:  gwapiv1.ObjectName(backendName),
			}},
			TLS: &kgwv1alpha1.TLS{
				WellKnownCACertificates: ptr.To(gwapiv1.WellKnownCACertificatesType(gwapiv1.WellKnownCACertificatesSystem)),
				Sni:                     ptr.To(host),
			},
		},
	}
}

// extensionRefFilter builds the HTTPRoute ExtensionRef filter that attaches the
// given TrafficPolicy to the rule, mirroring the kgateway YAML's
// filters[].extensionRef entry.
func extensionRefFilter(policyName string) gwapiv1.HTTPRouteFilter {
	return gwapiv1.HTTPRouteFilter{
		Type: gwapiv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gwapiv1.LocalObjectReference{
			Group: gwapiv1.Group(kgwv1alpha1.GroupName),
			Kind:  gwapiv1.Kind("TrafficPolicy"),
			Name:  gwapiv1.ObjectName(policyName),
		},
	}
}

// jwksURIFromIssuer derives the JWKS endpoint from a Keycloak issuer URL by the
// standard convention {issuer}/protocol/openid-connect/certs, matching the
// envoy path (internal/features/envoy/access_control.go:359).
func jwksURIFromIssuer(issuer string) string {
	return strings.TrimRight(issuer, "/") + "/protocol/openid-connect/certs"
}

// issuerEndpoint parses the issuer URL into the JWKS endpoint host, port and
// whether TLS origination is needed. https => 443 + tls, http => 80, an
// explicit :port overrides the scheme default. Falls back to (issuer, 443,
// true) when the URL does not parse, the safe default for public issuers.
func issuerEndpoint(issuer string) (host string, port int32, tls bool) {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return issuer, 443, true
	}
	tls = u.Scheme != "http"
	host = u.Hostname()
	if p := u.Port(); p != "" {
		if n, perr := strconv.Atoi(p); perr == nil {
			return host, int32(n), tls
		}
	}
	if tls {
		return host, 443, true
	}
	return host, 80, false
}

// issuerHost extracts the host from an issuer URL. Falls back to the raw issuer
// if it does not parse, matching the envoy path.
func issuerHost(issuer string) string {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" {
		return issuer
	}
	return u.Hostname()
}

// jwksBackendName is the Backend name for a given JWKS host, deduplicated by
// host so issuers sharing a host share one Backend.
func jwksBackendName(host string) string {
	return "jwks-" + host
}
