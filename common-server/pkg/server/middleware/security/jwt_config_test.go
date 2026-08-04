// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSecurityConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security Config Suite")
}

var _ = Describe("JWTConfig.ToSecurityOpts", func() {
	It("maps Mode and populates JWT and BusinessContext options", func() {
		cfg := JWTConfig{
			Mode:           ModeJWT,
			TrustedIssuers: []string{"https://issuer.example"},
			DefaultScope:   "default",
			ScopePrefix:    "prefix:",
			LMS:            LMSConfig{BasePath: "/lms"},
		}

		opts := cfg.ToSecurityOpts()

		Expect(opts.Mode).To(Equal(ModeJWT))
		Expect(opts.JWTOpts).NotTo(BeEmpty())
		Expect(opts.BusinessContextOpts).NotTo(BeEmpty())
		Expect(opts.CheckAccessOpts).To(BeEmpty(), "caller supplies check-access/templates")

		jwtOpts := &JWTOpts{}
		for _, f := range opts.JWTOpts {
			f(jwtOpts)
		}
		Expect(jwtOpts.TrustedIssuers).To(Equal([]string{"https://issuer.example"}))
		Expect(jwtOpts.PerformLMSCheck).To(BeTrue())
		Expect(jwtOpts.LmsBasePath).To(Equal("/lms"))

		bcOpts := &BusinessContextOpts{}
		for _, f := range opts.BusinessContextOpts {
			f(bcOpts)
		}
		Expect(bcOpts.DefaultScope).To(Equal("default"))
		Expect(bcOpts.ScopePrefix).To(Equal("prefix:"))
	})
})
