// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	"github.com/telekom/controlplane/gateway/pkg/kong/client/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeatures(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Features Suite")
}

func NewMockRoute() *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: gatewayv1.RouteSpec{
			Realm: types.ObjectRef{
				Name:      "realm",
				Namespace: "default",
			},
			PassThrough: false,
			Upstreams: []gatewayv1.Upstream{
				{
					Url: "http://upstream.url:8080/api/v1",
					// Default is used for Weight
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

func NewMockConsumeRoute(routeRef types.ObjectRef) *gatewayv1.ConsumeRoute {
	return &gatewayv1.ConsumeRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-consumer",
			Namespace: "default",
		},
		Spec: gatewayv1.ConsumeRouteSpec{
			ConsumerName: "test-consumer-name",
			Route:        routeRef,
			Security: &gatewayv1.ConsumerSecurity{
				M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Scopes: []string{"scope1", "scope2"},
				},
			},
		},
	}
}

func NewMockRealm() *gatewayv1.Realm {
	return &gatewayv1.Realm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-realm",
			Namespace: "default",
		},
		Spec: gatewayv1.RealmSpec{
			Url:       "https://realm.url",
			IssuerUrl: "https://issuer.url",
			DefaultConsumers: []string{
				"gateway",
				"test",
			},
		},
	}
}

func NewMockGateway() *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Admin: gatewayv1.AdminConfig{
				ClientId:     "admin",
				ClientSecret: "topsecret",
				IssuerUrl:    "https://issuer.url",
				Url:          "https://admin.test.url",
			},
		},
	}
}

var (
	mockKc   *mock.MockKongClient
	mockCtrl *gomock.Controller
	route    *gatewayv1.Route
	realm    *gatewayv1.Realm
	gateway  *gatewayv1.Gateway
)

var _ = BeforeSuite(func() {
	mockKc = mock.NewMockKongClient(mockCtrl)
	mockCtrl = gomock.NewController(GinkgoT())
	route = NewMockRoute()
	realm = NewMockRealm()
	gateway = NewMockGateway()
})
