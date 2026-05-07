// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalexpiration

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/config"
	"github.com/telekom/controlplane/common/pkg/reminder"
)

func TestApprovalExpirationHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApprovalExpiration Handler Suite")
}

var _ = Describe("ApprovalExpiration Handler", func() {
	var cfg *config.ExpirationConfig

	BeforeEach(func() {
		// Create config with default thresholds
		weeklyBefore := 30 * 24 * time.Hour
		dailyBefore := 7 * 24 * time.Hour
		dailyRepeat := 24 * time.Hour

		cfg = &config.ExpirationConfig{
			ExpirationDuration: 12 * 30 * 24 * time.Hour, // 12 months
			DefaultThresholds: []reminder.Threshold{
				{Before: metav1.Duration{Duration: weeklyBefore}},
				{Before: metav1.Duration{Duration: dailyBefore}, Repeat: &metav1.Duration{Duration: dailyRepeat}},
			},
		}
	})

	Describe("config integration", func() {
		It("should have valid default thresholds", func() {
			Expect(cfg.DefaultThresholds).To(HaveLen(2))
			Expect(cfg.DefaultThresholds[0].Before.Duration).To(Equal(30 * 24 * time.Hour))
			Expect(cfg.DefaultThresholds[0].Repeat).To(BeNil())
			Expect(cfg.DefaultThresholds[1].Before.Duration).To(Equal(7 * 24 * time.Hour))
			Expect(cfg.DefaultThresholds[1].Repeat).NotTo(BeNil())
			Expect(cfg.DefaultThresholds[1].Repeat.Duration).To(Equal(24 * time.Hour))
		})
	})

	Describe("reminder evaluation", func() {
		It("should use common/pkg/reminder for scheduling", func() {
			now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
			expiration := now.Add(10 * 24 * time.Hour) // 10 days out

			ae := &v1.ApprovalExpiration{
				Spec: v1.ApprovalExpirationSpec{
					Expiration: metav1.Time{Time: expiration},
					Thresholds: cfg.DefaultThresholds,
				},
				Status: v1.ApprovalExpirationStatus{
					SentReminders: []reminder.SentReminder{},
				},
			}

			// 10 days out: we're in the weekly window (30 days before) but not yet in daily window (7 days before)
			// reminder.Evaluate returns the tightest matching threshold
			pending := reminder.Evaluate(ae.Spec.Expiration.Time, ae.Spec.Thresholds, ae.Status.SentReminders, now)
			Expect(pending).To(HaveLen(1))
			Expect(pending[0].Key).To(Equal("720h0m0s")) // 30 days (weekly threshold)
		})
	})
})
