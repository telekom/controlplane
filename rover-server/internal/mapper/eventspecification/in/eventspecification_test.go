// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package in

import (
	"github.com/gkampitakis/go-snaps/snaps"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-server/internal/mapper"
)

var _ = Describe("EventSpecification Mapper", func() {
	Context("MapRequest", func() {
		It("must map an EventSpecification to a CRD correctly", func() {
			specOrFileId := "test-file-id"

			result, err := MapRequest(eventSpecification, specOrFileId, resourceIdInfo)
			Expect(err).To(BeNil())
			Expect(result).ToNot(BeNil())
			snaps.MatchSnapshot(GinkgoT(), result)
		})

		It("must return an error if the derived name does not match the resource id name", func() {
			mismatchedId := mapper.ResourceIdInfo{
				Name:        "wrong-name",
				Environment: "poc",
				Namespace:   "eni--hyperion",
			}

			result, err := MapRequest(eventSpecification, "test-file-id", mismatchedId)
			Expect(result).To(BeNil())
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("does not match expected name"))
		})
	})
})
