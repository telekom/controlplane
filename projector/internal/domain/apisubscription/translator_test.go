// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apisubscription_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/apisubscription"
)

var _ = Describe("ApiSubscription Translator", func() {
	var t apisubscription.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &apiv1.ApiSubscription{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &apiv1.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-subscription",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
					},
				},
				Spec: apiv1.ApiSubscriptionSpec{
					ApiBasePath: "/api/v1/users",
					Requestor: apiv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
					Security: &apiv1.SubscriberSecurity{
						M2M: &apiv1.SubscriberMachine2MachineAuthentication{
							Client: &apiv1.OAuth2ClientCredentials{
								ClientId:     "my-client-id",
								ClientSecret: "secret",
							},
							Scopes: []string{"read", "write"},
						},
					},
				},
				Status: apiv1.ApiSubscriptionStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionTrue,
							Message: "subscription active",
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Meta.Namespace).To(Equal("prod--platform--narvi"))
			Expect(data.Meta.Name).To(Equal("my-subscription"))
			Expect(data.Meta.Environment).To(Equal("prod"))
			Expect(data.StatusPhase).To(Equal("READY"))
			Expect(data.StatusMessage).To(Equal("subscription active"))
			Expect(data.BasePath).To(Equal("/api/v1/users"))
			Expect(data.M2MAuthMethod).To(Equal("OAUTH2_CLIENT"))
			Expect(data.ApprovedScopes).To(Equal([]string{"read", "write"}))
			Expect(data.OwnerAppName).To(Equal("consumer-app"))
			Expect(data.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(data.TargetBasePath).To(Equal("/api/v1/users"))
			Expect(data.TargetAppName).To(BeEmpty())
			Expect(data.TargetTeamName).To(BeEmpty())
		})
	})

	Describe("M2MAuthMethod derivation", func() {
		newObj := func(security *apiv1.SubscriberSecurity) *apiv1.ApiSubscription {
			return &apiv1.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub",
					Namespace: "prod--team--alpha",
				},
				Spec: apiv1.ApiSubscriptionSpec{
					ApiBasePath: "/api/test",
					Requestor: apiv1.Requestor{
						Application: ctypes.ObjectRef{Name: "app"},
					},
					Security: security,
				},
			}
		}

		It("should return NONE when security is nil", func() {
			data, err := t.Translate(context.Background(), newObj(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("NONE"))
			Expect(data.ApprovedScopes).To(Equal([]string{}))
		})

		It("should return NONE when M2M is nil", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("NONE"))
			Expect(data.ApprovedScopes).To(Equal([]string{}))
		})

		It("should return OAUTH2_CLIENT when Client is set", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Client: &apiv1.OAuth2ClientCredentials{ClientId: "cid", ClientSecret: "csec"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("OAUTH2_CLIENT"))
		})

		It("should return BASIC_AUTH when Basic is set", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Basic: &apiv1.BasicAuthCredentials{Username: "u", Password: "p"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("BASIC_AUTH"))
		})

		It("should return SCOPES_ONLY when only scopes are set", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Scopes: []string{"read"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("SCOPES_ONLY"))
			Expect(data.ApprovedScopes).To(Equal([]string{"read"}))
		})

		It("should return NONE when M2M is set but all fields nil/empty", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("NONE"))
		})

		It("should prefer OAUTH2_CLIENT over BASIC_AUTH and SCOPES_ONLY", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Client: &apiv1.OAuth2ClientCredentials{ClientId: "cid"},
					Basic:  &apiv1.BasicAuthCredentials{Username: "u", Password: "p"},
					Scopes: []string{"s"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("OAUTH2_CLIENT"))
		})

		It("should prefer BASIC_AUTH over SCOPES_ONLY", func() {
			data, err := t.Translate(context.Background(), newObj(&apiv1.SubscriberSecurity{
				M2M: &apiv1.SubscriberMachine2MachineAuthentication{
					Basic:  &apiv1.BasicAuthCredentials{Username: "u", Password: "p"},
					Scopes: []string{"s"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(data.M2MAuthMethod).To(Equal("BASIC_AUTH"))
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive all key fields from the live object", func() {
			obj := &apiv1.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-sub",
					Namespace: "prod--platform--narvi",
				},
				Spec: apiv1.ApiSubscriptionSpec{
					ApiBasePath: "/api/v1/users",
					Requestor: apiv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.BasePath).To(Equal("/api/v1/users"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("my-sub"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should derive from lastKnown when available", func() {
			lastKnown := &apiv1.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-sub",
					Namespace: "prod--platform--narvi",
				},
				Spec: apiv1.ApiSubscriptionSpec{
					ApiBasePath: "/api/v1/users",
					Requestor: apiv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "my-sub"}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("/api/v1/users"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("my-sub"))
		})

		It("should use best-effort fallback when lastKnown is nil", func() {
			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "some-hash-name"}
			key, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("some-hash-name"))
			Expect(key.OwnerAppName).To(Equal("some-hash-name"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("some-hash-name"))
		})

		It("should never return an error", func() {
			req := k8stypes.NamespacedName{Namespace: "ns", Name: "n"}
			_, err := t.KeyFromDelete(req, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
