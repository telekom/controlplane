// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package status

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("CalculateOverallStatus", func() {
	Context("when processing state is Processing", func() {
		It("returns Processing", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateProcessing)
			Expect(result).To(Equal(api.OverallStatusProcessing))
		})
	})

	Context("when processing state is Failed", func() {
		It("returns Failed", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateFailed)
			Expect(result).To(Equal(api.OverallStatusFailed))
		})
	})

	Context("when state is Blocked", func() {
		It("returns Blocked", func() {
			result := CalculateOverallStatus(api.Blocked, api.ProcessingStatePending)
			Expect(result).To(Equal(api.OverallStatusBlocked))
		})
	})

	Context("when state is Complete and processing state is Done", func() {
		It("returns Complete", func() {
			result := CalculateOverallStatus(api.Complete, api.ProcessingStateDone)
			Expect(result).To(Equal(api.OverallStatusComplete))
		})
	})

	Context("when state is unknown", func() {
		It("returns None", func() {
			result := CalculateOverallStatus("unknown", api.ProcessingStateNone)
			Expect(result).To(Equal(api.OverallStatusNone))
		})
	})
})

var _ = Describe("CompareAndReturn", func() {
	Context("when comparing Failed with other statuses", func() {
		It("Failed takes precedence", func() {
			result := CompareAndReturn(api.OverallStatusProcessing, api.OverallStatusFailed)
			Expect(result).To(Equal(api.OverallStatusFailed))
		})
	})

	Context("when comparing Blocked with Processing", func() {
		It("Blocked takes precedence over Processing", func() {
			result := CompareAndReturn(api.OverallStatusProcessing, api.OverallStatusBlocked)
			Expect(result).To(Equal(api.OverallStatusBlocked))
		})
	})

	Context("when comparing Processing with Pending", func() {
		It("Processing takes precedence over Pending", func() {
			result := CompareAndReturn(api.OverallStatusPending, api.OverallStatusProcessing)
			Expect(result).To(Equal(api.OverallStatusProcessing))
		})
	})

	Context("when comparing Complete with Complete", func() {
		It("returns Complete", func() {
			result := CompareAndReturn(api.OverallStatusComplete, api.OverallStatusComplete)
			Expect(result).To(Equal(api.OverallStatusComplete))
		})
	})

	Context("when comparing None with None", func() {
		It("returns None", func() {
			result := CompareAndReturn(api.OverallStatusNone, api.OverallStatusNone)
			Expect(result).To(Equal(api.OverallStatusNone))
		})
	})
})
