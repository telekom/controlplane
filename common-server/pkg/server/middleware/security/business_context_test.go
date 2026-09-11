// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package security_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
)

var _ = DescribeTable("DetermineClientType",
	func(scope string, expected security.ClientType) {
		actual, err := security.DetermineClientType([]string{scope}, "tardis")
		Expect(err).NotTo(HaveOccurred())
		Expect(actual).To(Equal(expected))
	},
	Entry("group", "tardis:group:read", security.ClientTypeGroup),
	Entry("legacy hub", "tardis:hub:read", security.ClientTypeGroup),
	Entry("team", "tardis:team:all", security.ClientTypeTeam),
	Entry("admin", "tardis:admin:obfuscated", security.ClientTypeAdmin),
)

var _ = DescribeTable("rejects invalid client scopes",
	func(scope string) {
		_, err := security.DetermineClientType([]string{scope}, "tardis")
		Expect(err).To(HaveOccurred())
	},
	Entry("wrong prefix", "other:group:read"),
	Entry("invalid client type", "tardis:organization:read"),
)
