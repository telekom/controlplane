// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/telekom/controlplane/rover-server/internal/api"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("validateChangelogItems", func() {
	valid := api.ApiChangelogItem{Date: "2022-06-20", Version: "2.0.2", Description: "Enhancements"}

	DescribeTable("accepts well-formed items",
		func(item api.ApiChangelogItem) {
			Expect(validateChangelogItems([]api.ApiChangelogItem{item})).To(Succeed())
		},
		Entry("plain release", valid),
		Entry("pre-release and build metadata", api.ApiChangelogItem{Date: "2022-06-20", Version: "1.0.0-rc.1+build.5", Description: "x"}),
		Entry("with version url", api.ApiChangelogItem{Date: "2022-06-20", Version: "0.1.0", Description: "x", VersionUrl: "https://example.com"}),
	)

	DescribeTable("rejects malformed items and names the offending field",
		func(item api.ApiChangelogItem, field string) {
			err := validateChangelogItems([]api.ApiChangelogItem{valid, item})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("items[1]." + field))
		},
		Entry("german date format", api.ApiChangelogItem{Date: "20.06.2022", Version: "1.0.0", Description: "x"}, "date"),
		Entry("impossible date", api.ApiChangelogItem{Date: "2022-02-30", Version: "1.0.0", Description: "x"}, "date"),
		Entry("empty date", api.ApiChangelogItem{Version: "1.0.0", Description: "x"}, "date"),
		Entry("leading v in version", api.ApiChangelogItem{Date: "2022-06-20", Version: "v1.0.0", Description: "x"}, "version"),
		Entry("two-part version", api.ApiChangelogItem{Date: "2022-06-20", Version: "1.0", Description: "x"}, "version"),
		Entry("empty version", api.ApiChangelogItem{Date: "2022-06-20", Description: "x"}, "version"),
		Entry("blank description", api.ApiChangelogItem{Date: "2022-06-20", Version: "1.0.0", Description: "  "}, "description"),
	)
})
