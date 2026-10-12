// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Kong controller setup", func() {
	It("rejects a Route controller without a Kong client factory", func() {
		err := (&RouteReconciler{}).SetupWithManager(nil)
		Expect(err).To(MatchError("setting up route controller: kong client factory is required"))
	})

	It("rejects a Consumer controller without a Kong client factory", func() {
		err := (&ConsumerReconciler{}).SetupWithManager(nil)
		Expect(err).To(MatchError("setting up consumer controller: kong client factory is required"))
	})
})
