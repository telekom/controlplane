// SPDX-FileCopyrightText: 2026 Deutsche Telekom AG
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common/pkg/config"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

var _ = Describe("EnsureLabelsOrDie", func() {
	DescribeTable("normalizes business-context labels and initializes a nil label map",
		func(value, expected string) {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				Environment: value, Team: value, Group: value,
			})
			obj := &roverv1.Rover{}

			EnsureLabelsOrDie(ctx, obj)

			Expect(obj.Labels).To(Equal(map[string]string{
				config.EnvironmentLabelKey:    expected,
				config.BuildLabelKey("team"):  expected,
				config.BuildLabelKey("group"): expected,
			}))
		},
		Entry("valid values", "my-team", "my-team"),
		Entry("values at the label limit", strings.Repeat("a", 63), strings.Repeat("a", 63)),
		Entry("uppercase and separators", "--My_Team/Name--", "my-team-name"),
	)

	DescribeTable("shortens oversized business-context labels while preserving existing labels",
		func(length int) {
			ctx := security.ToContext(context.Background(), &security.BusinessContext{
				Environment: strings.Repeat("e", length),
				Team:        strings.Repeat("t", length),
				Group:       strings.Repeat("g", length),
			})
			obj := &roverv1.Rover{}
			obj.Labels = map[string]string{
				config.EnvironmentLabelKey:          "old-environment",
				config.BuildLabelKey("team"):        "old-team",
				config.BuildLabelKey("group"):       "old-group",
				config.BuildLabelKey("application"): "my-application",
				"custom":                            "keep-me",
			}

			EnsureLabelsOrDie(ctx, obj)

			for key, prefix := range map[string]string{
				config.EnvironmentLabelKey:    "eeee",
				config.BuildLabelKey("team"):  "tttt",
				config.BuildLabelKey("group"): "gggg",
			} {
				Expect(obj.Labels[key]).To(HavePrefix(prefix))
				Expect(validation.IsValidLabelValue(obj.Labels[key])).To(BeEmpty())
			}
			Expect(obj.Labels).To(HaveKeyWithValue(config.BuildLabelKey("application"), "my-application"))
			Expect(obj.Labels).To(HaveKeyWithValue("custom", "keep-me"))
			original := obj.DeepCopy()
			EnsureLabelsOrDie(ctx, obj)
			Expect(obj.Labels).To(Equal(original.Labels))
		},
		Entry("one character above the label limit", 64),
		Entry("well above the label limit", 300),
	)

	It("panics when the security context is missing", func() {
		Expect(func() {
			EnsureLabelsOrDie(context.Background(), &roverv1.Rover{})
		}).To(PanicWith("security context not found"))
	})
})
