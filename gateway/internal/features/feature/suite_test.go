// Copyright 2025 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package feature_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFeature(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feature Suite")
}

func NewRoute() *gatewayv1.Route {
	return &gatewayv1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-route",
			Namespace: "test-namespace",
		},
	}
}

func NewGateway() *gatewayv1.Gateway {
	return &gatewayv1.Gateway{}
}

func NewRealm() *gatewayv1.Realm {
	return &gatewayv1.Realm{}
}
