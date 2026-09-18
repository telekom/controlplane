// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/prometheus/client_golang/prometheus/testutil"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kong reconciliation metrics", func() {
	It("records reconciliation outcomes independently", func() {
		unchanged := kongReconcileTotal.WithLabelValues("service", "unchanged")
		written := kongReconcileTotal.WithLabelValues("service", "written")
		unchangedBefore := testutil.ToFloat64(unchanged)
		writtenBefore := testutil.ToFloat64(written)

		unchanged.Inc()
		written.Inc()

		Expect(testutil.ToFloat64(unchanged)).To(Equal(unchangedBefore + 1))
		Expect(testutil.ToFloat64(written)).To(Equal(writtenBefore + 1))
	})
})
