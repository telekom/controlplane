// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	"github.com/telekom/controlplane/rover-ctl/test/mocks"
)

var _ = Describe("Registry", func() {
	var (
		testHandler1 *mocks.MockResourceHandler
		testHandler2 *mocks.MockResourceHandler
		kind1        string
		version1     string
		kind2        string
		version2     string
	)

	BeforeEach(func() {
		// Reset the registry before each test
		handlers.ResetRegistryForTest()

		// Set test values
		kind1 = "TestKind1"
		version1 = "v1"
		kind2 = "TestKind2"
		version2 = "v2"

		// Create test handlers
		testHandler1 = mocks.NewMockResourceHandler(GinkgoT())
		testHandler1.EXPECT().Priority().Return(100).Maybe()

		testHandler2 = mocks.NewMockResourceHandler(GinkgoT())
		testHandler2.EXPECT().Priority().Return(50).Maybe()
	})

	Describe("RegisterHandler", func() {
		Context("when registering a new handler", func() {
			It("should store the handler in the registry", func() {
				// Register the handler
				handlers.RegisterHandler(kind1, version1, testHandler1)

				// Retrieve the handler
				retrievedHandler, err := handlers.GetHandler(kind1, version1)

				// Verify
				Expect(err).NotTo(HaveOccurred())
				Expect(retrievedHandler).To(Equal(testHandler1))
			})

			It("should register multiple handlers with different kinds", func() {
				// Register handlers
				handlers.RegisterHandler(kind1, version1, testHandler1)
				handlers.RegisterHandler(kind2, version2, testHandler2)

				// Retrieve handlers
				handler1, err1 := handlers.GetHandler(kind1, version1)
				handler2, err2 := handlers.GetHandler(kind2, version2)

				// Verify
				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
				Expect(handler1).To(Equal(testHandler1))
				Expect(handler2).To(Equal(testHandler2))
			})

			It("should overwrite existing handler with same kind and version", func() {
				// Register handler
				handlers.RegisterHandler(kind1, version1, testHandler1)

				// Create new handler with same kind and version
				newHandler := mocks.NewMockResourceHandler(GinkgoT())
				newHandler.EXPECT().Priority().Return(200).Maybe()

				// Register the new handler, which should overwrite the existing one
				handlers.RegisterHandler(kind1, version1, newHandler)

				// Retrieve the handler
				retrievedHandler, err := handlers.GetHandler(kind1, version1)

				// Verify that the new handler was retrieved
				Expect(err).NotTo(HaveOccurred())
				Expect(retrievedHandler).To(Equal(newHandler))
				Expect(retrievedHandler).NotTo(Equal(testHandler1))
			})
		})
	})

	Describe("GetHandler", func() {
		Context("when retrieving a registered handler", func() {
			It("should return the handler", func() {
				// Register the handler
				handlers.RegisterHandler(kind1, version1, testHandler1)

				// Retrieve the handler
				retrievedHandler, err := handlers.GetHandler(kind1, version1)

				// Verify
				Expect(err).NotTo(HaveOccurred())
				Expect(retrievedHandler).To(Equal(testHandler1))
			})
		})

		Context("when retrieving a non-existent handler", func() {
			It("should return an error", func() {
				// Attempt to retrieve non-existent handler
				_, err := handlers.GetHandler("NonExistentKind", "v1")

				// Verify
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no handler found for the specified resource"))
			})
		})

		Context("when retrieving a handler with case-insensitive matching", func() {
			It("should return the handler regardless of case", func() {
				// Register with original case
				handlers.RegisterHandler("TestKind", "v1", testHandler1)

				// Retrieve with different case
				retrievedHandler, err := handlers.GetHandler("testkind", "V1")

				// Verify
				Expect(err).NotTo(HaveOccurred())
				Expect(retrievedHandler).To(Equal(testHandler1))
			})
		})
	})

	Describe("handlerKey", func() {
		Context("when generating a handler key", func() {
			It("should create consistent keys regardless of case", func() {
				// We're testing an internal function, but we can verify its behavior
				// through the register/get functions
				handlers.RegisterHandler("TestKind", "v1", testHandler1)

				// Try different case combinations
				handler1, err1 := handlers.GetHandler("TestKind", "v1")
				handler2, err2 := handlers.GetHandler("testkind", "V1")
				handler3, err3 := handlers.GetHandler("TESTKIND", "V1")

				// Verify all point to the same handler
				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
				Expect(err3).NotTo(HaveOccurred())
				Expect(handler1).To(Equal(testHandler1))
				Expect(handler2).To(Equal(testHandler1))
				Expect(handler3).To(Equal(testHandler1))
			})
		})
	})

	Describe("RegisterHandlers", func() {
		Context("when registering the default set of handlers", func() {
			It("should register ApiSpec and Rover handlers", func() {
				// Register default handlers
				handlers.RegisterHandlers()

				// Verify ApiSpecification handler is registered
				apiSpecHandler, apiSpecErr := handlers.GetHandler("ApiSpecification", "tcp.ei.telekom.de/v1")

				// Verify Rover handler is registered
				roverHandler, roverErr := handlers.GetHandler("Rover", "tcp.ei.telekom.de/v1")

				// Verify both handlers are registered
				Expect(apiSpecErr).NotTo(HaveOccurred())
				Expect(roverErr).NotTo(HaveOccurred())
				Expect(apiSpecHandler).NotTo(BeNil())
				Expect(roverHandler).NotTo(BeNil())

				// Verify they have different priorities (specific values not important)
				Expect(apiSpecHandler.Priority()).NotTo(Equal(roverHandler.Priority()))
			})
		})
	})
})
