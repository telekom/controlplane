// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/telekom/controlplane/rover/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Rover V1 Test Suite", func() {
	BeforeEach(func() {

	})

	Context("Rover Types", func() {
		It("should accept a minimal Rover", func() {

			rover := new(v1.Rover)
			rover.Name = "test-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, rover)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept an advanced Rover", func() {

			rover := new(v1.Rover)
			rover.Name = "test-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL:    "http://example.com",
									Weight: 50,
								},
								{
									URL:    "http://example.org",
									Weight: 50,
								},
							},
							Visibility: v1.VisibilityWorld,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyFourEyes,
								TrustedTeams: []v1.TrustedTeam{
									{
										Group: "eni",
										Team:  "hyperionn",
									},
								},
							},
							Transformation: &v1.Transformation{
								Request: v1.RequestResponseTransformation{
									Headers: v1.HeaderTransformation{
										Remove: []string{"X-Remove-Header"},
										Add:    []string{"X-Added-Header: value"},
									},
								},
							},
							Traffic: &v1.Traffic{
								LoadBalancing: &v1.LoadBalancing{
									Strategy: v1.LoadBalancingRoundRobin,
								},
							},
							Security: &v1.Security{
								M2M: &v1.Machine2MachineAuthentication{
									ExternalIDP: &v1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										Client: &v1.OAuth2ClientCredentials{
											ClientId:     "client-id",
											ClientSecret: "client-secret",
										},
									},
								},
							},
						},
					},
				},
			}

			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, rover)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Rover with an invalid zone", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))

			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"\": spec.zone in body should be at least 1 chars long",
				Field:   "spec.zone",
			}))

			Expect(len(statusErr.Status().Details.Causes)).To(Equal(1))

		})

		It("should reject a Rover with mutliple exposure types", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL: "http://example.com",
								},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
						},
						Event: &v1.EventExposure{},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(len(statusErr.Status().Details.Causes)).To(Equal(2))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"object\": Only one of api, event or file can be specified (XOR relationship)",
				Field:   "spec.exposures[0]",
			}))

			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"\": spec.exposures[0].event.eventType in body should be at least 1 chars long",
				Field:   "spec.exposures[0].event.eventType",
			}))

		})

		It("should reject a Rover with multiple security configs", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL: "http://example.com",
								},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
							Security: &v1.Security{
								M2M: &v1.Machine2MachineAuthentication{
									ExternalIDP: &v1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										Basic: &v1.BasicAuthCredentials{
											Username: "user",
											Password: "pass",
										},
									},
									Basic: &v1.BasicAuthCredentials{
										Username: "m2m-user",
										Password: "m2m-pass",
									},
								},
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(len(statusErr.Status().Details.Causes)).To(Equal(1))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"object\": ExternalIDP and basic authentication cannot be used together",
				Field:   "spec.exposures[0].api.security.m2m",
			}))

		})

		It("should reject a Rover with oauth2 clientSecret and clientKey configured", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL: "http://example.com",
								},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
							Security: &v1.Security{
								M2M: &v1.Machine2MachineAuthentication{
									ExternalIDP: &v1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										Client: &v1.OAuth2ClientCredentials{
											ClientId:     "client-id",
											ClientSecret: "client-secret",
											ClientKey:    "base64-encoded-private-key",
										},
									},
								},
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(len(statusErr.Status().Details.Causes)).To(Equal(1))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"object\": ClientSecret and ClientKey cannot be used together",
				Field:   "spec.exposures[0].api.security.m2m.externalIDP.client",
			}))

		})

		It("should reject a Rover with no oauth2 clientSecret or clientKey configured", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "test-zone",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL: "http://example.com",
								},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
							Security: &v1.Security{
								M2M: &v1.Machine2MachineAuthentication{
									ExternalIDP: &v1.ExternalIdentityProvider{
										TokenEndpoint: "https://idp.example.com/token",
										Client: &v1.OAuth2ClientCredentials{
											ClientId: "client-id",
										},
									},
								},
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(len(statusErr.Status().Details.Causes)).To(Equal(1))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"object\": At least one of clientSecret or clientKey must be provided",
				Field:   "spec.exposures[0].api.security.m2m.externalIDP.client",
			}))

		})

		It("should reject a Rover with invalid URL", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone: "test-zone",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{
									URL: "123", // Invalid URL
								},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())

			Expect(statusErr.Status().Reason).To(Equal(metav1.StatusReasonInvalid))
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeTypeInvalid,
				Message: "Invalid value: \"123\": spec.exposures[0].api.upstreams[0].url in body must be of type uri: \"123\"",
				Field:   "spec.exposures[0].api.upstreams[0].url",
			}))

			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"null\": some validation rules were not checked because the object was invalid; correct the existing errors to complete validation",
				Field:   "<nil>",
			}))

			Expect(len(statusErr.Status().Details.Causes)).To(Equal(2))
		})
	})

	Context("File Types (SFTP)", func() {
		It("should report the exposure and subscription type as file", func() {
			exp := v1.Exposure{File: &v1.FileExposure{FileType: "demo-sftp-spec-v1"}}
			Expect(exp.Type()).To(Equal(v1.TypeFile))

			sub := v1.Subscription{File: &v1.FileSubscription{FileType: "demo-sftp-spec-v1"}}
			Expect(sub.Type()).To(Equal(v1.TypeFile))
		})

		It("should only support file types on the cetus and canis zones", func() {
			Expect(v1.IsFileTypeZoneSupported("cetus")).To(BeTrue())
			Expect(v1.IsFileTypeZoneSupported("canis")).To(BeTrue())
			Expect(v1.IsFileTypeZoneSupported("aws")).To(BeFalse())
			Expect(v1.IsFileTypeZoneSupported("gaia")).To(BeFalse())
			Expect(v1.IsFileTypeZoneSupported("")).To(BeFalse())
		})

		It("should accept a Rover with a file type exposure and subscription", func() {
			rover := new(v1.Rover)
			rover.Name = "file-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "cetus",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						File: &v1.FileExposure{
							FileType:   "demo-sftp-spec-v1",
							Variant:    v1.FileVariantSFTP,
							Visibility: v1.VisibilityWorld,
							PublicKeys: []v1.PublicKey{
								{Label: "demo-provider-key", Key: "ssh-ed25519 AAAAprovider"},
							},
						},
					},
				},
				Subscriptions: []v1.Subscription{
					{
						File: &v1.FileSubscription{
							FileType: "demo-sftp-spec-v1",
							PublicKeys: []v1.PublicKey{
								{Label: "demo-consumer-key", Key: "ssh-ed25519 AAAAconsumer"},
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Delete(ctx, rover)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a file type exposure without any public keys", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-file-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "cetus",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						File: &v1.FileExposure{
							FileType:   "demo-sftp-spec-v1",
							Visibility: v1.VisibilityWorld,
							PublicKeys: []v1.PublicKey{},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject a file type exposure combined with an api exposure in the same entry", func() {
			rover := new(v1.Rover)
			rover.Name = "invalid-file-rover"
			rover.Namespace = "default"
			rover.Spec = v1.RoverSpec{
				Zone:         "cetus",
				ClientSecret: "topsecret",
				Exposures: []v1.Exposure{
					{
						Api: &v1.ApiExposure{
							BasePath: "/api",
							Upstreams: []v1.Upstream{
								{URL: "http://example.com"},
							},
							Visibility: v1.VisibilityEnterprise,
							Approval: v1.Approval{
								Strategy: v1.ApprovalStrategyAuto,
							},
						},
						File: &v1.FileExposure{
							FileType:   "demo-sftp-spec-v1",
							Visibility: v1.VisibilityWorld,
							PublicKeys: []v1.PublicKey{
								{Label: "demo-provider-key", Key: "ssh-ed25519 AAAAprovider"},
							},
						},
					},
				},
			}
			rover.Status = v1.RoverStatus{}

			err := k8sClient.Create(ctx, rover)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			statusErr, ok := err.(apierrors.APIStatus)
			Expect(ok).To(BeTrue())
			Expect(statusErr.Status().Details.Causes).To(ContainElement(metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "Invalid value: \"object\": Only one of api, event or file can be specified (XOR relationship)",
				Field:   "spec.exposures[0]",
			}))
		})
	})
})
