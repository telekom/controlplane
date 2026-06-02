// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/eventexposure"
)

var _ = Describe("EventExposure Translator", func() {
	var t eventexposure.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &eventv1.EventExposure{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-exposure",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "my-app",
					},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.eni.quickstart.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval: eventv1.Approval{
						Strategy:     eventv1.ApprovalStrategyAuto,
						TrustedTeams: []string{"team-a"},
					},
				},
				Status: eventv1.EventExposureStatus{
					Active: true,
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Message: "all good",
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(data.Visibility).To(Equal("WORLD"))
			Expect(data.Active).To(BeTrue())
			Expect(data.ApprovalConfig.Strategy).To(Equal("AUTO"))
			Expect(data.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-a"}))
			Expect(data.AppName).To(Equal("my-app"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("all good"))
			Expect(data.Meta.Environment).To(Equal("prod"))
		})

		It("should upper-case Zone visibility", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.zone.v1",
					Visibility: eventv1.VisibilityZone,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategySimple},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Visibility).To(Equal("ZONE"))
		})

		It("should upper-case Enterprise visibility", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ent-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.ent.v1",
					Visibility: eventv1.VisibilityEnterprise,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Visibility).To(Equal("ENTERPRISE"))
		})

		It("should map FourEyes approval strategy to FOUR_EYES", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foureyes-approval",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.foureyes.v1",
					Visibility: eventv1.VisibilityEnterprise,
					Approval: eventv1.Approval{
						Strategy:     eventv1.ApprovalStrategyFourEyes,
						TrustedTeams: []string{"team-x", "team-y"},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
			Expect(data.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-x", "team-y"}))
		})

		It("should derive UNKNOWN status when no conditions are set", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-conditions",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.nocond.v1",
					Visibility: eventv1.VisibilityEnterprise,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should return composite key from CR fields", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType: "de.telekom.eni.quickstart.v1",
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(key.AppName).To(Equal("my-app"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use CR fields from lastKnown when available", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-exposure",
			}
			lastKnown := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType: "de.telekom.eni.quickstart.v1",
				},
			}

			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.EventType).To(Equal("de.telekom.eni.quickstart.v1"))
			Expect(key.AppName).To(Equal("my-app"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})

		It("should fall back to convention when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-exposure",
			}

			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.EventType).To(Equal("my-exposure"))
			Expect(key.AppName).To(Equal("my-exposure"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})
})
