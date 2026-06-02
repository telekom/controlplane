// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package secrets_test

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/controlplane-api/internal/secrets"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resolver", func() {
	var (
		ctx           context.Context
		secretManager *fake.MockSecretManager
		resolver      *secrets.Resolver
	)

	BeforeEach(func() {
		ctx = context.Background()
		secretManager = fake.NewMockSecretManager(GinkgoT())
		resolver = secrets.NewResolver(secretManager)
	})

	Context("when value is nil", func() {
		It("should return nil", func() {
			result, err := resolver.Resolve(ctx, nil, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNil())
		})
	})

	Context("when value is not a secret reference", func() {
		It("should return the value unchanged", func() {
			plaintext := "plain-value"
			result, err := resolver.Resolve(ctx, &plaintext, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(&plaintext))
		})
	})

	Context("when value is a secret reference", func() {
		It("should resolve the secret from the secret manager", func() {
			ref := "$<env/team/app/secret/abc123>"
			secretManager.EXPECT().
				Get(mock.Anything, "env/team/app/secret/abc123").
				Return("resolved-secret-value", nil)

			result, err := resolver.Resolve(ctx, &ref, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
			Expect(*result).To(Equal("resolved-secret-value"))
		})

		It("should return a sanitized error and not leak upstream details", func() {
			ref := "$<env/team/app/secret/abc123>"
			secretManager.EXPECT().
				Get(mock.Anything, "env/team/app/secret/abc123").
				Return("", fmt.Errorf("unexpected http error (401): {\"detail\":\"Invalid token\"}"))

			result, err := resolver.Resolve(ctx, &ref, "clientSecret")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("failed to resolve secret clientSecret"))
			Expect(err.Error()).NotTo(ContainSubstring("401"))
			Expect(err.Error()).NotTo(ContainSubstring("Invalid token"))
			Expect(result).To(BeNil())
		})
	})

	Context("when the caller has obfuscated access", func() {
		BeforeEach(func() {
			ctx = security.ToContext(ctx, &security.BusinessContext{
				AccessType: security.AccessTypeObfuscated,
			})
		})

		It("should return a masked value instead of resolving", func() {
			ref := "$<env/team/app/secret/abc123>"

			result, err := resolver.Resolve(ctx, &ref, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
			Expect(*result).To(Equal("**********"))
		})

		It("should not call the secret manager", func() {
			ref := "$<env/team/app/secret/abc123>"
			// No mock expectations set — any call would fail the test

			_, err := resolver.Resolve(ctx, &ref, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return non-ref values unchanged even when obfuscated", func() {
			plain := "not-a-ref"
			result, err := resolver.Resolve(ctx, &plain, "clientSecret")
			Expect(err).NotTo(HaveOccurred())
			Expect(*result).To(Equal("not-a-ref"))
		})
	})
})
