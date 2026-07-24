// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package server_test

import (
	"github.com/telekom/controlplane/common-server/pkg/server"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K8sConfig.ToOptions", func() {
	It("returns one option per builder when not in-cluster", func() {
		cfg := server.K8sConfig{
			Audience:       "aud",
			TrustedIssuers: []string{"iss"},
			JWKSetURLs:     []string{"https://jwks"},
			AccessConfig:   []k8s.ServiceAccessConfig{{ServiceAccountName: "sa"}},
		}
		Expect(cfg.ToOptions()).To(HaveLen(4))
	})

	It("includes the in-cluster option when InCluster is true", func() {
		cfg := server.K8sConfig{InCluster: true}
		Expect(cfg.ToOptions()).To(HaveLen(5))
	})
})

var _ = Describe("ListenerConfig.Validate", func() {
	jwt := &security.JWTConfig{Mode: security.ModeJWT, TrustedIssuers: []string{"https://issuer.example"}}
	k8sInternal := &server.K8sConfig{} // k8s: {} — valid on an internal listener

	It("accepts a valid jwt-only listener", func() {
		Expect(server.ListenerConfig{Address: ":1", JWT: jwt}.Validate(false)).To(Succeed())
	})
	It("accepts a k8s-only internal listener with no accessConfig", func() {
		Expect(server.ListenerConfig{Address: ":1", K8s: k8sInternal}.Validate(true)).To(Succeed())
	})
	It("rejects both set", func() {
		Expect(server.ListenerConfig{Address: ":1", JWT: jwt, K8s: k8sInternal}.Validate(false)).To(HaveOccurred())
	})
	It("rejects neither set", func() {
		Expect(server.ListenerConfig{Address: ":1"}.Validate(false)).To(HaveOccurred())
	})
	It("rejects an empty address", func() {
		Expect(server.ListenerConfig{JWT: jwt}.Validate(false)).To(HaveOccurred())
	})
	It("rejects jwt mode=jwt without trustedIssuers", func() {
		bad := &security.JWTConfig{Mode: security.ModeJWT}
		Expect(server.ListenerConfig{Address: ":1", JWT: bad}.Validate(false)).To(HaveOccurred())
	})
	It("rejects an unknown jwt mode", func() {
		bad := &security.JWTConfig{Mode: "bogus"}
		Expect(server.ListenerConfig{Address: ":1", JWT: bad}.Validate(false)).To(HaveOccurred())
	})
	It("rejects a malformed jwt trustedIssuer URL", func() {
		bad := &security.JWTConfig{Mode: security.ModeJWT, TrustedIssuers: []string{"not a url"}}
		Expect(server.ListenerConfig{Address: ":1", JWT: bad}.Validate(false)).To(HaveOccurred())
	})
	It("rejects an external k8s listener with empty accessConfig", func() {
		Expect(server.ListenerConfig{Address: ":1", K8s: &server.K8sConfig{}}.Validate(false)).To(HaveOccurred())
	})
	It("rejects an accessConfig entry missing service account or namespace", func() {
		k := &server.K8sConfig{AccessConfig: []k8s.ServiceAccessConfig{{ServiceAccountName: "sa"}}}
		Expect(server.ListenerConfig{Address: ":1", K8s: k}.Validate(false)).To(HaveOccurred())
	})
	It("rejects duplicate accessConfig SA+namespace entries", func() {
		k := &server.K8sConfig{AccessConfig: []k8s.ServiceAccessConfig{
			{ServiceAccountName: "sa", Namespace: "ns"},
			{ServiceAccountName: "sa", Namespace: "ns"},
		}}
		Expect(server.ListenerConfig{Address: ":1", K8s: k}.Validate(false)).To(HaveOccurred())
	})
	It("accepts a valid external k8s listener", func() {
		k := &server.K8sConfig{
			TrustedIssuers: []string{"https://kube.example"},
			AccessConfig:   []k8s.ServiceAccessConfig{{ServiceAccountName: "sa", Namespace: "ns", AllowedAccess: []k8s.AccessType{k8s.AccessTypeRead}}},
		}
		Expect(server.ListenerConfig{Address: ":1", K8s: k}.Validate(false)).To(Succeed())
	})
	It("rejects an invalid allowed_access value", func() {
		k := &server.K8sConfig{AccessConfig: []k8s.ServiceAccessConfig{
			{ServiceAccountName: "sa", Namespace: "ns", AllowedAccess: []k8s.AccessType{"bogus"}},
		}}
		Expect(server.ListenerConfig{Address: ":1", K8s: k}.Validate(true)).To(HaveOccurred())
	})
})
