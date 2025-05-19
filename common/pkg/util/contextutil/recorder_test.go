// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/test/mock"
)

var _ = Describe("Recorder", func() {

	Context("Env", func() {

		It("should manage the recorder in the context", func() {
			ctx := context.Background()

			By("setting the recorder in the context")
			ctx = WithRecorder(ctx, &mock.EventRecorder{})

			By("getting the recorder from the context")
			recorder, ok := RecoderFromContext(ctx)

			Expect(ok).To(BeTrue())
			Expect(recorder).ToNot(BeNil())
		})

		It("should panic if the recorder is not found in the context", func() {
			ctx := context.Background()

			By("getting the recorder from the context")
			Expect(func() {
				RecorderFromContextOrDie(ctx)
			}).To(Panic())
		})

	})
})
