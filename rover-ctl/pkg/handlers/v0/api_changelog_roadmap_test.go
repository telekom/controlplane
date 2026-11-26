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

var _ = Describe("ApiChangelog and ApiRoadmap Handlers", func() {
	It("should be applied after the ApiSpecification they refer to", func() {
		specPriority := v0.NewApiSpecHandlerInstance().Priority()

		Expect(v0.NewApiChangelogHandlerInstance().Priority()).To(BeNumerically(">", specPriority))
		Expect(v0.NewApiRoadmapHandlerInstance().Priority()).To(BeNumerically(">", specPriority))
	})

	It("should be configured with the expected resource names", func() {
		changelog := v0.NewApiChangelogHandlerInstance().BaseHandler
		Expect(changelog.Kind).To(Equal("ApiChangelog"))
		Expect(changelog.Resource).To(Equal("apichangelogs"))

		roadmap := v0.NewApiRoadmapHandlerInstance().BaseHandler
		Expect(roadmap.Kind).To(Equal("ApiRoadmap"))
		Expect(roadmap.Resource).To(Equal("apiroadmaps"))
	})

	DescribeTable("sends only the spec to the API",
		func(patch func(context.Context, types.Object) error) {
			spec := map[string]any{
				"basePath": "/eni/atlas-test/v1",
				"items":    []any{map[string]any{"date": "2022-06-20"}},
			}
			obj := &types.UnstructuredObject{Content: map[string]any{
				"apiVersion": "rover.ei.telekom.de/v1",
				"metadata":   map[string]any{"name": "eni-atlas-test"},
				"spec":       spec,
			}}

			Expect(patch(context.Background(), obj)).To(Succeed())
			Expect(obj.GetContent()).To(Equal(spec))
		},
		Entry("ApiChangelog", v0.PatchApiChangelogRequest),
		Entry("ApiRoadmap", v0.PatchApiRoadmapRequest),
	)

	DescribeTable("rejects a missing or malformed spec",
		func(patch func(context.Context, types.Object) error) {
			noSpec := &types.UnstructuredObject{Content: map[string]any{"metadata": map[string]any{}}}
			Expect(patch(context.Background(), noSpec)).To(MatchError(ContainSubstring("Missing 'spec'")))

			badSpec := &types.UnstructuredObject{Content: map[string]any{"spec": "nope"}}
			Expect(patch(context.Background(), badSpec)).To(MatchError(ContainSubstring("should be an object")))
		},
		Entry("ApiChangelog", v0.PatchApiChangelogRequest),
		Entry("ApiRoadmap", v0.PatchApiRoadmapRequest),
	)
})
