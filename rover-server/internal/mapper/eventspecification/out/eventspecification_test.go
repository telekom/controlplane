// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventSpecificationResponse Mapper", func() {
	Context("MapResponse", func() {
		It("must map an EventSpecification CRD to an EventSpecificationResponse correctly", func() {
			specContent := map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{
						"type": "string",
					},
				},
			}

			output, err := MapResponse(ctx, eventSpecification, specContent)

			Expect(err).To(BeNil())
			Expect(output).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), output)
		})

		It("must return an error if the input EventSpecification CRD is nil", func() {
			output, err := MapResponse(ctx, nil, nil)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input event specification crd is nil"))
		})

		It("must omit specification from response when specContent is nil", func() {
			output, err := MapResponse(ctx, eventSpecification, nil)

			Expect(err).To(BeNil())
			Expect(output).ToNot(BeNil())
			Expect(output.Specification).To(BeNil())
			snaps.MatchJSON(GinkgoT(), output)
		})
	})
})
