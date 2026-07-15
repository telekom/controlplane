// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package routelistener_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	pkgclient "sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	handler "github.com/telekom/controlplane/gateway/internal/handler/routelistener"
)

var _ = Describe("RouteListenerHandler", func() {
	var (
		ctx           context.Context
		mockClient    *fakeclient.MockJanitorClient
		h             *handler.RouteListenerHandler
		routeListener *gatewayv1.RouteListener
	)

	BeforeEach(func() {
		mockClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(context.Background(), mockClient)
		h = &handler.RouteListenerHandler{}
		routeListener = &gatewayv1.RouteListener{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-routelistener",
				Namespace: "test-ns",
			},
			Spec: gatewayv1.RouteListenerSpec{
				Route: types.ObjectRef{
					Name:      "test-route",
					Namespace: "test-ns",
				},
				Consumer:     "consumer-app",
				ServiceOwner: "provider-app",
				Issue:        "/api/v1/events",
			},
		}
	})

	Describe("CreateOrUpdate()", func() {
		Context("when route is not found", func() {
			BeforeEach(func() {
				notFoundErr := apierrors.NewNotFound(
					schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "routes"},
					"test-route",
				)
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Return(notFoundErr)
			})

			It("sets Blocked and NotReady conditions", func() {
				err := h.CreateOrUpdate(ctx, routeListener)
				Expect(err).ToNot(HaveOccurred())

				Expect(meta.IsStatusConditionFalse(routeListener.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(routeListener.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())

				readyCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal("RouteNotFound"))
				Expect(readyCond.Message).To(Equal("Route not found"))

				processingCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeProcessing)
				Expect(processingCond).ToNot(BeNil())
				Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(processingCond.Reason).To(Equal("Blocked"))
			})
		})

		Context("when route exists but is not ready", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, _ pkgclient.ObjectKey, obj pkgclient.Object, _ ...pkgclient.GetOption) {
						r := obj.(*gatewayv1.Route)
						r.Name = "test-route"
						r.Namespace = "test-ns"
						// No Ready condition → route is not ready
					}).
					Return(nil)
			})

			It("sets Blocked and NotReady conditions", func() {
				err := h.CreateOrUpdate(ctx, routeListener)
				Expect(err).ToNot(HaveOccurred())

				Expect(meta.IsStatusConditionFalse(routeListener.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(routeListener.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())

				readyCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal("RouteNotReady"))
				Expect(readyCond.Message).To(Equal("Route is not ready"))

				processingCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeProcessing)
				Expect(processingCond).ToNot(BeNil())
				Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(processingCond.Reason).To(Equal("Blocked"))
			})
		})

		Context("when route exists and is ready", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Run(func(_ context.Context, _ pkgclient.ObjectKey, obj pkgclient.Object, _ ...pkgclient.GetOption) {
						r := obj.(*gatewayv1.Route)
						r.Name = "test-route"
						r.Namespace = "test-ns"
						meta.SetStatusCondition(&r.Status.Conditions, metav1.Condition{
							Type:   condition.ConditionTypeReady,
							Status: metav1.ConditionTrue,
							Reason: "Ready",
						})
					}).
					Return(nil)
			})

			It("sets DoneProcessing and Ready conditions", func() {
				err := h.CreateOrUpdate(ctx, routeListener)
				Expect(err).ToNot(HaveOccurred())

				Expect(meta.IsStatusConditionTrue(routeListener.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())
				Expect(meta.IsStatusConditionFalse(routeListener.GetConditions(), condition.ConditionTypeProcessing)).To(BeTrue())

				readyCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeReady)
				Expect(readyCond).ToNot(BeNil())
				Expect(readyCond.Reason).To(Equal("RouteListenerReady"))
				Expect(readyCond.Message).To(Equal("RouteListener is ready"))

				processingCond := meta.FindStatusCondition(routeListener.GetConditions(), condition.ConditionTypeProcessing)
				Expect(processingCond).ToNot(BeNil())
				Expect(processingCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(processingCond.Reason).To(Equal("Done"))
			})
		})

		Context("when getting route fails with unknown error", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(mock.Anything, mock.Anything, mock.Anything).
					Return(fmt.Errorf("connection refused"))
			})

			It("returns a wrapped error", func() {
				err := h.CreateOrUpdate(ctx, routeListener)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get route by ref"))
				Expect(err.Error()).To(ContainSubstring("connection refused"))
			})
		})
	})

	Describe("Delete()", func() {
		It("returns nil", func() {
			err := h.Delete(ctx, routeListener)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
