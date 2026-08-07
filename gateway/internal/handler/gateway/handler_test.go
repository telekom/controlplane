// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

import (
	"context"
	"errors"

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
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	gwhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

		It("limits the lookup and identifies a referencing route", func() {
			handler := &gwhandler.GatewayHandler{}
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.EXPECT().
				List(mock.Anything, mock.AnythingOfType("*v1.RouteList"), mock.Anything, mock.Anything).
				Run(func(_ context.Context, list client.ObjectList, opts ...client.ListOption) {
					list.(*gatewayv1.RouteList).Items = []gatewayv1.Route{{ObjectMeta: metav1.ObjectMeta{Name: "blocking-route"}}}
					listOptions := &client.ListOptions{}
					for _, opt := range opts {
						opt.ApplyToList(listOptions)
					}
					Expect(listOptions.Limit).To(Equal(int64(1)))
				}).
				Return(nil)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "test-gw"}}

			err := handler.Delete(cc.WithClient(context.Background(), mockClient), gw)

			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			Expect(err).To(MatchError(`gateway "test-gw" is still referenced by route "blocking-route"`))
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
