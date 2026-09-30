// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/telekom/controlplane/common/pkg/config"
	"github.com/telekom/controlplane/common/pkg/types"
	gatewayv1 "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConsumeRoute admission", func() {
	It("accepts consumer username/password together with scopes", func() {
		createNamespace(testEnvironment)
		consumeRoute := &gatewayv1.ConsumeRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "password-consumer",
				Namespace: testEnvironment,
				Labels:    map[string]string{config.EnvironmentLabelKey: testEnvironment},
			},
			Spec: gatewayv1.ConsumeRouteSpec{
				Route:        types.ObjectRef{Name: "password-route", Namespace: testEnvironment},
				ConsumerName: "password-consumer",
				Security: &gatewayv1.ConsumeRouteSecurity{M2M: &gatewayv1.ConsumerMachine2MachineAuthentication{
					Basic:  &gatewayv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
					Scopes: []string{"consumer:read", "consumer:write"},
				}},
			},
		}

		Expect(k8sClient.Create(ctx, consumeRoute)).To(Succeed())
		DeferCleanup(func() { Expect(k8sClient.Delete(ctx, consumeRoute)).To(Succeed()) })
	})
})
