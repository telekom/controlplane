// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Default Secret Implementation", func() {

	Context("DefaultSecret", func() {

		It("should return a new Secret", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())

			secret := backend.NewDefaultSecret[*mocks.MockSecretId](secretId, "test-value")
			Expect(secret).ToNot(BeNil())
			Expect(secret.Value()).To(Equal("test-value"))
			Expect(secret.Id()).To(Equal(secretId))
		})
	})

	Context("DefaultOnboardResponse", func() {
		It("should return a new OnboardResponse", func() {
			secretRef := backend.NewStringSecretRef("my-secret-id")
			Expect(secretRef).ToNot(BeNil())
			Expect(secretRef.String()).To(Equal("my-secret-id"))

			onboardResponse := backend.NewDefaultOnboardResponse(map[string]backend.SecretRef{
				"test": secretRef,
			})
			Expect(onboardResponse).ToNot(BeNil())
			Expect(onboardResponse.SecretRefs()).To(HaveLen(1))
			Expect(onboardResponse.SecretRefs()["test"]).To(Equal(secretRef))
		})
	})
})
