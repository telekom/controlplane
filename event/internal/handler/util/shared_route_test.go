// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"context"
	"fmt"

	mock "github.com/stretchr/testify/mock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/event/internal/handler/util"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ---------- DeleteRouteIfExists ----------

var _ = Describe("DeleteRouteIfExists", func() {
	var (
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	It("should return nil when ref is nil", func() {
		err := util.DeleteRouteIfExists(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should delete route when found", func() {
		ref := &ctypes.ObjectRef{Name: "test-route", Namespace: "default"}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "test-route", Namespace: "default"}, &gatewayapi.Route{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Route) = gatewayapi.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "default"},
				}
			}).
			Return(nil)

		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.Route")).
			Return(nil)

		err := util.DeleteRouteIfExists(ctx, ref)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return nil when route is not found", func() {
		ref := &ctypes.ObjectRef{Name: "missing-route", Namespace: "default"}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "missing-route", Namespace: "default"}, &gatewayapi.Route{}).
			Return(apierrors.NewNotFound(schema.GroupResource{Group: "gateway.cp.ei.telekom.de", Resource: "routes"}, "missing-route"))

		err := util.DeleteRouteIfExists(ctx, ref)
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return wrapped error when Get fails with unexpected error", func() {
		ref := &ctypes.ObjectRef{Name: "test-route", Namespace: "default"}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "test-route", Namespace: "default"}, &gatewayapi.Route{}).
			Return(fmt.Errorf("connection refused"))

		err := util.DeleteRouteIfExists(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get Route"))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("should return wrapped error when Delete fails", func() {
		ref := &ctypes.ObjectRef{Name: "test-route", Namespace: "default"}

		fakeClient.EXPECT().
			Get(ctx, k8stypes.NamespacedName{Name: "test-route", Namespace: "default"}, &gatewayapi.Route{}).
			Run(func(_ context.Context, _ k8stypes.NamespacedName, out client.Object, _ ...client.GetOption) {
				*out.(*gatewayapi.Route) = gatewayapi.Route{
					ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: "default"},
				}
			}).
			Return(nil)

		fakeClient.EXPECT().
			Delete(ctx, mock.AnythingOfType("*v1.Route")).
			Return(fmt.Errorf("forbidden"))

		err := util.DeleteRouteIfExists(ctx, ref)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to delete Route"))
		Expect(err.Error()).To(ContainSubstring("forbidden"))
	})
})

// ---------- WithOwner ----------

var _ = Describe("WithOwner", func() {
	It("should set Owner on Options", func() {
		owner := &metav1.ObjectMeta{Name: "my-owner", Namespace: "default"}
		opts := &util.Options{}
		util.WithOwner(owner)(opts)
		Expect(opts.Owner).To(Equal(owner))
	})
})

// ---------- WithProxyTarget ----------

var _ = Describe("WithProxyTarget", func() {
	It("should set IsProxyTarget to true", func() {
		opts := &util.Options{}
		util.WithProxyTarget(true)(opts)
		Expect(opts.IsProxyTarget).To(BeTrue())
	})

	It("should set IsProxyTarget to false", func() {
		opts := &util.Options{}
		util.WithProxyTarget(false)(opts)
		Expect(opts.IsProxyTarget).To(BeFalse())
	})
})
