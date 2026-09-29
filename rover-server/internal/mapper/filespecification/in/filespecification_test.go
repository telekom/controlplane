// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var _ = Describe("FileSpecification Mapper", func() {
	Context("MapRequest", func() {
		It("must map a FileSpecification to a CRD correctly", func() {
			result, err := MapRequest(fileSpecification, resourceIdInfo)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), result)
		})

		It("must return an error if the derived name does not match the resource id name", func() {
			mismatchedId := mapper.ResourceIdInfo{
				Name:        "wrong-name",
				Environment: "poc",
				Namespace:   "eni--galatea",
			}

			result, err := MapRequest(fileSpecification, mismatchedId)
			Expect(result).To(BeNil())
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("does not match expected name"))
		})
	})
})
