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

	DescribeTable("GetPathFromURL",
		func(rawURL, expected string) {
			Expect(parser.GetPathFromURL(rawURL)).To(Equal(expected))
		},
		Entry("absolute", "https://example.com/eni/api/v1", "/eni/api/v1"),
		Entry("templated authority without declarations", "https://{host}:{port}/eni/api/v1?x=1#f", "/eni/api/v1"),
		Entry("protocol-relative", "//{host}/eni/api/v1", "/eni/api/v1"),
		Entry("root-relative", "/eni/api/v1", "/eni/api/v1"),
		Entry("relative", "eni/api/v1", "eni/api/v1"),
		Entry("missing path", "https://{host}", ""),
		Entry("percent-encodings unchanged", "https://h/eni/my%20api%2Fx%7bv%7D", "/eni/my%20api%2Fx%7bv%7D"),
		Entry("reserved path chars", "https://h/a:b@c!$&'()*+,;=-._~", "/a:b@c!$&'()*+,;=-._~"),
		Entry("repeated slashes and dot segments", "https://h//eni/./api/../V1/", "//eni/./api/../V1/"),
		Entry("scheme without authority", "https:/eni/api/v1?x=1", "/eni/api/v1"),
		Entry("colon after first slash is path", "/eni/a://b", "/eni/a://b"),
		Entry("templated scheme", "{scheme}://{host}/eni/api", "/eni/api"),
	)

	DescribeTable("GetPathFromURL errors",
		func(rawURL, msg string) {
			_, err := parser.GetPathFromURL(rawURL)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("variable in path", "https://{host}/eni/{version}", "must not contain variables"),
		Entry("whole url variable", "{endpoint}", "must not contain variables"),
		Entry("space", "https://h/eni/my api", "invalid character"),
		Entry("tab", "/eni/\tapi", "invalid character"),
		Entry("control char", "/eni/\x01api", "invalid character"),
		Entry("non-ascii", "/eni/äpi", "invalid character"),
		Entry("backslash", "/eni\\api", "invalid character"),
		Entry("bad hex escape", "https://h/eni/%zz", "malformed percent-encoding"),
		Entry("truncated escape", "/eni/%2", "malformed percent-encoding"),
	)

	It("should keep percent-encodings in the derived name", func() {
		obj := &types.UnstructuredObject{Content: map[string]any{
			"openapi": "3.0.3",
			"info":    map[string]any{"title": "t", "version": "1"},
			"servers": []any{map[string]any{"url": "https://{host}:{port}/eni/My%20Api"}},
			"paths":   map[string]any{},
		}}

		Expect(parser.ParseApiSpecification(obj)).To(Succeed())
		Expect(obj.GetName()).To(Equal("eni-my%20api"))
	})

	It("should reject an OpenAPI server URL with a path variable", func() {
		obj := &types.UnstructuredObject{Content: map[string]any{
			"openapi": "3.0.3",
			"info":    map[string]any{"title": "t", "version": "1"},
			"servers": []any{map[string]any{"url": "https://{host}/eni/{version}"}},
			"paths":   map[string]any{},
		}}

		Expect(parser.ParseApiSpecification(obj)).To(MatchError(ContainSubstring("must not contain variables")))
	})
})
