// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package parser_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v0 "github.com/telekom/controlplane/rover-ctl/pkg/handlers/v0"
	"github.com/telekom/controlplane/rover-ctl/pkg/parser"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("AgentSpecification Parsing", func() {
	DescribeTable("preserves the nested card through parsing and request wrapping",
		func(extension string, validate bool) {
			content, err := os.ReadFile(filepath.Join("testdata", "agent-spec.yaml"))
			Expect(err).NotTo(HaveOccurred())
			var expected map[string]any
			Expect(yaml.Unmarshal(content, &expected)).To(Succeed())
			if extension == ".json" {
				content, err = json.Marshal(expected)
				Expect(err).NotTo(HaveOccurred())
			}
			path := filepath.Join(GinkgoT().TempDir(), "agent-spec"+extension)
			Expect(os.WriteFile(path, content, 0o600)).To(Succeed())

			opts := append([]parser.Option{}, parser.Opts...)
			if validate {
				opts = append(opts, parser.EnableValidation())
			}
			objectParser := parser.NewObjectParser(opts...)
			Expect(objectParser.Parse(path)).To(Succeed())
			Expect(objectParser.Objects()).To(HaveLen(1))
			obj := objectParser.Objects()[0]
			Expect(obj.GetKind()).To(Equal("AgentSpecification"))
			Expect(obj.GetApiVersion()).To(Equal("tcp.ei.telekom.de/v1"))
			Expect(obj.GetName()).To(Equal("team-assistant"))
			Expect(obj.GetContent()).To(Equal(expected))

			handler := v0.NewAgentSpecHandlerInstance()
			Expect(handler.ValidateObject(obj)).To(Succeed())
			Expect(v0.PatchAgentSpecificationRequest(context.Background(), obj)).To(Succeed())
			Expect(obj.GetContent()).To(Equal(map[string]any{"specification": expected}))
		},
		Entry("YAML", ".yaml", false),
		Entry("JSON", ".json", false),
		Entry("JSON with validation", ".json", true),
	)

	DescribeTable("detects either nested agent marker",
		func(card map[string]any) {
			obj := &types.UnstructuredObject{Content: map[string]any{
				"basePath":  "/team/assistant",
				"agentCard": card,
			}}
			Expect(parser.NewObjectParser(parser.Opts...).RunHooks(parser.HookAfterParse, obj)).To(Succeed())
			Expect(obj.GetKind()).To(Equal("AgentSpecification"))
			Expect(obj.GetName()).To(Equal("team-assistant"))
		},
		Entry("skills without capabilities", map[string]any{"skills": []any{}}),
		Entry("capabilities without skills", map[string]any{"capabilities": map[string]any{}}),
	)

	DescribeTable("does not detect documents without the nested structure",
		func(content map[string]any) {
			obj := &types.UnstructuredObject{Content: content}
			Expect(parser.NewObjectParser(parser.Opts...).RunHooks(parser.HookAfterParse, obj)).To(Succeed())
			Expect(obj.GetKind()).To(BeEmpty())
		},
		Entry("flat skills and capabilities", map[string]any{"basePath": "/team/assistant", "skills": []any{}, "capabilities": map[string]any{}}),
		Entry("missing card", map[string]any{"basePath": "/team/assistant"}),
		Entry("null card", map[string]any{"basePath": "/team/assistant", "agentCard": nil}),
		Entry("string card", map[string]any{"basePath": "/team/assistant", "agentCard": "invalid"}),
		Entry("array card", map[string]any{"basePath": "/team/assistant", "agentCard": []any{}}),
		Entry("empty card", map[string]any{"basePath": "/team/assistant", "agentCard": map[string]any{}}),
		Entry("nested basePath only", map[string]any{"agentCard": map[string]any{"basePath": "/team/assistant", "skills": []any{}}}),
	)

	DescribeTable("retains handler validation for invalid derived names",
		func(basePath any) {
			obj := &types.UnstructuredObject{Content: map[string]any{
				"basePath": basePath,
				"agentCard": map[string]any{
					"name":   "valid-card-name",
					"skills": []any{},
				},
			}}
			Expect(parser.NewObjectParser(parser.Opts...).RunHooks(parser.HookAfterParse, obj)).To(Succeed())
			Expect(obj.GetKind()).To(Equal("AgentSpecification"))
			Expect(v0.NewAgentSpecHandlerInstance().ValidateObject(obj)).To(HaveOccurred())
		},
		Entry("empty basePath", ""),
		Entry("non-string basePath", 123),
		Entry("root basePath", "/"),
		Entry("invalid name characters", "/team/bad_name"),
	)
})
