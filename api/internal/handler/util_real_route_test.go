// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	adminapi "github.com/telekom/controlplane/admin/api/v1"
	apiapi "github.com/telekom/controlplane/api/api/v1"
	"github.com/telekom/controlplane/api/internal/handler/util"
	"github.com/telekom/controlplane/common/pkg/client"
	"github.com/telekom/controlplane/common/pkg/types"
	"github.com/telekom/controlplane/common/pkg/util/contextutil"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Creation of Real-Routes", Ordered, func() {
	var (
		routeCtx     context.Context
		providerZone *adminapi.Zone
	)

	BeforeAll(func() {
		routeCtx = client.WithClient(context.Background(), client.NewJanitorClient(client.NewScopedClient(k8sClient, testEnvironment)))
		routeCtx = contextutil.WithEnv(routeCtx, testEnvironment)

		providerZone = CreateZone("encoded-upstream")
	})

	It("should persist the percent-encoded upstream path", func() {
		apiExposure := &apiapi.ApiExposure{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "encoded-upstream",
				Namespace: testEnvironment,
			},
			Spec: apiapi.ApiExposureSpec{
				ApiBasePath: "/encoded/upstream/v1",
				Upstreams: []apiapi.Upstream{
					{Url: "https://provider.example.com:8443/api/encoded%20url%20path"},
				},
			},
		}

		route, err := util.CreateRealRoute(routeCtx, *types.ObjectRefFromObject(providerZone), apiExposure)
		Expect(err).ToNot(HaveOccurred())

		persistedRoute := &gatewayapi.Route{}
		Expect(k8sClient.Get(routeCtx, k8sclient.ObjectKeyFromObject(route), persistedRoute)).To(Succeed())
		Expect(persistedRoute.Spec.Backend.Upstreams).To(Equal([]gatewayapi.Upstream{{
			Scheme:   "https",
			Hostname: "provider.example.com",
			Port:     8443,
			Path:     "/api/encoded%20url%20path",
		}}))
	})
})
