// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package backend_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/secret-manager/pkg/backend"
	"github.com/telekom/controlplane/secret-manager/test/mocks"
)

var _ = Describe("Errors", func() {

	Context("BackendError", func() {
		It("should create a new BackendError", func() {
			err := fmt.Errorf("some error")
			backendErr := backend.NewBackendError(nil, err, "SomeType")

			Expect(backendErr).To(HaveOccurred())
			Expect(backendErr.Error()).To(Equal("SomeType: some error"))
			Expect(backendErr.Type).To(Equal("SomeType"))
		})

		It("should create a new BackendError with an ID", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			err := fmt.Errorf("some error")
			backendErr := backend.NewBackendError(secretId, err, "SomeType")
			Expect(backendErr).To(HaveOccurred())
			Expect(backendErr.Error()).To(Equal("SomeType: some error"))
			Expect(backendErr.Type).To(Equal("SomeType"))
			Expect(backendErr.Id).To(Equal(secretId))
		})

		It("should create a NotFound error", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("mocked-secret-id").Times(2)
			backendErr := backend.ErrSecretNotFound(secretId)

			Expect(backendErr).To(HaveOccurred())
			Expect(backendErr.Error()).To(Equal("NotFound: resource " + secretId.String() + " not found"))
			Expect(backendErr.Type).To(Equal(backend.TypeErrNotFound))
			Expect(backendErr.Id).To(Equal(secretId))
		})

		It("should create a BadChecksum error", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("mocked-secret-id").Times(2)
			backendErr := backend.ErrBadChecksum(secretId)

			Expect(backendErr).To(HaveOccurred())
			Expect(backendErr.Error()).To(Equal("BadChecksum: bad checksum for secret " + secretId.String()))
			Expect(backendErr.Type).To(Equal(backend.TypeErrBadChecksum))
			Expect(backendErr.Id).To(Equal(secretId))
		})

		It("should create an InvalidSecretId error", func() {
			rawId := "invalid-id"
			backendErr := backend.ErrInvalidSecretId(rawId)

			Expect(backendErr).To(HaveOccurred())
			Expect(backendErr.Error()).To(Equal("InvalidSecretId: invalid secret id 'invalid-id'"))
			Expect(backendErr.Type).To(Equal(backend.TypeErrInvalidSecretId))
			Expect(backendErr.Id).To(BeNil())
		})
	})

	Context("IsNotFoundErr", func() {
		It("should return true for a NotFound error", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("mocked-secret-id").Times(1)
			backendErr := backend.ErrSecretNotFound(secretId)

			Expect(backend.IsNotFoundErr(backendErr)).To(BeTrue())
		})

		It("should return false for a different error type", func() {
			err := fmt.Errorf("some other error")
			Expect(backend.IsNotFoundErr(err)).To(BeFalse())
		})

		It("should return false for nil error", func() {
			Expect(backend.IsNotFoundErr(nil)).To(BeFalse())
		})
	})
})
