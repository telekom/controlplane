// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"
	"time"

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

	Context("ReconcileHint", func() {
		It("should store and retrieve the hint from context", func() {
			ctx := context.Background()
			hint := &ReconcileHint{}
			ctx = WithReconcileHint(ctx, hint)

			got, ok := ReconcileHintFromContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(got).To(BeIdenticalTo(hint))
		})

		It("should return false when no hint is in context", func() {
			ctx := context.Background()
			_, ok := ReconcileHintFromContext(ctx)
			Expect(ok).To(BeFalse())
		})

		It("should allow the handler to set RequeueAfter via SetRequeueAfter", func() {
			ctx := context.Background()
			hint := &ReconcileHint{}
			ctx = WithReconcileHint(ctx, hint)

			SetRequeueAfter(ctx, 10*time.Second)

			Expect(hint.RequeueAfter).ToNot(BeNil())
			Expect(*hint.RequeueAfter).To(Equal(10 * time.Second))
		})

		It("should be a no-op when context has no hint", func() {
			ctx := context.Background()
			Expect(func() {
				SetRequeueAfter(ctx, 5*time.Second)
			}).ToNot(Panic())
		})
	})

	Context("ReconcileHint", func() {

		It("should store and retrieve the hint from context", func() {
			ctx := context.Background()
			hint := &ReconcileHint{}
			ctx = WithReconcileHint(ctx, hint)

			got, ok := ReconcileHintFromContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(got).To(BeIdenticalTo(hint))
		})

		It("should return false when no hint is in context", func() {
			ctx := context.Background()
			_, ok := ReconcileHintFromContext(ctx)
			Expect(ok).To(BeFalse())
		})

		It("should allow the handler to set RequeueAfter via SetRequeueAfter", func() {
			ctx := context.Background()
			hint := &ReconcileHint{}
			ctx = WithReconcileHint(ctx, hint)

			SetRequeueAfter(ctx, 10*time.Second)

			Expect(hint.RequeueAfter).ToNot(BeNil())
			Expect(*hint.RequeueAfter).To(Equal(10 * time.Second))
		})

		It("should be a no-op when context has no hint", func() {
			ctx := context.Background()
			Expect(func() {
				SetRequeueAfter(ctx, 5*time.Second)
			}).ToNot(Panic())
		})
	})
})
