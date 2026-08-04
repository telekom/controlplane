// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	cc "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/types"
	secrets "github.com/telekom/controlplane/secret-manager/api"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	gwhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
)

var _ = Describe("GatewayHandler", func() {

	Describe("CreateOrUpdate()", func() {
		It("sets DoneProcessing and Ready conditions", func() {
			handler := &gwhandler.GatewayHandler{}
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			err := handler.CreateOrUpdate(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())

			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeProcessing)).To(BeFalse())
		})
	})

	Describe("Delete()", func() {
		It("returns nil", func() {
			handler := &gwhandler.GatewayHandler{}
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			err := handler.Delete(cc.WithClient(context.Background(), mockClient), gw)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetGatewayByRef()", func() {
		It("resolves secrets when the gateway is not ready", func() {
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.On("Get", mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					gateway := args.Get(2).(*gatewayv1.Gateway)
					gateway.Spec.Admin.ClientSecret = "admin-ref"
					gateway.Spec.Redis = &gatewayv1.RedisConfig{Password: "redis-ref"}
				}).Return(nil)
			originalGet := secrets.Get
			DeferCleanup(func() { secrets.Get = originalGet })
			secrets.Get = func(_ context.Context, ref string) (string, error) { return "resolved-" + ref, nil }

			ready, gateway, err := gwhandler.GetGatewayByRef(
				cc.WithClient(context.Background(), mockClient),
				types.ObjectRef{Name: "gateway", Namespace: "default"},
				true,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
			Expect(gateway.Spec.Admin.ClientSecret).To(Equal("resolved-admin-ref"))
			Expect(gateway.Spec.Redis.Password).To(Equal("resolved-redis-ref"))
		})
	})
})
