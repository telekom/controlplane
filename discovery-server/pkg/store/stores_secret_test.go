// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	apiv1 "github.com/telekom/controlplane/api/api/v1"
	csstore "github.com/telekom/controlplane/common-server/pkg/store"
	"github.com/telekom/controlplane/common-server/pkg/store/secrets"
	csmocks "github.com/telekom/controlplane/common-server/test/mocks"
	"github.com/telekom/controlplane/secret-manager/api/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const secretPlaceholder = "$<test-secret-id>"
const resolvedSecret = "super-secret-value"

var _ = Describe("Secret Store Integration", func() {

	Describe("ApiSubscription secrets", func() {

		var (
			ctx          context.Context
			mockStore    *csmocks.MockObjectStore[*apiv1.ApiSubscription]
			secretMgr    *fake.MockSecretManager
			wrappedStore *secrets.SecretStore[*apiv1.ApiSubscription]
		)

		BeforeEach(func() {
			ctx = context.Background()
			mockStore = csmocks.NewMockObjectStore[*apiv1.ApiSubscription](GinkgoT())
			secretMgr = fake.NewMockSecretManager(GinkgoT())
			wrappedStore = secrets.WrapStore(
				mockStore,
				[]string{
					"spec.security.m2m.client.clientSecret",
					"spec.security.m2m.basic.password",
				},
				secrets.NewSecretManagerResolver(secretMgr),
			)
		})

		newApiSubscription := func(clientSecret, password string) *apiv1.ApiSubscription {
			return &apiv1.ApiSubscription{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sub",
					Namespace: "default",
				},
				Spec: apiv1.ApiSubscriptionSpec{
					ApiBasePath: "/test",
					Security: &apiv1.SubscriberSecurity{
						M2M: &apiv1.SubscriberMachine2MachineAuthentication{
							Client: &apiv1.OAuth2ClientCredentials{
								ClientId:     "my-client",
								ClientSecret: clientSecret,
							},
							Basic: &apiv1.BasicAuthCredentials{
								Username: "user",
								Password: password,
							},
						},
					},
				},
			}
		}

		Context("Get", func() {
			It("resolves clientSecret placeholder", func() {
				obj := newApiSubscription(secretPlaceholder, "plain-password")
				mockStore.EXPECT().Get(ctx, "default", "test-sub").Return(obj, nil)
				secretMgr.EXPECT().Get(ctx, "test-secret-id").Return(resolvedSecret, nil)

				result, err := wrappedStore.Get(ctx, "default", "test-sub")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Spec.Security.M2M.Client.ClientSecret).To(Equal(resolvedSecret))
				// non-placeholder field unchanged
				Expect(result.Spec.Security.M2M.Basic.Password).To(Equal("plain-password"))
			})

			It("resolves basic password placeholder", func() {
				obj := newApiSubscription("plain-secret", secretPlaceholder)
				mockStore.EXPECT().Get(ctx, "default", "test-sub").Return(obj, nil)
				secretMgr.EXPECT().Get(ctx, "test-secret-id").Return(resolvedSecret, nil)

				result, err := wrappedStore.Get(ctx, "default", "test-sub")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Spec.Security.M2M.Basic.Password).To(Equal(resolvedSecret))
				Expect(result.Spec.Security.M2M.Client.ClientSecret).To(Equal("plain-secret"))
			})

		})

		Context("List", func() {
			It("resolves clientSecret placeholder in all items", func() {
				objs := []*apiv1.ApiSubscription{
					newApiSubscription(secretPlaceholder, "p1"),
					newApiSubscription(secretPlaceholder, "p2"),
				}
				listResp := &csstore.ListResponse[*apiv1.ApiSubscription]{Items: objs}
				mockStore.EXPECT().List(ctx, mock.Anything).Return(listResp, nil)
				secretMgr.EXPECT().Get(ctx, "test-secret-id").Return(resolvedSecret, nil).Times(2)

				result, err := wrappedStore.List(ctx, csstore.ListOpts{})
				Expect(err).NotTo(HaveOccurred())
				for _, item := range result.Items {
					Expect(item.Spec.Security.M2M.Client.ClientSecret).To(Equal(resolvedSecret))
				}
			})
		})
	})

	Describe("ApiExposure secrets", func() {

		var (
			ctx          context.Context
			mockStore    *csmocks.MockObjectStore[*apiv1.ApiExposure]
			secretMgr    *fake.MockSecretManager
			wrappedStore *secrets.SecretStore[*apiv1.ApiExposure]
		)

		BeforeEach(func() {
			ctx = context.Background()
			mockStore = csmocks.NewMockObjectStore[*apiv1.ApiExposure](GinkgoT())
			secretMgr = fake.NewMockSecretManager(GinkgoT())
			wrappedStore = secrets.WrapStore(
				mockStore,
				[]string{
					"spec.security.m2m.externalIDP.client.clientSecret",
					"spec.security.m2m.externalIDP.basic.password",
					"spec.security.m2m.basic.password",
				},
				secrets.NewSecretManagerResolver(secretMgr),
			)
		})

		newApiExposure := func(extIDPClientSecret, extIDPPassword string) *apiv1.ApiExposure {
			return &apiv1.ApiExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-exp",
					Namespace: "default",
				},
				Spec: apiv1.ApiExposureSpec{
					Security: &apiv1.Security{
						M2M: &apiv1.Machine2MachineAuthentication{
							ExternalIDP: &apiv1.ExternalIdentityProvider{
								TokenEndpoint: "https://idp.example.com/token",
								Client: &apiv1.OAuth2ClientCredentials{
									ClientId:     "ext-client",
									ClientSecret: extIDPClientSecret,
								},
								Basic: &apiv1.BasicAuthCredentials{
									Username: "ext-user",
									Password: extIDPPassword,
								},
							},
						},
					},
				},
			}
		}

		Context("Get", func() {
			It("resolves externalIDP clientSecret placeholder", func() {
				obj := newApiExposure(secretPlaceholder, "plain-pass")
				mockStore.EXPECT().Get(ctx, "default", "test-exp").Return(obj, nil)
				secretMgr.EXPECT().Get(ctx, "test-secret-id").Return(resolvedSecret, nil)

				result, err := wrappedStore.Get(ctx, "default", "test-exp")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Spec.Security.M2M.ExternalIDP.Client.ClientSecret).To(Equal(resolvedSecret))
			})

			It("resolves externalIDP basic password placeholder", func() {
				obj := newApiExposure("plain-secret", secretPlaceholder)
				mockStore.EXPECT().Get(ctx, "default", "test-exp").Return(obj, nil)
				secretMgr.EXPECT().Get(ctx, "test-secret-id").Return(resolvedSecret, nil)

				result, err := wrappedStore.Get(ctx, "default", "test-exp")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Spec.Security.M2M.ExternalIDP.Basic.Password).To(Equal(resolvedSecret))
			})
		})
	})
})
