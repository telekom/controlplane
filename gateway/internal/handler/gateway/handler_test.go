// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package gateway_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/condition"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	gwhandler "github.com/telekom/controlplane/gateway/internal/handler/gateway"
)

var _ = Describe("GatewayHandler", func() {

	Describe("CreateOrUpdate()", func() {
		It("sets DoneProcessing and Ready conditions", func() {
			// arrange
			handler := &gwhandler.GatewayHandler{}
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			// act
			err := handler.CreateOrUpdate(context.Background(), gw)

			// assert
			Expect(err).NotTo(HaveOccurred())

			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeProcessing)).To(BeFalse())
		})

		It("sets stable Envoy assignment status", func() {
			// arrange
			handler := gwhandler.NewGatewayHandler(nil)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: "envoy", UID: "gateway-uid"},
				Spec: gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy, RelayIdentity: "spiffe://example/relay"}}

			// act
			err := handler.CreateOrUpdate(context.Background(), gw)

			// assert
			Expect(err).NotTo(HaveOccurred())
			Expect(gw.Status.XDSNodeID).To(Equal("gateway:gateway-uid"))
			Expect(gw.Status.RelayIdentity).To(Equal("spiffe://example/relay"))
			Expect(meta.IsStatusConditionTrue(gw.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
			Expect(meta.IsStatusConditionFalse(gw.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())
		})

		It("rejects Envoy gateways without assignment", func() {
			handler := gwhandler.NewGatewayHandler(nil)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{UID: "gateway-uid"}, Spec: gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy}}
			Expect(handler.CreateOrUpdate(context.Background(), gw)).To(MatchError(ContainSubstring("relay identity is required")))
		})
	})

	Describe("Delete()", func() {
		It("clears the Envoy gateway", func() {
			// arrange
			xdsClient := &fakeXdsClient{}
			handler := gwhandler.NewGatewayHandler(xdsClient)
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "envoy", Namespace: "test-ns", UID: "gateway-uid"},
				Spec:       gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy},
			}

			// act
			err := handler.Delete(context.Background(), gw)

			// assert
			Expect(err).NotTo(HaveOccurred())
			Expect(xdsClient.clearedGateway).To(BeIdenticalTo(gw))
		})

		It("returns an error when clearing the Envoy gateway fails", func() {
			// arrange
			clearErr := errors.New("clear gateway")
			xdsClient := &fakeXdsClient{clearErr: clearErr}
			handler := gwhandler.NewGatewayHandler(xdsClient)
			gw := &gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{UID: "gateway-uid"}, Spec: gatewayv1.GatewaySpec{Type: gatewayv1.GatewayTypeEnvoy}}

			// act
			err := handler.Delete(context.Background(), gw)

			// assert
			Expect(err).To(MatchError(clearErr))
		})

		It("returns nil", func() {
			handler := &gwhandler.GatewayHandler{}
			gw := &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "test-gw"},
			}

			err := handler.Delete(context.Background(), gw)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
