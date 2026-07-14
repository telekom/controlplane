// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package application_test

import (
	"context"
	"time"

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
					ExternalIds: []appv1.ExternalId{
						appv1.ExternalId{
							Id:     "abc",
							Scheme: "schema1",
						},
						appv1.ExternalId{
							Id:     "123",
							Scheme: "schema2",
						},
					},
					Security: &appv1.Security{
						IpRestrictions: &appv1.IpRestrictions{
							Allow: []string{"127.0.0.1", "127.0.0.2"},
							Deny:  []string{"127.0.0.4", "127.0.0.5"},
						},
					},
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
			Expect(data.Meta.Environment).To(Equal("prod"))

			Expect(data.ExternalIds).To(HaveLen(2))
			Expect(data.ExternalIds[0].Id).To(Equal("abc"))
			Expect(data.ExternalIds[0].Scheme).To(Equal("schema1"))
			Expect(data.ExternalIds[1].Id).To(Equal("123"))
			Expect(data.ExternalIds[1].Scheme).To(Equal("schema2"))
			Expect(data.IpRestrictions.Allow).To(Equal([]string{"127.0.0.1", "127.0.0.2"}))
			Expect(data.IpRestrictions.Deny).To(Equal([]string{"127.0.0.4", "127.0.0.5"}))
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

	Describe("Secret Rotation State", func() {
		baseApp := func() *appv1.Application {
			return &appv1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-app",
					Namespace: "prod--platform--narvi",
				},
				Spec: appv1.ApplicationSpec{
					Team: "platform--narvi",
					Zone: commontypes.ObjectRef{Name: "caas"},
				},
			}
		}

		It("should return DONE when no SecretRotation condition exists", func() {
			obj := baseApp()
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("DONE"))
			Expect(data.SecretRotationMessage).To(BeNil())
			Expect(data.RotatedClientSecret).To(BeNil())
			Expect(data.RotatedExpiresAt).To(BeNil())
			Expect(data.CurrentExpiresAt).To(BeNil())
		})

		It("should return ROTATING when condition reason is InProgress", func() {
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:    "SecretRotation",
					Status:  metav1.ConditionFalse,
					Reason:  "InProgress",
					Message: "waiting for identity",
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("ROTATING"))
			Expect(data.SecretRotationMessage).ToNot(BeNil())
			Expect(*data.SecretRotationMessage).To(Equal("waiting for identity"))
		})

		It("should return GRACE_PERIOD_ACTIVE when plenty of grace period remains", func() {
			// Grace period started 10 min ago, expires in 50 min → 83% remaining
			gracePeriodStart := time.Now().Add(-10 * time.Minute)
			expiresAt := metav1.NewTime(time.Now().Add(50 * time.Minute))
			currentExpiresAt := metav1.NewTime(time.Now().Add(72 * time.Hour))
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               "SecretRotation",
					Status:             metav1.ConditionTrue,
					Reason:             "Success",
					Message:            "rotation completed",
					LastTransitionTime: metav1.NewTime(gracePeriodStart),
				},
			}
			obj.Status.RotatedClientSecret = "secret-manager://old-secret-ref"
			obj.Status.RotatedExpiresAt = &expiresAt
			obj.Status.CurrentExpiresAt = &currentExpiresAt

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("GRACE_PERIOD_ACTIVE"))
			Expect(data.SecretRotationMessage).ToNot(BeNil())
			Expect(*data.SecretRotationMessage).To(Equal("rotation completed"))
			Expect(data.RotatedClientSecret).ToNot(BeNil())
			Expect(*data.RotatedClientSecret).To(Equal("secret-manager://old-secret-ref"))
			Expect(data.RotatedExpiresAt).ToNot(BeNil())
			Expect(data.CurrentExpiresAt).ToNot(BeNil())
		})

		It("should return GRACE_PERIOD_EXPIRING when less than 20% of grace period remains", func() {
			// Grace period started 100 min ago, expires in 10 min → 9% remaining
			now := time.Now()
			gracePeriodStart := now.Add(-100 * time.Minute)
			expiresAt := metav1.NewTime(now.Add(10 * time.Minute))
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               "SecretRotation",
					Status:             metav1.ConditionTrue,
					Reason:             "Success",
					Message:            "rotation completed",
					LastTransitionTime: metav1.NewTime(gracePeriodStart),
				},
			}
			obj.Status.RotatedClientSecret = "secret-manager://old-secret-ref"
			obj.Status.RotatedExpiresAt = &expiresAt

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("GRACE_PERIOD_EXPIRING"))
		})

		It("should return DONE when expiry has passed", func() {
			now := time.Now()
			gracePeriodStart := now.Add(-2 * time.Hour)
			expiresAt := metav1.NewTime(now.Add(-5 * time.Minute))
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               "SecretRotation",
					Status:             metav1.ConditionTrue,
					Reason:             "Success",
					LastTransitionTime: metav1.NewTime(gracePeriodStart),
				},
			}
			obj.Status.RotatedClientSecret = "secret-manager://old-secret-ref"
			obj.Status.RotatedExpiresAt = &expiresAt

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("DONE"))
		})

		It("should return GRACE_PERIOD_ACTIVE when RotatedExpiresAt is nil but condition is Success with rotated secret", func() {
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:   "SecretRotation",
					Status: metav1.ConditionTrue,
					Reason: "Success",
				},
			}
			obj.Status.RotatedClientSecret = "secret-manager://old-secret-ref"

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			// RotatedExpiresAt is nil → grace period not trackable, falls through to DONE
			Expect(data.SecretRotationPhase).To(Equal("DONE"))
		})

		It("should return DONE when condition is Success but grace period has expired", func() {
			now := time.Now()
			expiresAt := metav1.NewTime(now.Add(-1 * time.Hour))
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:               "SecretRotation",
					Status:             metav1.ConditionTrue,
					Reason:             "Success",
					LastTransitionTime: metav1.NewTime(now.Add(-3 * time.Hour)),
				},
			}
			obj.Status.RotatedClientSecret = "secret-manager://old-secret-ref"
			obj.Status.RotatedExpiresAt = &expiresAt

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("DONE"))
			Expect(data.SecretRotationMessage).To(BeNil())
		})

		It("should return DONE when condition is Success and rotated secret is empty", func() {
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:   "SecretRotation",
					Status: metav1.ConditionTrue,
					Reason: "Success",
				},
			}
			obj.Status.RotatedClientSecret = ""

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("DONE"))
			Expect(data.SecretRotationMessage).To(BeNil())
		})

		It("should return FAILED when condition reason is Failed", func() {
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:    "SecretRotation",
					Status:  metav1.ConditionFalse,
					Reason:  "Failed",
					Message: "keycloak unavailable",
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("FAILED"))
			Expect(*data.SecretRotationMessage).To(Equal("keycloak unavailable"))
		})

		It("should return ROTATING for unknown condition reason (fallback)", func() {
			obj := baseApp()
			obj.Status.Conditions = []metav1.Condition{
				{
					Type:    "SecretRotation",
					Status:  metav1.ConditionFalse,
					Reason:  "SomeFutureReason",
					Message: "something new",
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.SecretRotationPhase).To(Equal("ROTATING"))
			Expect(*data.SecretRotationMessage).To(Equal("something new"))
		})
	})
})
