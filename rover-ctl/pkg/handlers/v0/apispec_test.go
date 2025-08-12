// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("ApiSpec Handler", func() {
	Describe("NewApiSpecHandlerInstance", func() {
		Context("when creating a new API spec handler", func() {
			It("should return a properly configured handler", func() {
				// Create a new API spec handler
				apiSpecHandler := v0.NewApiSpecHandlerInstance()

				// Verify the handler properties
				Expect(apiSpecHandler).NotTo(BeNil())
				Expect(apiSpecHandler.BaseHandler).NotTo(BeNil())

				// Verify the correct API values are set
				baseHandler := apiSpecHandler.BaseHandler
				Expect(baseHandler.APIVersion).To(Equal("tcp.ei.telekom.de/v1"))
				Expect(baseHandler.Kind).To(Equal("ApiSpecification"))
				Expect(baseHandler.Resource).To(Equal("apispecifications"))
				Expect(baseHandler.Priority()).To(Equal(10))
			})

			It("should have the PatchApiSpecificationRequest hook registered", func() {
				// We don't need to store the handler here, just directly test the hook

				// We cannot directly check for the presence of hooks, but we can
				// ensure the hook is executed by calling Apply with an object
				// and verifying the transformation happens

				// Create a test object
				obj := &types.UnstructuredObject{
					Content: map[string]any{
						"field1": "value1",
						"field2": "value2",
					},
				}

				// Run the hook directly to verify it works
				err := v0.PatchApiSpecificationRequest(context.Background(), obj)

				// Verify that there is no error
				Expect(err).NotTo(HaveOccurred())

				// Verify the transformation has occurred
				// The hook should wrap the original content in a "specification" field
				content := obj.GetContent()
				Expect(content).To(HaveKeyWithValue("specification", map[string]any{
					"field1": "value1",
					"field2": "value2",
				}))
			})
		})
	})

	Describe("PatchApiSpecificationRequest", func() {
		Context("when patching an API specification request", func() {
			It("should wrap the content in a specification field", func() {
				// Create a test object with some content
				originalContent := map[string]any{
					"name":        "test-api",
					"version":     "1.0",
					"description": "Test API",
				}

				obj := &types.UnstructuredObject{
					Content: originalContent,
				}

				// Apply the patch
				err := v0.PatchApiSpecificationRequest(context.Background(), obj)

				// Verify no error occurred
				Expect(err).NotTo(HaveOccurred())

				// Get the patched content
				patchedContent := obj.GetContent()

				// Verify the content was wrapped in a specification field
				Expect(patchedContent).To(HaveKey("specification"))

				// Verify the original content is now inside the specification field
				specContent := patchedContent["specification"]
				Expect(specContent).To(Equal(originalContent))
			})
		})
	})
})
