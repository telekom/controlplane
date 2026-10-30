// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package kongutil_test

import (
	"github.com/telekom/controlplane/gateway/pkg/kong/legacy"
	"github.com/telekom/controlplane/gateway/pkg/kongutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SetMigrator", func() {
	AfterEach(func() {
		kongutil.SetMigrator(nil)
	})

	It("drops cached clients so that new clients get the Migrator", func() {
		cfg := kongutil.NewGatewayConfig("http://kong.example/admin-api", "client", "secret", "http://issuer.example")
		before, err := kongutil.GetClientFor(cfg)
		Expect(err).NotTo(HaveOccurred())

		m, err := legacy.LoadMigrator("")
		Expect(err).NotTo(HaveOccurred())
		kongutil.SetMigrator(m)

		after, err := kongutil.GetClientFor(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(after).NotTo(BeIdenticalTo(before))

		again, err := kongutil.GetClientFor(cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(again).To(BeIdenticalTo(after))
	})
})
