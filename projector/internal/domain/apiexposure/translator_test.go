// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package apiexposure_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/telekom/controlplane/projector/internal/domain/apiexposure"
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
					Traffic: apiv1.Traffic{
						Failover: &apiv1.Failover{
							Zones: []ctypes.ObjectRef{
								{
									Name:      "zoneA",
									Namespace: "ns",
								},
								{
									Name:      "zoneB",
									Namespace: "ns",
								},
							},
						},
						RateLimit: &apiv1.RateLimit{
							Provider: &apiv1.RateLimitConfig{
								Limits: apiv1.Limits{
									Second: 1,
									Minute: 2,
									Hour:   3,
								},
								Options: apiv1.RateLimitOptions{
									HideClientHeaders: true,
									FaultTolerant:     true,
								},
							},
							SubscriberRateLimit: &apiv1.SubscriberRateLimits{
								Default: &apiv1.SubscriberRateLimitDefaults{
									Limits: apiv1.Limits{
										Second: 11,
										Minute: 22,
										Hour:   33,
									},
								},
								Overrides: []apiv1.RateLimitOverrides{
									{
										Subscriber: "s1",
										Limits: apiv1.Limits{
											Second: 111,
											Minute: 222,
											Hour:   333,
										},
									},
									{
										Subscriber: "s2",
										Limits: apiv1.Limits{
											Second: 1111,
											Minute: 2222,
											Hour:   3333,
										},
									},
								},
							},
						},
					},
					Security: &apiv1.Security{
						M2M: &apiv1.Machine2MachineAuthentication{
							ExternalIDP: &apiv1.ExternalIdentityProvider{
								TokenEndpoint: "https://tokenendpoint.example.com",
								TokenRequest:  apiv1.TokenRequestClientSecretBasic,
								GrantType:     "basic",
								Basic: &apiv1.BasicAuthCredentials{
									Username: "username",
									Password: "password",
								},
								Client: &apiv1.OAuth2ClientCredentials{
									ClientId:     "id",
									ClientSecret: "secret",
									ClientKey:    "key",
								},
							},
							Basic: &apiv1.BasicAuthCredentials{
								Username: "username",
								Password: "password",
							},
							Scopes: []string{"scope1", "scope2", "scope3"},
						},
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

			// Security
			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Basic).NotTo(BeNil())
			Expect(data.Security.M2M.Basic.Username).To(Equal("username"))
			Expect(data.Security.M2M.Basic.Password).To(Equal("password"))
			Expect(data.Security.M2M.Scopes).To(Equal([]string{"scope1", "scope2", "scope3"}))
			Expect(data.Security.M2M.ExternalIDP).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://tokenendpoint.example.com"))
			Expect(*data.Security.M2M.ExternalIDP.TokenRequest).To(Equal("client_secret_basic"))
			Expect(*data.Security.M2M.ExternalIDP.GrantType).To(Equal("basic"))
			Expect(data.Security.M2M.ExternalIDP.Basic).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.Basic.Username).To(Equal("username"))
			Expect(data.Security.M2M.ExternalIDP.Basic.Password).To(Equal("password"))
			Expect(data.Security.M2M.ExternalIDP.Client).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("id"))
			Expect(*data.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("secret"))
			Expect(*data.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("key"))

			// Traffic
			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.RateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider.Limits.Second).To(Equal(1))
			Expect(data.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(2))
			Expect(data.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(3))
			Expect(data.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
			Expect(data.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Second).To(Equal(11))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Minute).To(Equal(22))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Hour).To(Equal(33))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides).To(HaveLen(2))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Subscriber).To(Equal("s1"))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Second).To(Equal(111))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Minute).To(Equal(222))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Hour).To(Equal(333))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Subscriber).To(Equal("s2"))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Second).To(Equal(1111))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Minute).To(Equal(2222))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Hour).To(Equal(3333))

			Expect(data.Traffic.Failover.Zones).To(ContainElements("zoneA", "zoneB"))
			Expect(len(data.Traffic.Failover.Zones)).To(Equal(2))

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

		It("should map M2M security with ExternalIDP", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ext-idp-security",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/ext-idp",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Security: &apiv1.Security{
						M2M: &apiv1.Machine2MachineAuthentication{
							ExternalIDP: &apiv1.ExternalIdentityProvider{
								TokenEndpoint: "https://idp.example.com/token",
								TokenRequest:  apiv1.TokenRequestClientSecretPost,
								GrantType:     "client_credentials",
								Basic: &apiv1.BasicAuthCredentials{
									Username: "ext-user",
									Password: "ext-pass",
								},
								Client: &apiv1.OAuth2ClientCredentials{
									ClientId:     "ext-client-id",
									ClientSecret: "ext-client-secret",
									ClientKey:    "ext-client-key",
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.Basic).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(BeNil())
			Expect(data.Security.M2M.ExternalIDP).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.TokenEndpoint).To(Equal("https://idp.example.com/token"))
			Expect(*data.Security.M2M.ExternalIDP.TokenRequest).To(Equal("client_secret_post"))
			Expect(*data.Security.M2M.ExternalIDP.GrantType).To(Equal("client_credentials"))
			Expect(data.Security.M2M.ExternalIDP.Basic).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.Basic.Username).To(Equal("ext-user"))
			Expect(data.Security.M2M.ExternalIDP.Basic.Password).To(Equal("ext-pass"))
			Expect(data.Security.M2M.ExternalIDP.Client).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP.Client.ClientId).To(Equal("ext-client-id"))
			Expect(*data.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal("ext-client-secret"))
			Expect(*data.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("ext-client-key"))
		})

		It("should map M2M security with Basic auth", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-security",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/basic",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Security: &apiv1.Security{
						M2M: &apiv1.Machine2MachineAuthentication{
							Basic: &apiv1.BasicAuthCredentials{
								Username: "svc-account",
								Password: "svc-password",
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(BeNil())
			Expect(data.Security.M2M.Basic).NotTo(BeNil())
			Expect(data.Security.M2M.Basic.Username).To(Equal("svc-account"))
			Expect(data.Security.M2M.Basic.Password).To(Equal("svc-password"))
		})

		It("should map M2M security with Scopes only", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scopes-security",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/scopes",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Security: &apiv1.Security{
						M2M: &apiv1.Machine2MachineAuthentication{
							Scopes: []string{"read:api", "write:api"},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Security).NotTo(BeNil())
			Expect(data.Security.M2M).NotTo(BeNil())
			Expect(data.Security.M2M.ExternalIDP).To(BeNil())
			Expect(data.Security.M2M.Basic).To(BeNil())
			Expect(data.Security.M2M.Scopes).To(Equal([]string{"read:api", "write:api"}))
		})

		It("should map Traffic with full RateLimit config", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "full-traffic",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/full-traffic",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Traffic: apiv1.Traffic{
						RateLimit: &apiv1.RateLimit{
							Provider: &apiv1.RateLimitConfig{
								Limits: apiv1.Limits{
									Second: 10,
									Minute: 100,
									Hour:   1000,
								},
								Options: apiv1.RateLimitOptions{
									HideClientHeaders: true,
									FaultTolerant:     true,
								},
							},
							SubscriberRateLimit: &apiv1.SubscriberRateLimits{
								Default: &apiv1.SubscriberRateLimitDefaults{
									Limits: apiv1.Limits{
										Second: 5,
										Minute: 50,
										Hour:   500,
									},
								},
								Overrides: []apiv1.RateLimitOverrides{
									{
										Subscriber: "sub-a",
										Limits: apiv1.Limits{
											Second: 20,
											Minute: 200,
											Hour:   2000,
										},
									},
									{
										Subscriber: "sub-b",
										Limits: apiv1.Limits{
											Second: 30,
											Minute: 300,
											Hour:   3000,
										},
									},
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.RateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider.Limits.Second).To(Equal(10))
			Expect(data.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(100))
			Expect(data.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(1000))
			Expect(data.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeTrue())
			Expect(data.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Second).To(Equal(5))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Minute).To(Equal(50))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Hour).To(Equal(500))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides).To(HaveLen(2))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Subscriber).To(Equal("sub-a"))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Second).To(Equal(20))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Minute).To(Equal(200))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[0].Limits.Hour).To(Equal(2000))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Subscriber).To(Equal("sub-b"))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Second).To(Equal(30))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Minute).To(Equal(300))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides[1].Limits.Hour).To(Equal(3000))
		})

		It("should map Traffic with Provider only (no SubscriberRateLimit)", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "provider-only-traffic",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/provider-only",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Traffic: apiv1.Traffic{
						RateLimit: &apiv1.RateLimit{
							Provider: &apiv1.RateLimitConfig{
								Limits: apiv1.Limits{
									Second: 50,
									Minute: 500,
									Hour:   5000,
								},
								Options: apiv1.RateLimitOptions{
									HideClientHeaders: false,
									FaultTolerant:     true,
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.RateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider.Limits.Second).To(Equal(50))
			Expect(data.Traffic.RateLimit.Provider.Limits.Minute).To(Equal(500))
			Expect(data.Traffic.RateLimit.Provider.Limits.Hour).To(Equal(5000))
			Expect(data.Traffic.RateLimit.Provider.Options.HideClientHeaders).To(BeFalse())
			Expect(data.Traffic.RateLimit.Provider.Options.FaultTolerant).To(BeTrue())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit).To(BeNil())
		})

		It("should map Traffic with SubscriberRateLimit only (no Provider)", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "subscriber-only-traffic",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/subscriber-only",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
					Traffic: apiv1.Traffic{
						RateLimit: &apiv1.RateLimit{
							SubscriberRateLimit: &apiv1.SubscriberRateLimits{
								Default: &apiv1.SubscriberRateLimitDefaults{
									Limits: apiv1.Limits{
										Second: 7,
										Minute: 70,
										Hour:   700,
									},
								},
							},
						},
					},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.RateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.Provider).To(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit).NotTo(BeNil())
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Second).To(Equal(7))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Minute).To(Equal(70))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Default.Limits.Hour).To(Equal(700))
			Expect(data.Traffic.RateLimit.SubscriberRateLimit.Overrides).To(BeEmpty())
		})

		It("should produce empty Traffic when no RateLimit is set", func() {
			obj := &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-traffic",
					Namespace: "prod--platform--narvi",
					Labels:    map[string]string{"cp.ei.telekom.de/application": "app"},
				},
				Spec: apiv1.ApiExposureSpec{
					ApiBasePath: "/api/v1/no-traffic",
					Visibility:  apiv1.VisibilityWorld,
					Approval:    apiv1.Approval{Strategy: apiv1.ApprovalStrategyAuto},
				},
			}

			data, err := t.Translate(context.Background(), obj)
			Expect(err).NotTo(HaveOccurred())

			Expect(data.Traffic).NotTo(BeNil())
			Expect(data.Traffic.RateLimit).To(BeNil())
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
