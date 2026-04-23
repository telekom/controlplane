// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/projector/internal/domain/apiexposure"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ApiExposure Translator", func() {
	var t apiexposure.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &apiv1.ApiExposure{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-exposure",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
						"cp.ei.telekom.de/application": "my-app",
					},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/users",
					Upstreams: []apiv1.Upstream{
						{Url: "https://backend.example.com", Weight: 100},
					},
					Visibility: apiv1.VisibilityWorld,
					Approval: apiv1.Approval{
						Strategy:     apiv1.ApprovalStrategyAuto,
						TrustedTeams: []string{"team-a"},
					},
				},
				Status: apiv1.ApiExposureStatus{
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

			Expect(data.BasePath).To(Equal("/api/v1/users"))
			Expect(data.Visibility).To(Equal("WORLD"))
			Expect(data.Active).To(BeTrue())
			Expect(data.Features).To(Equal([]string{}))
			Expect(data.Upstreams).To(HaveLen(1))
			Expect(data.Upstreams[0].URL).To(Equal("https://backend.example.com"))
			Expect(data.Upstreams[0].Weight).To(Equal(100))
			Expect(data.ApprovalConfig.Strategy).To(Equal("AUTO"))
			Expect(data.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-a"}))
			Expect(data.APIVersion).To(BeNil())
			Expect(data.AppName).To(Equal("my-app"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("all good"))
			Expect(data.Meta.Environment).To(Equal("prod"))
		})

		It("should upper-case Zone visibility", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "zone-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/zones",
					Visibility:  apiv1.VisibilityZone,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategySimple},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Visibility).To(Equal("ZONE"))
		})

		It("should upper-case Enterprise visibility", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ent-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/ent",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Visibility).To(Equal("ENTERPRISE"))
		})

		It("should map Simple approval strategy", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "simple-approval",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/simple",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategySimple},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ApprovalConfig.Strategy).To(Equal("SIMPLE"))
		})

		It("should map FourEyes approval strategy to FOUR_EYES", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foureyes-approval",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/foureyes",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval: apiv1.Approval{
						Strategy:     apiv1.ApprovalStrategyFourEyes,
						TrustedTeams: []string{"team-x", "team-y"},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ApprovalConfig.Strategy).To(Equal("FOUR_EYES"))
			Expect(data.ApprovalConfig.TrustedTeams).To(Equal([]string{"team-x", "team-y"}))
		})

		It("should map multiple upstreams", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-upstream",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/multi",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Upstreams: []apiv1.Upstream{
						{Url: "https://primary.example.com", Weight: 80},
						{Url: "https://secondary.example.com", Weight: 20},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Upstreams).To(HaveLen(2))
			Expect(data.Upstreams[0].URL).To(Equal("https://primary.example.com"))
			Expect(data.Upstreams[0].Weight).To(Equal(80))
			Expect(data.Upstreams[1].URL).To(Equal("https://secondary.example.com"))
			Expect(data.Upstreams[1].Weight).To(Equal(20))
		})

		It("should derive UNKNOWN status when no conditions are set", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-conditions",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/unknown",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})

		It("should handle nil TrustedTeams", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nil-trusted",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/nil-trusted",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval: apiv1.Approval{
						Strategy:     apiv1.ApprovalStrategyAuto,
						TrustedTeams: nil,
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ApprovalConfig.TrustedTeams).To(BeNil())
		})

		It("should handle empty upstreams", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-ups",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/empty-ups",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Upstreams:   nil,
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Upstreams).To(BeEmpty())
		})

		It("should set Active to false when Status.Active is false", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "inactive",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/inactive",
					Visibility:  apiv1.VisibilityEnterprise,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				},
				Status: apiv1.ApiExposureStatus{
					Active: false,
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Active).To(BeFalse())
		})
	})

	Describe("KeyFromObject", func() {
		It("should return composite key from CR fields", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-exposure",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/users",
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.BasePath).To(Equal("/api/v1/users"))
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
			lastKnown := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "my-app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/users",
				},
			}

			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("/api/v1/users"))
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
			// best-effort: basePath = key.Name, appName = key.Name
			Expect(key.BasePath).To(Equal("my-exposure"))
			Expect(key.AppName).To(Equal("my-exposure"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})

		It("should handle namespace without -- separator", func() {
			req := k8stypes.NamespacedName{
				Namespace: "simple-ns",
				Name:      "some-exposure",
			}

			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("some-exposure"))
			Expect(key.AppName).To(Equal("some-exposure"))
			Expect(key.TeamName).To(Equal("simple-ns"))
		})
	})
})
