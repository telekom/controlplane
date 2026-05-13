// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package reminder

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func d(h int) metav1.Duration {
	return metav1.Duration{Duration: time.Duration(h) * time.Hour}
}

func dp() *metav1.Duration {
	d := d(24)
	return &d
}

func ref(name string) types.ObjectRef {
	return types.ObjectRef{Name: name, Namespace: "ns"}
}

var _ = Describe("Evaluate", func() {
	Context("single one-shot threshold", func() {
		It("should fire when inside the window and never sent", func() {
			deadline := time.Now().Add(5 * 24 * time.Hour) // 5 days out
			thresholds := []Threshold{{Before: d(168)}}    // 7 days

			pending := Evaluate(deadline, thresholds, nil, time.Now())
			Expect(pending).To(HaveLen(1))
			Expect(pending[0].Key).To(Equal(d(168).Duration.String()))
		})

		It("should not fire when already sent", func() {
			now := time.Now()
			deadline := now.Add(5 * 24 * time.Hour)
			thresholds := []Threshold{{Before: d(168)}}
			sent := []SentReminder{{
				Threshold: d(168).Duration.String(),
				Ref:       ref("n1"),
				SentAt:    metav1.NewTime(now.Add(-1 * time.Hour)),
			}}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(BeEmpty())
		})

		It("should not fire when outside the window", func() {
			deadline := time.Now().Add(30 * 24 * time.Hour) // 30 days out
			thresholds := []Threshold{{Before: d(168)}}     // 7 day window

			pending := Evaluate(deadline, thresholds, nil, time.Now())
			Expect(pending).To(BeEmpty())
		})
	})

	Context("past deadline", func() {
		It("should fire the largest threshold when never sent", func() {
			now := time.Now()
			deadline := now.Add(-1 * time.Hour)
			thresholds := []Threshold{
				{Before: d(24)},
				{Before: d(168)},
			}

			pending := Evaluate(deadline, thresholds, nil, now)
			Expect(pending).To(HaveLen(1))
			Expect(pending[0].Key).To(Equal(d(168).Duration.String()))
		})

		It("should not fire when all thresholds already sent", func() {
			now := time.Now()
			deadline := now.Add(-1 * time.Hour)
			thresholds := []Threshold{
				{Before: d(24)},
				{Before: d(168)},
			}
			sent := []SentReminder{
				{Threshold: d(24).Duration.String(), Ref: ref("n1"), SentAt: metav1.NewTime(now.Add(-2 * time.Hour))},
				{Threshold: d(168).Duration.String(), Ref: ref("n2"), SentAt: metav1.NewTime(now.Add(-48 * time.Hour))},
			}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(BeEmpty())
		})

		It("should fire the largest unsent threshold when partially sent", func() {
			now := time.Now()
			deadline := now.Add(-1 * time.Hour)
			thresholds := []Threshold{
				{Before: d(24)},
				{Before: d(168)},
			}
			sent := []SentReminder{
				{Threshold: d(168).Duration.String(), Ref: ref("n1"), SentAt: metav1.NewTime(now.Add(-48 * time.Hour))},
			}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(HaveLen(1))
			Expect(pending[0].Key).To(Equal(d(24).Duration.String()))
		})
	})

	Context("multiple thresholds", func() {
		It("should only fire the tightest matching threshold", func() {
			now := time.Now()
			deadline := now.Add(3 * 24 * time.Hour) // 3 days out

			thresholds := []Threshold{
				{Before: d(720)}, // 30 days — in window
				{Before: d(168)}, // 7 days — in window (tightest)
				{Before: d(24)},  // 1 day — NOT in window
			}

			pending := Evaluate(deadline, thresholds, nil, now)
			Expect(pending).To(HaveLen(1))
			Expect(pending[0].Key).To(Equal(d(168).Duration.String()))
		})

		It("should not fire when the tightest one-shot is already sent", func() {
			now := time.Now()
			deadline := now.Add(3 * 24 * time.Hour)

			thresholds := []Threshold{
				{Before: d(720)},
				{Before: d(168)},
				{Before: d(24)},
			}

			sent := []SentReminder{{
				Threshold: d(168).Duration.String(),
				Ref:       ref("n1"),
				SentAt:    metav1.NewTime(now.Add(-2 * 24 * time.Hour)),
			}}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(BeEmpty())
		})
	})

	Context("repeating thresholds", func() {
		It("should not fire when the repeat interval has not elapsed", func() {
			now := time.Now()
			deadline := now.Add(3 * 24 * time.Hour)
			thresholds := []Threshold{
				{Before: d(168), Repeat: dp()},
			}
			sent := []SentReminder{{
				Threshold: d(168).Duration.String(),
				Ref:       ref("n1"),
				SentAt:    metav1.NewTime(now.Add(-12 * time.Hour)),
			}}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(BeEmpty())
		})

		It("should fire when the repeat interval has elapsed", func() {
			now := time.Now()
			deadline := now.Add(3 * 24 * time.Hour)
			thresholds := []Threshold{
				{Before: d(168), Repeat: dp()},
			}
			sent := []SentReminder{{
				Threshold: d(168).Duration.String(),
				Ref:       ref("n1"),
				SentAt:    metav1.NewTime(now.Add(-25 * time.Hour)),
			}}

			pending := Evaluate(deadline, thresholds, sent, now)
			Expect(pending).To(HaveLen(1))
		})
	})
})

