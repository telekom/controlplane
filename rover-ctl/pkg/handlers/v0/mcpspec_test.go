// Copyright 2026 Deutsche Telekom IT GmbH
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

var _ = Describe("McpSpec Handler", func() {
	Describe("NewMcpSpecHandlerInstance", func() {
		It("should return a properly configured handler", func() {
			mcpSpecHandler := v0.NewMcpSpecHandlerInstance()

			Expect(mcpSpecHandler).NotTo(BeNil())
			Expect(mcpSpecHandler.BaseHandler).NotTo(BeNil())

			baseHandler := mcpSpecHandler.BaseHandler
			Expect(baseHandler.APIVersion).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(baseHandler.Kind).To(Equal("McpSpecification"))
			Expect(baseHandler.Resource).To(Equal("mcpspecifications"))
			Expect(baseHandler.Priority()).To(Equal(10))
		})
	})

	Describe("PatchMcpSpecificationRequest", func() {
		It("should wrap the content in a specification field", func() {
			originalContent := map[string]any{
				"basePath": "/team/assistant",
				"tools": []any{
					map[string]any{"name": "summarize"},
				},
			}
			obj := &types.UnstructuredObject{
				Content: originalContent,
			}

			err := v0.PatchMcpSpecificationRequest(context.Background(), obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetContent()).To(HaveKeyWithValue("specification", originalContent))
		})
	})
})
