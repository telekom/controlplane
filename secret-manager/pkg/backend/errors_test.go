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

	Context("Code", func() {
		It("should return 403 for Forbidden errors", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			backendErr := backend.Forbidden(secretId, fmt.Errorf("access denied"))
			Expect(backendErr.Code()).To(Equal(403))
		})

		It("should return 404 for NotFound errors", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("mocked-secret-id").Times(1)
			backendErr := backend.ErrSecretNotFound(secretId)
			Expect(backendErr.Code()).To(Equal(404))
		})

		It("should return 400 for BadChecksum errors", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			secretId.EXPECT().String().Return("mocked-secret-id").Times(1)
			backendErr := backend.ErrBadChecksum(secretId)
			Expect(backendErr.Code()).To(Equal(400))
		})

		It("should return 400 for InvalidSecretId errors", func() {
			backendErr := backend.ErrInvalidSecretId("bad-id")
			Expect(backendErr.Code()).To(Equal(400))
		})

		It("should return 429 for TooManyRequests errors", func() {
			backendErr := backend.NewBackendError(nil, fmt.Errorf("rate limited"), backend.TypeErrTooManyRequests)
			Expect(backendErr.Code()).To(Equal(429))
		})

		It("should default to 500 for unknown error types without StatusCode", func() {
			secretId := mocks.NewMockSecretId(GinkgoT())
			backendErr := backend.ErrIncorrectState(secretId, fmt.Errorf("bad state"))
			Expect(backendErr.Code()).To(Equal(500))
		})

		It("should use explicit StatusCode for unknown error types", func() {
			backendErr := backend.NewBackendError(nil, fmt.Errorf("custom"), "CustomType").WithStatusCode(503)
			Expect(backendErr.Code()).To(Equal(503))
		})
	})

	Context("WithStatusCode", func() {
		It("should set the status code and return the same error for chaining", func() {
			backendErr := backend.NewBackendError(nil, fmt.Errorf("test"), "SomeType")
			result := backendErr.WithStatusCode(418)
			Expect(result).To(BeIdenticalTo(backendErr))
			Expect(backendErr.Code()).To(Equal(418))
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
