// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package eventexposure_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	eventv1 "github.com/telekom/controlplane/event/api/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
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
					Scopes: []eventv1.EventScope{
						{
							Name: "scope-a",
							Trigger: eventv1.EventTrigger{
								ResponseFilter: &eventv1.ResponseFilter{
									Paths: []string{"$.data.id", "$.data.name"},
									Mode:  eventv1.ResponseFilterModeInclude,
								},
								SelectionFilter: &eventv1.SelectionFilter{
									Attributes: map[string]string{"type": "de.telekom.eni.quickstart.v1"},
									Expression: &apiextensionsv1.JSON{Raw: []byte(`{"op":"eq","path":"$.source","value":"my-app"}`)}},
							},
						},
						{
							Name: "scope-b",
							Trigger: eventv1.EventTrigger{
								ResponseFilter: &eventv1.ResponseFilter{
									Paths: []string{"$.data.secret"},
									Mode:  eventv1.ResponseFilterModeExclude,
								},
							},
						},
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

			Expect(data.Scopes).To(HaveLen(2))

			Expect(data.Scopes[0].Name).To(Equal("scope-a"))
			Expect(data.Scopes[0].Trigger.ResponseFilter).NotTo(BeNil())
			Expect(data.Scopes[0].Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.id", "$.data.name"}))
			Expect(data.Scopes[0].Trigger.ResponseFilter.Mode).To(Equal("Include"))
			Expect(data.Scopes[0].Trigger.SelectionFilter).NotTo(BeNil())
			Expect(data.Scopes[0].Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"type": "de.telekom.eni.quickstart.v1"}))
			Expect(data.Scopes[0].Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.source","value":"my-app"}`))

			Expect(data.Scopes[1].Name).To(Equal("scope-b"))
			Expect(data.Scopes[1].Trigger.ResponseFilter).NotTo(BeNil())
			Expect(data.Scopes[1].Trigger.ResponseFilter.Paths).To(Equal([]string{"$.data.secret"}))
			Expect(data.Scopes[1].Trigger.ResponseFilter.Mode).To(Equal("Exclude"))
			Expect(data.Scopes[1].Trigger.SelectionFilter).To(BeNil())
		})

		It("should translate a single scope with only a selection filter", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "single-scope",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.single.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
					Scopes: []eventv1.EventScope{
						{
							Name: "only-selection",
							Trigger: eventv1.EventTrigger{
								SelectionFilter: &eventv1.SelectionFilter{
									Attributes: map[string]string{"source": "my-service"},
									Expression: &apiextensionsv1.JSON{Raw: []byte(`{"op":"eq","path":"$.type","value":"order.created"}`)},
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Scopes).To(HaveLen(1))
			Expect(data.Scopes[0].Name).To(Equal("only-selection"))
			Expect(data.Scopes[0].Trigger.ResponseFilter).To(BeNil())
			Expect(data.Scopes[0].Trigger.SelectionFilter).NotTo(BeNil())
			Expect(data.Scopes[0].Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"source": "my-service"}))
			Expect(data.Scopes[0].Trigger.SelectionFilter.Expression).To(Equal(`{"op":"eq","path":"$.type","value":"order.created"}`))
		})

		It("should return empty scopes when none are set", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-scopes",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.noscopes.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
					Scopes:     nil,
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Scopes).To(BeEmpty())
		})

		It("should handle selection filter with nil expression", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-expr",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.nilexpr.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
					Scopes: []eventv1.EventScope{
						{
							Name: "attrs-only",
							Trigger: eventv1.EventTrigger{
								SelectionFilter: &eventv1.SelectionFilter{
									Attributes: map[string]string{"source": "svc"},
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Scopes).To(HaveLen(1))
			Expect(data.Scopes[0].Name).To(Equal("attrs-only"))
			Expect(data.Scopes[0].Trigger.SelectionFilter).NotTo(BeNil())
			Expect(data.Scopes[0].Trigger.SelectionFilter.Attributes).To(Equal(map[string]string{"source": "svc"}))
			Expect(data.Scopes[0].Trigger.SelectionFilter.Expression).To(BeEmpty())
		})

		It("should handle scope with empty trigger", func() {
			obj := &eventv1.EventExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-trigger",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: eventv1.EventExposureSpec{
					EventType:  "de.telekom.empty.v1",
					Visibility: eventv1.VisibilityWorld,
					Approval:   eventv1.Approval{Strategy: eventv1.ApprovalStrategyAuto},
					Scopes: []eventv1.EventScope{
						{
							Name:    "bare",
							Trigger: eventv1.EventTrigger{},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Scopes).To(HaveLen(1))
			Expect(data.Scopes[0].Name).To(Equal("bare"))
			Expect(data.Scopes[0].Trigger.ResponseFilter).To(BeNil())
			Expect(data.Scopes[0].Trigger.SelectionFilter).To(BeNil())
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
