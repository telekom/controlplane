// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package shared_test

import (
	"github.com/telekom/controlplane/projector/internal/domain/shared"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Approval mapping", func() {
	Describe("MapApprovalState", func() {
		It("maps PascalCase states to upper-case enums", func() {
			Expect(shared.MapApprovalState("Approved")).To(Equal("APPROVED"))
			Expect(shared.MapApprovalState("Pending")).To(Equal("PENDING"))
		})

		It("keeps SCREAMING_SNAKE states unchanged", func() {
			Expect(shared.MapApprovalState("WAITING_FOR_APPROVAL")).To(Equal("WAITING_FOR_APPROVAL"))
		})

		It("returns empty string for empty input", func() {
			Expect(shared.MapApprovalState("")).To(BeEmpty())
		})
	})

	Describe("MapApprovalStrategy", func() {
		It("maps known strategies", func() {
			Expect(shared.MapApprovalStrategy("Auto")).To(Equal("AUTO"))
			Expect(shared.MapApprovalStrategy("Simple")).To(Equal("SIMPLE"))
			Expect(shared.MapApprovalStrategy("FourEyes")).To(Equal("FOUR_EYES"))
		})

		It("maps unknown strategies by upper-casing", func() {
			Expect(shared.MapApprovalStrategy("ManualReview")).To(Equal("MANUALREVIEW"))
		})

		It("keeps SCREAMING_SNAKE strategies unchanged", func() {
			Expect(shared.MapApprovalStrategy("NEEDS_ESCALATION")).To(Equal("NEEDS_ESCALATION"))
		})

		It("returns empty string for empty input", func() {
			Expect(shared.MapApprovalStrategy("")).To(BeEmpty())
		})
	})
})
