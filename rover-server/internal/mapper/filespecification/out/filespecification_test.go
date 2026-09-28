// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package out

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileSpecificationResponse Mapper", func() {
	Context("MapResponse", func() {
		It("must map a FileSpecification CRD to a FileSpecificationResponse correctly", func() {
			output, err := MapResponse(fileSpecification, nil)

			Expect(err).To(BeNil())
			Expect(output).ToNot(BeNil())
			snaps.MatchJSON(GinkgoT(), output)

			specContent := map[string]any{"type": "object"}
			withSpecification, err := MapResponse(fileSpecification, specContent)
			Expect(err).To(BeNil())
			Expect(withSpecification.Specification).To(Equal(specContent))
		})

		It("must return an error if the input FileSpecification CRD is nil", func() {
			output, err := MapResponse(nil, nil)

			Expect(output).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), output)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input file specification crd is nil"))
		})
	})
})
