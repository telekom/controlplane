// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package approvalexpiration

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/telekom/controlplane/approval/api/v1"
	"github.com/telekom/controlplane/approval/internal/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApprovalExpirationHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApprovalExpiration Handler Suite")
}

var _ = Describe("ApprovalExpiration Handler", func() {
	var handler *Handler
	var cfg *config.ExpirationConfig

	BeforeEach(func() {
		cfg = &config.ExpirationConfig{
			ExpirationPeriodMonths:       12,
			LastMonthsWithWeeklyReminder: 2,
			LastWeeksWithDailyReminder:   2,
		}
		handler = &Handler{
			client: nil, // Not needed for unit tests
			config: cfg,
		}
	})

	Describe("shouldRemind", func() {
		var ae *v1.ApprovalExpiration
		var now time.Time

		BeforeEach(func() {
			now = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
			// Realistic timeline: expiration in 3 months
			expirationDate := now.AddDate(0, 3, 0)     // 3 months from now (Aug 1)
			weeklyReminderDate := now.AddDate(0, 1, 0) // 1 month from now (Jun 1) - last 2 months
			dailyReminderDate := now.AddDate(0, 2, 17) // 2 months 17 days from now (Jul 18) - last 2 weeks

			ae = &v1.ApprovalExpiration{
				Spec: v1.ApprovalExpirationSpec{
					Expiration:     metav1.Time{Time: expirationDate},
					WeeklyReminder: metav1.Time{Time: weeklyReminderDate},
					DailyReminder:  metav1.Time{Time: dailyReminderDate},
				},
				Status: v1.ApprovalExpirationStatus{},
			}
		})

		Context("when before weekly reminder period", func() {
			It("should not send reminder", func() {
				testNow := ae.Spec.WeeklyReminder.Add(-24 * time.Hour) // 1 day before weekly starts
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeFalse())
			})
		})

		Context("when in weekly reminder period", func() {
			It("should send reminder if never reminded", func() {
				testNow := ae.Spec.WeeklyReminder.Add(1 * time.Hour)
				ae.Status.LastReminder = nil
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should send reminder if last reminded over a week ago", func() {
				testNow := ae.Spec.WeeklyReminder.Add(8 * 24 * time.Hour) // 8 days after weekly start
				lastReminder := testNow.Add(-8 * 24 * time.Hour)          // 8 days ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should not send reminder if last reminded less than a week ago", func() {
				// Still in weekly period, not yet in daily period
				testNow := ae.Spec.WeeklyReminder.Add(10 * 24 * time.Hour) // 10 days after weekly start, still before daily
				lastReminder := testNow.Add(-5 * 24 * time.Hour)           // 5 days ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeFalse())
			})
		})

		Context("when in daily reminder period", func() {
			It("should send reminder if never reminded", func() {
				testNow := ae.Spec.DailyReminder.Add(1 * time.Hour)
				ae.Status.LastReminder = nil
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should send reminder if last reminded over a day ago", func() {
				testNow := ae.Spec.DailyReminder.Add(2 * 24 * time.Hour) // 2 days after daily start
				lastReminder := testNow.Add(-25 * time.Hour)             // 25 hours ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should not send reminder if last reminded less than a day ago", func() {
				testNow := ae.Spec.DailyReminder.Add(2 * 24 * time.Hour) // 2 days after daily start
				lastReminder := testNow.Add(-20 * time.Hour)             // 20 hours ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeFalse())
			})
		})

		Context("when expired", func() {
			It("should send reminder if never reminded", func() {
				testNow := ae.Spec.Expiration.Add(1 * time.Hour) // 1 hour after expiration
				ae.Status.LastReminder = nil
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should send reminder if last reminded over a day ago", func() {
				testNow := ae.Spec.Expiration.Add(3 * 24 * time.Hour) // 3 days after expiration
				lastReminder := testNow.Add(-25 * time.Hour)          // 25 hours ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})

			It("should not send reminder if last reminded less than a day ago", func() {
				testNow := ae.Spec.Expiration.Add(3 * 24 * time.Hour) // 3 days after expiration
				lastReminder := testNow.Add(-20 * time.Hour)          // 20 hours ago
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeFalse())
			})
		})

		Context("when exactly at expiration time", func() {
			It("should send reminder", func() {
				testNow := ae.Spec.Expiration.Time
				ae.Status.LastReminder = nil
				result := handler.shouldRemind(ae, testNow)
				Expect(result).To(BeTrue())
			})
		})
	})

	Describe("requeueAtNextEvent", func() {
		var ae *v1.ApprovalExpiration
		var now time.Time

		BeforeEach(func() {
			now = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
			// Realistic timeline: expiration in 3 months
			expirationDate := now.AddDate(0, 3, 0)     // 3 months from now (Aug 1)
			weeklyReminderDate := now.AddDate(0, 1, 0) // 1 month from now (Jun 1) - last 2 months
			dailyReminderDate := now.AddDate(0, 2, 17) // 2 months 17 days from now (Jul 18) - last 2 weeks

			ae = &v1.ApprovalExpiration{
				Spec: v1.ApprovalExpirationSpec{
					Expiration:     metav1.Time{Time: expirationDate},
					WeeklyReminder: metav1.Time{Time: weeklyReminderDate},
					DailyReminder:  metav1.Time{Time: dailyReminderDate},
				},
				Status: v1.ApprovalExpirationStatus{},
			}
		})

		Context("when before weekly reminder period", func() {
			It("should requeue at weekly reminder time", func() {
				testNow := ae.Spec.WeeklyReminder.Add(-1 * time.Hour)
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})
		})

		Context("when in weekly reminder period", func() {
			It("should requeue at next weekly interval if reminded", func() {
				testNow := ae.Spec.WeeklyReminder.Add(8 * 24 * time.Hour)
				lastReminder := testNow.Add(-1 * time.Hour)
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})

			It("should requeue at daily reminder time if next weekly is after daily", func() {
				testNow := ae.Spec.WeeklyReminder.Add(1 * 24 * time.Hour)
				lastReminder := testNow.Add(-1 * time.Hour)
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})
		})

		Context("when in daily reminder period", func() {
			It("should requeue at next daily interval if reminded", func() {
				testNow := ae.Spec.DailyReminder.Add(25 * time.Hour)
				lastReminder := testNow.Add(-1 * time.Hour)
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})

			It("should requeue at expiration time if next daily is after expiration", func() {
				testNow := ae.Spec.Expiration.Add(-1 * time.Hour)
				lastReminder := testNow.Add(-1 * time.Hour)
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})
		})

		Context("when expired", func() {
			It("should requeue at next daily interval for continued reminders", func() {
				testNow := ae.Spec.Expiration.Add(3 * 24 * time.Hour)
				lastReminder := testNow.Add(-1 * time.Hour)
				ae.Status.LastReminder = &metav1.Time{Time: lastReminder}
				err := handler.requeueAtNextEvent(ae, testNow)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("next expiration event"))
			})
		})
	})
})
