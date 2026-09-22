// SPDX-FileCopyrightText: 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler_test

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	applicationv1 "github.com/telekom/controlplane/application/api/v1"
	cconfig "github.com/telekom/controlplane/common/pkg/config"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/spectre/internal/handler"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("verifyProviderBinding", func() {
	var (
		h   *handler.ListenerHandler
		ctx context.Context
	)

	BeforeEach(func() {
		h = &handler.ListenerHandler{}
		ctx = context.Background()
	})

	makeRoute := func(labels map[string]string) *gatewayv1.Route {
		return &gatewayv1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-route",
				Namespace: "zone-ns",
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

	Context("when Route has owner.uid label", func() {
		It("should pass (stub: full check requires api/api import)", func() {
			route := makeRoute(map[string]string{
				cconfig.OwnerUidLabelKey: "some-apiexposure-uid",
			})
			err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when Route has no owner.uid label", func() {
		It("should pass without error", func() {
			route := makeRoute(nil)
			err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when Route has empty labels map", func() {
		It("should pass without error", func() {
			route := makeRoute(map[string]string{})
			err := h.VerifyProviderBinding(ctx, route, makeProvider())
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
