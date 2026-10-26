// Copyright 2026 Deutsche Telekom IT GmbH
//
// SPDX-License-Identifier: Apache-2.0

package util

import (
	agenticv1 "github.com/telekom/controlplane/agentic/api/v1"
	gatewayapi "github.com/telekom/controlplane/gateway/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MapSubscriberSecurityToGateway", func() {
	DescribeTable("passes the consumer grant type through", func(grantType agenticv1.GrantType, expected gatewayapi.GrantType) {
		mapped := MapSubscriberSecurityToGateway(&agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
			Basic:     &agenticv1.BasicAuthCredentials{Username: "consumer-user", Password: "consumer-pass"},
			Scopes:    []string{"read"},
			GrantType: grantType,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.GrantType).To(Equal(expected))
	},
		Entry("empty without defaulting", agenticv1.GrantType(""), gatewayapi.GrantType("")),
		Entry("password", agenticv1.GrantTypePassword, gatewayapi.GrantTypePassword),
		Entry("client_credentials", agenticv1.GrantTypeClientCredentials, gatewayapi.GrantTypeClientCredentials),
		Entry("authorization_code", agenticv1.GrantTypeAuthorizationCode, gatewayapi.GrantTypeAuthorizationCode),
		Entry("unrecognized without validation", agenticv1.GrantType("unrecognized"), gatewayapi.GrantType("unrecognized")),
	)

	DescribeTable("passes the consumer token request method through", func(tokenRequest agenticv1.TokenRequestMethod, expected gatewayapi.TokenRequestMethod) {
		mapped := MapSubscriberSecurityToGateway(&agenticv1.SubscriberSecurity{M2M: &agenticv1.SubscriberMachine2MachineAuthentication{
			Client:       &agenticv1.OAuth2ClientCredentials{ClientId: "consumer-id", ClientSecret: "consumer-secret"},
			TokenRequest: tokenRequest,
		}})

		Expect(mapped).NotTo(BeNil())
		Expect(mapped.M2M).NotTo(BeNil())
		Expect(mapped.M2M.TokenRequest).To(Equal(expected))
	},
		Entry("empty without defaulting", agenticv1.TokenRequestMethod(""), gatewayapi.TokenRequestMethod("")),
		Entry("client_secret_basic", agenticv1.TokenRequestClientSecretBasic, gatewayapi.TokenRequestClientSecretBasic),
		Entry("client_secret_post", agenticv1.TokenRequestClientSecretPost, gatewayapi.TokenRequestClientSecretPost),
	)
})
