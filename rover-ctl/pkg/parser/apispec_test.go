// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("ApiSpecification Parsing", func() {
	DescribeTable("SanitizeName",
		func(basePath, expected string) {
			Expect(parser.SanitizeName(basePath)).To(Equal(expected))
		},
		Entry("lowercase path", "/eni/api/v1", "eni-api-v1"),
		Entry("mixed-case path", "/ENI/MyApi/V1/", "eni-myapi-v1"),
	)

	It("should derive a lowercase name from a mixed-case Swagger basePath", func() {
		obj := &types.UnstructuredObject{Content: map[string]any{
			"swagger":  "2.0",
			"info":     map[string]any{"title": "t", "version": "1"},
			"basePath": "/ENI/MyApi/V1",
			"paths":    map[string]any{},
		}}

		Expect(parser.ParseApiSpecification(obj)).To(Succeed())

		Expect(obj.GetName()).To(Equal("eni-myapi-v1"))
		Expect(obj.GetContent()).To(HaveKeyWithValue("basePath", "/ENI/MyApi/V1"))
	})

	It("should derive a lowercase name from a mixed-case OpenAPI server URL", func() {
		servers := []any{map[string]any{"url": "https://Example.com/ENI/MyApi/V1"}}
		obj := &types.UnstructuredObject{Content: map[string]any{
			"openapi": "3.0.3",
			"info":    map[string]any{"title": "t", "version": "1"},
			"servers": servers,
			"paths":   map[string]any{},
		}}

		Expect(parser.ParseApiSpecification(obj)).To(Succeed())

		Expect(obj.GetName()).To(Equal("eni-myapi-v1"))
		Expect(obj.GetContent()).To(HaveKeyWithValue("servers", Equal(servers)))
		Expect(servers[0]).To(HaveKeyWithValue("url", "https://Example.com/ENI/MyApi/V1"))
	})
})
