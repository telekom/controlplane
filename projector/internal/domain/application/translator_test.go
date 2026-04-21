// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appv1 "github.com/telekom/controlplane/application/api/v1"
	commontypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/application"
)

var _ = Describe("Application Translator", func() {
	var t application.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &appv1.Application{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/environment": "prod"},
				},
				Spec: appv1.ApplicationSpec{
					Team: "platform--narvi",
					Zone: commontypes.ObjectRef{Name: "caas"},
				},
				Status: appv1.ApplicationStatus{
					ClientId: "client-123",
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
			Expect(data.Name).To(Equal("my-app"))
			Expect(data.TeamName).To(Equal("platform--narvi"))
			Expect(data.ZoneName).To(Equal("caas"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("all good"))
			Expect(data.ClientID).ToNot(BeNil())
			Expect(*data.ClientID).To(Equal("client-123"))
			Expect(data.IssuerURL).To(BeNil())
			Expect(data.Meta.Environment).To(Equal("prod"))
		})

		It("should set ClientID to nil when Status.ClientId is empty", func() {
			obj := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-client-app",
					Namespace: "prod--platform--narvi",
				},
				Spec: appv1.ApplicationSpec{
					Team: "platform--narvi",
					Zone: commontypes.ObjectRef{Name: "caas"},
				},
				Status: appv1.ApplicationStatus{
					ClientId: "",
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.ClientID).To(BeNil())
			Expect(data.IssuerURL).To(BeNil())
		})

		It("should derive UNKNOWN status when no conditions are set", func() {
			obj := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pending-app",
					Namespace: "prod--platform--narvi",
				},
				Spec: appv1.ApplicationSpec{
					Team: "platform--narvi",
					Zone: commontypes.ObjectRef{Name: "caas"},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.StatusPhase).To(Equal("UNKNOWN"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should return composite key with Name and TeamName", func() {
			obj := &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
				Spec:       appv1.ApplicationSpec{Team: "platform--narvi"},
			}

			key := t.KeyFromObject(obj)
			Expect(key.Name).To(Equal("my-app"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should use Spec.Team from lastKnown when available", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-app",
			}
			lastKnown := &appv1.Application{
				Spec: appv1.ApplicationSpec{Team: "platform--narvi"},
			}

			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.Name).To(Equal("my-app"))
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})

		It("should fall back to TeamNameFromNamespace when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{
				Namespace: "prod--platform--narvi",
				Name:      "my-app",
			}

			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.Name).To(Equal("my-app"))
			// TeamNameFromNamespace("prod--platform--narvi") → "platform--narvi"
			Expect(key.TeamName).To(Equal("platform--narvi"))
		})

		It("should handle namespace without -- separator", func() {
			req := k8stypes.NamespacedName{
				Namespace: "simple-ns",
				Name:      "my-app",
			}

			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.Name).To(Equal("my-app"))
			// No "--" in namespace → returns full namespace
			Expect(key.TeamName).To(Equal("simple-ns"))
		})
	})
})
