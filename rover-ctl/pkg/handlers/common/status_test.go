// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/rover-ctl/pkg/handlers/common"
	"github.com/telekom/controlplane/rover-ctl/pkg/types"
)

var _ = Describe("ObjectStatusResponse", func() {
	var (
		status *common.ObjectStatusResponse
	)

	BeforeEach(func() {
		status = &common.ObjectStatusResponse{
			OverallStatus:   types.OverallStatusComplete,
			ProcessingState: types.ProcessingStateDone,
			Errors:          []types.StatusInfo{},
			Warnings:        []types.StatusInfo{},
			Info:            []types.StatusInfo{},
			Gone:            false,
		}
	})

	It("should implement the types.ObjectStatus interface", func() {
		// This is a compile-time check
		var _ types.ObjectStatus = &common.ObjectStatusResponse{}
	})

	Describe("GetOverallStatus", func() {
		Context("when getting the overall status", func() {
			It("should return the correct value", func() {
				status.OverallStatus = types.OverallStatusPending
				Expect(status.GetOverallStatus()).To(Equal(types.OverallStatusPending))
			})
		})
	})

	Describe("GetProcessingState", func() {
		Context("when getting the processing state", func() {
			It("should return the correct value", func() {
				status.ProcessingState = types.ProcessingStateProcessing
				Expect(status.GetProcessingState()).To(Equal(types.ProcessingStateProcessing))
			})
		})
	})

	Describe("HasErrors", func() {
		Context("when there are no errors", func() {
			It("should return false", func() {
				status.Errors = []types.StatusInfo{}
				Expect(status.HasErrors()).To(BeFalse())
			})
		})

		Context("when there are errors", func() {
			It("should return true", func() {
				status.Errors = []types.StatusInfo{
					{
						Cause:   "Error",
						Message: "An error occurred",
					},
				}
				Expect(status.HasErrors()).To(BeTrue())
			})
		})
	})

	Describe("HasWarnings", func() {
		Context("when there are no warnings", func() {
			It("should return false", func() {
				status.Warnings = []types.StatusInfo{}
				Expect(status.HasWarnings()).To(BeFalse())
			})
		})

		Context("when there are warnings", func() {
			It("should return true", func() {
				status.Warnings = []types.StatusInfo{
					{
						Cause:   "Warning",
						Message: "A warning occurred",
					},
				}
				Expect(status.HasWarnings()).To(BeTrue())
			})
		})
	})

	Describe("HasInfo", func() {
		Context("when there is no info", func() {
			It("should return false", func() {
				status.Info = []types.StatusInfo{}
				Expect(status.HasInfo()).To(BeFalse())
			})
		})

		Context("when there is info", func() {
			It("should return true", func() {
				status.Info = []types.StatusInfo{
					{
						Cause:   "Info",
						Message: "Information",
					},
				}
				Expect(status.HasInfo()).To(BeTrue())
			})
		})
	})

	Describe("IsGone", func() {
		Context("when checking the gone status", func() {
			It("should return true when gone is true", func() {
				status.Gone = true
				Expect(status.IsGone()).To(BeTrue())
			})

			It("should return false when gone is false", func() {
				status.Gone = false
				Expect(status.IsGone()).To(BeFalse())
			})
		})
	})

	Describe("GetErrors", func() {
		Context("when getting error information", func() {
			It("should return the complete errors slice", func() {
				errors := []types.StatusInfo{
					{
						Cause:   "Error1",
						Message: "First error",
					},
					{
						Cause:   "Error2",
						Message: "Second error",
					},
				}
				status.Errors = errors
				Expect(status.GetErrors()).To(Equal(errors))
			})
		})
	})

	Describe("GetWarnings", func() {
		Context("when getting warning information", func() {
			It("should return the complete warnings slice", func() {
				warnings := []types.StatusInfo{
					{
						Cause:   "Warning1",
						Message: "First warning",
					},
				}
				status.Warnings = warnings
				Expect(status.GetWarnings()).To(Equal(warnings))
			})
		})
	})

	Describe("GetInfo", func() {
		Context("when getting informational messages", func() {
			It("should return the complete info slice", func() {
				info := []types.StatusInfo{
					{
						Cause:   "Info1",
						Message: "First info",
					},
				}
				status.Info = info
				Expect(status.GetInfo()).To(Equal(info))
			})
		})
	})
})
