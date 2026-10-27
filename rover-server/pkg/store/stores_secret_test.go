// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	roverv1 "github.com/telekom/controlplane/rover/api/v1"
	"github.com/telekom/controlplane/secret-manager/api/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const obfuscated = "**********"

func newRoverWithSecretRefs() *roverv1.Rover {
	return &roverv1.Rover{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rover", Namespace: "default"},
		Spec: roverv1.RoverSpec{
			Subscriptions: []roverv1.Subscription{
				{
					Api: &roverv1.ApiSubscription{
						BasePath: "/api",
						Security: &roverv1.SubscriberSecurity{
							M2M: &roverv1.SubscriberMachine2MachineAuthentication{
								Client: &roverv1.OAuth2ClientCredentials{ClientId: "api-client", ClientKey: "$<api-sub-clientKey>"},
							},
						},
					},
				},
				{
					Agentic: &roverv1.AgenticSubscription{
						BasePath: "/mcp-sub",
						Security: &roverv1.SubscriberSecurity{
							M2M: &roverv1.SubscriberMachine2MachineAuthentication{
								Client: &roverv1.OAuth2ClientCredentials{
									ClientId:     "agentic-client",
									ClientSecret: "$<agentic-sub-clientSecret>",
									RefreshToken: "$<agentic-sub-refreshToken>",
								},
								Basic: &roverv1.BasicAuthCredentials{Username: "user", Password: "$<agentic-sub-password>"},
							},
						},
					},
				},
			},
			Exposures: []roverv1.Exposure{
				{
					Api: &roverv1.ApiExposure{
						BasePath: "/api",
						Security: &roverv1.Security{
							M2M: &roverv1.Machine2MachineAuthentication{
								ExternalIDP: &roverv1.ExternalIdentityProvider{
									Client: &roverv1.OAuth2ClientCredentials{ClientId: "api-idp", ClientKey: "$<api-exp-clientKey>"},
								},
							},
						},
					},
				},
				{
					Agentic: &roverv1.AgenticExposure{
						BasePath: "/mcp-exp",
						Security: &roverv1.Security{
							M2M: &roverv1.Machine2MachineAuthentication{
								ExternalIDP: &roverv1.ExternalIdentityProvider{
									Client: &roverv1.OAuth2ClientCredentials{
										ClientId:     "agentic-idp",
										ClientKey:    "$<agentic-exp-clientKey>",
										RefreshToken: "$<agentic-exp-refreshToken>",
									},
									Basic: &roverv1.BasicAuthCredentials{Username: "idp-user", Password: "$<agentic-exp-idpPassword>"},
								},
							},
						},
					},
				},
				{
					Agentic: &roverv1.AgenticExposure{
						BasePath: "/mcp-basic",
						Security: &roverv1.Security{
							M2M: &roverv1.Machine2MachineAuthentication{
								Basic: &roverv1.BasicAuthCredentials{Username: "basic-user", Password: "$<agentic-exp-basicPassword>"},
							},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Rover secret paths", func() {
	var (
		ctx          context.Context
		mockStore    *csmocks.MockObjectStore[*roverv1.Rover]
		secretMgr    *fake.MockSecretManager
		wrappedStore *secrets.SecretStore[*roverv1.Rover]
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockStore = csmocks.NewMockObjectStore[*roverv1.Rover](GinkgoT())
		secretMgr = fake.NewMockSecretManager(GinkgoT())
		wrappedStore = secrets.WrapStore(mockStore, secretsForKinds["Rover"], secrets.NewSecretManagerResolver(secretMgr))
	})

	It("resolves api and agentic secret references", func() {
		mockStore.EXPECT().Get(ctx, "default", "my-rover").Return(newRoverWithSecretRefs(), nil)
		secretMgr.EXPECT().Get(ctx, mock.Anything).RunAndReturn(func(_ context.Context, id string) (string, error) {
			return "resolved:" + id, nil
		})

		result, err := wrappedStore.Get(ctx, "default", "my-rover")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientKey).To(Equal("resolved:api-sub-clientKey"))

		agenticSub := result.Spec.Subscriptions[1].Agentic.Security.M2M
		Expect(agenticSub.Client.ClientSecret).To(Equal("resolved:agentic-sub-clientSecret"))
		Expect(agenticSub.Client.RefreshToken).To(Equal("resolved:agentic-sub-refreshToken"))
		Expect(agenticSub.Basic.Password).To(Equal("resolved:agentic-sub-password"))

		Expect(result.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal("resolved:api-exp-clientKey"))

		agenticIdp := result.Spec.Exposures[1].Agentic.Security.M2M.ExternalIDP
		Expect(agenticIdp.Client.ClientKey).To(Equal("resolved:agentic-exp-clientKey"))
		Expect(agenticIdp.Client.RefreshToken).To(Equal("resolved:agentic-exp-refreshToken"))
		Expect(agenticIdp.Basic.Password).To(Equal("resolved:agentic-exp-idpPassword"))

		Expect(result.Spec.Exposures[2].Agentic.Security.M2M.Basic.Password).To(Equal("resolved:agentic-exp-basicPassword"))
	})

	It("obfuscates api and agentic secrets for obfuscated callers", func() {
		ctx = security.ToContext(ctx, &security.BusinessContext{AccessType: security.AccessTypeObfuscated})
		mockStore.EXPECT().Get(ctx, "default", "my-rover").Return(newRoverWithSecretRefs(), nil)

		result, err := wrappedStore.Get(ctx, "default", "my-rover")
		Expect(err).NotTo(HaveOccurred())

		Expect(result.Spec.Subscriptions[0].Api.Security.M2M.Client.ClientKey).To(Equal(obfuscated))

		agenticSub := result.Spec.Subscriptions[1].Agentic.Security.M2M
		Expect(agenticSub.Client.ClientSecret).To(Equal(obfuscated))
		Expect(agenticSub.Client.RefreshToken).To(Equal(obfuscated))
		Expect(agenticSub.Basic.Password).To(Equal(obfuscated))

		Expect(result.Spec.Exposures[0].Api.Security.M2M.ExternalIDP.Client.ClientKey).To(Equal(obfuscated))

		agenticIdp := result.Spec.Exposures[1].Agentic.Security.M2M.ExternalIDP
		Expect(agenticIdp.Client.ClientKey).To(Equal(obfuscated))
		Expect(agenticIdp.Client.RefreshToken).To(Equal(obfuscated))
		Expect(agenticIdp.Basic.Password).To(Equal(obfuscated))

		Expect(result.Spec.Exposures[2].Agentic.Security.M2M.Basic.Password).To(Equal(obfuscated))
	})
})
