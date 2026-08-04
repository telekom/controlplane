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
	"github.com/telekom/controlplane/common/pkg/errors/ctrlerrors"
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

		It("blocks deletion while a route references the gateway", func() {
			handler := &gwhandler.GatewayHandler{}
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					args.Get(1).(*gatewayv1.RouteList).Items = []gatewayv1.Route{{}}
				}).Return(nil)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "test-gw"}}

			err := handler.Delete(cc.WithClient(context.Background(), mockClient), gw)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
		})

		It("blocks deletion while a consumer references the gateway", func() {
			handler := &gwhandler.GatewayHandler{}
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					switch list := args.Get(1).(type) {
					case *gatewayv1.RouteList:
						list.Items = nil
					case *gatewayv1.ConsumerList:
						list.Items = []gatewayv1.Consumer{{}}
					}
				}).Return(nil)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "test-gw"}}

			err := handler.Delete(cc.WithClient(context.Background(), mockClient), gw)
			var blocked ctrlerrors.BlockedError
			Expect(errors.As(err, &blocked)).To(BeTrue())
			mockClient.AssertNumberOfCalls(GinkgoT(), "List", 2)
		})

		It("returns reference lookup errors", func() {
			handler := &gwhandler.GatewayHandler{}
			mockClient := fakeclient.NewMockJanitorClient(GinkgoT())
			mockClient.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(errors.New("list failed"))
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "test-gw"}}

			err := handler.Delete(cc.WithClient(context.Background(), mockClient), gw)
			Expect(err).To(MatchError(ContainSubstring("listing routes")))
		})
	})
})
