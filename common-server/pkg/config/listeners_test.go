// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"github.com/telekom/controlplane/common-server/pkg/config"
	k8s "github.com/telekom/controlplane/common-server/pkg/server/middleware/kubernetes"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Load with TLS and Listeners", func() {
	It("decodes tls and internal + external listeners", func() {
		yaml := `
tls:
  cert: /etc/tls/tls.crt
  key: /etc/tls/tls.key
listeners:
  external:
    address: ":8443"
    jwt:
      mode: jwt
      trustedIssuers:
        - https://issuer.example
      defaultScope: read
      scopePrefix: "cp:"
      lms:
        basePath: /lms
  internal:
    address: ":9443"
    k8s:
      audience: my-aud
      trustedIssuers:
        - https://kube.example
      jwkSetURLs:
        - https://kube.example/keys
      inCluster: true
      accessConfig:
        - service_account_name: my-sa
          deployment_name: my-deploy
          namespace: my-ns
          allowed_access:
            - read
`
		path := writeTempYAML(yaml)
		cfg, err := config.Load(path, &squashCfg{})
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.TLS).NotTo(BeNil())
		Expect(cfg.TLS.Cert).To(Equal("/etc/tls/tls.crt"))
		Expect(cfg.TLS.Key).To(Equal("/etc/tls/tls.key"))

		Expect(cfg.Listeners.External).NotTo(BeNil())
		Expect(cfg.Listeners.Internal).NotTo(BeNil())

		ext := cfg.Listeners.External
		Expect(ext.Address).To(Equal(":8443"))
		Expect(ext.JWT).NotTo(BeNil())
		Expect(ext.K8s).To(BeNil())
		Expect(ext.JWT.Mode).To(Equal(security.ModeJWT))
		Expect(ext.JWT.TrustedIssuers).To(Equal([]string{"https://issuer.example"}))
		Expect(ext.JWT.DefaultScope).To(Equal("read"))
		Expect(ext.JWT.ScopePrefix).To(Equal("cp:"))
		Expect(ext.JWT.LMS.BasePath).To(Equal("/lms"))
		Expect(ext.Validate(false)).To(Succeed())

		in := cfg.Listeners.Internal
		Expect(in.Address).To(Equal(":9443"))
		Expect(in.K8s).NotTo(BeNil())
		Expect(in.JWT).To(BeNil())
		Expect(in.K8s.Audience).To(Equal("my-aud"))
		Expect(in.K8s.TrustedIssuers).To(Equal([]string{"https://kube.example"}))
		Expect(in.K8s.JWKSetURLs).To(Equal([]string{"https://kube.example/keys"}))
		Expect(in.K8s.InCluster).To(BeTrue())
		Expect(in.K8s.AccessConfig).To(HaveLen(1))
		Expect(in.K8s.AccessConfig[0].ServiceAccountName).To(Equal("my-sa"))
		Expect(in.K8s.AccessConfig[0].DeploymentName).To(Equal("my-deploy"))
		Expect(in.K8s.AccessConfig[0].Namespace).To(Equal("my-ns"))
		Expect(in.K8s.AccessConfig[0].AllowedAccess).To(Equal([]k8s.AccessType{k8s.AccessTypeRead}))
		Expect(in.Validate(true)).To(Succeed())
	})
})
