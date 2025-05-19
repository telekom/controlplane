// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Contextutil", func() {

	Context("Env", func() {

		It("should manage the environment in the context", func() {
			ctx := context.Background()

			By("setting the environment in the context")
			ctx = WithEnv(ctx, "test")

			By("getting the environment from the context")
			env, ok := EnvFromContext(ctx)

			Expect(ok).To(BeTrue())
			Expect(env).To(Equal("test"))
		})

		It("should panic if the environment is not found in the context", func() {
			ctx := context.Background()

			By("getting the environment from the context")
			Expect(func() {
				EnvFromContextOrDie(ctx)
			}).To(Panic())
		})

	})
})
