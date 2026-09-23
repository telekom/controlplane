// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"encoding/json"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("McpSpecification Parsing", func() {
	Describe("ParseMcpSpecification", func() {
		It("should derive the name from basePath", func() {
			obj := &types.UnstructuredObject{
				Content: map[string]any{
					"basePath": "/team/assistant",
				},
			}

			err := parser.ParseMcpSpecification(obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(Equal("team-assistant"))
		})

		It("should ignore empty basePath", func() {
			obj := &types.UnstructuredObject{
				Content: map[string]any{},
			}

			err := parser.ParseMcpSpecification(obj)

			Expect(err).NotTo(HaveOccurred())
			Expect(obj.GetName()).To(BeEmpty())
		})
	})

	Describe("custom parser hooks", func() {
		It("should detect MCP specs automatically", func() {
			mcpParser := parser.NewObjectParser(parser.Opts...)

			err := mcpParser.Parse(filepath.Join("testdata", "mcp-spec.yaml"))

			Expect(err).NotTo(HaveOccurred())
			objects := mcpParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetKind()).To(Equal("McpSpecification"))
			Expect(objects[0].GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(objects[0].GetName()).To(Equal("team-assistant"))
		})

		It("should expand anchors and merge keys", func() {
			mcpParser := parser.NewObjectParser(parser.Opts...)
			Expect(mcpParser.Parse(filepath.Join("testdata", "mcp-spec-with-anchors.yaml"))).To(Succeed())

			objects := mcpParser.Objects()
			Expect(objects).To(HaveLen(1))
			Expect(objects[0].GetContent()["tools"]).To(ConsistOf(
				And(
					HaveKeyWithValue("name", "summarize"),
					HaveKeyWithValue("description", "Process the provided text"),
					HaveKeyWithValue("inputSchema", HaveKeyWithValue("type", "object")),
				),
				And(
					HaveKeyWithValue("name", "translate"),
					HaveKeyWithValue("inputSchema", HaveKeyWithValue("type", "object")),
				),
			))

			parsedJSON, err := json.Marshal(objects[0].GetContent())
			Expect(err).NotTo(HaveOccurred())
			// Parsers that do not resolve aliases correctly can strip * but leave the anchor name as a value.
			// Checking the name catches that case and also catches any unexpanded * or & syntax.
			Expect(parsedJSON).NotTo(ContainSubstring("mcp_tool_defaults_anchor"))
			Expect(parsedJSON).NotTo(ContainSubstring("mcp_text_schema_anchor"))
		})
	})
})
