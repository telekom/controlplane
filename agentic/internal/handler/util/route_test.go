// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"
)

var _ = Describe("MapSubscriberSecurityToGateway", func() {
	It("passes the consumer grant type through", func() {
		mapped := MapSubscriberSecurityToGateway(&agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
			Basic:     &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
			Scopes:    []string{"read"},
			GrantType: agenticv1.GrantTypePassword,
		}})

		Expect(mapped.M2M.GrantType).To(Equal(gatewayapi.GrantTypePassword))
	})
})
