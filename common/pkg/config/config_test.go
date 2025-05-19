// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config Test", func() {
	BeforeEach(func() {

	})

	Context("RetryNWithJitterOnError", func() {

		It("should return a time.Duration that jitters", func() {
			var lastDelay time.Duration
			for i := 0; i < 10; i++ {
				delay := RetryNWithJitterOnError(0)
				Expect(delay).To(BeNumerically(">", RequeueAfterOnError))
				Expect(delay).ToNot(Equal(lastDelay))
				lastDelay = delay
			}
		})
	})

	Context("RequestWithJitter", func() {

		It("should return a time.Duration that jitters", func() {
			var lastDelay time.Duration
			for i := 0; i < 10; i++ {
				delay := RequeueWithJitter()
				Expect(delay).To(BeNumerically(">", RequeueAfter))
				Expect(delay).ToNot(Equal(lastDelay))
				lastDelay = delay
			}
		})
	})

})
