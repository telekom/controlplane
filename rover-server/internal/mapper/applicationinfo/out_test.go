// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package applicationinfo

import (
	"github.com/gkampitakis/go-snaps/snaps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/rover-server/internal/api"
)

var _ = Describe("ApplicationInfo Mapper", func() {
	Context("FillExposureInfo", func() {
		It("must fill exposure info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, rover, applicationInfo)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillExposureInfo(ctx, nil, applicationInfo)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillExposureInfo(ctx, rover, nil)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})
	})

	Context("FillSubscriptionInfo", func() {
		It("must fill subscription info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, rover, applicationInfo)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillSubscriptionInfo(ctx, nil, applicationInfo)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillSubscriptionInfo(ctx, rover, nil)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})
	})

	Context("FillApplicationInfo", func() {
		It("must fill application info correctly", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, rover, applicationInfo)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), applicationInfo)
		})

		It("must return an error if the input rover is nil", func() {
			var applicationInfo = &api.ApplicationInfo{}
			err := FillApplicationInfo(ctx, nil, applicationInfo)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("rover resource is not processed and does not contain an application"))
		})

		It("must return an error if the input application info is nil", func() {
			err := FillApplicationInfo(ctx, rover, nil)

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input applicationInfo is nil"))
		})
	})

	Context("MapApplicationInfo", func() {
		It("must map application info correctly", func() {
			output, err := MapApplicationInfo(ctx, rover)

			Expect(err).To(BeNil())

			snaps.MatchJSON(GinkgoT(), output)
		})

		It("must return an error if the input rover is nil", func() {
			output, err := MapApplicationInfo(ctx, nil)

			Expect(output).To(BeNil())

			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("input rover is nil"))
		})
	})

})
