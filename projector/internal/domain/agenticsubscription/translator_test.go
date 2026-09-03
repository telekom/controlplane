// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package agenticsubscription_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/agenticsubscription"
)

var _ = Describe("AgenticSubscription Translator", func() {
	var t agenticsubscription.Translator

	Describe("ShouldSkip", func() {
		It("should never skip", func() {
			obj := &agenticv1.AgenticSubscription{}
			skip, reason := t.ShouldSkip(obj)
			Expect(skip).To(BeFalse())
			Expect(reason).To(BeEmpty())
		})
	})

	Describe("Translate", func() {
		It("should populate all fields from the CR", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-subscription",
					Namespace: "prod--platform--narvi",
					Labels: map[string]string{
						"cp.ei.telekom.de/environment": "prod",
					},
				},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath: "/mcp/v1/tools",
					Requestor: agenticv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
					Security: &agenticv1.SubscriberSecurity{
						M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
							Client: &agenticv1.OAuth2ClientCredentials{
								ClientId:     "my-client-id",
								ClientSecret: "secret",
							},
							Scopes: []string{"read", "write"},
						},
					},
					Traffic: agenticv1.SubscriberTraffic{
						Failover: &agenticv1.SubscriberFailover{
							Enabled: true,
						},
					},
				},
				Status: agenticv1.AgenticSubscriptionStatus{
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
			Expect(data.BasePath).To(Equal("/mcp/v1/tools"))
			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Client).NotTo(BeNil())
			Expect(data.Security.M2M.Client.ClientId).To(Equal("my-client-id"))
			Expect(*data.Security.M2M.Client.ClientSecret).To(Equal("secret"))
			Expect(data.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.Failover).NotTo(BeNil())
			Expect(data.Traffic.Failover.Enabled).To(BeTrue())
			Expect(data.OwnerAppName).To(Equal("consumer-app"))
			Expect(data.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(data.TargetBasePath).To(Equal("/mcp/v1/tools"))
		})

		It("should set Traffic to nil when Failover is not set", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sub-no-failover",
					Namespace: "prod--platform--narvi",
				},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath: "/mcp/test",
					Requestor: agenticv1.Requestor{
						Application: ctypes.ObjectRef{Name: "app"},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Traffic).To(BeNil())
		})
	})

	Describe("Security mapping", func() {
		It("should map nil security to nil", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Security:  nil,
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Security).To(BeNil())
		})

		It("should map security with nil M2M to nil Security", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Security:  &agenticv1.SubscriberSecurity{},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Security).To(BeNil())
		})

		It("should map security with client credentials", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Security: &agenticv1.SubscriberSecurity{
						M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
							Client: &agenticv1.OAuth2ClientCredentials{
								ClientId:     "test-client-id",
								ClientSecret: "test-dummy-secret",
							},
							Scopes: []string{"read", "write"},
						},
					},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Client).NotTo(BeNil())
			Expect(data.Security.M2M.Client.ClientId).To(Equal("test-client-id"))
			Expect(*data.Security.M2M.Client.ClientSecret).To(Equal("test-dummy-secret"))
			Expect(data.Security.M2M.Basic).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(Equal([]string{"read", "write"}))
		})

		It("should map security with basic auth", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Security: &agenticv1.SubscriberSecurity{
						M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
							Basic: &agenticv1.BasicAuthCredentials{
								Username: "test-user",
								Password: "test-dummy-pass",
							},
						},
					},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Basic).NotTo(BeNil())
			Expect(data.Security.M2M.Basic.Username).To(Equal("test-user"))
			Expect(data.Security.M2M.Basic.Password).To(Equal("test-dummy-pass"))
			Expect(data.Security.M2M.Client).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(BeNil())
		})

		It("should map security with scopes only", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Security: &agenticv1.SubscriberSecurity{
						M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
							Scopes: []string{"admin"},
						},
					},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Client).To(BeNil())
			Expect(data.Security.M2M.Basic).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(Equal([]string{"admin"}))
		})
	})

	Describe("Traffic mapping", func() {
		It("should map failover enabled true", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Traffic: agenticv1.SubscriberTraffic{
						Failover: &agenticv1.SubscriberFailover{Enabled: true},
					},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.Failover).NotTo(BeNil())
			Expect(data.Traffic.Failover.Enabled).To(BeTrue())
		})

		It("should map failover enabled false", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Traffic: agenticv1.SubscriberTraffic{
						Failover: &agenticv1.SubscriberFailover{Enabled: false},
					},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.Failover).NotTo(BeNil())
			Expect(data.Traffic.Failover.Enabled).To(BeFalse())
		})

		It("should map nil Failover to nil Traffic", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{Name: "sub", Namespace: "prod--team--alpha"},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath:  "/mcp/test",
					Requestor: agenticv1.Requestor{Application: ctypes.ObjectRef{Name: "app"}},
					Traffic:   agenticv1.SubscriberTraffic{},
				},
			}
			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(data.Traffic).To(BeNil())
		})
	})

	Describe("KeyFromObject", func() {
		It("should derive all key fields from the live object", func() {
			obj := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-sub",
					Namespace: "prod--platform--narvi",
				},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath: "/mcp/v1/tools",
					Requestor: agenticv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			key := t.KeyFromObject(obj)
			Expect(key.BasePath).To(Equal("/mcp/v1/tools"))
			Expect(key.OwnerAppName).To(Equal("consumer-app"))
			Expect(key.OwnerTeamName).To(Equal("platform--narvi"))
			Expect(key.Namespace).To(Equal("prod--platform--narvi"))
			Expect(key.Name).To(Equal("my-sub"))
		})
	})

	Describe("KeyFromDelete", func() {
		It("should derive from lastKnown when available", func() {
			lastKnown := &agenticv1.AgenticSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-sub",
					Namespace: "prod--platform--narvi",
				},
				Spec: agenticv1.AgenticSubscriptionSpec{
					BasePath: "/mcp/v1/tools",
					Requestor: agenticv1.Requestor{
						Application: ctypes.ObjectRef{Name: "consumer-app"},
					},
				},
			}

			req := k8stypes.NamespacedName{Namespace: "prod--platform--narvi", Name: "my-sub"}
			key, err := t.KeyFromDelete(req, lastKnown)
			Expect(err).NotTo(HaveOccurred())
			Expect(key.BasePath).To(Equal("/mcp/v1/tools"))
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
