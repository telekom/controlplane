// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/telekom/controlplane/api/api/v1"
	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cclient "github.com/telekom/controlplane/common/pkg/client"
	fakeclient "github.com/telekom/controlplane/common/pkg/client/fake"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	ctypes "github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("verifyProviderBinding", func() {
	const (
		routeName       = "api-v1-orders"
		routeNamespace  = "zone-ns"
		exposureName    = "provider-app--api-v1-orders"
		exposureUID     = "ae-uid-001"
		applicationName = providerAppName
	)

	var (
		h          *handler.ListenerHandler
		ctx        context.Context
		fakeClient *fakeclient.MockJanitorClient
	)

	BeforeEach(func() {
		h = &handler.ListenerHandler{}
		ctx = context.Background()
		fakeClient = fakeclient.NewMockJanitorClient(GinkgoT())
		ctx = cclient.WithClient(ctx, fakeClient)
	})

	makeRoute := func(ownerUID string) *gatewayv1.Route {
		labels := map[string]string{}
		if ownerUID != "" {
			labels[cconfig.OwnerUidLabelKey] = ownerUID
		}
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeName,
				Namespace: routeNamespace,
				Labels:    labels,
			},
		}
	}

	makeProvider := func() *applicationv1.Application {
		return &applicationv1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      providerAppName,
				Namespace: listenerNamespace,
				UID:       "provider-uid-001",
			},
			Spec: applicationv1.ApplicationSpec{
				Team: providerTeam,
			},
			Status: applicationv1.ApplicationStatus{
				ClientId: providerClientId,
			},
		}
	}

	// makeExposure creates an ApiExposure in the provider's team namespace
	// (listenerNamespace), matching real Rover behavior where ApiExposures
	// live alongside the Application, not in the zone namespace.
	makeExposure := func(uid, appLabel string, active bool, routeRef *ctypes.ObjectRef, proxyRoutes []ctypes.ObjectRef) apiv1.ApiExposure {
		labels := map[string]string{}
		if appLabel != "" {
			labels[cconfig.BuildLabelKey("application")] = appLabel
		}
		return apiv1.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:      exposureName,
				Namespace: listenerNamespace, // team namespace, same as providerApp
				UID:       k8stypes.UID(uid),
				Labels:    labels,
			},
			Status: apiv1.ApiExposureStatus{
				Active:      active,
				Route:       routeRef,
				ProxyRoutes: proxyRoutes,
			},
		}
	}

	mockListExposures := func(items []apiv1.ApiExposure) {
		fakeClient.EXPECT().
			List(ctx, mock.AnythingOfType("*v1.ApiExposureList"), mock.Anything).
			Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
				out := list.(*apiv1.ApiExposureList)
				out.Items = items
			}).
			Return(nil).Once()
	}

	Context("matching provider", func() {
		It("should return a ProviderBinding when everything matches", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				applicationName,
				true,
				&ctypes.ObjectRef{Name: routeName, Namespace: routeNamespace},
				nil,
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).NotTo(HaveOccurred())
			Expect(binding).NotTo(BeNil())
			Expect(binding.ApiExposureName).To(Equal(exposureName))
			Expect(binding.ApiExposureNamespace).To(Equal(listenerNamespace))
			Expect(binding.ApiExposureUID).To(Equal(exposureUID))
			Expect(binding.ApplicationName).To(Equal(applicationName))
			Expect(binding.IsPrimaryRoute).To(BeTrue())
			Expect(binding.ProxyRoutes).To(BeEmpty())
		})
	})

	Context("matching provider via proxyRoutes", func() {
		It("should return a ProviderBinding when Route is in proxyRoutes", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				applicationName,
				true,
				nil, // no direct route ref
				[]ctypes.ObjectRef{{Name: routeName, Namespace: routeNamespace}},
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).NotTo(HaveOccurred())
			Expect(binding).NotTo(BeNil())
			Expect(binding.ApiExposureName).To(Equal(exposureName))
			Expect(binding.IsPrimaryRoute).To(BeFalse())
			Expect(binding.ProxyRoutes).To(Equal([]ctypes.ObjectRef{{Name: routeName, Namespace: routeNamespace}}))
			// The binding holds a copy, not the exposure's status slice.
			exposure.Status.ProxyRoutes[0].Name = "mutated"
			Expect(binding.ProxyRoutes[0].Name).To(Equal(routeName))
		})
	})

	Context("wrong provider", func() {
		It("should return a BlockedError when application label does not match provider", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				"other-application", // different from providerAppName
				true,
				&ctypes.ObjectRef{Name: routeName, Namespace: routeNamespace},
				nil,
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provider binding mismatch"))
			Expect(binding).To(BeNil())
		})
	})

	Context("missing owner.uid label", func() {
		It("should return a BlockedError when Route has no owner.uid", func() {
			route := makeRoute("") // empty owner UID
			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no owner.uid label"))
			Expect(binding).To(BeNil())
		})

		It("should return a BlockedError when Route has nil labels", func() {
			route := &gatewayv1.Route{
				ObjectMeta: metav1.ObjectMeta{
					Name:      routeName,
					Namespace: routeNamespace,
				},
			}
			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no owner.uid label"))
			Expect(binding).To(BeNil())
		})
	})

	Context("missing ApiExposure", func() {
		It("should return a BlockedError when no ApiExposure matches the UID", func() {
			route := makeRoute(exposureUID)
			// Return an empty list — no matching exposure.
			mockListExposures([]apiv1.ApiExposure{})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no ApiExposure with UID"))
			Expect(binding).To(BeNil())
		})
	})

	Context("inactive ApiExposure", func() {
		It("should return a BlockedError when ApiExposure is not active", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				applicationName,
				false, // inactive
				&ctypes.ObjectRef{Name: routeName, Namespace: routeNamespace},
				nil,
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not active"))
			Expect(binding).To(BeNil())
		})
	})

	Context("Route not in ApiExposure status", func() {
		It("should return a BlockedError when Route is not referenced", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				applicationName,
				true,
				&ctypes.ObjectRef{Name: "different-route", Namespace: routeNamespace},
				nil,
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not referenced"))
			Expect(binding).To(BeNil())
		})

		It("should return a BlockedError when both route and proxyRoutes reference different Routes", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				applicationName,
				true,
				nil, // no direct route
				[]ctypes.ObjectRef{{Name: "other-route", Namespace: routeNamespace}},
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not referenced"))
			Expect(binding).To(BeNil())
		})
	})

	Context("missing application label", func() {
		It("should return a BlockedError when ApiExposure has no application label", func() {
			route := makeRoute(exposureUID)
			exposure := makeExposure(
				exposureUID,
				"", // no application label
				true,
				&ctypes.ObjectRef{Name: routeName, Namespace: routeNamespace},
				nil,
			)
			mockListExposures([]apiv1.ApiExposure{exposure})

			binding, err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no application label"))
			Expect(binding).To(BeNil())
		})
	})
})
