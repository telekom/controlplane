// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package contextutil

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/test/mock"
	"k8s.io/client-go/tools/events"
)

var _ = Describe("Recorder", func() {

	Context("record.EventRecorder", func() {

		It("should manage the recorder in the context", func() {
			ctx := context.Background()

			By("setting the recorder in the context")
			ctx = WithRecorder(ctx, &mock.EventRecorder{})

			By("getting the recorder from the context")
			recorder, ok := RecoderFromContext(ctx)

			Expect(ok).To(BeTrue())
			Expect(recorder).ToNot(BeNil())
		})

		It("should return false when no recorder is in the context", func() {
			ctx := context.Background()

			recorder, ok := RecoderFromContext(ctx)
			Expect(ok).To(BeFalse())
			Expect(recorder).To(BeNil())
		})

		It("should panic if the recorder is not found in the context", func() {
			ctx := context.Background()

			Expect(func() {
				RecorderFromContextOrDie(ctx)
			}).To(Panic())
		})

		It("should not panic when the recorder exists in the context", func() {
			ctx := WithRecorder(context.Background(), &mock.EventRecorder{})

			Expect(func() {
				recorder := RecorderFromContextOrDie(ctx)
				Expect(recorder).ToNot(BeNil())
			}).ToNot(Panic())
		})
	})

	Context("events.EventRecorder", func() {

		It("should manage the event recorder in the context", func() {
			ctx := WithEventRecorder(context.Background(), &NoopRecorder{})

			recorder, ok := EventRecorderFromContext(ctx)
			Expect(ok).To(BeTrue())
			Expect(recorder).ToNot(BeNil())
		})

		It("should return false when no event recorder is in the context", func() {
			ctx := context.Background()

			recorder, ok := EventRecorderFromContext(ctx)
			Expect(ok).To(BeFalse())
			Expect(recorder).To(BeNil())
		})

		It("should return false when the context holds a record.EventRecorder instead", func() {
			ctx := WithRecorder(context.Background(), &mock.EventRecorder{})

			recorder, ok := EventRecorderFromContext(ctx)
			Expect(ok).To(BeFalse())
			Expect(recorder).To(BeNil())
		})

		It("should return the noop recorder when none is in the context", func() {
			ctx := context.Background()

			recorder := EventRecorderFromContextOrDiscard(ctx)
			Expect(recorder).ToNot(BeNil())
			// Should be a NoopRecorder — calling Eventf should not panic
			Expect(func() {
				recorder.Eventf(nil, nil, "Normal", "Test", "action", "note %s", "arg")
			}).ToNot(Panic())
		})

		It("should return the real recorder when one exists in the context", func() {
			expected := &NoopRecorder{}
			ctx := WithEventRecorder(context.Background(), expected)

			recorder := EventRecorderFromContextOrDiscard(ctx)
			Expect(recorder).To(BeIdenticalTo(expected))
		})
	})

	Context("NoopRecorder", func() {
		It("should implement events.EventRecorder", func() {
			var _ events.EventRecorder = &NoopRecorder{}
		})

		It("should not panic on Eventf call", func() {
			recorder := &NoopRecorder{}
			Expect(func() {
				recorder.Eventf(nil, nil, "Normal", "Reason", "action", "msg %s", "arg")
			}).ToNot(Panic())
		})
	})
})
