// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v0_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("FileSpec Handler", func() {
	It("configures the FileSpecification endpoint", func() {
		handler := v0.NewFileSpecHandlerInstance()

		Expect(handler.APIVersion).To(Equal("rover.cp.ei.telekom.de/v1"))
		Expect(handler.Kind).To(Equal("FileSpecification"))
		Expect(handler.Resource).To(Equal("filespecifications"))
	})

	It("is registered for the FileSpecification manifest API version", func() {
		handlers.RegisterHandlers()

		handler, err := handlers.GetHandler("FileSpecification", "rover.cp.ei.telekom.de/v1")
		Expect(err).NotTo(HaveOccurred())
		Expect(handler).NotTo(BeNil())
	})

	It("sends only the FileSpecification spec to the REST API", func() {
		obj := &types.UnstructuredObject{Content: map[string]any{
			"apiVersion": "rover.cp.ei.telekom.de/v1",
			"kind":       "FileSpecification",
			"metadata":   map[string]any{"name": "demo-invoices-v1"},
			"spec": map[string]any{
				"type":        "demo.invoices.v1",
				"version":     "1.0.0",
				"description": "Invoices",
			},
		}}

		Expect(v0.PatchFileSpecificationRequest(context.Background(), obj)).To(Succeed())
		Expect(obj.GetContent()).To(Equal(map[string]any{
			"type":        "demo.invoices.v1",
			"version":     "1.0.0",
			"description": "Invoices",
		}))
	})

	DescribeTable("rejects an invalid FileSpecification body",
		func(content map[string]any) {
			obj := &types.UnstructuredObject{Content: content}
			Expect(v0.PatchFileSpecificationRequest(context.Background(), obj)).To(HaveOccurred())
		},
		Entry("nil object", nil),
		Entry("missing spec", map[string]any{}),
		Entry("non-object spec", map[string]any{"spec": "invalid"}),
	)
})