var _ = Describe("NextRequeue", func() {
	It("should return time until entering the window when not yet in window", func() {
		now := time.Now()
		deadline := now.Add(10 * 24 * time.Hour)
		thresholds := []Threshold{{Before: d(168)}}

		requeue := NextRequeue(deadline, thresholds, nil, now)
		expected := 10*24*time.Hour - 168*time.Hour
		Expect(requeue.Seconds()).To(BeNumerically("~", expected.Seconds(), 1))
	})

	It("should return 0 when a reminder should fire now", func() {
		now := time.Now()
		deadline := now.Add(5 * 24 * time.Hour)
		thresholds := []Threshold{{Before: d(168)}}

		requeue := NextRequeue(deadline, thresholds, nil, now)
		Expect(requeue).To(Equal(time.Duration(0)))
	})

	It("should return remaining repeat interval when repeat is scheduled", func() {
		now := time.Now()
		deadline := now.Add(3 * 24 * time.Hour)
		thresholds := []Threshold{
			{Before: d(168), Repeat: dp()},
		}
		sent := []SentReminder{{
			Threshold: d(168).Duration.String(),
			Ref:       ref("n1"),
			SentAt:    metav1.NewTime(now.Add(-10 * time.Hour)),
		}}

		requeue := NextRequeue(deadline, thresholds, sent, now)
		Expect(requeue.Seconds()).To(BeNumerically("~", (14 * time.Hour).Seconds(), 1))
	})

	It("should wait for next threshold when one-shot is already sent", func() {
		now := time.Now()
		deadline := now.Add(3 * 24 * time.Hour)

		thresholds := []Threshold{
			{Before: d(168)},
			{Before: d(24)},
		}
		sent := []SentReminder{{
			Threshold: d(168).Duration.String(),
			Ref:       ref("n1"),
			SentAt:    metav1.NewTime(now.Add(-1 * 24 * time.Hour)),
		}}

		requeue := NextRequeue(deadline, thresholds, sent, now)
		expected := 3*24*time.Hour - 24*time.Hour
		Expect(requeue.Seconds()).To(BeNumerically("~", expected.Seconds(), 1))
	})

	It("should return 0 when past deadline", func() {
		requeue := NextRequeue(time.Now().Add(-1*time.Hour), []Threshold{{Before: d(168)}}, nil, time.Now())
		Expect(requeue).To(Equal(time.Duration(0)))
	})
})

var _ = Describe("UpsertSent", func() {
	It("should insert a new entry", func() {
		entry := &SentReminder{Threshold: "168h0m0s", Ref: ref("n1"), SentAt: metav1.Now()}
		result := UpsertSent(nil, entry)
		Expect(result).To(HaveLen(1))
		Expect(result[0].Threshold).To(Equal("168h0m0s"))
	})

	It("should update an existing entry", func() {
		existing := []SentReminder{
			{Threshold: "168h0m0s", Ref: ref("n1"), SentAt: metav1.NewTime(time.Now().Add(-1 * time.Hour))},
		}
		updated := &SentReminder{Threshold: "168h0m0s", Ref: ref("n2"), SentAt: metav1.Now()}

		result := UpsertSent(existing, updated)
		Expect(result).To(HaveLen(1))
		Expect(result[0].Ref.Name).To(Equal("n2"))
	})
})

