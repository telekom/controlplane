// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSpecification Mapper", func() {
	Context("MapRequest", func() {
		It("must map a ApiSpecificationUpdateRequest to an ApiSpecification correctly", func() {
			output, err := MapRequest(apiSpecification, resourceIdInfo)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)
		})

		It("must return an error if the input ApiSpecificationUpdateRequest is nil", func() {
			output, err := MapRequest(nil, resourceIdInfo)

			Expect(output).To(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input api specification is nil"))
		})

	})
})
