// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/match"
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiSpecificationResponse Mapper", func() {
	Context("MapRequest", func() {
		It("must map a ApiSpecification to an ApiSpecificationResponse correctly", func() {
			output, err := MapResponse(apiSpecification, openapi)

			Expect(err).To(BeNil())

			Expect(output).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), output, match.Any("status.time"))
		})

		It("must return an error if the input ApiSpecification is nil", func() {
			output, err := MapResponse(nil, nil)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input api specification crd is nil"))
		})

	})
})