var _ = Describe("NextRequeue edge cases", func() {
	It("should return 0 for a repeating threshold that was never sent and is in window", func() {
		now := time.Now()
		deadline := now.Add(3 * 24 * time.Hour)
		thresholds := []Threshold{
			{Before: d(168), Repeat: dp()},
		}

		requeue := NextRequeue(deadline, thresholds, nil, now)
		Expect(requeue).To(Equal(time.Duration(0)))
	})

	It("should return 0 for a repeating threshold that is overdue", func() {
		now := time.Now()
		deadline := now.Add(3 * 24 * time.Hour)
		thresholds := []Threshold{
			{Before: d(168), Repeat: dp()},
		}
		sent := []SentReminder{{
			Threshold: d(168).Duration.String(),
			Ref:       ref("n1"),
			SentAt:    metav1.NewTime(now.Add(-48 * time.Hour)), // 48h ago, repeat is 24h
		}}

		requeue := NextRequeue(deadline, thresholds, sent, now)
		Expect(requeue).To(Equal(time.Duration(0)))
	})

	It("should return timeUntilDeadline when no threshold windows exist", func() {
		now := time.Now()
		deadline := now.Add(1 * time.Hour)
		// All thresholds have Before smaller than timeUntilDeadline — none match
		thresholds := []Threshold{
			{Before: d(0)}, // 0h window — never matches
		}

		requeue := NextRequeue(deadline, thresholds, nil, now)
		Expect(requeue.Seconds()).To(BeNumerically("~", (1 * time.Hour).Seconds(), 1))
	})
})

var _ = Describe("SortDesc", func() {
	It("should sort thresholds descending without modifying the original", func() {
		thresholds := []Threshold{
			{Before: d(24)},
			{Before: d(720)},
			{Before: d(168)},
		}
		sorted := SortDesc(thresholds)
		Expect(sorted[0].Before.Duration).To(Equal(d(720).Duration))
		Expect(sorted[1].Before.Duration).To(Equal(d(168).Duration))
		Expect(sorted[2].Before.Duration).To(Equal(d(24).Duration))

		// Original unchanged
		Expect(thresholds[0].Before.Duration).To(Equal(d(24).Duration))
	})
})

var _ = Describe("Threshold DeepCopy", func() {
	It("should deep copy a Threshold without Repeat", func() {
		original := &Threshold{Before: d(168)}
		copied := original.DeepCopy()

		Expect(copied.Before.Duration).To(Equal(original.Before.Duration))
		Expect(copied.Repeat).To(BeNil())
		Expect(copied).ToNot(BeIdenticalTo(original))
	})

	It("should deep copy a Threshold with Repeat", func() {
		original := &Threshold{Before: d(168), Repeat: dp()}
		copied := original.DeepCopy()

		Expect(copied.Before.Duration).To(Equal(original.Before.Duration))
		Expect(copied.Repeat).ToNot(BeNil())
		Expect(copied.Repeat.Duration).To(Equal(original.Repeat.Duration))
		// Ensure it's a different pointer
		Expect(copied.Repeat).ToNot(BeIdenticalTo(original.Repeat))
	})

	It("should return nil when copying a nil Threshold", func() {
		var original *Threshold
		Expect(original.DeepCopy()).To(BeNil())
	})

	It("should deep copy into an existing Threshold", func() {
		original := Threshold{Before: d(168), Repeat: dp()}
		out := Threshold{}
		original.DeepCopyInto(&out)

		Expect(out.Before.Duration).To(Equal(original.Before.Duration))
		Expect(out.Repeat).ToNot(BeNil())
		Expect(out.Repeat.Duration).To(Equal(original.Repeat.Duration))
		Expect(out.Repeat).ToNot(BeIdenticalTo(original.Repeat))
	})
})

var _ = Describe("SentReminder DeepCopy", func() {
	It("should deep copy a SentReminder", func() {
		original := &SentReminder{
			Threshold: "168h0m0s",
			Ref:       ref("n1"),
			SentAt:    metav1.Now(),
		}
		copied := original.DeepCopy()

		Expect(copied.Threshold).To(Equal(original.Threshold))
		Expect(copied.Ref.Name).To(Equal(original.Ref.Name))
		Expect(copied.Ref.Namespace).To(Equal(original.Ref.Namespace))
		Expect(copied).ToNot(BeIdenticalTo(original))
	})

	It("should return nil when copying a nil SentReminder", func() {
		var original *SentReminder
		Expect(original.DeepCopy()).To(BeNil())
	})

	It("should deep copy into an existing SentReminder", func() {
		original := SentReminder{
			Threshold: "168h0m0s",
			Ref:       ref("n1"),
			SentAt:    metav1.Now(),
		}
		out := SentReminder{}
		original.DeepCopyInto(&out)

		Expect(out.Threshold).To(Equal(original.Threshold))
		Expect(out.Ref.Name).To(Equal(original.Ref.Name))
	})
})
