// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/telekom/controlplane/common/pkg/condition"
	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"go.uber.org/mock/gomock"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewRoute(name string, realmRef types.ObjectRef) *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				config.EnvironmentLabelKey: testEnvironment,
			},
		},
		Spec: gatewayv1.RouteSpec{
			Realm:       realmRef,
			PassThrough: false,
			Upstreams: []gatewayv1.Upstream{
				{
					Scheme: "http",
					Host:   "upstream.url",
					Port:   8080,
					Path:   "/api/v1",
				},
			},
			Downstreams: []gatewayv1.Downstream{
				{
					Host:      "downstream.url",
					Port:      8080,
					Path:      "/test/v1",
					IssuerUrl: "issuer.url",
				},
			},
		},
	}
}

var _ = Describe("Route Controller", Ordered, func() {

	var gateway *gatewayv1.Gateway
	var realm *gatewayv1.Realm

	var route *gatewayv1.Route

	BeforeAll(func() {
		By("Creating the Gateway and Realm")
		gateway = NewGateway("test-route")
		err := k8sClient.Create(ctx, gateway)
		Expect(err).NotTo(HaveOccurred())
		realm = NewRealm("test-route")
		realm.Spec.Gateway = types.ObjectRefFromObject(gateway)

		err = k8sClient.Create(ctx, realm)
		Expect(err).NotTo(HaveOccurred())

		By("Initializing the Route")
		route = NewRoute("test-v1", *types.ObjectRefFromObject(realm))

	})

	AfterAll(func() {
		By("Cleaning up the resources")
		err := k8sClient.Delete(ctx, gateway)
		Expect(err).NotTo(HaveOccurred())

		err = k8sClient.Delete(ctx, realm)
		Expect(err).NotTo(HaveOccurred())

	})

	Context("Handling a Route", func() {
		It("should successfully provision the Route", func() {

			By("Creating the Route")
			err := k8sClient.Create(ctx, route)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(err).NotTo(HaveOccurred())

				By("Checking if the Route is ready")
				g.Expect(meta.IsStatusConditionTrue(route.GetConditions(), condition.ConditionTypeReady)).To(BeTrue())

			}, timeout, interval).Should(Succeed())

		})

		It("should successfully delete the Route", func() {
			By("setting up the mocks")
			GetMockClientFor(gateway).EXPECT().DeleteRoute(gomock.Any(), gomock.Any()).Return(nil).MinTimes(1)

			err := k8sClient.Delete(ctx, route)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(route), route)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})
